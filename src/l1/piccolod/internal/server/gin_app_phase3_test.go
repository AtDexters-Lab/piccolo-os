package server

import (
    "net/http"
    "net/http/httptest"
    "os"
    "testing"
)

func TestAppPhase3_EndpointsReturnOK(t *testing.T) {
    srv := createGinTestServer(t, t.TempDir())

    // Logs endpoint for non-existing app in demo should still return 200
    os.Setenv("PICCOLO_DEMO", "1")
    t.Cleanup(func(){ os.Unsetenv("PICCOLO_DEMO") })

    w := httptest.NewRecorder()
    req, _ := http.NewRequest("GET", "/api/v1/apps/demo/logs", nil)
    srv.router.ServeHTTP(w, req)
    if w.Code != http.StatusOK { t.Fatalf("logs status %d", w.Code) }

    // Update/revert endpoints should 200 in demo
    w = httptest.NewRecorder(); req, _ = http.NewRequest("POST", "/api/v1/apps/demo/update", nil)
    srv.router.ServeHTTP(w, req); if w.Code != http.StatusOK { t.Fatalf("update status %d", w.Code) }
    w = httptest.NewRecorder(); req, _ = http.NewRequest("POST", "/api/v1/apps/demo/revert", nil)
    srv.router.ServeHTTP(w, req); if w.Code != http.StatusOK { t.Fatalf("revert status %d", w.Code) }

    // Catalog
    w = httptest.NewRecorder(); req, _ = http.NewRequest("GET", "/api/v1/catalog", nil)
    srv.router.ServeHTTP(w, req); if w.Code != http.StatusOK { t.Fatalf("catalog status %d", w.Code) }
}

