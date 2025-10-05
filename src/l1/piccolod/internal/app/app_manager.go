package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"piccolod/internal/api"
	"piccolod/internal/cluster"
	"piccolod/internal/container"
	"piccolod/internal/events"
	"piccolod/internal/router"
	"piccolod/internal/services"
)

// AppManager manages application lifecycle with filesystem-based state storage
type AppManager struct {
	containerManager ContainerManager
	stateManager     *FilesystemStateManager
	serviceManager   *services.ServiceManager
	routeRegistrar   router.Registrar
	eventsMu         sync.Mutex
	eventCancel      context.CancelFunc
	eventsWG         sync.WaitGroup
	stateMu          sync.RWMutex
	locked           bool
	leadershipMu     sync.RWMutex
	leadershipState  map[string]cluster.Role
}

var (
	ErrLocked    = errors.New("app manager: persistence locked")
	ErrNotLeader = errors.New("app manager: not leader")
)

// NewAppManagerWithServices creates a new filesystem-based app manager with an injected ServiceManager
func NewAppManagerWithServices(containerManager ContainerManager, stateDir string, serviceManager *services.ServiceManager) (*AppManager, error) {
	stateManager, err := NewFilesystemStateManager(stateDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create state manager: %w", err)
	}

	return &AppManager{
		containerManager: containerManager,
		stateManager:     stateManager,
		serviceManager:   serviceManager,
		locked:           true,
		leadershipState:  make(map[string]cluster.Role),
	}, nil
}

// SetRouter wires the router registrar for leadership-based routing decisions.
func (m *AppManager) SetRouter(reg router.Registrar) {
	m.stateMu.Lock()
	m.routeRegistrar = reg
	m.stateMu.Unlock()
}

func (m *AppManager) currentRouter() router.Registrar {
	m.stateMu.RLock()
	defer m.stateMu.RUnlock()
	return m.routeRegistrar
}

// ObserveRuntimeEvents subscribes to leadership and lock-state events for logging.
func (m *AppManager) ObserveRuntimeEvents(bus *events.Bus) {
	if bus == nil {
		return
	}
	m.eventsMu.Lock()
	if m.eventCancel != nil {
		m.eventCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.eventCancel = cancel
	m.eventsMu.Unlock()

	leaders := bus.Subscribe(events.TopicLeadershipRoleChanged, 16)
	locks := bus.Subscribe(events.TopicLockStateChanged, 8)
	loopCtx := ctx

	m.eventsWG.Add(1)
	go func() {
		defer m.eventsWG.Done()
		for {
			select {
			case evt, ok := <-leaders:
				if !ok {
					leaders = nil
					if leaders == nil && locks == nil {
						return
					}
					continue
				}
				payload, ok := evt.Payload.(events.LeadershipChanged)
				if !ok {
					log.Printf("WARN: app-manager received unexpected leadership payload: %#v", evt.Payload)
					continue
				}
				m.leadershipMu.Lock()
				m.leadershipState[string(payload.Resource)] = payload.Role
				m.leadershipMu.Unlock()
				log.Printf("INFO: app-manager observed leadership change resource=%s role=%s", payload.Resource, payload.Role)
				m.handleLeadershipChange(loopCtx, payload)
			case evt, ok := <-locks:
				if !ok {
					locks = nil
					if leaders == nil && locks == nil {
						return
					}
					continue
				}
				payload, ok := evt.Payload.(events.LockStateChanged)
				if !ok {
					log.Printf("WARN: app-manager received unexpected lock payload: %#v", evt.Payload)
					continue
				}
				m.setLocked(payload.Locked)
				state := "unlocked"
				if payload.Locked {
					state = "locked"
				}
				log.Printf("INFO: app-manager observed control lock state=%s", state)
			case <-ctx.Done():
				return
			}
			if leaders == nil && locks == nil {
				return
			}
		}
	}()
}

// StopRuntimeEvents stops event observers and waits for goroutines to exit.
func (m *AppManager) StopRuntimeEvents() {
	m.eventsMu.Lock()
	if m.eventCancel != nil {
		m.eventCancel()
		m.eventCancel = nil
	}
	m.eventsMu.Unlock()
	m.eventsWG.Wait()
}

func (m *AppManager) setLocked(val bool) {
	m.stateMu.Lock()
	m.locked = val
	m.stateMu.Unlock()
}

func (m *AppManager) isLocked() bool {
	m.stateMu.RLock()
	defer m.stateMu.RUnlock()
	return m.locked
}

// LastObservedRole returns the most recently observed leadership role for the provided resource.
func (m *AppManager) LastObservedRole(resource string) cluster.Role {
	m.leadershipMu.RLock()
	defer m.leadershipMu.RUnlock()
	if role, ok := m.leadershipState[resource]; ok {
		return role
	}
	return cluster.RoleUnknown
}

func (m *AppManager) ensureUnlocked() error {
	if m.isLocked() {
		return ErrLocked
	}
	return nil
}

func (m *AppManager) ensureKernelLeader() error {
	role := m.LastObservedRole(cluster.ResourceKernel)
	if role == cluster.RoleFollower {
		return ErrNotLeader
	}
	return nil
}

func (m *AppManager) handleLeadershipChange(ctx context.Context, change events.LeadershipChanged) {
	switch {
	case change.Resource == cluster.ResourceKernel:
		// No global stop; per-app leadership events drive app lifecycle.
	case strings.HasPrefix(change.Resource, cluster.ResourceAppPrefix):
		appName := strings.TrimPrefix(change.Resource, cluster.ResourceAppPrefix)
		if appName == "" {
			return
		}
		if change.Role == cluster.RoleFollower {
			if err := m.stopInternal(ctx, appName); err != nil {
				log.Printf("WARN: follower transition stop app %s failed: %v", appName, err)
			}
		}
		if reg := m.currentRouter(); reg != nil {
			mode := router.ModeLocal
			if change.Role == cluster.RoleFollower {
				mode = router.ModeTunnel
			}
			reg.RegisterAppRoute(appName, mode, "")
		}
	}
}

func (m *AppManager) enterFollower(ctx context.Context) {
	apps := m.stateManager.ListApps()
	for _, app := range apps {
		if err := m.stopInternal(ctx, app.Name); err != nil {
			log.Printf("WARN: follower transition stop app %s failed: %v", app.Name, err)
		}
	}
}

// Locked reports the last observed lock state.
func (m *AppManager) Locked() bool {
	return m.isLocked()
}

// ForceLockState allows tests or orchestration code to override the lock flag directly.
func (m *AppManager) ForceLockState(lock bool) {
	m.setLocked(lock)
}

// NewAppManager creates a new filesystem-based app manager with default ServiceManager
func NewAppManager(containerManager ContainerManager, stateDir string) (*AppManager, error) {
	return NewAppManagerWithServices(containerManager, stateDir, services.NewServiceManager())
}

// RestoreServices rebuilds service proxies for running apps based on current container port bindings.
func (m *AppManager) RestoreServices(ctx context.Context) {
	apps := m.stateManager.ListApps()
	for _, app := range apps {
		if app.ContainerID == "" {
			continue
		}
		def, err := m.stateManager.GetAppDefinition(app.Name)
		if err != nil {
			log.Printf("WARN: restore services: failed to read app definition for %s: %v", app.Name, err)
			continue
		}
		ports, err := container.InspectPublishedPorts(ctx, app.ContainerID)
		if err != nil {
			log.Printf("WARN: restore services: podman port inspect failed for %s: %v", app.Name, err)
			continue
		}
		if len(ports) == 0 {
			m.serviceManager.RemoveApp(app.Name)
			continue
		}
		if _, err := m.serviceManager.RestoreFromPodman(app.Name, def.Listeners, ports); err != nil {
			log.Printf("WARN: restore services: failed to restore proxies for %s: %v", app.Name, err)
			continue
		}
		m.serviceManager.SetAppContainerID(app.Name, app.ContainerID)
	}
}

// Install installs a new application from its definition
func (m *AppManager) Install(ctx context.Context, appDef *api.AppDefinition) (*AppInstance, error) {
	if err := m.ensureUnlocked(); err != nil {
		return nil, err
	}
	if err := m.ensureKernelLeader(); err != nil {
		return nil, err
	}
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
	endpoints, err := m.serviceManager.AllocateForApp(appDef.Name, appDef.Listeners)
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
func (m *AppManager) Upsert(ctx context.Context, appDef *api.AppDefinition) (*AppInstance, error) {
	if err := m.ensureUnlocked(); err != nil {
		return nil, err
	}
	if err := m.ensureKernelLeader(); err != nil {
		return nil, err
	}
	if existing, exists := m.stateManager.GetApp(appDef.Name); exists {
		// Reconcile listeners first
		rec, containerChange, err := m.serviceManager.Reconcile(appDef.Name, appDef.Listeners)
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
func (m *AppManager) List(ctx context.Context) ([]*AppInstance, error) {
	return m.stateManager.ListApps(), nil
}

// Get returns a specific application by name
func (m *AppManager) Get(ctx context.Context, name string) (*AppInstance, error) {
	app, exists := m.stateManager.GetApp(name)
	if !exists {
		return nil, fmt.Errorf("app not found: %s", name)
	}

	return app, nil
}

// Start starts an application
func (m *AppManager) Start(ctx context.Context, name string) error {
	if err := m.ensureUnlocked(); err != nil {
		return err
	}
	if err := m.ensureKernelLeader(); err != nil {
		return err
	}
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

	// Rehydrate service proxies if they were removed while the app was stopped
	if _, err := m.serviceManager.GetByApp(name); err != nil {
		def, defErr := m.stateManager.GetAppDefinition(name)
		if defErr != nil {
			log.Printf("WARN: start app %s: failed to load app definition: %v", name, defErr)
		} else {
			ports, portErr := container.InspectPublishedPorts(ctx, app.ContainerID)
			if portErr != nil {
				log.Printf("WARN: start app %s: inspect ports failed: %v", name, portErr)
			} else if len(ports) == 0 {
				log.Printf("WARN: start app %s: no published ports found during restore", name)
			} else {
				if _, restoreErr := m.serviceManager.RestoreFromPodman(name, def.Listeners, ports); restoreErr != nil {
					log.Printf("WARN: start app %s: failed to restore services: %v", name, restoreErr)
				} else {
					m.serviceManager.SetAppContainerID(name, app.ContainerID)
				}
			}
		}
	}

	return nil
}

// Stop stops an application
func (m *AppManager) Stop(ctx context.Context, name string) error {
	if err := m.ensureUnlocked(); err != nil {
		return err
	}
	if err := m.ensureKernelLeader(); err != nil {
		return err
	}
	return m.stopInternal(ctx, name)
}

func (m *AppManager) stopInternal(ctx context.Context, name string) error {
	app, exists := m.stateManager.GetApp(name)
	if !exists {
		return fmt.Errorf("app not found: %s", name)
	}

	if err := m.containerManager.StopContainer(ctx, app.ContainerID); err != nil {
		_ = m.stateManager.UpdateAppStatus(name, "error")
		return fmt.Errorf("failed to stop container: %w", err)
	}

	if err := m.stateManager.UpdateAppStatus(name, "stopped"); err != nil {
		return fmt.Errorf("failed to update app status: %w", err)
	}

	if m.serviceManager != nil {
		m.serviceManager.RemoveApp(name)
	}

	return nil
}

// Uninstall removes an application completely
func (m *AppManager) Uninstall(ctx context.Context, name string) error {
	if err := m.ensureUnlocked(); err != nil {
		return err
	}
	if err := m.ensureKernelLeader(); err != nil {
		return err
	}
	return m.UninstallWithOptions(ctx, name, false)
}

// UninstallWithOptions removes an application; when purge is true, also deletes app data directories
func (m *AppManager) UninstallWithOptions(ctx context.Context, name string, purge bool) error {
	if err := m.ensureUnlocked(); err != nil {
		return err
	}
	if err := m.ensureKernelLeader(); err != nil {
		return err
	}
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
func (m *AppManager) Enable(ctx context.Context, name string) error {
	if err := m.ensureUnlocked(); err != nil {
		return err
	}
	if err := m.ensureKernelLeader(); err != nil {
		return err
	}
	if _, exists := m.stateManager.GetApp(name); !exists {
		return fmt.Errorf("app not found: %s", name)
	}

	return m.stateManager.EnableApp(name)
}

// Disable disables an application (systemctl-style)
func (m *AppManager) Disable(ctx context.Context, name string) error {
	if err := m.ensureUnlocked(); err != nil {
		return err
	}
	if err := m.ensureKernelLeader(); err != nil {
		return err
	}
	if _, exists := m.stateManager.GetApp(name); !exists {
		return fmt.Errorf("app not found: %s", name)
	}

	return m.stateManager.DisableApp(name)
}

// IsEnabled checks if an application is enabled
func (m *AppManager) IsEnabled(ctx context.Context, name string) (bool, error) {
	if _, exists := m.stateManager.GetApp(name); !exists {
		return false, fmt.Errorf("app not found: %s", name)
	}

	return m.stateManager.IsAppEnabled(name), nil
}

// ListEnabled returns names of all enabled apps
func (m *AppManager) ListEnabled(ctx context.Context) ([]string, error) {
	return m.stateManager.ListEnabledApps()
}

// UpdateImage updates an app's container image tag and recreates the container preserving services
func (m *AppManager) UpdateImage(ctx context.Context, name string, tag *string) error {
	if err := m.ensureUnlocked(); err != nil {
		return err
	}
	if err := m.ensureKernelLeader(); err != nil {
		return err
	}
	appInst, exists := m.stateManager.GetApp(name)
	if !exists {
		return fmt.Errorf("app not found: %s", name)
	}
	// Load current app definition
	curDef, err := m.stateManager.GetAppDefinition(name)
	if err != nil {
		return fmt.Errorf("failed to read current app.yaml: %w", err)
	}
	// Compute new image
	newImage := curDef.Image
	if tag != nil {
		// Replace tag portion if present, or append
		img := curDef.Image
		// Split on ':' but be careful with registry includes ':'
		// Strategy: if '@' digest present, ignore; else change last ':' segment after last '/'
		if i := strings.LastIndex(img, "/"); i >= 0 {
			repo := img[:i+1]
			rest := img[i+1:]
			if j := strings.LastIndex(rest, ":"); j >= 0 {
				newImage = repo + rest[:j] + ":" + *tag
			} else {
				newImage = repo + rest + ":" + *tag
			}
		} else {
			if j := strings.LastIndex(img, ":"); j >= 0 {
				newImage = img[:j] + ":" + *tag
			} else {
				newImage = img + ":" + *tag
			}
		}
	}
	// Prepare new def
	newDef := *curDef
	newDef.Image = newImage
	// Backup current YAML and validate new
	if err := ValidateAppDefinition(&newDef); err != nil {
		return fmt.Errorf("invalid new app definition: %w", err)
	}
	if err := m.stateManager.BackupCurrentAppDefinition(name); err != nil {
		return fmt.Errorf("backup app.yaml: %w", err)
	}
	// Pull image (best effort)
	_ = m.containerManager.PullImage(ctx, newImage)
	// Preserve endpoints
	endpoints, _ := m.serviceManager.GetByApp(name)
	// Stop and remove old container
	_ = m.containerManager.StopContainer(ctx, appInst.ContainerID)
	_ = m.containerManager.RemoveContainer(ctx, appInst.ContainerID)
	// Create new container with same endpoints
	spec, err := m.appDefToContainerSpec(&newDef, endpoints)
	if err != nil {
		return fmt.Errorf("build container spec: %w", err)
	}
	newCID, err := m.containerManager.CreateContainer(ctx, spec)
	if err != nil {
		return fmt.Errorf("create container: %w", err)
	}
	if m.serviceManager != nil {
		m.serviceManager.SetAppContainerID(name, newCID)
	}
	// Update instance and persist app.yaml + metadata
	appInst.Image = newImage
	appInst.ContainerID = newCID
	appInst.Status = "created"
	appInst.UpdatedAt = time.Now()
	if err := m.stateManager.StoreApp(appInst, &newDef); err != nil {
		return fmt.Errorf("store app: %w", err)
	}
	return nil
}

// Revert reverts an app to the previous app.yaml (if available) and recreates container
func (m *AppManager) Revert(ctx context.Context, name string) error {
	if err := m.ensureUnlocked(); err != nil {
		return err
	}
	appInst, exists := m.stateManager.GetApp(name)
	if !exists {
		return fmt.Errorf("app not found: %s", name)
	}
	// Read previous def
	prevDef, err := m.stateManager.GetPreviousAppDefinition(name)
	if err != nil {
		return fmt.Errorf("no previous version to revert to: %w", err)
	}
	// Backup current before writing previous
	if err := m.stateManager.BackupCurrentAppDefinition(name); err != nil {
		return fmt.Errorf("backup current: %w", err)
	}
	// Preserve endpoints
	endpoints, _ := m.serviceManager.GetByApp(name)
	// Stop and remove current container
	_ = m.containerManager.StopContainer(ctx, appInst.ContainerID)
	_ = m.containerManager.RemoveContainer(ctx, appInst.ContainerID)
	// Pull best-effort
	if prevDef.Image != "" {
		_ = m.containerManager.PullImage(ctx, prevDef.Image)
	}
	// Create new container from prev
	spec, err := m.appDefToContainerSpec(prevDef, endpoints)
	if err != nil {
		return fmt.Errorf("build container spec: %w", err)
	}
	newCID, err := m.containerManager.CreateContainer(ctx, spec)
	if err != nil {
		return fmt.Errorf("create container: %w", err)
	}
	if m.serviceManager != nil {
		m.serviceManager.SetAppContainerID(name, newCID)
	}
	// Update instance and persist prev as current
	appInst.Image = prevDef.Image
	appInst.ContainerID = newCID
	appInst.Status = "created"
	appInst.UpdatedAt = time.Now()
	if err := m.stateManager.StoreApp(appInst, prevDef); err != nil {
		return fmt.Errorf("store app: %w", err)
	}
	return nil
}

// Logs fetches recent container logs for an app
func (m *AppManager) Logs(ctx context.Context, name string, lines int) ([]string, error) {
	appInst, exists := m.stateManager.GetApp(name)
	if !exists {
		return nil, fmt.Errorf("app not found: %s", name)
	}
	if lines <= 0 {
		lines = 200
	}
	return m.containerManager.Logs(ctx, appInst.ContainerID, lines)
}

// appDefToContainerSpec converts an AppDefinition to a ContainerCreateSpec
func (m *AppManager) appDefToContainerSpec(appDef *api.AppDefinition, endpoints []services.ServiceEndpoint) (container.ContainerCreateSpec, error) {
	spec := container.ContainerCreateSpec{
		Name:        appDef.Name,
		Image:       appDef.Image,
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
func (m *AppManager) purgeAppData(name string) error {
	if err := m.ensureUnlocked(); err != nil {
		return err
	}
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

func (m *AppManager) purgeDefaultPaths(name string) error {
	if err := m.ensureUnlocked(); err != nil {
		return err
	}
	const persistentBase = "/var/piccolo/storage"
	const temporaryBase = "/tmp/piccolo/apps"
	_ = os.RemoveAll(filepath.Join(persistentBase, name))
	_ = os.RemoveAll(filepath.Join(temporaryBase, name))
	return nil
}
