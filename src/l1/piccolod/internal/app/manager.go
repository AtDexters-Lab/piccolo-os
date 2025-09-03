package app

import (
	"context"
	"fmt"
	"sync"
	"time"

	"piccolod/internal/api"
	"piccolod/internal/container"
)

// ContainerManager interface for container operations
type ContainerManager interface {
	CreateContainer(ctx context.Context, spec container.ContainerCreateSpec) (string, error)
	StartContainer(ctx context.Context, containerID string) error
	StopContainer(ctx context.Context, containerID string) error
	RemoveContainer(ctx context.Context, containerID string) error
}

// AppInstance represents a running application instance
type AppInstance struct {
	// App metadata
	Name        string            `json:"name"`
	Image       string            `json:"image"`
	Subdomain   string            `json:"subdomain"`
	Type        string            `json:"type"`
	Status      string            `json:"status"` // "created", "running", "stopped", "error"
	
	// Container information
	ContainerID string            `json:"container_id"`
	
    // Configuration
    Environment map[string]string `json:"environment,omitempty"`
	
	// Metadata
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// Manager manages application lifecycle
type Manager struct {
	containerManager ContainerManager
	apps            map[string]*AppInstance
	mutex           sync.RWMutex
}

// NewManager creates a new app manager
func NewManager(containerManager ContainerManager) *Manager {
	return &Manager{
		containerManager: containerManager,
		apps:            make(map[string]*AppInstance),
	}
}

// Install installs a new application from its definition
func (m *Manager) Install(ctx context.Context, appDef *api.AppDefinition) (*AppInstance, error) {
    m.mutex.Lock()
    defer m.mutex.Unlock()
    
    // Set defaults then validate
    SetDefaults(appDef)
    if err := ValidateAppDefinition(appDef); err != nil {
        return nil, fmt.Errorf("invalid app definition: %w", err)
    }
	
	// Check if app already exists
	if _, exists := m.apps[appDef.Name]; exists {
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
        Environment: appDef.Environment,
        CreatedAt:   now,
        UpdatedAt:   now,
    }
	
	// Store app instance
	m.apps[appDef.Name] = app
	
	return app, nil
}

// List returns all installed applications
func (m *Manager) List(ctx context.Context) ([]*AppInstance, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	apps := make([]*AppInstance, 0, len(m.apps))
	for _, app := range m.apps {
		apps = append(apps, app)
	}
	
	return apps, nil
}

// Get returns a specific application by name
func (m *Manager) Get(ctx context.Context, name string) (*AppInstance, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	app, exists := m.apps[name]
	if !exists {
		return nil, fmt.Errorf("app not found: %s", name)
	}
	
	return app, nil
}

// Start starts an application
func (m *Manager) Start(ctx context.Context, name string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	app, exists := m.apps[name]
	if !exists {
		return fmt.Errorf("app not found: %s", name)
	}
	
	// Start the container
	if err := m.containerManager.StartContainer(ctx, app.ContainerID); err != nil {
		app.Status = "error"
		app.UpdatedAt = time.Now()
		return fmt.Errorf("failed to start container: %w", err)
	}
	
	// Update status
	app.Status = "running"
	app.UpdatedAt = time.Now()
	
	return nil
}

// Stop stops an application
func (m *Manager) Stop(ctx context.Context, name string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	app, exists := m.apps[name]
	if !exists {
		return fmt.Errorf("app not found: %s", name)
	}
	
	// Stop the container
	if err := m.containerManager.StopContainer(ctx, app.ContainerID); err != nil {
		app.Status = "error"
		app.UpdatedAt = time.Now()
		return fmt.Errorf("failed to stop container: %w", err)
	}
	
	// Update status
	app.Status = "stopped"
	app.UpdatedAt = time.Now()
	
	return nil
}

// Uninstall removes an application completely
func (m *Manager) Uninstall(ctx context.Context, name string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	app, exists := m.apps[name]
	if !exists {
		return fmt.Errorf("app not found: %s", name)
	}
	
	// Stop container first (ignore error if already stopped)
	_ = m.containerManager.StopContainer(ctx, app.ContainerID)
	
	// Remove container
	if err := m.containerManager.RemoveContainer(ctx, app.ContainerID); err != nil {
		return fmt.Errorf("failed to remove container: %w", err)
	}
	
	// Remove from our app registry
	delete(m.apps, name)
	
	return nil
}

// appDefToContainerSpec converts an AppDefinition to a ContainerCreateSpec
func (m *Manager) appDefToContainerSpec(appDef *api.AppDefinition) (container.ContainerCreateSpec, error) {
    spec := container.ContainerCreateSpec{
        Name:  appDef.Name,
        Image: appDef.Image,
        Environment: appDef.Environment,
    }
    
    // Convert listeners: map guest_port to equal host port for this simple manager
    for _, l := range appDef.Listeners {
        spec.Ports = append(spec.Ports, container.PortMapping{Host: l.GuestPort, Container: l.GuestPort})
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
