package app

import (
	"context"
	"fmt"
	"time"

	"piccolod/internal/api"
	"piccolod/internal/container"
)

// FSManager manages application lifecycle with filesystem-based state storage
type FSManager struct {
	containerManager ContainerManager
	stateManager     *FilesystemStateManager
}

// NewFSManager creates a new filesystem-based app manager
func NewFSManager(containerManager ContainerManager, stateDir string) (*FSManager, error) {
	stateManager, err := NewFilesystemStateManager(stateDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create state manager: %w", err)
	}
	
	return &FSManager{
		containerManager: containerManager,
		stateManager:     stateManager,
	}, nil
}

// Install installs a new application from its definition
func (m *FSManager) Install(ctx context.Context, appDef *api.AppDefinition) (*AppInstance, error) {
	// Validate app definition
	if err := ValidateAppDefinition(appDef); err != nil {
		return nil, fmt.Errorf("invalid app definition: %w", err)
	}
	
	// Set defaults
	SetDefaults(appDef)
	
	// Check if app already exists
	if _, exists := m.stateManager.GetApp(appDef.Name); exists {
		return nil, fmt.Errorf("app already exists: %s", appDef.Name)
	}
	
	// Convert app definition to container spec
	containerSpec, err := m.appDefToContainerSpec(appDef)
	if err != nil {
		return nil, fmt.Errorf("failed to create container spec: %w", err)
	}
	
	// Create container
	containerID, err := m.containerManager.CreateContainer(ctx, containerSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}
	
	// Create app instance
	now := time.Now()
	app := &AppInstance{
		Name:        appDef.Name,
		Image:       appDef.Image,
		Subdomain:   appDef.Subdomain,
		Type:        appDef.Type,
		Status:      "created",
		ContainerID: containerID,
		Ports:       appDef.Ports,
		Environment: appDef.Environment,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	
	// Store app to filesystem
	if err := m.stateManager.StoreApp(app, appDef); err != nil {
		// Cleanup container if storage fails
		_ = m.containerManager.RemoveContainer(ctx, containerID)
		return nil, fmt.Errorf("failed to store app: %w", err)
	}
	
	return app, nil
}

// List returns all installed applications
func (m *FSManager) List(ctx context.Context) ([]*AppInstance, error) {
	return m.stateManager.ListApps(), nil
}

// Get returns a specific application by name
func (m *FSManager) Get(ctx context.Context, name string) (*AppInstance, error) {
	app, exists := m.stateManager.GetApp(name)
	if !exists {
		return nil, fmt.Errorf("app not found: %s", name)
	}
	
	return app, nil
}

// Start starts an application
func (m *FSManager) Start(ctx context.Context, name string) error {
	app, exists := m.stateManager.GetApp(name)
	if !exists {
		return fmt.Errorf("app not found: %s", name)
	}
	
	// Start the container
	if err := m.containerManager.StartContainer(ctx, app.ContainerID); err != nil {
		// Update status to error
		_ = m.stateManager.UpdateAppStatus(name, "error")
		return fmt.Errorf("failed to start container: %w", err)
	}
	
	// Update status to running
	if err := m.stateManager.UpdateAppStatus(name, "running"); err != nil {
		return fmt.Errorf("failed to update app status: %w", err)
	}
	
	return nil
}

// Stop stops an application
func (m *FSManager) Stop(ctx context.Context, name string) error {
	app, exists := m.stateManager.GetApp(name)
	if !exists {
		return fmt.Errorf("app not found: %s", name)
	}
	
	// Stop the container
	if err := m.containerManager.StopContainer(ctx, app.ContainerID); err != nil {
		// Update status to error
		_ = m.stateManager.UpdateAppStatus(name, "error")
		return fmt.Errorf("failed to stop container: %w", err)
	}
	
	// Update status to stopped
	if err := m.stateManager.UpdateAppStatus(name, "stopped"); err != nil {
		return fmt.Errorf("failed to update app status: %w", err)
	}
	
	return nil
}

// Uninstall removes an application completely
func (m *FSManager) Uninstall(ctx context.Context, name string) error {
	app, exists := m.stateManager.GetApp(name)
	if !exists {
		return fmt.Errorf("app not found: %s", name)
	}
	
	// Stop container first (ignore error if already stopped)
	_ = m.containerManager.StopContainer(ctx, app.ContainerID)
	
	// Remove container
	if err := m.containerManager.RemoveContainer(ctx, app.ContainerID); err != nil {
		return fmt.Errorf("failed to remove container: %w", err)
	}
	
	// Remove from filesystem and cache
	if err := m.stateManager.RemoveApp(name); err != nil {
		return fmt.Errorf("failed to remove app from storage: %w", err)
	}
	
	return nil
}

// Enable enables an application (systemctl-style)
func (m *FSManager) Enable(ctx context.Context, name string) error {
	if _, exists := m.stateManager.GetApp(name); !exists {
		return fmt.Errorf("app not found: %s", name)
	}
	
	return m.stateManager.EnableApp(name)
}

// Disable disables an application (systemctl-style)
func (m *FSManager) Disable(ctx context.Context, name string) error {
	if _, exists := m.stateManager.GetApp(name); !exists {
		return fmt.Errorf("app not found: %s", name)
	}
	
	return m.stateManager.DisableApp(name)
}

// IsEnabled checks if an application is enabled
func (m *FSManager) IsEnabled(ctx context.Context, name string) (bool, error) {
	if _, exists := m.stateManager.GetApp(name); !exists {
		return false, fmt.Errorf("app not found: %s", name)
	}
	
	return m.stateManager.IsAppEnabled(name), nil
}

// ListEnabled returns names of all enabled apps
func (m *FSManager) ListEnabled(ctx context.Context) ([]string, error) {
	return m.stateManager.ListEnabledApps()
}

// appDefToContainerSpec converts an AppDefinition to a ContainerCreateSpec
func (m *FSManager) appDefToContainerSpec(appDef *api.AppDefinition) (container.ContainerCreateSpec, error) {
	spec := container.ContainerCreateSpec{
		Name:  appDef.Name,
		Image: appDef.Image,
		Environment: appDef.Environment,
	}
	
	// Convert ports
	if appDef.Ports != nil {
		for _, port := range appDef.Ports {
			spec.Ports = append(spec.Ports, container.PortMapping{
				Host:      port.Host,
				Container: port.Container,
			})
		}
	}
	
	// Convert resources if present
	if appDef.Resources != nil && appDef.Resources.Limits != nil {
		spec.Resources = container.ResourceLimits{
			Memory: appDef.Resources.Limits.Memory,
			CPU:    fmt.Sprintf("%.1f", appDef.Resources.Limits.CPU),
		}
	}
	
	// Set network mode based on permissions
	if appDef.Permissions != nil && appDef.Permissions.Network != nil {
		if appDef.Permissions.Network.Internet == "deny" {
			spec.NetworkMode = "none"
		}
	}
	
	// Set restart policy for system apps
	if appDef.Type == "system" {
		spec.RestartPolicy = "always"
	}
	
	// Validate the container spec
	if err := container.ValidateContainerSpec(spec); err != nil {
		return spec, fmt.Errorf("invalid container spec: %w", err)
	}
	
	return spec, nil
}