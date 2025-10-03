package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"piccolod/internal/remote"
)

func TestRemote_Configure_Status_Disable_Rotate(t *testing.T) {
	srv := createGinTestServer(t, t.TempDir())
	sessionCookie, csrfToken := setupTestAdminSession(t, srv)

	// Initial status disabled
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/remote/status", nil)
	attachAuth(req, sessionCookie, csrfToken)
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}

	// Configure
	payload := map[string]interface{}{
		"endpoint":        "wss://nexus.example.com/connect",
		"device_secret":   "super-secret",
		"solver":          "http-01",
		"tld":             "example.com",
		"portal_hostname": "portal.example.com",
	}
	body, _ := json.Marshal(payload)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/remote/configure", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	attachAuth(req, sessionCookie, csrfToken)
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("configure %d body=%s", w.Code, w.Body.String())
	}

	// Status enabled
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/remote/status", nil)
	attachAuth(req, sessionCookie, csrfToken)
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status2 %d", w.Code)
	}
	var st struct {
		Enabled        bool   `json:"enabled"`
		PortalHostname string `json:"portal_hostname"`
		TLD            string `json:"tld"`
		State          string `json:"state"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if !st.Enabled {
		t.Fatalf("expected enabled remote")
	}
	if st.PortalHostname != "portal.example.com" {
		t.Fatalf("unexpected portal hostname %s", st.PortalHostname)
	}
	if st.TLD != "example.com" {
		t.Fatalf("unexpected tld %s", st.TLD)
	}

	// Rotate
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/remote/rotate", nil)
	attachAuth(req, sessionCookie, csrfToken)
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("rotate %d", w.Code)
	}
	var rotateResp struct {
		DeviceSecret string `json:"device_secret"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &rotateResp); err != nil {
		t.Fatalf("rotate decode: %v", err)
	}
	if rotateResp.DeviceSecret == "" {
		t.Fatalf("expected rotated secret in response")
	}

	// Disable
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/remote/disable", nil)
	attachAuth(req, sessionCookie, csrfToken)
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("disable %d", w.Code)
	}

	// Status disabled
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/remote/status", nil)
	attachAuth(req, sessionCookie, csrfToken)
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status3 %d", w.Code)
	}
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if st.Enabled {
		t.Fatalf("expected disabled")
	}
}

type lockedRemoteStorage struct{}

func (lockedRemoteStorage) Load(ctx context.Context) (remote.Config, error) {
	return remote.Config{}, remote.ErrLocked
}

func (lockedRemoteStorage) Save(ctx context.Context, cfg remote.Config) error {
	return remote.ErrLocked
}

func TestRemote_Configure_WhenLocked(t *testing.T) {
	srv := createGinTestServer(t, t.TempDir())
	lockedMgr, err := remote.NewManagerWithStorage(lockedRemoteStorage{})
	if err != nil {
		t.Fatalf("locked manager init: %v", err)
	}
	srv.remoteManager = lockedMgr
	sessionCookie, csrfToken := setupTestAdminSession(t, srv)

	payload := map[string]interface{}{
		"endpoint":        "wss://nexus.example.com/connect",
		"device_secret":   "super-secret",
		"solver":          "http-01",
		"tld":             "example.com",
		"portal_hostname": "portal.example.com",
	}
	body, _ := json.Marshal(payload)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/remote/configure", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	attachAuth(req, sessionCookie, csrfToken)
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusLocked {
		t.Fatalf("expected 423 Locked, got %d body=%s", w.Code, w.Body.String())
	}
}
