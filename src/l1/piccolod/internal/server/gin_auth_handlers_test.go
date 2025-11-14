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
	"time"

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
	repo := newMemoryAuthRepo()
	authStorage := newPersistenceAuthStorage(repo)
	am, err := authpkg.NewManagerWithStorage(authStorage)
	if err != nil {
		t.Fatalf("auth manager: %v", err)
	}
	srv.authManager = am
	srv.authRepo = repo
	srv.sessions = authpkg.NewSessionStore()
	return srv
}

type memoryAuthRepo struct {
	mu          sync.Mutex
	initialized bool
	hash        string
	staleness   persistence.AuthStaleness
}

func newMemoryAuthRepo() *memoryAuthRepo {
	return &memoryAuthRepo{}
}

func (m *memoryAuthRepo) IsInitialized(ctx context.Context) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.initialized, nil
}

func (m *memoryAuthRepo) SetInitialized(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.initialized = true
	return nil
}

func (m *memoryAuthRepo) PasswordHash(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.hash, nil
}

func (m *memoryAuthRepo) SavePasswordHash(ctx context.Context, hash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hash = hash
	return nil
}

func (m *memoryAuthRepo) Staleness(ctx context.Context) (persistence.AuthStaleness, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.staleness, nil
}

func (m *memoryAuthRepo) UpdateStaleness(ctx context.Context, update persistence.AuthStalenessUpdate) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if update.PasswordStale != nil {
		m.staleness.PasswordStale = *update.PasswordStale
	}
	if update.PasswordStaleAt != nil {
		m.staleness.PasswordStaleAt = *update.PasswordStaleAt
	}
	if update.PasswordAckAt != nil {
		m.staleness.PasswordAckAt = *update.PasswordAckAt
	}
	if update.RecoveryStale != nil {
		m.staleness.RecoveryStale = *update.RecoveryStale
	}
	if update.RecoveryStaleAt != nil {
		m.staleness.RecoveryStaleAt = *update.RecoveryStaleAt
	}
	if update.RecoveryAckAt != nil {
		m.staleness.RecoveryAckAt = *update.RecoveryAckAt
	}
	return nil
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

	// 4 failed attempts
	for i := 0; i < 4; i++ {
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

func TestAuthSessionIncludesStaleness(t *testing.T) {
	srv := setupAuthTestServer(t)
	ctx := context.Background()

	// initialize auth
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/auth/setup", strings.NewReader(`{"password":"pw123456"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("setup: %d", w.Code)
	}

	repo, ok := srv.authRepo.(*memoryAuthRepo)
	if !ok {
		t.Fatalf("unexpected repo type %T", srv.authRepo)
	}
	if err := repo.UpdateStaleness(ctx, persistence.AuthStalenessUpdate{
		PasswordStale: boolPtr(true),
	}); err != nil {
		t.Fatalf("update staleness: %v", err)
	}

	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/auth/session", nil)
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("session: %d", w.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !resp["password_stale"].(bool) {
		t.Fatalf("expected password_stale=true, got %v", resp["password_stale"])
	}
	if resp["recovery_stale"].(bool) {
		t.Fatalf("expected recovery_stale=false by default")
	}
}

func TestAuthStalenessAckClearsFlags(t *testing.T) {
	srv := setupAuthTestServer(t)
	sessionCookie, csrf := setupTestAdminSession(t, srv)

	repo, ok := srv.authRepo.(*memoryAuthRepo)
	if !ok {
		t.Fatalf("unexpected repo type %T", srv.authRepo)
	}
	now := time.Now().UTC()
	if err := repo.UpdateStaleness(context.Background(), persistence.AuthStalenessUpdate{
		PasswordStale:   boolPtr(true),
		PasswordStaleAt: timePtr(now),
		RecoveryStale:   boolPtr(true),
		RecoveryStaleAt: timePtr(now),
	}); err != nil {
		t.Fatalf("update staleness: %v", err)
	}

	body := `{"password":true,"recovery":true}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/auth/staleness/ack", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	attachAuth(req, sessionCookie, csrf)
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("ack: %d body=%s", w.Code, w.Body.String())
	}

	state, err := repo.Staleness(context.Background())
	if err != nil {
		t.Fatalf("repo staleness: %v", err)
	}
	if state.PasswordStale || state.RecoveryStale {
		t.Fatalf("expected flags cleared, got %+v", state)
	}
	if state.PasswordAckAt.IsZero() || state.RecoveryAckAt.IsZero() {
		t.Fatalf("expected ack timestamps set")
	}
}

func TestCryptoRecoveryStatusStale(t *testing.T) {
	srv := setupAuthTestServer(t)
	// Setup auth to ensure repo initialized to avoid ErrLocked
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/auth/setup", strings.NewReader(`{"password":"pw123456"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("setup: %d", w.Code)
	}
	repo := srv.authRepo.(*memoryAuthRepo)
	if err := repo.UpdateStaleness(context.Background(), persistence.AuthStalenessUpdate{
		RecoveryStale: boolPtr(true),
	}); err != nil {
		t.Fatalf("update staleness: %v", err)
	}

	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/crypto/recovery-key", nil)
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("recovery status: %d", w.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !resp["stale"].(bool) {
		t.Fatalf("expected stale=true, got %v", resp["stale"])
	}
}

func TestCryptoResetPasswordFlow(t *testing.T) {
	srv := setupAuthTestServer(t)
	ctx := context.Background()

	// Setup crypto
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/crypto/setup", strings.NewReader(`{"password":"OrigPass123!"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("crypto setup: %d", w.Code)
	}
	// Setup auth
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/auth/setup", strings.NewReader(`{"password":"OrigPass123!"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("auth setup: %d", w.Code)
	}

	// Generate recovery key
	words, err := srv.cryptoManager.GenerateRecoveryKeyWithPassword("OrigPass123!", false)
	if err != nil {
		t.Fatalf("generate recovery key: %v", err)
	}
	recoveryKey := strings.Join(words, " ")

	// Ensure locked before reset
	srv.cryptoManager.Lock()

	// Reset password with recovery key
	body := fmt.Sprintf(`{"recovery_key":%q,"new_password":"NewPass456!"}`, recoveryKey)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/crypto/reset-password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("reset password: status=%d body=%s", w.Code, w.Body.String())
	}

	if !srv.cryptoManager.IsLocked() {
		t.Fatalf("expected crypto to relock after reset")
	}
	if err := srv.cryptoManager.Unlock("OrigPass123!"); err == nil {
		t.Fatalf("expected old password to fail after reset")
	}
	if err := srv.cryptoManager.Unlock("NewPass456!"); err != nil {
		t.Fatalf("new password unlock failed: %v", err)
	}

	ok, err := srv.authManager.Verify(ctx, "admin", "NewPass456!")
	if err != nil || !ok {
		t.Fatalf("expected auth manager to accept new password, ok=%v err=%v", ok, err)
	}
	ok, err = srv.authManager.Verify(ctx, "admin", "OrigPass123!")
	if err != nil {
		t.Fatalf("verify old password err: %v", err)
	}
	if ok {
		t.Fatalf("expected old password to fail verification")
	}

	state, err := srv.authRepo.Staleness(ctx)
	if err != nil {
		t.Fatalf("staleness fetch: %v", err)
	}
	if !state.PasswordStale || !state.RecoveryStale {
		t.Fatalf("expected staleness flags set, got %+v", state)
	}
}

func TestCryptoRecoveryKeyGenerateRotatesAndClearsStaleness(t *testing.T) {
	srv := setupAuthTestServer(t)
	sessionCookie, csrf := setupTestAdminSession(t, srv)
	repo, ok := srv.authRepo.(*memoryAuthRepo)
	if !ok {
		t.Fatalf("unexpected repo type %T", srv.authRepo)
	}

	// Initialize crypto with the same password used for auth setup.
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/crypto/setup", strings.NewReader(`{"password":"TestPass123!"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("crypto setup: %d body=%s", w.Code, w.Body.String())
	}

	initialWords, err := srv.cryptoManager.GenerateRecoveryKeyWithPassword("TestPass123!", false)
	if err != nil {
		t.Fatalf("initial generate: %v", err)
	}
	now := time.Now().UTC()
	if err := repo.UpdateStaleness(context.Background(), persistence.AuthStalenessUpdate{
		RecoveryStale:   boolPtr(true),
		RecoveryStaleAt: timePtr(now),
	}); err != nil {
		t.Fatalf("set staleness: %v", err)
	}
	if err := srv.cryptoManager.Unlock("TestPass123!"); err != nil {
		t.Fatalf("unlock before rotation: %v", err)
	}

	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/crypto/recovery-key/generate", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	attachAuth(req, sessionCookie, csrf)
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("rotate generate: status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Words []string `json:"words"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if len(resp.Words) != 24 {
		t.Fatalf("expected 24 words, got %d", len(resp.Words))
	}
	if strings.Join(resp.Words, " ") == strings.Join(initialWords, " ") {
		t.Fatalf("expected rotation to return new mnemonic")
	}
	state, err := repo.Staleness(context.Background())
	if err != nil {
		t.Fatalf("staleness fetch: %v", err)
	}
	if state.RecoveryStale {
		t.Fatalf("expected recovery staleness cleared, got %+v", state)
	}
	if !state.RecoveryStaleAt.IsZero() {
		t.Fatalf("expected stale timestamp reset, got %+v", state)
	}
	if !state.RecoveryAckAt.IsZero() {
		t.Fatalf("expected ack timestamp reset, got %+v", state)
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
