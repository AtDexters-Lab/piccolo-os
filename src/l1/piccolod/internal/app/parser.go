package app

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
	"piccolod/internal/api"
)

var (
	// Valid app name pattern: lowercase letters, numbers, hyphens
	// Must start with letter, end with letter or number
	appNameRegex = regexp.MustCompile(`^[a-z][a-z0-9-]*[a-z0-9]$|^[a-z]$`)
)

// ParseAppDefinition parses YAML content into AppDefinition struct with validation
func ParseAppDefinition(content []byte) (*api.AppDefinition, error) {
	var app api.AppDefinition

	// Parse YAML
	if err := yaml.Unmarshal(content, &app); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Set defaults
	SetDefaults(&app)

	// Validate
	if err := ValidateAppDefinition(&app); err != nil {
		return nil, err
	}

	return &app, nil
}

// SerializeAppDefinition serializes AppDefinition to YAML bytes
func SerializeAppDefinition(app *api.AppDefinition) ([]byte, error) {
	data, err := yaml.Marshal(app)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal YAML: %w", err)
	}
	return data, nil
}

// SetDefaults sets default values for AppDefinition fields
func SetDefaults(app *api.AppDefinition) {
	// Default type is "user"
	if app.Type == "" {
		app.Type = "user"
	}

	// Listeners defaults
	for i := range app.Listeners {
		if app.Listeners[i].Flow == api.FlowUnknown {
			app.Listeners[i].Flow = api.FlowTCP
		}
		if app.Listeners[i].Protocol == api.ListenerProtocolUnknown {
			app.Listeners[i].Protocol = api.ListenerProtocolRaw
		}
	}
}

// ValidateAppDefinition validates an AppDefinition struct
func ValidateAppDefinition(app *api.AppDefinition) error {
	// Validate name
	if err := validateName(app.Name); err != nil {
		return err
	}

	// Validate image/build requirement
	if err := validateImageOrBuild(app); err != nil {
		return err
	}

	// Validate type
	if err := validateType(app.Type); err != nil {
		return err
	}

	// Validate listeners (service-oriented)
	if err := validateListeners(app.Listeners); err != nil {
		return err
	}

	// Validate storage
	if err := validateStorage(app.Storage); err != nil {
		return err
	}

	// Validate resources
	if err := validateResources(app.Resources); err != nil {
		return err
	}

	// Validate permissions
	if err := validatePermissions(app.Permissions); err != nil {
		return err
	}

	return nil
}

// validateName validates app name follows naming conventions
func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("name is required")
	}

	if len(name) > 50 {
		return fmt.Errorf("name must be 50 characters or less")
	}

	if !appNameRegex.MatchString(name) {
		return fmt.Errorf("name must contain only lowercase letters, numbers, and hyphens, and must start with a letter")
	}

	// Reserved names check
	reserved := []string{"api", "www", "admin", "root", "system", "piccolo"}
	for _, r := range reserved {
		if name == r {
			return fmt.Errorf("name '%s' is reserved", name)
		}
	}

	return nil
}

// validateImageOrBuild ensures either image or build is specified (but not both)
func validateImageOrBuild(app *api.AppDefinition) error {
	hasImage := app.Image != ""
	hasBuild := app.Build != nil && (app.Build.Containerfile != "" || app.Build.Git != "")

	if !hasImage && !hasBuild {
		return fmt.Errorf("either image or build must be specified")
	}

	if hasImage && hasBuild {
		return fmt.Errorf("cannot specify both image and build")
	}

	// If build is specified, validate it
	if hasBuild {
		return validateBuild(app.Build)
	}

	return nil
}

// validateType validates app type field
func validateType(appType string) error {
	validTypes := []string{"user", "system"}
	for _, valid := range validTypes {
		if appType == valid {
			return nil
		}
	}
	return fmt.Errorf("type must be either 'user' or 'system', got '%s'", appType)
}

// validatePorts validates port mappings
func validateListeners(listeners []api.AppListener) error {
	if len(listeners) == 0 {
		return fmt.Errorf("listeners are required; legacy ports are no longer supported")
	}

	names := make(map[string]struct{})
	guestPorts := make(map[int]string)

	for i, l := range listeners {
		// name required
		if strings.TrimSpace(l.Name) == "" {
			return fmt.Errorf("listener[%d] name is required", i)
		}
		// unique name per app
		if _, ok := names[l.Name]; ok {
			return fmt.Errorf("duplicate listener name '%s'", l.Name)
		}
		names[l.Name] = struct{}{}

		// guest_port required and valid
		if l.GuestPort < 1 || l.GuestPort > 65535 {
			return fmt.Errorf("listener '%s' guest_port must be between 1 and 65535", l.Name)
		}
		if existing, ok := guestPorts[l.GuestPort]; ok {
			return fmt.Errorf("guest_port %d used by both '%s' and '%s'", l.GuestPort, existing, l.Name)
		}
		guestPorts[l.GuestPort] = l.Name

		if l.Flow != api.FlowTCP && l.Flow != api.FlowTLS {
			return fmt.Errorf("listener '%s' flow must be 'tcp' or 'tls'", l.Name)
		}

		switch l.Protocol {
		case api.ListenerProtocolRaw, api.ListenerProtocolHTTP, api.ListenerProtocolWebsocket:
			// ok
		default:
			return fmt.Errorf("listener '%s' protocol '%s' not supported in v1", l.Name, l.Protocol.String())
		}

		// middleware entries: ensure names present
		for j, m := range l.Middleware {
			if strings.TrimSpace(m.Name) == "" {
				return fmt.Errorf("listener '%s' middleware[%d] name is required", l.Name, j)
			}
		}
	}
	return nil
}

// validateStorage validates storage configuration
func validateStorage(storage *api.AppStorage) error {
	if storage == nil {
		return nil // Storage is optional
	}

	// Validate persistent storage
	if err := validateStorageVolumes(storage.Persistent, "persistent"); err != nil {
		return err
	}

	// Validate temporary storage
	if err := validateStorageVolumes(storage.Temporary, "temporary"); err != nil {
		return err
	}

	return nil
}

// validateStorageVolumes validates a map of storage volumes
func validateStorageVolumes(volumes map[string]api.AppVolume, storageType string) error {
	if volumes == nil {
		return nil
	}

	for name, volume := range volumes {
		if name == "" {
			return fmt.Errorf("%s storage volume name cannot be empty", storageType)
		}

		if volume.Container == "" {
			return fmt.Errorf("%s storage volume '%s' must specify container path", storageType, name)
		}

		// Validate container path is absolute
		if !strings.HasPrefix(volume.Container, "/") {
			return fmt.Errorf("%s storage volume '%s' container path must be absolute", storageType, name)
		}

		// Validate size limit format if specified
		if volume.SizeLimit != "" {
			if err := validateSizeLimit(volume.SizeLimit); err != nil {
				return fmt.Errorf("%s storage volume '%s' size limit invalid: %w", storageType, name, err)
			}
		}
	}

	return nil
}

// validateResources validates resource limits
func validateResources(resources *api.AppResources) error {
	if resources == nil || resources.Limits == nil {
		return nil // Resources are optional
	}

	limits := resources.Limits

	// Validate memory limit
	if limits.Memory != "" {
		if err := validateSizeLimit(limits.Memory); err != nil {
			return fmt.Errorf("invalid memory limit: %w", err)
		}
	}

	// Validate CPU limit
	if limits.CPU < 0 {
		return fmt.Errorf("CPU limit must be non-negative")
	}
	if limits.CPU > 64 { // Reasonable upper limit
		return fmt.Errorf("CPU limit cannot exceed 64 cores")
	}

	// Validate storage limit
	if limits.Storage != "" {
		if err := validateSizeLimit(limits.Storage); err != nil {
			return fmt.Errorf("invalid storage limit: %w", err)
		}
	}

	return nil
}

// validatePermissions validates permissions configuration
func validatePermissions(permissions *api.AppPermissions) error {
	if permissions == nil {
		return nil // Permissions are optional
	}

	// Validate network permissions
	if permissions.Network != nil {
		if err := validateNetworkPermissions(permissions.Network); err != nil {
			return err
		}
	}

	// Validate resource permissions
	if permissions.Resources != nil {
		if err := validateResourcePermissions(permissions.Resources); err != nil {
			return err
		}
	}

	return nil
}

// validateNetworkPermissions validates network permission settings
func validateNetworkPermissions(network *api.AppNetworkPermissions) error {
	validValues := []string{"allow", "deny", ""}

	for _, field := range []struct {
		name  string
		value string
	}{
		{"internet", network.Internet},
		{"local_network", network.LocalNetwork},
		{"dns", network.DNS},
	} {
		if field.value != "" {
			found := false
			for _, valid := range validValues {
				if field.value == valid {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("network.%s must be 'allow' or 'deny', got '%s'", field.name, field.value)
			}
		}
	}

	return nil
}

// validateResourcePermissions validates resource permission settings
func validateResourcePermissions(resources *api.AppResourcePermissions) error {
	if resources.MaxProcesses < 0 {
		return fmt.Errorf("max_processes must be non-negative")
	}
	if resources.MaxOpenFiles < 0 {
		return fmt.Errorf("max_open_files must be non-negative")
	}
	return nil
}

// validateBuild validates build configuration
func validateBuild(build *api.AppBuild) error {
	if build == nil {
		return nil
	}

	hasContainerfile := build.Containerfile != ""
	hasGit := build.Git != ""

	if !hasContainerfile && !hasGit {
		return fmt.Errorf("build must specify either containerfile or git")
	}

	if hasContainerfile && hasGit {
		return fmt.Errorf("build cannot specify both containerfile and git")
	}

	return nil
}

// validateSizeLimit validates size limit format (e.g., "1GB", "500MB")
func validateSizeLimit(limit string) error {
	if limit == "" {
		return nil
	}

	// Simple validation for size format
	validSuffixes := []string{"B", "KB", "MB", "GB", "TB"}

	for _, suffix := range validSuffixes {
		if strings.HasSuffix(strings.ToUpper(limit), suffix) {
			// Extract number part and validate it's positive
			numPart := strings.TrimSuffix(strings.ToUpper(limit), suffix)
			if numPart == "" {
				return fmt.Errorf("size limit must have a numeric value")
			}
			// Basic check - should be more thorough with strconv.ParseFloat
			if strings.Contains(numPart, "-") {
				return fmt.Errorf("size limit must be positive")
			}
			return nil
		}
	}

	return fmt.Errorf("size limit must end with B, KB, MB, GB, or TB")
}
