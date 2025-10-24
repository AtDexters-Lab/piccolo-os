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
	"time"

	"github.com/gin-gonic/gin"

	"piccolod/internal/api"
	"piccolod/internal/app"
	authpkg "piccolod/internal/auth"
	"piccolod/internal/cluster"
	"piccolod/internal/container"
	crypt "piccolod/internal/crypt"
	"piccolod/internal/events"
	"piccolod/internal/health"
	"piccolod/internal/mdns"
	"piccolod/internal/remote"
	"piccolod/internal/remote/nexusclient"
	"piccolod/internal/services"
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
	sessionCookie, csrfToken := setupTestAdminSession(t, server)

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
listeners:
  - name: web
    guest_port: 80
    flow: tcp
    protocol: http
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
			name:           "install with invalid yaml",
			method:         "POST",
			contentType:    "application/x-yaml",
			body:           "invalid: yaml: content:",
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name:           "wrong http method",
			method:         "PUT",
			contentType:    "application/x-yaml",
			body:           "name: test",
			expectedStatus: http.StatusNotFound, // Gin returns 404 for unregistered routes
			expectError:    false,               // 404 responses are plain text, not JSON
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
			attachAuth(req, sessionCookie, csrfToken)

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
	sessionCookie, csrfToken := setupTestAdminSession(t, server)

	// Test empty list initially
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/apps", nil)
	attachAuth(req, sessionCookie, csrfToken)
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
		Name:      "test-app",
		Image:     "nginx:alpine",
		Type:      "user",
		Listeners: []api.AppListener{{Name: "web", GuestPort: 80}},
	}

	_, err = server.appManager.Install(context.Background(), appDef)
	if err != nil {
		t.Fatalf("Failed to install app: %v", err)
	}

	// Test list with one app
	w = httptest.NewRecorder()
	attachAuth(req, sessionCookie, csrfToken)
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

func TestGinAppServices_RemoteHost(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tempDir := t.TempDir()
	srv := createGinTestServer(t, tempDir)
	sessionCookie, csrf := setupTestAdminSession(t, srv)

	if err := srv.remoteManager.Configure(remote.ConfigureRequest{
		Endpoint:       "wss://nexus.example.com/connect",
		DeviceSecret:   "secret-value",
		Solver:         "http-01",
		TLD:            "example.com",
		PortalHostname: "portal.example.com",
	}); err != nil {
		t.Fatalf("remote configure: %v", err)
	}
	status := srv.remoteManager.Status()
	if !status.Enabled {
		t.Fatalf("remote status not enabled: %+v", status)
	}
	if strings.TrimSpace(status.TLD) == "" {
		t.Fatalf("remote status missing tld: %+v", status)
	}
	if host := srv.remoteServiceHostname(&status, services.ServiceEndpoint{Name: "web"}); host == "" {
		t.Fatalf("remote hostname derivation failed")
	}
	srv.refreshRemoteRuntime()

	_, err := srv.appManager.Install(context.Background(), &api.AppDefinition{
		Name:  "blog",
		Image: "docker.io/library/nginx:alpine",
		Type:  "user",
		Listeners: []api.AppListener{{
			Name:      "web",
			GuestPort: 80,
			Flow:      api.FlowTCP,
			Protocol:  api.ListenerProtocolHTTP,
		}},
	})
	if err != nil {
		t.Fatalf("install app: %v", err)
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/apps/blog", nil)
	attachAuth(req, sessionCookie, csrf)
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	var resp GinAppResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("response data not object: %#v", resp.Data)
	}
	rawServices, ok := data["services"].([]interface{})
	if !ok || len(rawServices) == 0 {
		t.Fatalf("expected services list in response: %#v", data)
	}
	first, ok := rawServices[0].(map[string]interface{})
	if !ok {
		t.Fatalf("service entry not object: %#v", rawServices[0])
	}

	remoteHost, ok := first["remote_host"].(string)
	if !ok {
		t.Fatalf("expected remote_host field on service: %#v", first)
	}
	if remoteHost != "web.example.com" {
		t.Fatalf("unexpected remote_host %q", remoteHost)
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
	sessionCookie, csrfToken := setupTestAdminSession(t, server)

	appDef := &api.AppDefinition{
		Name:      "test-app",
		Image:     "nginx:alpine",
		Type:      "user",
		Listeners: []api.AppListener{{Name: "web", GuestPort: 80}},
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
			attachAuth(req, sessionCookie, csrfToken)
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
	sessionCookie, csrfToken := setupTestAdminSession(t, server)

	appDef := &api.AppDefinition{
		Name:      "test-app",
		Image:     "alpine:latest",
		Type:      "user",
		Listeners: []api.AppListener{{Name: "web", GuestPort: 80}},
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
			name:           "wrong method for action",
			method:         "GET",
			url:            "/api/v1/apps/test-app/start",
			expectedStatus: http.StatusNotFound, // Gin returns 404 for unregistered routes
			expectError:    false,               // 404 responses are plain text, not JSON
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
			attachAuth(req, sessionCookie, csrfToken)
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
	sessionCookie, csrfToken := setupTestAdminSession(t, server)

	appYAML := `name: lifecycle-test
image: docker.io/library/nginx:alpine
type: user
listeners:
  - name: web
    guest_port: 80
    flow: tcp
    protocol: http
environment:
  TEST_ENV: "lifecycle"`

	// 1. Install app via HTTP API
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/apps", strings.NewReader(appYAML))
	req.Header.Set("Content-Type", "application/x-yaml")
	attachAuth(req, sessionCookie, csrfToken)
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("Failed to install app: status %d, body: %s", w.Code, w.Body.String())
	}

	// 2. Verify app appears in list
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/apps", nil)
	attachAuth(req, sessionCookie, csrfToken)
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Failed to list apps: status %d", w.Code)
	}

	// 3. Get specific app details
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/apps/lifecycle-test", nil)
	attachAuth(req, sessionCookie, csrfToken)
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Failed to get app details: status %d", w.Code)
	}

	// 4. Start the app
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/apps/lifecycle-test/start", nil)
	attachAuth(req, sessionCookie, csrfToken)
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Failed to start app: status %d, body: %s", w.Code, w.Body.String())
	}

	// 5. Stop the app
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/apps/lifecycle-test/stop", nil)
	attachAuth(req, sessionCookie, csrfToken)
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Failed to stop app: status %d", w.Code)
	}

	// 6. Uninstall the app
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("DELETE", "/api/v1/apps/lifecycle-test", nil)
	attachAuth(req, sessionCookie, csrfToken)
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Failed to uninstall app: status %d", w.Code)
	}

	// 7. Verify app is gone
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/apps", nil)
	attachAuth(req, sessionCookie, csrfToken)
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
	sessionCookie, csrfToken := setupTestAdminSession(t, server)

	appDef := &api.AppDefinition{
		Name:      "test-app",
		Image:     "alpine:latest",
		Type:      "user",
		Listeners: []api.AppListener{{Name: "web", GuestPort: 80}},
	}

	_, err = server.appManager.Install(context.Background(), appDef)
	if err != nil {
		t.Fatalf("Failed to install app: %v", err)
	}

	// Test successful uninstall
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/api/v1/apps/test-app", nil)
	attachAuth(req, sessionCookie, csrfToken)
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
	attachAuth(req, sessionCookie, csrfToken)
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
	sessionCookie, csrfToken := setupTestAdminSession(t, server)

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
			expectedStatus: http.StatusNotFound, // Trailing slash redirect disabled; expect 404
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
			attachAuth(req, sessionCookie, csrfToken)
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

	// Create filesystem app manager with service manager
	svcMgr := services.NewServiceManager()
    appMgr, err := app.NewAppManagerWithServices(mockContainer, tempDir, svcMgr, nil)
	if err != nil {
		t.Fatalf("Failed to create app manager: %v", err)
	}
	eventsBus := events.NewBus()
	appMgr.ObserveRuntimeEvents(eventsBus)
	eventsBus.Publish(events.Event{Topic: events.TopicLockStateChanged, Payload: events.LockStateChanged{Locked: false}})
	appMgr.ForceLockState(false)

	// Supporting managers for auth and crypto
	authMgr, err := authpkg.NewManager(tempDir)
	if err != nil {
		t.Fatalf("auth manager init: %v", err)
	}
	cryptoMgr, err := crypt.NewManager(tempDir)
	if err != nil {
		t.Fatalf("crypto manager init: %v", err)
	}

	// Create minimal server instance for testing
	rm, err := remote.NewManager(tempDir)
	if err != nil {
		t.Fatalf("remote mgr: %v", err)
	}
	rm.SetNexusAdapter(nexusclient.NewStub())
	tlsMux := services.NewTlsMux(svcMgr)
	remoteResolver := newServiceRemoteResolver(svcMgr)
	server := &GinServer{
		appManager:     appMgr,
		serviceManager: svcMgr,
		mdnsManager:    mdns.NewManager(),
		remoteManager:  rm,
		authManager:    authMgr,
		sessions:       authpkg.NewSessionStore(),
		cryptoManager:  cryptoMgr,
		version:        "test-gin",
		healthTracker:  health.NewTracker(),
		tlsMux:         tlsMux,
		remoteResolver: remoteResolver,
	}
	server.events = eventsBus
	server.healthTracker.Setf("app-manager", health.LevelOK, "test app manager ready")
	server.healthTracker.Setf("service-manager", health.LevelOK, "test service manager ready")
	server.healthTracker.Setf("mdns", health.LevelOK, "mdns stub")
	server.healthTracker.Setf("remote", health.LevelOK, "remote stub")
	server.healthTracker.Setf("persistence", health.LevelOK, "stub persistence ready")
	server.registerUnlockReloader(rm)
	server.observeRemoteConfig(eventsBus)
	rm.SetEventsBus(eventsBus)

	// Setup Gin routes
	server.setupGinRoutes()
	if err := server.initSecureLoopback(); err != nil {
		t.Fatalf("secure loopback init: %v", err)
	}
	server.refreshRemoteRuntime()

	return server
}

func TestLeadership_FollowerStopsApp(t *testing.T) {
	srv := createGinTestServer(t, t.TempDir())
	sessionCookie, csrf := setupTestAdminSession(t, srv)

	// Install a simple app via API
	payload := "name: blog\nimage: docker.io/library/nginx:alpine\ntype: user\nlisteners:\n  - name: web\n    guest_port: 80\n    flow: tcp\n    protocol: http\n"
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/apps", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/x-yaml")
	attachAuth(req, sessionCookie, csrf)
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("install status=%d body=%s", w.Code, w.Body.String())
	}

	// Publish follower role for this app
	srv.events.Publish(events.Event{Topic: events.TopicLeadershipRoleChanged, Payload: events.LeadershipChanged{Resource: cluster.ResourceForApp("blog"), Role: cluster.RoleFollower}})

	// Wait briefly for goroutine to act
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		app, err := srv.appManager.Get(context.Background(), "blog")
		if err == nil && app.Status == "stopped" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	app, _ := srv.appManager.Get(context.Background(), "blog")
	t.Fatalf("expected app to be stopped after follower event, got status=%v", app.Status)
}

// setupTestAdminSession provisions the admin password and returns session cookie/CSRF token.
func setupTestAdminSession(t *testing.T, server *GinServer) (*http.Cookie, string) {
	t.Helper()
	const password = "TestPass123!"

	// First-run setup
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/auth/setup", strings.NewReader(fmt.Sprintf(`{"password":"%s"}`, password)))
	req.Header.Set("Content-Type", "application/json")
	server.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		// Allow already-initialized if tests re-use the helper on same server
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "already") {
			t.Fatalf("auth setup failed: status=%d body=%s", w.Code, w.Body.String())
		}
	}

	// Login to obtain session cookie
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(fmt.Sprintf(`{"username":"admin","password":"%s"}`, password)))
	req.Header.Set("Content-Type", "application/json")
	server.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("auth login failed: status=%d body=%s", w.Code, w.Body.String())
	}
	var sessionCookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookieName {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatalf("missing session cookie in login response")
	}

	// Fetch CSRF token
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/api/v1/auth/csrf", nil)
	req.AddCookie(sessionCookie)
	server.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("csrf fetch failed: status=%d body=%s", w.Code, w.Body.String())
	}
	var csrfResp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &csrfResp); err != nil {
		t.Fatalf("parse csrf response: %v", err)
	}
	if csrfResp.Token == "" {
		t.Fatalf("csrf token empty")
	}

	return sessionCookie, csrfResp.Token
}

// attachAuth applies session cookie and CSRF header when required for the request.
func attachAuth(req *http.Request, cookie *http.Cookie, csrfToken string) {
	if cookie != nil {
		req.AddCookie(cookie)
	}
	switch req.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return
	}
	if csrfToken != "" {
		req.Header.Set("X-CSRF-Token", csrfToken)
	}
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
	containers  map[string]*MockContainer
	nextID      int
	createError error
	startError  error
	stopError   error
	removeError error
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

func (m *GinMockContainerManager) PullImage(ctx context.Context, image string) error {
	return nil
}

func (m *GinMockContainerManager) Logs(ctx context.Context, containerID string, lines int) ([]string, error) {
	if _, ok := m.containers[containerID]; !ok {
		return nil, container.ErrContainerNotFound(containerID)
	}
	if lines <= 0 {
		lines = 2
	}
	out := []string{}
	for i := 0; i < lines; i++ {
		out = append(out, "demo log entry")
	}
	return out, nil
}
