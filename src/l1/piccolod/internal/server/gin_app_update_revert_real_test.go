package server

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestAppUpdateRevertAndLogs_RealHandlers(t *testing.T) {
    srv := createGinTestServer(t, t.TempDir())

    // Install via API
    body := []byte("name: demo\nimage: alpine:3.18\ntype: user\nlisteners:\n - name: web\n   guest_port: 80\n")
    w := httptest.NewRecorder()
    req, _ := http.NewRequest("POST", "/api/v1/apps", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/x-yaml")
    srv.router.ServeHTTP(w, req)
    if w.Code != http.StatusCreated { t.Fatalf("install status %d body=%s", w.Code, w.Body.String()) }

    // Update tag
    w = httptest.NewRecorder()
    payload := map[string]string{"tag":"3.19"}
    pbytes, _ := json.Marshal(payload)
    req, _ = http.NewRequest("POST", "/api/v1/apps/demo/update", bytes.NewReader(pbytes))
    req.Header.Set("Content-Type", "application/json")
    srv.router.ServeHTTP(w, req)
    if w.Code != http.StatusOK { t.Fatalf("update status %d body=%s", w.Code, w.Body.String()) }

    // Verify image changed
    w = httptest.NewRecorder()
    req, _ = http.NewRequest("GET", "/api/v1/apps/demo", nil)
    srv.router.ServeHTTP(w, req)
    if w.Code != http.StatusOK { t.Fatalf("get after update %d", w.Code) }
    var resp struct{ Data struct{ App struct{ Image string `json:"image"` } `json:"app"` } `json:"data"` }
    if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil { t.Fatal(err) }
    if resp.Data.App.Image != "alpine:3.19" { t.Fatalf("expected alpine:3.19, got %s", resp.Data.App.Image) }

    // Revert
    w = httptest.NewRecorder()
    req, _ = http.NewRequest("POST", "/api/v1/apps/demo/revert", nil)
    srv.router.ServeHTTP(w, req)
    if w.Code != http.StatusOK { t.Fatalf("revert status %d body=%s", w.Code, w.Body.String()) }
    w = httptest.NewRecorder()
    req, _ = http.NewRequest("GET", "/api/v1/apps/demo", nil)
    srv.router.ServeHTTP(w, req)
    if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil { t.Fatal(err) }
    if resp.Data.App.Image != "alpine:3.18" { t.Fatalf("expected alpine:3.18 after revert, got %s", resp.Data.App.Image) }

    // Logs
    w = httptest.NewRecorder()
    req, _ = http.NewRequest("GET", "/api/v1/apps/demo/logs?tail=3", nil)
    srv.router.ServeHTTP(w, req)
    if w.Code != http.StatusOK { t.Fatalf("logs status %d", w.Code) }
    var logs struct{ Entries []string `json:"entries"` }
    if err := json.Unmarshal(w.Body.Bytes(), &logs); err != nil { t.Fatal(err) }
    if len(logs.Entries) != 3 { t.Fatalf("expected 3 log entries, got %d", len(logs.Entries)) }
}

