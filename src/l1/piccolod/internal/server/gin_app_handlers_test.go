package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"piccolod/internal/api"
	"piccolod/internal/app"
	"piccolod/internal/container"
)

// TestGinAppAPI_Install tests POST /api/v1/apps endpoint with Gin
func TestGinAppAPI_Install(t *testing.T) {
	gin.SetMode(gin.TestMode)
	
	// Create temporary directory for filesystem state
	tempDir, err := os.MkdirTemp("", "gin_app_api_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	
	// Create test server with Gin
	server := createGinTestServer(t, tempDir)
	
	tests := []struct {
		name           string
		method         string
		contentType    string
		body           string
		expectedStatus int
		expectError    bool
	}{
		{
			name:        "install valid nginx app",
			method:      "POST",
			contentType: "application/x-yaml",
			body: `name: test-nginx
image: docker.io/library/nginx:alpine
type: user
subdomain: test-nginx
ports:
  web:
    container: 80
    host: 8080
environment:
  NGINX_HOST: localhost
  NGINX_PORT: "80"`,
			expectedStatus: http.StatusCreated,
			expectError:    false,
		},
		{
			name:           "install with wrong content type",
			method:         "POST",
			contentType:    "application/json",
			body:           `{"name": "test"}`,
			expectedStatus: http.StatusUnsupportedMediaType,
			expectError:    true,
		},
		{
			name:           "install with empty body",
			method:         "POST",
			contentType:    "application/x-yaml",
			body:           "",
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name:        "install with invalid yaml",
			method:      "POST",
			contentType: "application/x-yaml",
			body:        "invalid: yaml: content:",
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name:           "wrong http method",
			method:         "PUT",
			contentType:    "application/x-yaml",
			body:           "name: test",
			expectedStatus: http.StatusNotFound, // Gin returns 404 for unregistered routes
			expectError:    false, // 404 responses are plain text, not JSON
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			
			var req *http.Request
			if tt.body != "" {
				req, _ = http.NewRequest(tt.method, "/api/v1/apps", strings.NewReader(tt.body))
			} else {
				req, _ = http.NewRequest(tt.method, "/api/v1/apps", nil)
			}
			
			req.Header.Set("Content-Type", tt.contentType)
			
			server.router.ServeHTTP(w, req)
			
			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
				t.Logf("Response body: %s", w.Body.String())
			}
			
			// Only check JSON for non-404 responses
			if w.Code != http.StatusNotFound {
				// Verify response is valid JSON
				var response GinAppResponse
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
					t.Errorf("Response is not valid JSON: %v", err)
				}
				
				// Check error field matches expectation
				if tt.expectError && response.Error == nil {
					t.Error("Expected error in response but got none")
				}
				
				if !tt.expectError && response.Error != nil {
					t.Errorf("Expected no error but got: %+v", response.Error)
				}
			}
		})
	}
}

// TestGinAppAPI_List tests GET /api/v1/apps endpoint with Gin
func TestGinAppAPI_List(t *testing.T) {
	gin.SetMode(gin.TestMode)
	
	// Create temporary directory for filesystem state
	tempDir, err := os.MkdirTemp("", "gin_app_api_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	
	// Create test server
	server := createGinTestServer(t, tempDir)
	
	// Test empty list initially
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/apps", nil)
	server.router.ServeHTTP(w, req)
	
	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
	
	var response GinAppResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	
	// Should return empty array
	apps, ok := response.Data.([]interface{})
	if !ok {
		t.Fatalf("Expected array in response data")
	}
	
	if len(apps) != 0 {
		t.Errorf("Expected 0 apps, got %d", len(apps))
	}
	
	// Install an app via the app manager directly
	appDef := &api.AppDefinition{
		Name:  "test-app",
		Image: "nginx:alpine",
		Type:  "user",
	}
	
	_, err = server.appManager.Install(context.Background(), appDef)
	if err != nil {
		t.Fatalf("Failed to install app: %v", err)
	}
	
	// Test list with one app
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	
	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
	
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	
	apps, ok = response.Data.([]interface{})
	if !ok {
		t.Fatalf("Expected array in response data")
	}
	
	if len(apps) != 1 {
		t.Errorf("Expected 1 app, got %d", len(apps))
	}
}

// TestGinAppAPI_GetApp tests GET /api/v1/apps/:name endpoint with Gin
func TestGinAppAPI_GetApp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	
	// Create temporary directory for filesystem state
	tempDir, err := os.MkdirTemp("", "gin_app_api_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	
	// Create test server and install an app
	server := createGinTestServer(t, tempDir)
	
	appDef := &api.AppDefinition{
		Name:      "test-app",
		Image:     "nginx:alpine",
		Type:      "user",
		Subdomain: "test",
	}
	
	_, err = server.appManager.Install(context.Background(), appDef)
	if err != nil {
		t.Fatalf("Failed to install app: %v", err)
	}
	
	tests := []struct {
		name           string
		appName        string
		expectedStatus int
		expectError    bool
	}{
		{
			name:           "get existing app",
			appName:        "test-app",
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "get non-existent app",
			appName:        "nonexistent",
			expectedStatus: http.StatusNotFound,
			expectError:    true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/api/v1/apps/"+tt.appName, nil)
			server.router.ServeHTTP(w, req)
			
			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}
			
			var response GinAppResponse
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Errorf("Response is not valid JSON: %v", err)
			}
			
			if tt.expectError && response.Error == nil {
				t.Error("Expected error in response but got none")
			}
			
			if !tt.expectError && response.Error != nil {
				t.Errorf("Expected no error but got: %+v", response.Error)
			}
		})
	}
}

// TestGinAppAPI_AppActions tests POST /api/v1/apps/:name/{action} endpoints with Gin
func TestGinAppAPI_AppActions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	
	// Create temporary directory for filesystem state
	tempDir, err := os.MkdirTemp("", "gin_app_api_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	
	// Create test server and install an app
	server := createGinTestServer(t, tempDir)
	
	appDef := &api.AppDefinition{
		Name:  "test-app",
		Image: "alpine:latest",
		Type:  "user",
	}
	
	_, err = server.appManager.Install(context.Background(), appDef)
	if err != nil {
		t.Fatalf("Failed to install app: %v", err)
	}
	
	tests := []struct {
		name           string
		method         string
		url            string
		expectedStatus int
		expectError    bool
	}{
		{
			name:           "start app",
			method:         "POST",
			url:            "/api/v1/apps/test-app/start",
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "stop app",
			method:         "POST",
			url:            "/api/v1/apps/test-app/stop",
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "enable app",
			method:         "POST",
			url:            "/api/v1/apps/test-app/enable",
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "disable app",
			method:         "POST",
			url:            "/api/v1/apps/test-app/disable",
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "wrong method for action",
			method:         "GET",
			url:            "/api/v1/apps/test-app/start",
			expectedStatus: http.StatusNotFound, // Gin returns 404 for unregistered routes
			expectError:    false, // 404 responses are plain text, not JSON
		},
		{
			name:           "action on non-existent app",
			method:         "POST",
			url:            "/api/v1/apps/nonexistent/start",
			expectedStatus: http.StatusNotFound,
			expectError:    true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(tt.method, tt.url, nil)
			server.router.ServeHTTP(w, req)
			
			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
				t.Logf("Response body: %s", w.Body.String())
			}
			
			// Only check JSON for non-404 responses
			if w.Code != http.StatusNotFound {
				var response GinAppResponse
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
					t.Errorf("Response is not valid JSON: %v", err)
				}
				
				if tt.expectError && response.Error == nil {
					t.Error("Expected error in response but got none")
				}
				
				if !tt.expectError && response.Error != nil {
					t.Errorf("Expected no error but got: %+v", response.Error)
				}
			}
		})
	}
}

// TestGinAppAPI_FullLifecycle tests complete app lifecycle via Gin HTTP API
func TestGinAppAPI_FullLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	
	// Create temporary directory for filesystem state
	tempDir, err := os.MkdirTemp("", "gin_app_api_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	
	// Create test server
	server := createGinTestServer(t, tempDir)
	
	appYAML := `name: lifecycle-test
image: docker.io/library/nginx:alpine
type: user
subdomain: lifecycle-test
ports:
  web:
    container: 80
    host: 8090
environment:
  TEST_ENV: "lifecycle"`
	
	// 1. Install app via HTTP API
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/apps", strings.NewReader(appYAML))
	req.Header.Set("Content-Type", "application/x-yaml")
	server.router.ServeHTTP(w, req)
	
	if w.Code != http.StatusCreated {
		t.Fatalf("Failed to install app: status %d, body: %s", w.Code, w.Body.String())
	}
	
	// 2. Verify app appears in list
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/apps", nil)
	server.router.ServeHTTP(w, req)
	
	if w.Code != http.StatusOK {
		t.Fatalf("Failed to list apps: status %d", w.Code)
	}
	
	// 3. Get specific app details
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/apps/lifecycle-test", nil)
	server.router.ServeHTTP(w, req)
	
	if w.Code != http.StatusOK {
		t.Fatalf("Failed to get app details: status %d", w.Code)
	}
	
	// 4. Start the app
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/apps/lifecycle-test/start", nil)
	server.router.ServeHTTP(w, req)
	
	if w.Code != http.StatusOK {
		t.Fatalf("Failed to start app: status %d, body: %s", w.Code, w.Body.String())
	}
	
	// 5. Enable the app
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/apps/lifecycle-test/enable", nil)
	server.router.ServeHTTP(w, req)
	
	if w.Code != http.StatusOK {
		t.Fatalf("Failed to enable app: status %d", w.Code)
	}
	
	// 6. Stop the app
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/apps/lifecycle-test/stop", nil)
	server.router.ServeHTTP(w, req)
	
	if w.Code != http.StatusOK {
		t.Fatalf("Failed to stop app: status %d", w.Code)
	}
	
	// 7. Disable the app
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/apps/lifecycle-test/disable", nil)
	server.router.ServeHTTP(w, req)
	
	if w.Code != http.StatusOK {
		t.Fatalf("Failed to disable app: status %d", w.Code)
	}
	
	// 8. Uninstall the app
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("DELETE", "/api/v1/apps/lifecycle-test", nil)
	server.router.ServeHTTP(w, req)
	
	if w.Code != http.StatusOK {
		t.Fatalf("Failed to uninstall app: status %d", w.Code)
	}
	
	// 9. Verify app is gone
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/apps", nil)
	server.router.ServeHTTP(w, req)
	
	if w.Code != http.StatusOK {
		t.Fatalf("Failed to list apps after uninstall: status %d", w.Code)
	}
	
	var response GinAppResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse final response: %v", err)
	}
	
	apps, ok := response.Data.([]interface{})
	if !ok {
		t.Fatalf("Expected array in response data")
	}
	
	if len(apps) != 0 {
		t.Errorf("Expected 0 apps after full lifecycle, got %d", len(apps))
	}
}

// TestGinAppAPI_Uninstall tests DELETE /api/v1/apps/:name endpoint with Gin
func TestGinAppAPI_Uninstall(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create temporary directory for filesystem state
	tempDir, err := os.MkdirTemp("", "gin_app_api_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test server and install an app
	server := createGinTestServer(t, tempDir)

	appDef := &api.AppDefinition{
		Name:  "test-app",
		Image: "alpine:latest",
		Type:  "user",
	}

	_, err = server.appManager.Install(context.Background(), appDef)
	if err != nil {
		t.Fatalf("Failed to install app: %v", err)
	}

	// Test successful uninstall
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/api/v1/apps/test-app", nil)
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response GinAppResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response.Error != nil {
		t.Errorf("Expected no error but got: %+v", response.Error)
	}

	// Verify app is actually uninstalled
	apps, err := server.appManager.List(context.Background())
	if err != nil {
		t.Fatalf("Failed to list apps: %v", err)
	}

	if len(apps) != 0 {
		t.Errorf("Expected 0 apps after uninstall, got %d", len(apps))
	}

	// Test uninstall non-existent app
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("DELETE", "/api/v1/apps/nonexistent", nil)
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

// TestInvalidRoutes tests invalid route handling with Gin
func TestInvalidRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create temporary directory for filesystem state
	tempDir, err := os.MkdirTemp("", "gin_app_api_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	server := createGinTestServer(t, tempDir)

	tests := []struct {
		name           string
		method         string
		url            string
		expectedStatus int
	}{
		{
			name:           "empty app name",
			method:         "GET",
			url:            "/api/v1/apps/",
			expectedStatus: http.StatusMovedPermanently, // Gin treats this as a different route
		},
		{
			name:           "too many path segments",
			method:         "POST",
			url:            "/api/v1/apps/test/start/extra",
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(tt.method, tt.url, nil)
			server.router.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}


// createGinTestServer creates a Gin test server instance with filesystem state management
func createGinTestServer(t *testing.T, tempDir string) *GinServer {
	// Create mock container manager for app manager
	mockContainer := &GinMockContainerManager{
		containers: make(map[string]*MockContainer),
		nextID:     1,
	}
	
	// Create filesystem app manager
	appMgr, err := app.NewFSManager(mockContainer, tempDir)
	if err != nil {
		t.Fatalf("Failed to create app manager: %v", err)
	}
	
	// Create minimal server instance for testing
	server := &GinServer{
		appManager: appMgr,
		version:    "test-gin",
	}
	
	// Setup Gin routes
	server.setupGinRoutes()
	
	return server
}

// MockContainer represents a mock container for testing
type MockContainer struct {
	ID     string
	Name   string
	Image  string
	Status string
	Spec   container.ContainerCreateSpec
}

// generateMockContainerID generates a mock container ID for testing
func generateMockContainerID(id int) string {
	return fmt.Sprintf("mock-container-%d", id)
}

// GinMockContainerManager implements the ContainerManager interface for Gin testing
type GinMockContainerManager struct {
	containers map[string]*MockContainer
	nextID     int
	createError   error
	startError    error
	stopError     error
	removeError   error
}

func (m *GinMockContainerManager) CreateContainer(ctx context.Context, spec container.ContainerCreateSpec) (string, error) {
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

func (m *GinMockContainerManager) StartContainer(ctx context.Context, containerID string) error {
	if m.startError != nil {
		return m.startError
	}
	
	if container, exists := m.containers[containerID]; exists {
		container.Status = "running"
		return nil
	}
	return container.ErrContainerNotFound(containerID)
}

func (m *GinMockContainerManager) StopContainer(ctx context.Context, containerID string) error {
	if m.stopError != nil {
		return m.stopError
	}
	
	if container, exists := m.containers[containerID]; exists {
		container.Status = "stopped"
		return nil
	}
	return container.ErrContainerNotFound(containerID)
}

func (m *GinMockContainerManager) RemoveContainer(ctx context.Context, containerID string) error {
	if m.removeError != nil {
		return m.removeError
	}
	
	if _, exists := m.containers[containerID]; exists {
		delete(m.containers, containerID)
		return nil
	}
	return container.ErrContainerNotFound(containerID)
}