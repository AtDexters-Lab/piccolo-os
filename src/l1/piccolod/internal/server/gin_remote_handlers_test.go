package server

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestRemote_Configure_Status_Disable_Rotate(t *testing.T) {
    srv := createGinTestServer(t, t.TempDir())

    // Initial status disabled
    w := httptest.NewRecorder()
    req, _ := http.NewRequest("GET", "/api/v1/remote/status", nil)
    srv.router.ServeHTTP(w, req)
    if w.Code != http.StatusOK { t.Fatalf("status %d", w.Code) }

    // Configure
    payload := map[string]string{
        "endpoint": "https://nexus.example.com",
        "device_id": "dev123",
        "hostname": "host.example.com",
    }
    body, _ := json.Marshal(payload)
    w = httptest.NewRecorder()
    req, _ = http.NewRequest("POST", "/api/v1/remote/configure", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    srv.router.ServeHTTP(w, req)
    if w.Code != http.StatusOK { t.Fatalf("configure %d body=%s", w.Code, w.Body.String()) }

    // Status enabled
    w = httptest.NewRecorder()
    req, _ = http.NewRequest("GET", "/api/v1/remote/status", nil)
    srv.router.ServeHTTP(w, req)
    if w.Code != http.StatusOK { t.Fatalf("status2 %d", w.Code) }
    var st struct{ Enabled bool `json:"enabled"`; PublicURL *string `json:"public_url"` }
    if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil { t.Fatal(err) }
    if !st.Enabled || st.PublicURL == nil { t.Fatalf("expected enabled remote with url") }

    // Rotate
    w = httptest.NewRecorder()
    req, _ = http.NewRequest("POST", "/api/v1/remote/rotate", nil)
    srv.router.ServeHTTP(w, req)
    if w.Code != http.StatusOK { t.Fatalf("rotate %d", w.Code) }

    // Disable
    w = httptest.NewRecorder()
    req, _ = http.NewRequest("POST", "/api/v1/remote/disable", nil)
    srv.router.ServeHTTP(w, req)
    if w.Code != http.StatusOK { t.Fatalf("disable %d", w.Code) }

    // Status disabled
    w = httptest.NewRecorder()
    req, _ = http.NewRequest("GET", "/api/v1/remote/status", nil)
    srv.router.ServeHTTP(w, req)
    if w.Code != http.StatusOK { t.Fatalf("status3 %d", w.Code) }
    if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil { t.Fatal(err) }
    if st.Enabled { t.Fatalf("expected disabled") }
}

