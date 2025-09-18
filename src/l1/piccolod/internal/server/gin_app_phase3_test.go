package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestAppPhase3_EndpointsReturnOK(t *testing.T) {
	srv := createGinTestServer(t, t.TempDir())
	sessionCookie, csrfToken := setupTestAdminSession(t, srv)

	// Demo mode allows lifecycle actions to succeed without real backend
	os.Setenv("PICCOLO_DEMO", "1")
	t.Cleanup(func() { os.Unsetenv("PICCOLO_DEMO") })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/apps/demo/start", nil)
	attachAuth(req, sessionCookie, csrfToken)
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("start status %d", w.Code)
	}

	// Stop should also succeed in demo mode
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/apps/demo/stop", nil)
	attachAuth(req, sessionCookie, csrfToken)
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("stop status %d", w.Code)
	}

	// Catalog
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/catalog", nil)
	attachAuth(req, sessionCookie, csrfToken)
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("catalog status %d", w.Code)
	}

	// Catalog template
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/catalog/wordpress/template", nil)
	attachAuth(req, sessionCookie, csrfToken)
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("catalog template status %d", w.Code)
	}
}
