package app

import (
	"context"
	"testing"

	"piccolod/internal/api"
	"piccolod/internal/container"
)

// MockContainerManager implements container operations for testing
type MockContainerManager struct {
	containers    map[string]*MockContainer
	nextID        int
	createError   error
	startError    error
	stopError     error
	removeError   error
}

type MockContainer struct {
	ID     string
	Name   string
	Image  string
	Status string
	Spec   container.ContainerCreateSpec
}

func NewMockContainerManager() *MockContainerManager {
	return &MockContainerManager{
		containers: make(map[string]*MockContainer),
		nextID:     1,
	}
}

func (m *MockContainerManager) CreateContainer(ctx context.Context, spec container.ContainerCreateSpec) (string, error) {
	if m.createError != nil {
		return "", m.createError
	}
	
	// Initialize containers map if nil (safety check)
	if m.containers == nil {
		m.containers = make(map[string]*MockContainer)
	}

	containerID := generateMockContainerID(m.nextID)
	m.nextID++

	m.containers[containerID] = &MockContainer{
		ID:     containerID,
		Name:   spec.Name,
		Image:  spec.Image,
		Status: "created",
		Spec:   spec,
	}

	return containerID, nil
}

func (m *MockContainerManager) StartContainer(ctx context.Context, containerID string) error {
	if m.startError != nil {
		return m.startError
	}

	if container, exists := m.containers[containerID]; exists {
		container.Status = "running"
		return nil
	}

	return container.ErrContainerNotFound(containerID)
}

func (m *MockContainerManager) StopContainer(ctx context.Context, containerID string) error {
	if m.stopError != nil {
		return m.stopError
	}

	if container, exists := m.containers[containerID]; exists {
		container.Status = "stopped"
		return nil
	}

	return container.ErrContainerNotFound(containerID)
}

func (m *MockContainerManager) RemoveContainer(ctx context.Context, containerID string) error {
	if m.removeError != nil {
		return m.removeError
	}

	if _, exists := m.containers[containerID]; exists {
		delete(m.containers, containerID)
		return nil
	}

	return container.ErrContainerNotFound(containerID)
}

func generateMockContainerID(id int) string {
	// Generate a mock container ID that passes validation
	return "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcd" + string(rune('0'+id%10))
}

// TestAppManager_Install tests the core Install operation
func TestAppManager_Install(t *testing.T) {
	tests := []struct {
		name        string
		appDef      *api.AppDefinition
		expectError bool
		errorContains string
		validateResult func(*testing.T, *AppInstance)
	}{
		{
			name: "install simple app",
            appDef: &api.AppDefinition{
                Name:  "test-app",
                Image: "nginx:latest",
                Type:  "user",
                Listeners: []api.AppListener{{Name:"web", GuestPort:80}},
            },
			expectError: false,
			validateResult: func(t *testing.T, app *AppInstance) {
				if app.Name != "test-app" {
					t.Errorf("Expected name 'test-app', got %s", app.Name)
				}
				if app.Image != "nginx:latest" {
					t.Errorf("Expected image 'nginx:latest', got %s", app.Image)
				}
				if app.Status != "created" {
					t.Errorf("Expected status 'created', got %s", app.Status)
				}
				if app.ContainerID == "" {
					t.Error("Expected container ID to be set")
				}
			},
		},
        {
            name: "install app with listeners",
            appDef: &api.AppDefinition{
                Name:  "web-app",
                Image: "nginx:alpine",
                Type:  "user",
                Listeners: []api.AppListener{{Name:"web", GuestPort:80, Flow:"tcp", Protocol:"http"}},
            },
            expectError: false,
            validateResult: func(t *testing.T, app *AppInstance) {
                if app.Name == "" { t.Errorf("app name should be set") }
            },
        },
        {
            name: "install app with environment",
            appDef: &api.AppDefinition{
                Name:  "env-app",
                Image: "alpine:latest",
                Type:  "user",
                Listeners: []api.AppListener{{Name:"web", GuestPort:80}},
                Environment: map[string]string{
                    "NODE_ENV": "production",
                    "DEBUG":    "false",
                },
            },
			expectError: false,
			validateResult: func(t *testing.T, app *AppInstance) {
				if len(app.Environment) != 2 {
					t.Errorf("Expected 2 environment variables, got %d", len(app.Environment))
				}
				if app.Environment["NODE_ENV"] != "production" {
					t.Error("Expected NODE_ENV=production")
				}
			},
		},
        {
            name: "duplicate app name",
            appDef: &api.AppDefinition{
                Name:  "test-app", // Same as first test
                Image: "alpine:latest",
                Type:  "user",
                Listeners: []api.AppListener{{Name:"web", GuestPort:80}},
            },
            expectError: true,
            errorContains: "app already exists",
        },
		{
			name: "invalid app definition",
			appDef: &api.AppDefinition{
				Name: "", // Invalid: empty name
				Image: "nginx:latest",
				Type: "user",
			},
			expectError: true,
			errorContains: "name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fresh manager for each test
			containerManager := NewMockContainerManager()
    manager := NewManager(containerManager)

			// Pre-install the first app for duplicate test
            if tt.name == "duplicate app name" {
                firstApp := &api.AppDefinition{
                    Name:  "test-app",
                    Image: "nginx:latest",
                    Type:  "user",
                    Listeners: []api.AppListener{{Name:"web", GuestPort:80}},
                }
				_, err := manager.Install(context.Background(), firstApp)
				if err != nil {
					t.Fatalf("Failed to install first app: %v", err)
				}
			}

			// Test the install operation
			result, err := manager.Install(context.Background(), tt.appDef)

			if tt.expectError {
				if err == nil {
					t.Fatalf("Expected error but got none")
				}
				if tt.errorContains != "" && !containsString(err.Error(), tt.errorContains) {
					t.Errorf("Expected error to contain %q, got %q", tt.errorContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if result == nil {
				t.Fatal("Expected result but got nil")
			}

			// Run custom validation
			if tt.validateResult != nil {
				tt.validateResult(t, result)
			}
		})
	}
}

// TestAppManager_List tests listing all apps
func TestAppManager_List(t *testing.T) {
	containerManager := NewMockContainerManager()
	manager := NewManager(containerManager)

	ctx := context.Background()

	// Initially empty
	apps, err := manager.List(ctx)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(apps) != 0 {
		t.Errorf("Expected 0 apps initially, got %d", len(apps))
	}

	// Install a few apps
    app1 := &api.AppDefinition{Name: "app1", Image: "nginx:latest", Type: "user", Listeners: []api.AppListener{{Name:"web", GuestPort:80}}}
    app2 := &api.AppDefinition{Name: "app2", Image: "alpine:latest", Type: "user", Listeners: []api.AppListener{{Name:"web", GuestPort:80}}}

	_, err = manager.Install(ctx, app1)
	if err != nil {
		t.Fatalf("Failed to install app1: %v", err)
	}

	_, err = manager.Install(ctx, app2)
	if err != nil {
		t.Fatalf("Failed to install app2: %v", err)
	}

	// List should return both apps
	apps, err = manager.List(ctx)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(apps) != 2 {
		t.Errorf("Expected 2 apps, got %d", len(apps))
	}

	// Check apps are in the list
	appNames := make(map[string]bool)
	for _, app := range apps {
		appNames[app.Name] = true
	}

	if !appNames["app1"] {
		t.Error("Expected app1 in list")
	}
	if !appNames["app2"] {
		t.Error("Expected app2 in list")
	}
}

// TestAppManager_Get tests getting a specific app
func TestAppManager_Get(t *testing.T) {
	containerManager := NewMockContainerManager()
	manager := NewManager(containerManager)

	ctx := context.Background()

	// Get non-existent app
	_, err := manager.Get(ctx, "nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent app")
	}

	// Install an app
    appDef := &api.AppDefinition{Name: "test-app", Image: "nginx:latest", Type: "user", Listeners: []api.AppListener{{Name:"web", GuestPort:80}}}
	_, err = manager.Install(ctx, appDef)
	if err != nil {
		t.Fatalf("Failed to install app: %v", err)
	}

	// Get the app
	retrieved, err := manager.Get(ctx, "test-app")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if retrieved.Name != "test-app" {
		t.Errorf("Expected name 'test-app', got %s", retrieved.Name)
	}
	if retrieved.ContainerID == "" {
		t.Error("Expected container ID to be set")
	}
}

// TestAppManager_Start tests starting an app
func TestAppManager_Start(t *testing.T) {
	containerManager := NewMockContainerManager()
	manager := NewManager(containerManager)

	ctx := context.Background()

	// Start non-existent app
	err := manager.Start(ctx, "nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent app")
	}

	// Install and start app
    appDef := &api.AppDefinition{Name: "test-app", Image: "nginx:latest", Type: "user", Listeners: []api.AppListener{{Name:"web", GuestPort:80}}}
	_, err = manager.Install(ctx, appDef)
	if err != nil {
		t.Fatalf("Failed to install app: %v", err)
	}

	// Start the app
	err = manager.Start(ctx, "test-app")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify status changed
	app, err := manager.Get(ctx, "test-app")
	if err != nil {
		t.Fatalf("Failed to get app: %v", err)
	}

	if app.Status != "running" {
		t.Errorf("Expected status 'running', got %s", app.Status)
	}
}

// TestAppManager_Stop tests stopping an app
func TestAppManager_Stop(t *testing.T) {
	containerManager := NewMockContainerManager()
	manager := NewManager(containerManager)

	ctx := context.Background()

	// Install, start, then stop app
    appDef := &api.AppDefinition{Name: "test-app", Image: "nginx:latest", Type: "user", Listeners: []api.AppListener{{Name:"web", GuestPort:80}}}
	_, err := manager.Install(ctx, appDef)
	if err != nil {
		t.Fatalf("Failed to install app: %v", err)
	}

	err = manager.Start(ctx, "test-app")
	if err != nil {
		t.Fatalf("Failed to start app: %v", err)
	}

	err = manager.Stop(ctx, "test-app")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify status changed
	app, err := manager.Get(ctx, "test-app")
	if err != nil {
		t.Fatalf("Failed to get app: %v", err)
	}

	if app.Status != "stopped" {
		t.Errorf("Expected status 'stopped', got %s", app.Status)
	}
}

// TestAppManager_Uninstall tests uninstalling an app
func TestAppManager_Uninstall(t *testing.T) {
	containerManager := NewMockContainerManager()
	manager := NewManager(containerManager)

	ctx := context.Background()

	// Uninstall non-existent app
	err := manager.Uninstall(ctx, "nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent app")
	}

	// Install and uninstall app
    appDef := &api.AppDefinition{Name: "test-app", Image: "nginx:latest", Type: "user", Listeners: []api.AppListener{{Name:"web", GuestPort:80}}}
	_, err = manager.Install(ctx, appDef)
	if err != nil {
		t.Fatalf("Failed to install app: %v", err)
	}

	// Verify app exists
	_, err = manager.Get(ctx, "test-app")
	if err != nil {
		t.Fatalf("App should exist before uninstall: %v", err)
	}

	// Uninstall
	err = manager.Uninstall(ctx, "test-app")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify app is gone
	_, err = manager.Get(ctx, "test-app")
	if err == nil {
		t.Error("Expected error after uninstall")
	}

	// Verify container was removed from mock
	if len(containerManager.containers) != 0 {
		t.Errorf("Expected 0 containers after uninstall, got %d", len(containerManager.containers))
	}
}
