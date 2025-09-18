package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAppServicesDiscovery_RealHandlers(t *testing.T) {
	srv := createGinTestServer(t, t.TempDir())
	sessionCookie, csrfToken := setupTestAdminSession(t, srv)

	// Install via API
	body := []byte("name: demo\nimage: alpine:3.18\ntype: user\nlisteners:\n - name: web\n   guest_port: 80\n")
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/apps", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/x-yaml")
	attachAuth(req, sessionCookie, csrfToken)
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("install status %d body=%s", w.Code, w.Body.String())
	}

	// Global services listing should include the app listener
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/services", nil)
	attachAuth(req, sessionCookie, csrfToken)
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("services status %d", w.Code)
	}
	var servicesResp struct {
		Services []map[string]any `json:"services"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &servicesResp); err != nil {
		t.Fatal(err)
	}
	if len(servicesResp.Services) == 0 {
		t.Fatalf("expected at least one service, got 0")
	}

	// Per-app services
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/apps/demo/services", nil)
	attachAuth(req, sessionCookie, csrfToken)
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("app services status %d", w.Code)
	}
	var appServicesResp struct {
		Services []map[string]any `json:"services"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &appServicesResp); err != nil {
		t.Fatal(err)
	}
	if len(appServicesResp.Services) == 0 {
		t.Fatalf("expected at least one app service, got 0")
	}
}
