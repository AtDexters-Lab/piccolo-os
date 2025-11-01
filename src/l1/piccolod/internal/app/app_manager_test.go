package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"piccolod/internal/api"
	"piccolod/internal/cluster"
	"piccolod/internal/events"
	"piccolod/internal/router"
	"piccolod/internal/services"
	"piccolod/internal/state/paths"
)

type stubLockReader struct {
	mu     sync.RWMutex
	locked bool
}

func (s *stubLockReader) ControlLocked() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.locked
}

func (s *stubLockReader) set(locked bool) {
	s.mu.Lock()
	s.locked = locked
	s.mu.Unlock()
}

func allowHostStorage(t *testing.T, m *AppManager) {
	t.Helper()
	if os.Getenv("PICCOLO_ALLOW_UNMOUNTED_TESTS") != "1" {
		t.Skip("set PICCOLO_ALLOW_UNMOUNTED_TESTS=1 to run without mounted volumes")
	}
	m.SetMountVerifier(func(string) error { return nil })
}

func TestAppManager_LazyStateInitialization(t *testing.T) {
	tempDir := t.TempDir()
	mock := NewMockContainerManager()
	manager, err := NewAppManager(mock, tempDir)
	if err != nil {
		t.Fatalf("Failed to create AppManager: %v", err)
	}
	allowHostStorage(t, manager)
	manager.ForceLockState(true)

	// While locked we should refuse to touch the filesystem.
	if _, err := manager.List(context.Background()); !errors.Is(err, ErrLocked) {
		t.Fatalf("expected ErrLocked when listing while locked, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(tempDir, AppsDir)); err == nil {
		t.Fatalf("apps directory should not be created while locked")
	}

	// Unlock and ensure state initializes on-demand.
	manager.ForceLockState(false)
	apps, err := manager.List(context.Background())
	if err != nil {
		t.Fatalf("list after unlock: %v", err)
	}
	if len(apps) != 0 {
		t.Fatalf("expected no apps, got %d", len(apps))
	}
	if _, err := os.Stat(filepath.Join(tempDir, AppsDir)); err != nil {
		t.Fatalf("expected apps directory after unlock, stat err=%v", err)
	}
}

func TestAppManager_DefaultStateDirWhenEmpty(t *testing.T) {
	tempDir := t.TempDir()
	prev, had := os.LookupEnv("PICCOLO_STATE_DIR")
	if err := os.Setenv("PICCOLO_STATE_DIR", tempDir); err != nil {
		t.Fatalf("set env: %v", err)
	}
	paths.SetRootForTest(tempDir)
	t.Cleanup(func() {
		if had {
			_ = os.Setenv("PICCOLO_STATE_DIR", prev)
		} else {
			_ = os.Unsetenv("PICCOLO_STATE_DIR")
		}
		paths.SetRootForTest("")
	})

	mock := NewMockContainerManager()
	manager, err := NewAppManager(mock, "")
	if err != nil {
		t.Fatalf("NewAppManager with empty dir: %v", err)
	}
	allowHostStorage(t, manager)
	manager.ForceLockState(false)

	if _, err := manager.List(context.Background()); err != nil {
		t.Fatalf("list with default dir: %v", err)
	}

	appDir := filepath.Join(tempDir, AppsDir)
	if _, err := os.Stat(appDir); os.IsNotExist(err) {
		t.Fatalf("expected apps dir under default root, stat err=%v", err)
	}
}

func TestAppManager_ListRespectsLockAfterInitialization(t *testing.T) {
	tempDir := t.TempDir()
	mock := NewMockContainerManager()
	manager, err := NewAppManager(mock, tempDir)
	if err != nil {
		t.Fatalf("Failed to create AppManager: %v", err)
	}
	allowHostStorage(t, manager)
	manager.ForceLockState(false)

	if _, err := manager.List(context.Background()); err != nil {
		t.Fatalf("expected list while unlocked to succeed, got %v", err)
	}

	manager.ForceLockState(true)
	if _, err := manager.List(context.Background()); !errors.Is(err, ErrLocked) {
		t.Fatalf("expected ErrLocked when listing after re-lock, got %v", err)
	}
}

func TestAppManager_RestoreServicesDefersUntilUnlock(t *testing.T) {
	tempDir := t.TempDir()
	mock := NewMockContainerManager()
	svcMgr := services.NewServiceManager()
	manager, err := NewAppManagerWithServices(mock, tempDir, svcMgr, nil)
	if err != nil {
		t.Fatalf("Failed to create AppManager: %v", err)
	}
	allowHostStorage(t, manager)
	bus := events.NewBus()
	manager.ObserveRuntimeEvents(bus)
	t.Cleanup(manager.StopRuntimeEvents)

	manager.ForceLockState(true)
	manager.RestoreServices(context.Background())

	manager.restoreMu.Lock()
	pending := manager.pendingRestore
	manager.restoreMu.Unlock()
	if !pending {
		t.Fatalf("expected restore to be pending while locked")
	}

	manager.ForceLockState(false)
	bus.Publish(events.Event{Topic: events.TopicLockStateChanged, Payload: events.LockStateChanged{Locked: false}})

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		manager.restoreMu.Lock()
		pending = manager.pendingRestore
		manager.restoreMu.Unlock()
		manager.stateInitMu.Lock()
		initialized := manager.stateManager != nil
		manager.stateInitMu.Unlock()
		if !pending && initialized {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	manager.restoreMu.Lock()
	finalPending := manager.pendingRestore
	manager.restoreMu.Unlock()
	manager.stateInitMu.Lock()
	initialized := manager.stateManager != nil
	manager.stateInitMu.Unlock()
	t.Fatalf("expected restore to run after unlock; pending=%v initialized=%v", finalPending, initialized)
}

// TestAppManager_Install tests app installation with filesystem persistence
func TestAppManager_Install(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "fs_manager_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create filesystem manager with mock container manager
	mockContainer := NewMockContainerManager()
	manager, err := NewAppManager(mockContainer, tempDir)
	if err != nil {
		t.Fatalf("Failed to create AppManager: %v", err)
	}
	allowHostStorage(t, manager)
	manager.ForceLockState(false)

	ctx := context.Background()

	// Test app definition
	appDef := &api.AppDefinition{
		Name:      "test-app",
		Image:     "nginx:alpine",
		Type:      "user",
		Listeners: []api.AppListener{{Name: "web", GuestPort: 80, Flow: api.FlowTCP, Protocol: api.ListenerProtocolHTTP}},
		Environment: map[string]string{
			"ENV_VAR": "test-value",
		},
	}

	// Install the app
	app, err := manager.Install(ctx, appDef)
	if err != nil {
		t.Fatalf("Failed to install app: %v", err)
	}

	// Verify app was created correctly
	if app.Name != "test-app" {
		t.Errorf("Expected app name 'test-app', got %s", app.Name)
	}

	if app.Status != "created" {
		t.Errorf("Expected app status 'created', got %s", app.Status)
	}

	// Verify container was created
	if len(mockContainer.containers) != 1 {
		t.Errorf("Expected 1 container created, got %d", len(mockContainer.containers))
	}

	// Verify filesystem structure was created
	appDir := filepath.Join(tempDir, AppsDir, "test-app")
	if _, err := os.Stat(appDir); os.IsNotExist(err) {
		t.Error("App directory was not created")
	}

	// Verify app.yaml exists
	appYamlPath := filepath.Join(appDir, "app.yaml")
	if _, err := os.Stat(appYamlPath); os.IsNotExist(err) {
		t.Error("app.yaml was not created")
	}

	// Verify metadata.json exists
	metadataPath := filepath.Join(appDir, "metadata.json")
	if _, err := os.Stat(metadataPath); os.IsNotExist(err) {
		t.Error("metadata.json was not created")
	}

	// Test duplicate installation should fail
	_, err = manager.Install(ctx, appDef)
	if err == nil {
		t.Error("Expected error when installing duplicate app")
	}
}

func TestAppManager_Install_NotLeader(t *testing.T) {
	tempDir := t.TempDir()
	mockContainer := NewMockContainerManager()
	manager, err := NewAppManager(mockContainer, tempDir)
	if err != nil {
		t.Fatalf("Failed to create AppManager: %v", err)
	}
	allowHostStorage(t, manager)
	manager.ForceLockState(false)
	if _, err := manager.Install(context.Background(), &api.AppDefinition{
		Name: "demo", Image: "alpine:latest", Type: "user",
		Listeners: []api.AppListener{{Name: "web", GuestPort: 80, Flow: api.FlowTCP, Protocol: api.ListenerProtocolHTTP}},
	}); err != nil {
		t.Fatalf("seed install: %v", err)
	}

	bus := events.NewBus()
	manager.ObserveRuntimeEvents(bus)
	defer manager.StopRuntimeEvents()

	bus.Publish(events.Event{Topic: events.TopicLeadershipRoleChanged, Payload: events.LeadershipChanged{Resource: cluster.ResourceKernel, Role: cluster.RoleFollower}})

	deadline := time.Now().Add(100 * time.Millisecond)
	for time.Now().Before(deadline) {
		if manager.LastObservedRole(cluster.ResourceKernel) == cluster.RoleFollower {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	appDef := &api.AppDefinition{Name: "nope", Image: "nginx:alpine", Type: "user", Listeners: []api.AppListener{{Name: "web", GuestPort: 80}}}
	if _, err := manager.Install(context.Background(), appDef); !errors.Is(err, ErrNotLeader) {
		t.Fatalf("expected ErrNotLeader, got %v", err)
	}
}

func TestAppManager_RouterUpdatesOnLeadership(t *testing.T) {
	tempDir := t.TempDir()
	mockContainer := NewMockContainerManager()
	manager, err := NewAppManager(mockContainer, tempDir)
	if err != nil {
		t.Fatalf("Failed to create AppManager: %v", err)
	}
	allowHostStorage(t, manager)
	manager.ForceLockState(false)
	routeMgr := router.NewManager()
	manager.SetRouter(routeMgr)

	bus := events.NewBus()
	manager.ObserveRuntimeEvents(bus)
	defer manager.StopRuntimeEvents()

	bus.Publish(events.Event{Topic: events.TopicLeadershipRoleChanged, Payload: events.LeadershipChanged{Resource: cluster.ResourceForApp("demo"), Role: cluster.RoleLeader}})

	waitFor := func(cond func() bool) bool {
		deadline := time.Now().Add(150 * time.Millisecond)
		for time.Now().Before(deadline) {
			if cond() {
				return true
			}
			time.Sleep(5 * time.Millisecond)
		}
		return cond()
	}

	if !waitFor(func() bool { return routeMgr.AppRoute("demo").Mode == router.ModeLocal }) {
		t.Fatalf("expected local route when leader, got %s", routeMgr.AppRoute("demo").Mode)
	}

	bus.Publish(events.Event{Topic: events.TopicLeadershipRoleChanged, Payload: events.LeadershipChanged{Resource: cluster.ResourceForApp("demo"), Role: cluster.RoleFollower}})

	if !waitFor(func() bool { return routeMgr.AppRoute("demo").Mode == router.ModeTunnel }) {
		t.Fatalf("expected tunnel route when follower, got %s", routeMgr.AppRoute("demo").Mode)
	}
}

// TestAppManager_List tests listing apps from filesystem
func TestAppManager_List(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "fs_manager_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create filesystem manager with mock container manager
	mockContainer := NewMockContainerManager()
	manager, err := NewAppManager(mockContainer, tempDir)
	if err != nil {
		t.Fatalf("Failed to create AppManager: %v", err)
	}
	allowHostStorage(t, manager)
	manager.ForceLockState(false)
	manager.ForceLockState(false)
	manager.ForceLockState(false)
	manager.ForceLockState(false)
	manager.ForceLockState(false)

	ctx := context.Background()

	// Initially should be empty
	apps, err := manager.List(ctx)
	if err != nil {
		t.Fatalf("Failed to list apps: %v", err)
	}

	if len(apps) != 0 {
		t.Errorf("Expected 0 apps, got %d", len(apps))
	}

	// Install two apps
	appDef1 := &api.AppDefinition{Name: "app1", Image: "nginx:alpine", Type: "user", Listeners: []api.AppListener{{Name: "web", GuestPort: 80}}}
	appDef2 := &api.AppDefinition{Name: "app2", Image: "alpine:latest", Type: "user", Listeners: []api.AppListener{{Name: "web", GuestPort: 80}}}

	_, err = manager.Install(ctx, appDef1)
	if err != nil {
		t.Fatalf("Failed to install app1: %v", err)
	}

	_, err = manager.Install(ctx, appDef2)
	if err != nil {
		t.Fatalf("Failed to install app2: %v", err)
	}

	// List should return both apps
	apps, err = manager.List(ctx)
	if err != nil {
		t.Fatalf("Failed to list apps: %v", err)
	}

	if len(apps) != 2 {
		t.Errorf("Expected 2 apps, got %d", len(apps))
	}

	// Verify app names are present
	appNames := make(map[string]bool)
	for _, app := range apps {
		appNames[app.Name] = true
	}

	if !appNames["app1"] || !appNames["app2"] {
		t.Error("Not all apps were returned from List()")
	}
}

func TestAppManager_RequiresMountedVolume(t *testing.T) {
	tempDir := t.TempDir()
	mock := NewMockContainerManager()
	manager, err := NewAppManager(mock, tempDir)
	if err != nil {
		t.Fatalf("Failed to create AppManager: %v", err)
	}
	manager.SetMountVerifier(func(string) error {
		return ErrVolumeUnavailable
	})
	manager.ForceLockState(false)
	if _, err := manager.List(context.Background()); !errors.Is(err, ErrVolumeUnavailable) {
		t.Fatalf("expected ErrVolumeUnavailable, got %v", err)
	}
}

// TestAppManager_Get tests getting specific app
func TestAppManager_Get(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "fs_manager_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create filesystem manager with mock container manager
	mockContainer := NewMockContainerManager()
	manager, err := NewAppManager(mockContainer, tempDir)
	if err != nil {
		t.Fatalf("Failed to create AppManager: %v", err)
	}
	allowHostStorage(t, manager)
	manager.ForceLockState(false)
	manager.ForceLockState(false)
	manager.ForceLockState(false)
	manager.ForceLockState(false)
	manager.ForceLockState(false)

	ctx := context.Background()

	// Test getting non-existent app
	_, err = manager.Get(ctx, "nonexistent")
	if err == nil {
		t.Error("Expected error when getting nonexistent app")
	}

	// Install an app
	appDef := &api.AppDefinition{Name: "test-app", Image: "nginx:alpine", Type: "user", Listeners: []api.AppListener{{Name: "web", GuestPort: 80}}}
	installedApp, err := manager.Install(ctx, appDef)
	if err != nil {
		t.Fatalf("Failed to install app: %v", err)
	}

	// Get the app
	retrievedApp, err := manager.Get(ctx, "test-app")
	if err != nil {
		t.Fatalf("Failed to get app: %v", err)
	}

	// Verify app details
	if retrievedApp.Name != installedApp.Name {
		t.Errorf("Expected name %s, got %s", installedApp.Name, retrievedApp.Name)
	}

	if retrievedApp.Status != installedApp.Status {
		t.Errorf("Expected status %s, got %s", installedApp.Status, retrievedApp.Status)
	}
}

func TestAppManagerLeadershipTracking(t *testing.T) {
	tempDir := t.TempDir()
	mockContainer := NewMockContainerManager()
	manager, err := NewAppManager(mockContainer, tempDir)
	if err != nil {
		t.Fatalf("Failed to create AppManager: %v", err)
	}
	allowHostStorage(t, manager)
	reader := &stubLockReader{}
	manager.SetLockReader(reader)
	reader.set(true)
	if !manager.Locked() {
		t.Fatalf("expected manager to report locked when reader reports locked")
	}

	reader.set(false)
	if manager.Locked() {
		t.Fatalf("expected manager to report unlocked when reader reports unlocked")
	}

	bus := events.NewBus()
	manager.ObserveRuntimeEvents(bus)
	defer manager.StopRuntimeEvents()

	// Leadership event should be recorded
	bus.Publish(events.Event{Topic: events.TopicLeadershipRoleChanged, Payload: events.LeadershipChanged{Resource: "control", Role: cluster.RoleLeader}})
	deadline := time.Now().Add(100 * time.Millisecond)
	for time.Now().Before(deadline) {
		if manager.LastObservedRole("control") == cluster.RoleLeader {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if manager.LastObservedRole("control") != cluster.RoleLeader {
		t.Fatalf("expected control role=leader, got %s", manager.LastObservedRole("control"))
	}
}

// TestAppManager_StartStop tests starting and stopping apps with status updates
func TestAppManager_StartStop(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "fs_manager_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create filesystem manager with mock container manager
	mockContainer := NewMockContainerManager()
	manager, err := NewAppManager(mockContainer, tempDir)
	if err != nil {
		t.Fatalf("Failed to create AppManager: %v", err)
	}
	allowHostStorage(t, manager)
	manager.ForceLockState(false)

	ctx := context.Background()

	// Install an app
	appDef := &api.AppDefinition{Name: "test-app", Image: "nginx:alpine", Type: "user", Listeners: []api.AppListener{{Name: "web", GuestPort: 80}}}
	_, err = manager.Install(ctx, appDef)
	if err != nil {
		t.Fatalf("Failed to install app: %v", err)
	}

	// Start the app
	err = manager.Start(ctx, "test-app")
	if err != nil {
		t.Fatalf("Failed to start app: %v", err)
	}

	// Verify container was started (check status)
	var startedContainers int
	for _, container := range mockContainer.containers {
		if container.Status == "running" {
			startedContainers++
		}
	}
	if startedContainers != 1 {
		t.Errorf("Expected 1 container started, got %d", startedContainers)
	}

	// Verify status was updated
	app, err := manager.Get(ctx, "test-app")
	if err != nil {
		t.Fatalf("Failed to get app: %v", err)
	}

	if app.Status != "running" {
		t.Errorf("Expected status 'running', got %s", app.Status)
	}

	// Stop the app
	err = manager.Stop(ctx, "test-app")
	if err != nil {
		t.Fatalf("Failed to stop app: %v", err)
	}

	// Verify container was stopped (check status)
	var stoppedContainers int
	for _, container := range mockContainer.containers {
		if container.Status == "stopped" {
			stoppedContainers++
		}
	}
	if stoppedContainers != 1 {
		t.Errorf("Expected 1 container stopped, got %d", stoppedContainers)
	}

	// Verify status was updated
	app, err = manager.Get(ctx, "test-app")
	if err != nil {
		t.Fatalf("Failed to get app: %v", err)
	}

	if app.Status != "stopped" {
		t.Errorf("Expected status 'stopped', got %s", app.Status)
	}

	// Test start/stop nonexistent app should fail
	err = manager.Start(ctx, "nonexistent")
	if err == nil {
		t.Error("Expected error when starting nonexistent app")
	}

	err = manager.Stop(ctx, "nonexistent")
	if err == nil {
		t.Error("Expected error when stopping nonexistent app")
	}
}

// TestAppManager_Uninstall tests app uninstallation
func TestAppManager_Uninstall(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "fs_manager_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create filesystem manager with mock container manager
	mockContainer := NewMockContainerManager()
	manager, err := NewAppManager(mockContainer, tempDir)
	if err != nil {
		t.Fatalf("Failed to create AppManager: %v", err)
	}
	allowHostStorage(t, manager)
	manager.ForceLockState(false)

	ctx := context.Background()

	// Install an app
	appDef := &api.AppDefinition{Name: "test-app", Image: "nginx:alpine", Type: "user", Listeners: []api.AppListener{{Name: "web", GuestPort: 80}}}
	_, err = manager.Install(ctx, appDef)
	if err != nil {
		t.Fatalf("Failed to install app: %v", err)
	}

	// Verify app directory exists
	appDir := filepath.Join(tempDir, AppsDir, "test-app")
	if _, err := os.Stat(appDir); os.IsNotExist(err) {
		t.Error("App directory was not created")
	}

	// Uninstall the app
	err = manager.Uninstall(ctx, "test-app")
	if err != nil {
		t.Fatalf("Failed to uninstall app: %v", err)
	}

	// Verify container was removed
	if len(mockContainer.containers) != 0 {
		t.Errorf("Expected 0 containers after removal, got %d", len(mockContainer.containers))
	}

	// Verify app directory was removed
	if _, err := os.Stat(appDir); !os.IsNotExist(err) {
		t.Error("App directory was not removed")
	}

	// Verify app is no longer in list
	apps, err := manager.List(ctx)
	if err != nil {
		t.Fatalf("Failed to list apps: %v", err)
	}

	if len(apps) != 0 {
		t.Errorf("Expected 0 apps after uninstall, got %d", len(apps))
	}

	// Test uninstalling nonexistent app should fail
	err = manager.Uninstall(ctx, "nonexistent")
	if err == nil {
		t.Error("Expected error when uninstalling nonexistent app")
	}
}

// TestAppManager_EnableDisable tests systemctl-style enable/disable functionality
func TestAppManager_EnableDisable(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "fs_manager_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create filesystem manager with mock container manager
	mockContainer := NewMockContainerManager()
	manager, err := NewAppManager(mockContainer, tempDir)
	if err != nil {
		t.Fatalf("Failed to create AppManager: %v", err)
	}
	allowHostStorage(t, manager)
	manager.ForceLockState(false)

	ctx := context.Background()

	// Install an app
	appDef := &api.AppDefinition{Name: "test-app", Image: "nginx:alpine", Type: "user", Listeners: []api.AppListener{{Name: "web", GuestPort: 80}}}
	_, err = manager.Install(ctx, appDef)
	if err != nil {
		t.Fatalf("Failed to install app: %v", err)
	}

	// Initially app should not be enabled
	enabled, err := manager.IsEnabled(ctx, "test-app")
	if err != nil {
		t.Fatalf("Failed to check if app is enabled: %v", err)
	}

	if enabled {
		t.Error("App should not be enabled initially")
	}

	// Enable the app
	err = manager.Enable(ctx, "test-app")
	if err != nil {
		t.Fatalf("Failed to enable app: %v", err)
	}

	// Verify app is now enabled
	enabled, err = manager.IsEnabled(ctx, "test-app")
	if err != nil {
		t.Fatalf("Failed to check if app is enabled: %v", err)
	}

	if !enabled {
		t.Error("App should be enabled after Enable()")
	}

	// Verify symlink was created
	enabledPath := filepath.Join(tempDir, EnabledDir, "test-app")
	if _, err := os.Lstat(enabledPath); err != nil {
		t.Errorf("Enabled symlink was not created: %v", err)
	}

	// List enabled apps
	enabledApps, err := manager.ListEnabled(ctx)
	if err != nil {
		t.Fatalf("Failed to list enabled apps: %v", err)
	}

	if len(enabledApps) != 1 || enabledApps[0] != "test-app" {
		t.Errorf("Expected ['test-app'], got %v", enabledApps)
	}

	// Disable the app
	err = manager.Disable(ctx, "test-app")
	if err != nil {
		t.Fatalf("Failed to disable app: %v", err)
	}

	// Verify app is now disabled
	enabled, err = manager.IsEnabled(ctx, "test-app")
	if err != nil {
		t.Fatalf("Failed to check if app is enabled: %v", err)
	}

	if enabled {
		t.Error("App should be disabled after Disable()")
	}

	// Verify symlink was removed
	if _, err := os.Lstat(enabledPath); !os.IsNotExist(err) {
		t.Error("Enabled symlink was not removed")
	}

	// Test enable/disable nonexistent app should fail
	err = manager.Enable(ctx, "nonexistent")
	if err == nil {
		t.Error("Expected error when enabling nonexistent app")
	}

	err = manager.Disable(ctx, "nonexistent")
	if err == nil {
		t.Error("Expected error when disabling nonexistent app")
	}
}

// TestAppManager_PersistenceAcrossRestarts tests that state persists across manager restarts
func TestAppManager_PersistenceAcrossRestarts(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "fs_manager_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create first filesystem manager instance
	mockContainer1 := &MockContainerManager{}
	manager1, err := NewAppManager(mockContainer1, tempDir)
	if err != nil {
		t.Fatalf("Failed to create AppManager: %v", err)
	}
	allowHostStorage(t, manager1)
	manager1.ForceLockState(false)

	ctx := context.Background()

	// Install an app and enable it
	appDef := &api.AppDefinition{
		Name:      "persistent-app",
		Image:     "nginx:alpine",
		Type:      "user",
		Listeners: []api.AppListener{{Name: "web", GuestPort: 80}},
		Environment: map[string]string{
			"TEST_VAR": "persistent-value",
		},
	}

	_, err = manager1.Install(ctx, appDef)
	if err != nil {
		t.Fatalf("Failed to install app: %v", err)
	}

	err = manager1.Enable(ctx, "persistent-app")
	if err != nil {
		t.Fatalf("Failed to enable app: %v", err)
	}

	err = manager1.Start(ctx, "persistent-app")
	if err != nil {
		t.Fatalf("Failed to start app: %v", err)
	}

	// Get installation time
	app1, err := manager1.Get(ctx, "persistent-app")
	if err != nil {
		t.Fatalf("Failed to get app: %v", err)
	}

	installTime := app1.CreatedAt

	// Simulate restart by creating new manager instance with same state dir
	mockContainer2 := &MockContainerManager{}
	manager2, err := NewAppManager(mockContainer2, tempDir)
	if err != nil {
		t.Fatalf("Failed to create second AppManager: %v", err)
	}
	allowHostStorage(t, manager2)
	manager2.ForceLockState(false)

	// Verify app still exists and has correct state
	apps, err := manager2.List(ctx)
	if err != nil {
		t.Fatalf("Failed to list apps after restart: %v", err)
	}

	if len(apps) != 1 {
		t.Errorf("Expected 1 app after restart, got %d", len(apps))
	}

	app2, err := manager2.Get(ctx, "persistent-app")
	if err != nil {
		t.Fatalf("Failed to get app after restart: %v", err)
	}

	// Verify all properties were preserved
	if app2.Name != "persistent-app" {
		t.Errorf("Expected name 'persistent-app', got %s", app2.Name)
	}

	if app2.Image != "nginx:alpine" {
		t.Errorf("Expected image 'nginx:alpine', got %s", app2.Image)
	}

	if app2.Status != "running" {
		t.Errorf("Expected status 'running', got %s", app2.Status)
	}

	if app2.Environment["TEST_VAR"] != "persistent-value" {
		t.Errorf("Expected TEST_VAR='persistent-value', got %s", app2.Environment["TEST_VAR"])
	}

	if !app2.CreatedAt.Equal(installTime) {
		t.Errorf("Created time not preserved across restart")
	}

	// Verify enabled state persisted
	enabled, err := manager2.IsEnabled(ctx, "persistent-app")
	if err != nil {
		t.Fatalf("Failed to check enabled state: %v", err)
	}

	if !enabled {
		t.Error("Enabled state was not preserved across restart")
	}
}

func TestAppManager_BlockedWhenLocked(t *testing.T) {
	mock := NewMockContainerManager()
	tempDir := t.TempDir()
	mgr, err := NewAppManager(mock, tempDir)
	if err != nil {
		t.Fatalf("NewAppManager: %v", err)
	}
	allowHostStorage(t, mgr)
	mgr.ForceLockState(true)
	ctx := context.Background()
	_, err = mgr.Install(ctx, &api.AppDefinition{
		Name: "locked-app", Image: "nginx:latest", Type: "user",
		Listeners: []api.AppListener{{Name: "web", GuestPort: 80}},
	})
	if !errors.Is(err, ErrLocked) {
		t.Fatalf("expected ErrLocked, got %v", err)
	}
}
