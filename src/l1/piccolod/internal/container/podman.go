package container

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
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
	Name         string
	Image        string
	Ports        []PortMapping
	Volumes      []VolumeMapping
	Environment  map[string]string
	Resources    ResourceLimits
	NetworkMode  string
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

// CreateContainer creates a container using pre-validated arguments
func (p *PodmanCLI) CreateContainer(ctx context.Context, spec ContainerCreateSpec) (string, error) {
	// All inputs must be validated before calling this method
	
	// Build command with pre-validated arguments
	args := []string{"run", "-d"}
	
	// Add name
	if spec.Name != "" {
		args = append(args, "--name", spec.Name)
	}
	
	// Add port mappings
	for _, port := range spec.Ports {
		args = append(args, "--publish", 
			fmt.Sprintf("%d:%d", port.Host, port.Container))
	}
	
	// Add volume mappings
	for _, volume := range spec.Volumes {
		volumeArg := fmt.Sprintf("%s:%s", volume.Host, volume.Container)
		if volume.Options != "" {
			volumeArg += ":" + volume.Options
		}
		args = append(args, "--volume", volumeArg)
	}
	
	// Add resource limits
	if spec.Resources.Memory != "" {
		args = append(args, "--memory", spec.Resources.Memory)
	}
	if spec.Resources.CPU != "" {
		args = append(args, "--cpus", spec.Resources.CPU)
	}
	
	// Add environment variables (pre-validated keys and values)
	for key, value := range spec.Environment {
		args = append(args, "--env", fmt.Sprintf("%s=%s", key, value))
	}
	
	// Add network mode
	if spec.NetworkMode != "" {
		args = append(args, "--network", spec.NetworkMode)
	}
	
	// Add restart policy
	if spec.RestartPolicy != "" {
		args = append(args, "--restart", spec.RestartPolicy)
	}
	
	// Add image (must be last positional argument)
	if spec.Image != "" {
		args = append(args, spec.Image)
	}
	
	// Execute command using exec.CommandContext (no shell interpretation)
	cmd := exec.CommandContext(ctx, "podman", args...)
	output, err := cmd.CombinedOutput()
	
	if err != nil {
		return "", fmt.Errorf("podman run failed: %w, output: %s", err, string(output))
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