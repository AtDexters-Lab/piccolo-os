//go:build integration

package app

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"piccolod/internal/api"
	"piccolod/internal/container"
)

// TestAppManager_InstallIntegration tests installing apps with real Podman
func TestAppManager_InstallIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	manager := setupRealManager(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	tests := []struct {
		name     string
		appDef   *api.AppDefinition
		validate func(*testing.T, *AppInstance, string)
	}{
		{
			name: "install nginx app",
			appDef: &api.AppDefinition{
				Name:  "test-nginx-integration",
				Image: "docker.io/library/nginx:alpine",
				Type:  "user",
				Ports: map[string]api.AppPort{
					"web": {Container: 80, Host: 8081},
				},
			},
			validate: func(t *testing.T, app *AppInstance, containerID string) {
				// Verify container exists
				if !containerExists(t, containerID) {
					t.Errorf("Container %s should exist", containerID)
				}

				// Verify container has correct image
				image := getContainerImage(t, containerID)
				if !strings.Contains(image, "nginx") {
					t.Errorf("Expected nginx image, got %s", image)
				}
			},
		},
		{
			name: "install alpine with environment",
			appDef: &api.AppDefinition{
				Name:  "test-alpine-integration",
				Image: "docker.io/library/alpine:latest",
				Type:  "user",
				Environment: map[string]string{
					"TEST_ENV": "integration-test",
					"NODE_ENV": "production",
				},
			},
			validate: func(t *testing.T, app *AppInstance, containerID string) {
				// Verify container exists
				if !containerExists(t, containerID) {
					t.Errorf("Container %s should exist", containerID)
				}

				// Verify environment variables are set
				envVars := getContainerEnvVars(t, containerID)
				if envVars["TEST_ENV"] != "integration-test" {
					t.Errorf("Expected TEST_ENV=integration-test, got %s", envVars["TEST_ENV"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Install the app
			result, err := manager.Install(ctx, tt.appDef)
			if err != nil {
				t.Fatalf("Failed to install app: %v", err)
			}

			// Ensure cleanup happens
			defer func() {
				cleanupApp(t, manager, tt.appDef.Name)
			}()

			// Validate the installation
			if result.ContainerID == "" {
				t.Fatal("Container ID should not be empty")
			}

			// Run custom validation
			tt.validate(t, result, result.ContainerID)
		})
	}
}

// TestAppManager_StartStopIntegration tests starting and stopping apps
func TestAppManager_StartStopIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	manager := setupRealManager(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Install nginx app with unique name
	uniqueName := fmt.Sprintf("test-nginx-startstop-%d", time.Now().UnixNano())
	appDef := &api.AppDefinition{
		Name:  uniqueName,
		Image: "docker.io/library/nginx:alpine",
		Type:  "user",
		Ports: map[string]api.AppPort{
			"web": {Container: 80, Host: 8082},
		},
	}

	app, err := manager.Install(ctx, appDef)
	if err != nil {
		t.Fatalf("Failed to install app: %v", err)
	}
	defer cleanupApp(t, manager, uniqueName)

	// Test starting the app
	err = manager.Start(ctx, uniqueName)
	if err != nil {
		t.Fatalf("Failed to start app: %v", err)
	}

	// Wait a moment for container to fully start
	time.Sleep(2 * time.Second)

	// Verify container is running
	if !containerIsRunning(t, app.ContainerID) {
		t.Error("Container should be running after start")
	}

	// Test HTTP connectivity
	if err := waitForHTTPResponse("http://localhost:8082", 10*time.Second); err != nil {
		t.Errorf("Failed to connect to nginx: %v", err)
	}

	// Test stopping the app
	err = manager.Stop(ctx, uniqueName)
	if err != nil {
		t.Fatalf("Failed to stop app: %v", err)
	}

	// Wait a moment for container to stop
	time.Sleep(2 * time.Second)

	// Verify container is stopped
	if containerIsRunning(t, app.ContainerID) {
		t.Error("Container should be stopped after stop command")
	}
}

// TestAppManager_FullLifecycleIntegration tests complete app lifecycle
func TestAppManager_FullLifecycleIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	manager := setupRealManager(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	appName := "test-full-lifecycle"
	appDef := &api.AppDefinition{
		Name:  appName,
		Image: "docker.io/library/nginx:alpine",
		Type:  "user",
		Ports: map[string]api.AppPort{
			"web": {Container: 80, Host: 8083},
		},
		Environment: map[string]string{
			"NGINX_PORT": "80",
		},
	}

	// 1. Install
	t.Log("Installing app...")
	installedApp, err := manager.Install(ctx, appDef)
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	if installedApp.Status != "created" {
		t.Errorf("Expected status 'created', got %s", installedApp.Status)
	}

	// 2. Verify it appears in list
	t.Log("Checking app list...")
	apps, err := manager.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	found := false
	for _, app := range apps {
		if app.Name == appName {
			found = true
			break
		}
	}
	if !found {
		t.Error("App should appear in list after install")
	}

	// 3. Get specific app
	t.Log("Getting specific app...")
	retrievedApp, err := manager.Get(ctx, appName)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrievedApp.Name != appName {
		t.Errorf("Expected name %s, got %s", appName, retrievedApp.Name)
	}

	// 4. Start
	t.Log("Starting app...")
	err = manager.Start(ctx, appName)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait for startup
	time.Sleep(3 * time.Second)

	// Verify running status
	runningApp, err := manager.Get(ctx, appName)
	if err != nil {
		t.Fatalf("Get after start failed: %v", err)
	}
	if runningApp.Status != "running" {
		t.Errorf("Expected status 'running', got %s", runningApp.Status)
	}

	// Test connectivity
	t.Log("Testing HTTP connectivity...")
	if err := waitForHTTPResponse("http://localhost:8083", 15*time.Second); err != nil {
		t.Errorf("HTTP connectivity test failed: %v", err)
	}

	// 5. Stop
	t.Log("Stopping app...")
	err = manager.Stop(ctx, appName)
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Wait for stop
	time.Sleep(2 * time.Second)

	// Verify stopped status
	stoppedApp, err := manager.Get(ctx, appName)
	if err != nil {
		t.Fatalf("Get after stop failed: %v", err)
	}
	if stoppedApp.Status != "stopped" {
		t.Errorf("Expected status 'stopped', got %s", stoppedApp.Status)
	}

	// 6. Uninstall
	t.Log("Uninstalling app...")
	err = manager.Uninstall(ctx, appName)
	if err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}

	// Verify app is gone
	_, err = manager.Get(ctx, appName)
	if err == nil {
		t.Error("App should not exist after uninstall")
	}

	// Verify container is removed
	if containerExists(t, installedApp.ContainerID) {
		t.Errorf("Container %s should be removed after uninstall", installedApp.ContainerID)
	}

	t.Log("Full lifecycle test completed successfully!")
}

// TestAppManager_ErrorScenariosIntegration tests error handling with real Podman
func TestAppManager_ErrorScenariosIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	manager := setupRealManager(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test invalid image
	t.Run("invalid image", func(t *testing.T) {
		appDef := &api.AppDefinition{
			Name:  "test-invalid-image",
			Image: "nonexistent/invalid:tag",
			Type:  "user",
		}

		_, err := manager.Install(ctx, appDef)
		if err == nil {
			t.Error("Expected error for invalid image")
			// Cleanup if somehow succeeded
			cleanupApp(t, manager, appDef.Name)
		}
	})

	// Test port conflict
	t.Run("port conflict", func(t *testing.T) {
		// First app on port 8084 with unique name
		timestamp := time.Now().UnixNano()
		app1Name := fmt.Sprintf("test-port-conflict-1-%d", timestamp)
		app1 := &api.AppDefinition{
			Name:  app1Name,
			Image: "docker.io/library/nginx:alpine",
			Type:  "user",
			Ports: map[string]api.AppPort{
				"web": {Container: 80, Host: 8084},
			},
		}

		_, err := manager.Install(ctx, app1)
		if err != nil {
			t.Fatalf("Failed to install first app: %v", err)
		}
		defer cleanupApp(t, manager, app1Name)

		// Start first app
		err = manager.Start(ctx, app1Name)
		if err != nil {
			t.Fatalf("Failed to start first app: %v", err)
		}

		// Second app on same port 8084 with unique name
		app2Name := fmt.Sprintf("test-port-conflict-2-%d", timestamp+1)
		app2 := &api.AppDefinition{
			Name:  app2Name,
			Image: "docker.io/library/nginx:alpine",
			Type:  "user",
			Ports: map[string]api.AppPort{
				"web": {Container: 80, Host: 8084},
			},
		}

		// Installing second app should fail due to port conflict
		_, err = manager.Install(ctx, app2)
		if err == nil {
			t.Error("Expected error when installing app with conflicting port")
			// Cleanup if somehow succeeded
			cleanupApp(t, manager, app2Name)
		} else {
			// Even if Install failed, there might be a leftover container
			// Try direct cleanup via podman
			cleanupOrphanedContainer(t, app2Name)
		}
	})
}

// Helper functions

func setupRealManager(t *testing.T) *Manager {
	// Check if Podman is available
	if _, err := exec.LookPath("podman"); err != nil {
		t.Skip("Podman not available, skipping integration test")
	}

	// Create manager with real PodmanCLI
	podmanCLI := &container.PodmanCLI{}
	return NewManager(podmanCLI)
}

func cleanupApp(t *testing.T, manager *Manager, appName string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Try to uninstall, ignore errors since app might not exist
	_ = manager.Uninstall(ctx, appName)
}

func cleanupOrphanedContainer(t *testing.T, containerName string) {
	// Try to remove container directly via podman, ignore errors
	cmd := exec.Command("podman", "rm", "-f", containerName)
	_ = cmd.Run()
}

func containerExists(t *testing.T, containerID string) bool {
	cmd := exec.Command("podman", "inspect", containerID)
	err := cmd.Run()
	return err == nil
}

func containerIsRunning(t *testing.T, containerID string) bool {
	cmd := exec.Command("podman", "inspect", "--format", "{{.State.Running}}", containerID)
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) == "true"
}

func getContainerImage(t *testing.T, containerID string) string {
	cmd := exec.Command("podman", "inspect", "--format", "{{.ImageName}}", containerID)
	output, err := cmd.Output()
	if err != nil {
		t.Logf("Failed to get container image: %v", err)
		return ""
	}
	return strings.TrimSpace(string(output))
}

func getContainerEnvVars(t *testing.T, containerID string) map[string]string {
	cmd := exec.Command("podman", "inspect", "--format", "{{range .Config.Env}}{{.}}\n{{end}}", containerID)
	output, err := cmd.Output()
	if err != nil {
		t.Logf("Failed to get container env vars: %v", err)
		return nil
	}

	envVars := make(map[string]string)
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			envVars[parts[0]] = parts[1]
		}
	}
	return envVars
}

func waitForHTTPResponse(url string, timeout time.Duration) error {
	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for HTTP response from %s", url)
}

// TestAppManager_FromYAMLIntegration tests installing apps from YAML files
func TestAppManager_FromYAMLIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	manager := setupRealManager(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	tests := []struct {
		name     string
		yamlFile string
		testPort string
	}{
		{
			name:     "nginx from yaml",
			yamlFile: "testdata/integration/simple-nginx.yaml",
			testPort: "8090",
		},
		{
			name:     "alpine with env from yaml",
			yamlFile: "testdata/integration/alpine-with-env.yaml",
			testPort: "", // No HTTP test for Alpine
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Read and parse the YAML file
			content, err := os.ReadFile(tt.yamlFile)
			if err != nil {
				t.Fatalf("Failed to read YAML file: %v", err)
			}

			appDef, err := ParseAppDefinition(content)
			if err != nil {
				t.Fatalf("Failed to parse app definition: %v", err)
			}

			// Install the app
			installedApp, err := manager.Install(ctx, appDef)
			if err != nil {
				t.Fatalf("Failed to install app from YAML: %v", err)
			}

			// Ensure cleanup
			defer cleanupApp(t, manager, appDef.Name)

			// Verify installation
			if installedApp.Name != appDef.Name {
				t.Errorf("Expected app name %s, got %s", appDef.Name, installedApp.Name)
			}

			// Start the app if it has ports (like nginx)
			if len(appDef.Ports) > 0 {
				err = manager.Start(ctx, appDef.Name)
				if err != nil {
					t.Fatalf("Failed to start app: %v", err)
				}

				// Wait for startup
				time.Sleep(3 * time.Second)

				// Test HTTP connectivity if port specified
				if tt.testPort != "" {
					url := fmt.Sprintf("http://localhost:%s", tt.testPort)
					if err := waitForHTTPResponse(url, 10*time.Second); err != nil {
						t.Errorf("Failed to connect to app: %v", err)
					}
				}
			}

			// Verify container has expected environment variables
			if len(appDef.Environment) > 0 {
				envVars := getContainerEnvVars(t, installedApp.ContainerID)
				for key, expectedValue := range appDef.Environment {
					if actualValue, ok := envVars[key]; !ok {
						t.Errorf("Expected environment variable %s not found", key)
					} else if actualValue != expectedValue {
						t.Errorf("Expected %s=%s, got %s=%s", key, expectedValue, key, actualValue)
					}
				}
			}
		})
	}
}
