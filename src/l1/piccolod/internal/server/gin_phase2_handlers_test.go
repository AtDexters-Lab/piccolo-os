package server

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "os"
    "testing"
    "piccolod/internal/remote"
)

func setupBasicServer(t *testing.T) *GinServer {
    t.Helper()
    tempDir, err := os.MkdirTemp("", "phase2")
    if err != nil { t.Fatalf("tempdir: %v", err) }
    t.Cleanup(func(){ _ = os.RemoveAll(tempDir) })
    srv := createGinTestServer(t, tempDir)
    if srv.remoteManager == nil {
        rm, err := remote.NewManager(tempDir)
        if err != nil { t.Fatalf("remote mgr: %v", err) }
        srv.remoteManager = rm
        srv.setupGinRoutes()
    }
    return srv
}

func TestOSUpdateStatus_OK(t *testing.T) {
    srv := setupBasicServer(t)
    w := httptest.NewRecorder()
    req, _ := http.NewRequest("GET", "/api/v1/updates/os", nil)
    srv.router.ServeHTTP(w, req)
    if w.Code != http.StatusOK { t.Fatalf("status %d", w.Code) }
    var m map[string]any
    _ = json.Unmarshal(w.Body.Bytes(), &m)
    if _, ok := m["current_version"]; !ok { t.Fatalf("missing current_version") }
}

func TestRemoteStatus_OK(t *testing.T) {
    srv := setupBasicServer(t)
    w := httptest.NewRecorder()
    req, _ := http.NewRequest("GET", "/api/v1/remote/status", nil)
    srv.router.ServeHTTP(w, req)
    if w.Code != http.StatusOK { t.Fatalf("status %d", w.Code) }
}

func TestStorageDisks_OK(t *testing.T) {
    srv := setupBasicServer(t)
    w := httptest.NewRecorder()
    req, _ := http.NewRequest("GET", "/api/v1/storage/disks", nil)
    srv.router.ServeHTTP(w, req)
    if w.Code != http.StatusOK { t.Fatalf("status %d", w.Code) }
}
