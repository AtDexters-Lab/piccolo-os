package app

import (
    "context"
    "fmt"
    "time"
    "os"
    "path/filepath"

    "piccolod/internal/api"
    "piccolod/internal/container"
    "piccolod/internal/services"
)

// FSManager manages application lifecycle with filesystem-based state storage
type FSManager struct {
    containerManager ContainerManager
    stateManager     *FilesystemStateManager
    serviceManager   *services.ServiceManager
}

// NewFSManagerWithServices creates a new filesystem-based app manager with an injected ServiceManager
func NewFSManagerWithServices(containerManager ContainerManager, stateDir string, serviceManager *services.ServiceManager) (*FSManager, error) {
    stateManager, err := NewFilesystemStateManager(stateDir)
    if err != nil {
        return nil, fmt.Errorf("failed to create state manager: %w", err)
    }
    
    return &FSManager{
        containerManager: containerManager,
        stateManager:     stateManager,
        serviceManager:   serviceManager,
    }, nil
}

// NewFSManager creates a new filesystem-based app manager with default ServiceManager
func NewFSManager(containerManager ContainerManager, stateDir string) (*FSManager, error) {
    return NewFSManagerWithServices(containerManager, stateDir, services.NewServiceManager())
}

// Install installs a new application from its definition
func (m *FSManager) Install(ctx context.Context, appDef *api.AppDefinition) (*AppInstance, error) {
    // Set defaults then validate
    SetDefaults(appDef)
    if err := ValidateAppDefinition(appDef); err != nil {
        return nil, fmt.Errorf("invalid app definition: %w", err)
    }
	
	// Check if app already exists
	if _, exists := m.stateManager.GetApp(appDef.Name); exists {
		return nil, fmt.Errorf("app already exists: %s", appDef.Name)
	}
	
    // Allocate services and convert to container spec
    endpoints, err := m.serviceManager.AllocateForApp(appDef.Name, appDef.Subdomain, appDef.Listeners)
    if err != nil {
        return nil, fmt.Errorf("failed to allocate service ports: %w", err)
    }

    containerSpec, err := m.appDefToContainerSpec(appDef, endpoints)
    if err != nil {
        return nil, fmt.Errorf("failed to create container spec: %w", err)
    }
	
    // Create container
    containerID, err := m.containerManager.CreateContainer(ctx, containerSpec)
    if err != nil {
        return nil, fmt.Errorf("failed to create container: %w", err)
    }
    // Record container ID for watcher reconciliation
    if m.serviceManager != nil {
        m.serviceManager.SetAppContainerID(appDef.Name, containerID)
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
	
	// Store app to filesystem
	if err := m.stateManager.StoreApp(app, appDef); err != nil {
		// Cleanup container if storage fails
		_ = m.containerManager.RemoveContainer(ctx, containerID)
        return nil, fmt.Errorf("failed to store app: %w", err)
    }
	
	return app, nil
}

// Upsert installs or updates an application by name. If the app exists, it is uninstalled and reinstalled.
func (m *FSManager) Upsert(ctx context.Context, appDef *api.AppDefinition) (*AppInstance, error) {
    if existing, exists := m.stateManager.GetApp(appDef.Name); exists {
        // Reconcile listeners first
        rec, containerChange, err := m.serviceManager.Reconcile(appDef.Name, appDef.Subdomain, appDef.Listeners)
        if err != nil {
            return nil, fmt.Errorf("failed to reconcile services: %w", err)
        }

        // Try in-place publish updates via Podman for adds/removes/guest port changes
        // Added
        for _, ep := range rec.Added {
            _ = m.containerManager.(*container.PodmanCLI).UpdatePublishAdd(ctx, existing.ContainerID, ep.HostBind, ep.GuestPort)
        }
        // Guest port changes: add new mapping, then remove old
        for _, ch := range rec.GuestPortChanged {
            _ = m.containerManager.(*container.PodmanCLI).UpdatePublishAdd(ctx, existing.ContainerID, ch.New.HostBind, ch.New.GuestPort)
            _ = m.containerManager.(*container.PodmanCLI).UpdatePublishRemove(ctx, existing.ContainerID, ch.Old.HostBind, ch.Old.GuestPort)
        }
        // Removed
        for _, ep := range rec.Removed {
            _ = m.containerManager.(*container.PodmanCLI).UpdatePublishRemove(ctx, existing.ContainerID, ep.HostBind, ep.GuestPort)
        }

        if containerChange {
            // If some podman updates failed silently, a full recreate could be a fallback in future.
        }

        // Persist new app.yaml and metadata
        if err := m.stateManager.StoreApp(existing, appDef); err != nil {
            return nil, fmt.Errorf("failed to store app: %w", err)
        }
        return existing, nil
    }
    return m.Install(ctx, appDef)
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
    return m.UninstallWithOptions(ctx, name, false)
}

// UninstallWithOptions removes an application; when purge is true, also deletes app data directories
func (m *FSManager) UninstallWithOptions(ctx context.Context, name string, purge bool) error {
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

    // Stop and remove service listeners for this app
    if m.serviceManager != nil {
        m.serviceManager.RemoveApp(name)
    }

    // Optionally purge app data (based on app definition storage)
    if purge {
        _ = m.purgeAppData(name)
    }

    // Remove from filesystem and cache (state only)
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
func (m *FSManager) appDefToContainerSpec(appDef *api.AppDefinition, endpoints []services.ServiceEndpoint) (container.ContainerCreateSpec, error) {
    spec := container.ContainerCreateSpec{
        Name:  appDef.Name,
        Image: appDef.Image,
        Environment: appDef.Environment,
    }
    
    // Convert listeners to port mappings using allocated endpoints
    for _, ep := range endpoints {
        spec.Ports = append(spec.Ports, container.PortMapping{
            Host:      ep.HostBind,
            Container: ep.GuestPort,
        })
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

// purgeAppData attempts to remove persistent and temporary storage directories for an app
func (m *FSManager) purgeAppData(name string) error {
    appDef, err := m.stateManager.GetAppDefinition(name)
    if err != nil {
        // If we cannot read app.yaml, fall back to default base deletion
        return m.purgeDefaultPaths(name)
    }

    const persistentBase = "/var/piccolo/storage"
    const temporaryBase = "/tmp/piccolo/apps"

    var toRemove []string
    if appDef.Storage != nil {
        for volName, vol := range appDef.Storage.Persistent {
            if vol.Host != "" {
                toRemove = append(toRemove, vol.Host)
            } else {
                toRemove = append(toRemove, filepath.Join(persistentBase, name, volName))
            }
        }
        for volName, vol := range appDef.Storage.Temporary {
            if vol.Host != "" {
                toRemove = append(toRemove, vol.Host)
            } else {
                toRemove = append(toRemove, filepath.Join(temporaryBase, name, volName))
            }
        }
    }

    for _, p := range toRemove {
        _ = os.RemoveAll(p)
    }
    return nil
}

func (m *FSManager) purgeDefaultPaths(name string) error {
    const persistentBase = "/var/piccolo/storage"
    const temporaryBase = "/tmp/piccolo/apps"
    _ = os.RemoveAll(filepath.Join(persistentBase, name))
    _ = os.RemoveAll(filepath.Join(temporaryBase, name))
    return nil
}
