package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	authpkg "piccolod/internal/auth"
	"piccolod/internal/crypt"
	"piccolod/internal/persistence"
	"piccolod/internal/runtime/commands"
)

// setupAuthTestServer returns a GinServer ready to serve auth endpoints with isolated state.
func setupAuthTestServer(t *testing.T) *GinServer {
	t.Helper()
	gin.SetMode(gin.TestMode)
	tempDir, err := os.MkdirTemp("", "auth_test")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })

	// Reuse createGinTestServer to get a minimal server/router
	srv := createGinTestServer(t, tempDir)
	am, err := authpkg.NewManager(tempDir)
	if err != nil {
		t.Fatalf("auth manager: %v", err)
	}
	srv.authManager = am
	srv.sessions = authpkg.NewSessionStore()
	return srv
}

func TestAuth_Setup_Login_Session_Logout(t *testing.T) {
	srv := setupAuthTestServer(t)

	// 1) session should be unauthenticated initially
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/auth/session", nil)
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("session status %d", w.Code)
	}
	var sess map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &sess)
	if sess["authenticated"].(bool) {
		t.Fatalf("expected unauthenticated")
	}

	// 2) setup admin
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/auth/setup", strings.NewReader(`{"password":"pw123456"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("setup status %d body=%s", w.Code, w.Body.String())
	}

	// 3) wrong login -> 401
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{"username":"admin","password":"wrong"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}

	// 4) correct login -> Set-Cookie piccolo_session
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{"username":"admin","password":"pw123456"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("login status %d body=%s", w.Code, w.Body.String())
	}
	cookie := w.Result().Cookies()
	var sessCookie string
	for _, c := range cookie {
		if c.Name == sessionCookieName {
			sessCookie = c.Value
		}
	}
	if sessCookie == "" {
		t.Fatalf("missing session cookie")
	}

	// 5) session now authenticated
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/auth/session", nil)
	req.Header.Set("Cookie", sessionCookieName+"="+sessCookie)
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("session2 status %d", w.Code)
	}
	_ = json.Unmarshal(w.Body.Bytes(), &sess)
	if !sess["authenticated"].(bool) {
		t.Fatalf("expected authenticated")
	}

	// 6) csrf token available
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/auth/csrf", nil)
	req.Header.Set("Cookie", sessionCookieName+"="+sessCookie)
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("csrf status %d", w.Code)
	}
	var csrf map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &csrf)
	token := csrf["token"]

	// 7) change password wrong old -> 401
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/auth/password", strings.NewReader(`{"old_password":"bad","new_password":"pw234567"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", sessionCookieName+"="+sessCookie)
	req.Header.Set("X-CSRF-Token", token)
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("password expected 401, got %d", w.Code)
	}

	// 8) correct change password -> 200
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/auth/password", strings.NewReader(`{"old_password":"pw123456","new_password":"pw234567"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", sessionCookieName+"="+sessCookie)
	req.Header.Set("X-CSRF-Token", token)
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("password change status %d", w.Code)
	}

	// 9) logout
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/auth/logout", nil)
	req.Header.Set("Cookie", sessionCookieName+"="+sessCookie)
	req.Header.Set("X-CSRF-Token", token)
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("logout status %d", w.Code)
	}

	// 10) session should be unauthenticated again
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/auth/session", nil)
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("final session status %d", w.Code)
	}
	_ = json.Unmarshal(w.Body.Bytes(), &sess)
	if sess["authenticated"].(bool) {
		t.Fatalf("expected unauthenticated after logout")
	}
}

func TestAuth_LoginRateLimit(t *testing.T) {
	srv := setupAuthTestServer(t)
	// setup
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/auth/setup", strings.NewReader(`{"password":"pw123456"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("setup: %d", w.Code)
	}

	// 5 failed attempts
	for i := 0; i < 5; i++ {
		w = httptest.NewRecorder()
		req, _ = http.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{"username":"admin","password":"bad"}`))
		req.Header.Set("Content-Type", "application/json")
		srv.router.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("try %d expected 401, got %d", i+1, w.Code)
		}
	}
	// Next should yield 429
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{"username":"admin","password":"bad"}`))
	req.Header.Set("Content-Type", "application/json")
	// (Next line intentionally left unchanged)
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", w.Code)
	}
	if got := w.Result().Header.Get("Retry-After"); got == "" {
		t.Fatalf("missing Retry-After")
	}
}

func TestAuthLoginUnlocksLockedControlStore(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const password = "pw123456"

	state := mustAuthState(t, password)
	storage := &lockingAuthStorage{state: state, locked: true}
	authMgr, err := authpkg.NewManagerWithStorage(storage)
	if err != nil {
		t.Fatalf("auth manager init: %v", err)
	}

	tempDir := t.TempDir()
	cryptoMgr, err := crypt.NewManager(tempDir)
	if err != nil {
		t.Fatalf("crypto manager init: %v", err)
	}
	if err := cryptoMgr.Setup(password); err != nil {
		t.Fatalf("crypto setup: %v", err)
	}
	cryptoMgr.Lock()

	srv := &GinServer{
		authManager:   authMgr,
		cryptoManager: cryptoMgr,
		sessions:      authpkg.NewSessionStore(),
		dispatcher:    commands.NewDispatcher(),
	}

	srv.dispatcher.Register(persistence.CommandRecordLockState, commands.HandlerFunc(func(ctx context.Context, cmd commands.Command) (commands.Response, error) {
		record, ok := cmd.(persistence.RecordLockStateCommand)
		if !ok {
			t.Fatalf("unexpected command type %#v", cmd)
		}
		storage.setLocked(record.Locked)
		return nil, nil
	}))

	router := gin.New()
	srv.router = router
	router.POST("/api/v1/auth/login", srv.handleAuthLogin)

	w := httptest.NewRecorder()
	reqBody := fmt.Sprintf(`{"username":"admin","password":"%s"}`, password)
	req, _ := http.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if storage.isLocked() {
		t.Fatalf("expected control store unlocked")
	}
	if cryptoMgr.IsLocked() {
		t.Fatalf("expected crypto manager unlocked")
	}
	found := false
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookieName && c.Value != "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected session cookie in response")
	}
}

func mustAuthState(t *testing.T, password string) authpkg.State {
	t.Helper()
	tempDir := t.TempDir()
	manager, err := authpkg.NewManager(tempDir)
	if err != nil {
		t.Fatalf("auth manager init: %v", err)
	}
	if err := manager.Setup(context.Background(), password); err != nil {
		t.Fatalf("auth setup: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(tempDir, "auth", "admin.json"))
	if err != nil {
		t.Fatalf("read auth state: %v", err)
	}
	var raw struct {
		Initialized bool   `json:"initialized"`
		Hash        string `json:"password_hash"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal auth state: %v", err)
	}
	return authpkg.State{Initialized: raw.Initialized, PasswordHash: raw.Hash}
}

type lockingAuthStorage struct {
	mu     sync.RWMutex
	state  authpkg.State
	locked bool
}

func (s *lockingAuthStorage) Load(ctx context.Context) (authpkg.State, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.locked {
		return authpkg.State{}, persistence.ErrLocked
	}
	return s.state, nil
}

func (s *lockingAuthStorage) Save(ctx context.Context, state authpkg.State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.locked {
		return persistence.ErrLocked
	}
	s.state = state
	return nil
}

func (s *lockingAuthStorage) setLocked(v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.locked = v
}

func (s *lockingAuthStorage) isLocked() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.locked
}
