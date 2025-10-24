package container

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// ErrContainerNotFound returns an error for when a container is not found
func ErrContainerNotFound(containerID string) error {
	return fmt.Errorf("container not found: %s", containerID)
}

// PodmanCLI provides safe Podman CLI integration with injection prevention
type PodmanCLI struct{}

// Validation patterns for different argument types
var (
	// Container/image names: lowercase letters, numbers, hyphens, slashes, colons
	namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._:/-]*[a-z0-9]$|^[a-z0-9]$`)

	// Volume paths: absolute paths only, no special chars
	pathPattern = regexp.MustCompile(`^/[a-zA-Z0-9._/-]*$`)

	// Resource values: numbers with units
	resourcePattern = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?[kmgtKMGT]?[bB]?$`)

	// Environment variable keys
	envKeyPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
)

var portInUseRe = regexp.MustCompile(`:(\d+): bind: address already in use`)

// PortInUseError indicates Podman failed to bind the requested host port.
type PortInUseError struct {
	Port   int
	Output string
	Err    error
}

func (e *PortInUseError) Error() string {
	if e.Port > 0 {
		return fmt.Sprintf("podman port %d already in use: %v", e.Port, e.Err)
	}
	return fmt.Sprintf("podman host port already in use: %v", e.Err)
}

func (e *PortInUseError) Unwrap() error {
	return e.Err
}

// ValidateContainerName validates container/image names for security
func ValidateContainerName(name string) error {
	if name == "" {
		return fmt.Errorf("name cannot be empty")
	}
	if len(name) > 255 {
		return fmt.Errorf("name too long (max 255 chars)")
	}
	if !namePattern.MatchString(name) {
		return fmt.Errorf("name contains invalid characters: %s", name)
	}
	return nil
}

// ValidatePort validates port numbers
func ValidatePort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, got %d", port)
	}
	return nil
}

// ValidatePath validates filesystem paths for security
func ValidatePath(path string) error {
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}
	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("path must be absolute: %s", path)
	}
	if !pathPattern.MatchString(path) {
		return fmt.Errorf("path contains invalid characters: %s", path)
	}
	// Additional security checks
	if strings.Contains(path, "..") {
		return fmt.Errorf("path traversal not allowed: %s", path)
	}
	return nil
}

// ValidateResource validates resource limits (memory, CPU)
func ValidateResource(resource string) error {
	if resource == "" {
		return fmt.Errorf("resource cannot be empty")
	}
	if !resourcePattern.MatchString(resource) {
		return fmt.Errorf("invalid resource format: %s", resource)
	}
	return nil
}

// InspectPublishedPorts returns a map of guest_port -> host_port for a container.
func InspectPublishedPorts(ctx context.Context, containerID string) (map[int]int, error) {
	if containerID == "" {
		return nil, fmt.Errorf("container ID required")
	}
	cmd := exec.CommandContext(ctx, "podman", "port", containerID)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("podman port failed: %w", err)
	}

	result := make(map[int]int)
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Split(line, "->")
		if len(parts) != 2 {
			continue
		}
		left := strings.TrimSpace(parts[0])  // e.g. "80/tcp"
		right := strings.TrimSpace(parts[1]) // e.g. "127.0.0.1:15001"
		guestStr := strings.Split(left, "/")[0]
		guest, _ := strconv.Atoi(strings.TrimSpace(guestStr))
		hostParts := strings.Split(right, ":")
		hostStr := hostParts[len(hostParts)-1]
		host, _ := strconv.Atoi(strings.TrimSpace(hostStr))
		if guest > 0 && host > 0 {
			result[guest] = host
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// ValidateEnvKey validates environment variable keys
func ValidateEnvKey(key string) error {
	if key == "" {
		return fmt.Errorf("environment key cannot be empty")
	}
	if len(key) > 255 {
		return fmt.Errorf("environment key too long")
	}
	if !envKeyPattern.MatchString(key) {
		return fmt.Errorf("invalid environment key format: %s", key)
	}
	return nil
}

// ValidateEnvValue validates environment variable values
func ValidateEnvValue(value string) error {
	// Environment values can contain most characters but not control characters
	if strings.ContainsAny(value, "\x00\x01\x02\x03\x04\x05\x06\x07\x08\x0B\x0C\x0E\x0F\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1A\x1B\x1C\x1D\x1E\x1F\x7F") {
		return fmt.Errorf("environment value contains control characters")
	}
	if len(value) > 4096 {
		return fmt.Errorf("environment value too long (max 4096 chars)")
	}
	return nil
}

// ContainerCreateSpec defines validated parameters for container creation
type ContainerCreateSpec struct {
	Name          string
	Image         string
	Ports         []PortMapping
	Volumes       []VolumeMapping
	Environment   map[string]string
	Resources     ResourceLimits
	NetworkMode   string
	RestartPolicy string
}

type PortMapping struct {
	Host      int
	Container int
}

type VolumeMapping struct {
	Host      string
	Container string
	Options   string // "ro", "rw", etc.
}

type ResourceLimits struct {
	Memory string
	CPU    string
}

func buildRunArgs(spec ContainerCreateSpec) []string {
	args := []string{"run", "-d"}

	if spec.Name != "" {
		args = append(args, "--name", spec.Name, "--replace")
	}

	for _, port := range spec.Ports {
		args = append(args, "--publish",
			fmt.Sprintf("127.0.0.1:%d:%d", port.Host, port.Container))
	}

	for _, volume := range spec.Volumes {
		volumeArg := fmt.Sprintf("%s:%s", volume.Host, volume.Container)
		if volume.Options != "" {
			volumeArg += ":" + volume.Options
		}
		args = append(args, "--volume", volumeArg)
	}

	if spec.Resources.Memory != "" {
		args = append(args, "--memory", spec.Resources.Memory)
	}
	if spec.Resources.CPU != "" {
		args = append(args, "--cpus", spec.Resources.CPU)
	}

	for key, value := range spec.Environment {
		args = append(args, "--env", fmt.Sprintf("%s=%s", key, value))
	}

	if spec.NetworkMode != "" {
		args = append(args, "--network", spec.NetworkMode)
	}

	if spec.RestartPolicy != "" {
		args = append(args, "--restart", spec.RestartPolicy)
	}

	if spec.Image != "" {
		args = append(args, spec.Image)
	}

	return args
}

// CreateContainer creates a container using pre-validated arguments
func (p *PodmanCLI) CreateContainer(ctx context.Context, spec ContainerCreateSpec) (string, error) {
	// All inputs must be validated before calling this method

	// Execute command using exec.CommandContext (no shell interpretation)
	args := buildRunArgs(spec)
	cmd := exec.CommandContext(ctx, "podman", args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		outStr := string(output)
		if strings.Contains(outStr, "address already in use") {
			port := 0
			if match := portInUseRe.FindStringSubmatch(outStr); len(match) == 2 {
				if parsed, perr := strconv.Atoi(match[1]); perr == nil {
					port = parsed
				}
			}
			return "", &PortInUseError{Port: port, Output: outStr, Err: fmt.Errorf("podman run failed: %w", err)}
		}
		return "", fmt.Errorf("podman run failed: %w, output: %s", err, outStr)
	}

	// Extract container ID from output - look for the actual hex container ID
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && isValidContainerID(line) {
			return line, nil
		}
	}

	return "", fmt.Errorf("could not extract valid container ID from output: %s", string(output))
}

// StartContainer starts a container by validated ID
func (p *PodmanCLI) StartContainer(ctx context.Context, containerID string) error {
	// Validate container ID format (typically hex string)
	if !isValidContainerID(containerID) {
		return fmt.Errorf("invalid container ID format: %s", containerID)
	}

	cmd := exec.CommandContext(ctx, "podman", "start", containerID)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf("podman start failed: %w, output: %s", err, string(output))
	}

	return nil
}

// StopContainer stops a container by validated ID
func (p *PodmanCLI) StopContainer(ctx context.Context, containerID string) error {
	if !isValidContainerID(containerID) {
		return fmt.Errorf("invalid container ID format: %s", containerID)
	}

	cmd := exec.CommandContext(ctx, "podman", "stop", containerID)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf("podman stop failed: %w, output: %s", err, string(output))
	}

	return nil
}

// RemoveContainer removes a container by validated ID
func (p *PodmanCLI) RemoveContainer(ctx context.Context, containerID string) error {
	if !isValidContainerID(containerID) {
		return fmt.Errorf("invalid container ID format: %s", containerID)
	}

	cmd := exec.CommandContext(ctx, "podman", "rm", containerID)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf("podman rm failed: %w, output: %s", err, string(output))
	}

	return nil
}

// PullImage pulls an image by name
func (p *PodmanCLI) PullImage(ctx context.Context, image string) error {
	if err := ValidateContainerName(image); err != nil {
		return fmt.Errorf("invalid image name: %w", err)
	}
	cmd := exec.CommandContext(ctx, "podman", "pull", image)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("podman pull failed: %w, output: %s", err, string(output))
	}
	return nil
}

// Logs returns recent log lines from a container
func (p *PodmanCLI) Logs(ctx context.Context, containerID string, lines int) ([]string, error) {
	if !isValidContainerID(containerID) {
		return nil, fmt.Errorf("invalid container ID format: %s", containerID)
	}
	if lines <= 0 {
		lines = 200
	}
	args := []string{"logs", "--tail", fmt.Sprintf("%d", lines)}
	args = append(args, containerID)
	cmd := exec.CommandContext(ctx, "podman", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("podman logs failed: %w, output: %s", err, string(output))
	}
	// Split into lines
	var linesOut []string
	for _, ln := range strings.Split(strings.ReplaceAll(string(output), "\r\n", "\n"), "\n") {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		linesOut = append(linesOut, ln)
	}
	return linesOut, nil
}

// UpdatePublishAdd adds a port publish mapping to a running container
func (p *PodmanCLI) UpdatePublishAdd(ctx context.Context, containerID string, hostBind, guestPort int) error {
	if !isValidContainerID(containerID) {
		return fmt.Errorf("invalid container ID format: %s", containerID)
	}
	if err := ValidatePort(hostBind); err != nil {
		return err
	}
	if err := ValidatePort(guestPort); err != nil {
		return err
	}
	mapping := fmt.Sprintf("127.0.0.1:%d:%d", hostBind, guestPort)
	cmd := exec.CommandContext(ctx, "podman", "container", "update", "--publish-add", mapping, containerID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("podman update --publish-add failed: %w, output: %s", err, string(output))
	}
	return nil
}

// UpdatePublishRemove removes a port publish mapping from a running container
func (p *PodmanCLI) UpdatePublishRemove(ctx context.Context, containerID string, hostBind, guestPort int) error {
	if !isValidContainerID(containerID) {
		return fmt.Errorf("invalid container ID format: %s", containerID)
	}
	if err := ValidatePort(hostBind); err != nil {
		return err
	}
	if err := ValidatePort(guestPort); err != nil {
		return err
	}
	mapping := fmt.Sprintf("127.0.0.1:%d:%d", hostBind, guestPort)
	cmd := exec.CommandContext(ctx, "podman", "container", "update", "--publish-rm", mapping, containerID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("podman update --publish-rm failed: %w, output: %s", err, string(output))
	}
	return nil
}

// isValidContainerID validates container ID format
func isValidContainerID(id string) bool {
	// Container IDs are typically 64-character hex strings (may be shortened)
	if len(id) < 12 || len(id) > 64 {
		return false
	}
	// Check for hex characters only
	for _, r := range id {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

// ValidateContainerSpec validates all fields in a ContainerCreateSpec
func ValidateContainerSpec(spec ContainerCreateSpec) error {
	// Validate name
	if err := ValidateContainerName(spec.Name); err != nil {
		return fmt.Errorf("invalid container name: %w", err)
	}

	// Validate image
	if err := ValidateContainerName(spec.Image); err != nil {
		return fmt.Errorf("invalid image name: %w", err)
	}

	// Validate ports
	for i, port := range spec.Ports {
		if err := ValidatePort(port.Host); err != nil {
			return fmt.Errorf("invalid host port at index %d: %w", i, err)
		}
		if err := ValidatePort(port.Container); err != nil {
			return fmt.Errorf("invalid container port at index %d: %w", i, err)
		}
	}

	// Validate volumes
	for i, volume := range spec.Volumes {
		if err := ValidatePath(volume.Host); err != nil {
			return fmt.Errorf("invalid host path at index %d: %w", i, err)
		}
		if err := ValidatePath(volume.Container); err != nil {
			return fmt.Errorf("invalid container path at index %d: %w", i, err)
		}
	}

	// Validate environment variables
	for key, value := range spec.Environment {
		if err := ValidateEnvKey(key); err != nil {
			return fmt.Errorf("invalid environment key '%s': %w", key, err)
		}
		if err := ValidateEnvValue(value); err != nil {
			return fmt.Errorf("invalid environment value for key '%s': %w", key, err)
		}
	}

	// Validate resources
	if spec.Resources.Memory != "" {
		if err := ValidateResource(spec.Resources.Memory); err != nil {
			return fmt.Errorf("invalid memory resource: %w", err)
		}
	}
	if spec.Resources.CPU != "" {
		if err := ValidateResource(spec.Resources.CPU); err != nil {
			return fmt.Errorf("invalid CPU resource: %w", err)
		}
	}

	return nil
}
