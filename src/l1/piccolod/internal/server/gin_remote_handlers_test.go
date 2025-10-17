package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"piccolod/internal/events"
	"piccolod/internal/health"
	"piccolod/internal/remote"
	"piccolod/internal/remote/nexusclient"
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

type toggledRemoteStorage struct {
	mu     sync.Mutex
	locked bool
	cfg    remote.Config
}

func newToggledRemoteStorage() *toggledRemoteStorage {
	return &toggledRemoteStorage{}
}

func (s *toggledRemoteStorage) Load(ctx context.Context) (remote.Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.locked {
		return remote.Config{}, remote.ErrLocked
	}
	return cloneRemoteConfig(s.cfg), nil
}

func (s *toggledRemoteStorage) Save(ctx context.Context, cfg remote.Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.locked {
		return remote.ErrLocked
	}
	s.cfg = cloneRemoteConfig(cfg)
	return nil
}

func (s *toggledRemoteStorage) SetLocked(v bool) {
	s.mu.Lock()
	s.locked = v
	s.mu.Unlock()
}

func cloneRemoteConfig(cfg remote.Config) remote.Config {
	out := cfg
	if cfg.DNSCredentials != nil {
		out.DNSCredentials = make(map[string]string, len(cfg.DNSCredentials))
		for k, v := range cfg.DNSCredentials {
			out.DNSCredentials[k] = v
		}
	}
	if cfg.Aliases != nil {
		out.Aliases = append([]remote.Alias(nil), cfg.Aliases...)
	}
	if cfg.Certificates != nil {
		out.Certificates = append([]remote.Certificate(nil), cfg.Certificates...)
	}
	if cfg.Events != nil {
		out.Events = append([]remote.Event(nil), cfg.Events...)
	}
	if cfg.GuideVerifiedAt != nil {
		ts := *cfg.GuideVerifiedAt
		out.GuideVerifiedAt = &ts
	}
	if cfg.LastPreflight != nil {
		ts := *cfg.LastPreflight
		out.LastPreflight = &ts
	}
	return out
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

func TestRemote_ReloadsConfigAfterUnlockEvent(t *testing.T) {
	t.Setenv("PICCOLO_REMOTE_FAKE_ACME", "1")

	storage := newToggledRemoteStorage()
	storage.SetLocked(false)

	primaryMgr, err := remote.NewManagerWithStorage(storage)
	if err != nil {
		t.Fatalf("primary manager init: %v", err)
	}
	if err := primaryMgr.Configure(remote.ConfigureRequest{
		Endpoint:       "wss://nexus.example.com/connect",
		DeviceSecret:   "primary-secret",
		Solver:         "http-01",
		TLD:            "example.com",
		PortalHostname: "portal.example.com",
	}); err != nil {
		t.Fatalf("configure primary: %v", err)
	}

	storage.SetLocked(true)

	restartedMgr, err := remote.NewManagerWithStorage(storage)
	if err != nil {
		t.Fatalf("restarted manager init: %v", err)
	}
	restartedMgr.SetNexusAdapter(nexusclient.NewStub())

	if st := restartedMgr.Status(); st.Enabled || st.PortalHostname != "" {
		t.Fatalf("expected remote config unavailable before unlock, got %+v", st)
	}

	server := &GinServer{
		remoteManager: restartedMgr,
		healthTracker: health.NewTracker(),
	}
	server.registerUnlockReloader(restartedMgr)
	bus := events.NewBus()
	server.observeLockState(bus)

	storage.SetLocked(false)
	bus.Publish(events.Event{
		Topic: events.TopicLockStateChanged,
		Payload: events.LockStateChanged{
			Locked: false,
		},
	})

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		st := restartedMgr.Status()
		if st.Enabled && st.PortalHostname == "portal.example.com" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	st := restartedMgr.Status()
	t.Fatalf("expected remote configuration to reload after unlock, got enabled=%v portal=%s", st.Enabled, st.PortalHostname)
}

func TestRemote_TlsMuxRestartsAfterReload(t *testing.T) {
	t.Setenv("PICCOLO_REMOTE_FAKE_ACME", "1")

	tempDir := t.TempDir()
	srv := createGinTestServer(t, tempDir)
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
	if w.Code != http.StatusOK {
		t.Fatalf("configure status=%d body=%s", w.Code, w.Body.String())
	}
	if port := srv.tlsMux.Port(); port == 0 {
		t.Fatalf("expected tls mux to start after configure")
	}

	// Wait for asynchronous certificate issuance to complete to avoid concurrent config writes.
	certDeadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(certDeadline) {
		done := false
		for _, cert := range srv.remoteManager.ListCertificates() {
			if cert.ID == "portal" && strings.EqualFold(cert.Status, "ok") {
				done = true
				break
			}
		}
		if done {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Ensure config is durably written before simulating restart.
	configPath := filepath.Join(tempDir, "remote", "config.json")
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(configPath)
		if err != nil || len(data) == 0 {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		var cfg remote.Config
		if err := json.Unmarshal(data, &cfg); err != nil {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		break
	}

	// Simulate restart by creating a new server instance with the same state dir.
	srv2 := createGinTestServer(t, tempDir)
	if st := srv2.remoteManager.Status(); !st.Enabled {
		t.Fatalf("expected remote manager to be enabled after restart, got %+v", st)
	}
	if port := srv2.tlsMux.Port(); port != 0 {
		// Unexpected but harmless: mux already running.
		return
	}

	// Publish unlock event to trigger reload logic and TLS mux startup.
	srv2.events.Publish(events.Event{
		Topic: events.TopicLockStateChanged,
		Payload: events.LockStateChanged{
			Locked: false,
		},
	})

	deadline = time.Now().Add(750 * time.Millisecond)
	for time.Now().Before(deadline) {
		if srv2.tlsMux.Port() != 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected tls mux to start after unlock reload; port=%d", srv2.tlsMux.Port())
}
