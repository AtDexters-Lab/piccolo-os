package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	authpkg "piccolod/internal/auth"
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
	srv.loginLimiter = newLoginRateLimiter()
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

	// 2) setup admin with a strong password -> auto-login
	const setupPassword = "StrongPass123!"
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/auth/setup", strings.NewReader(`{"password":"`+setupPassword+`"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("setup status %d body=%s", w.Code, w.Body.String())
	}
	var setupCookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookieName {
			setupCookie = c
			break
		}
	}
	if setupCookie == nil {
		t.Fatalf("missing session cookie after setup")
	}

	// 3) session should report authenticated immediately after setup
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/auth/session", nil)
	req.AddCookie(setupCookie)
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("post-setup session status %d", w.Code)
	}
	_ = json.Unmarshal(w.Body.Bytes(), &sess)
	if !sess["authenticated"].(bool) {
		t.Fatalf("expected authenticated after setup")
	}

	// 4) logout to exercise login flow separately
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/auth/logout", nil)
	req.AddCookie(setupCookie)
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("logout after setup status %d", w.Code)
	}

	// 5) wrong login -> 401
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{"username":"admin","password":"wrong"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.0.2.10:1234"
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}

	// 6) correct login -> Set-Cookie piccolo_session
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{"username":"admin","password":"`+setupPassword+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.0.2.10:1234"
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("login status %d body=%s", w.Code, w.Body.String())
	}
	var sessCookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookieName {
			sessCookie = c
			break
		}
	}
	if sessCookie == nil {
		t.Fatalf("missing session cookie")
	}

	// 7) session now authenticated
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/auth/session", nil)
	req.AddCookie(sessCookie)
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("session2 status %d", w.Code)
	}
	_ = json.Unmarshal(w.Body.Bytes(), &sess)
	if !sess["authenticated"].(bool) {
		t.Fatalf("expected authenticated")
	}

	// 8) csrf token available
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/auth/csrf", nil)
	req.AddCookie(sessCookie)
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("csrf status %d", w.Code)
	}

	// 9) change password wrong old -> 401
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/auth/password", strings.NewReader(`{"old_password":"bad","new_password":"EvenStronger123!"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessCookie)
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("password expected 401, got %d", w.Code)
	}

	// 10) correct change password -> 200
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/auth/password", strings.NewReader(`{"old_password":"`+setupPassword+`","new_password":"EvenStronger123!"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessCookie)
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("password change status %d", w.Code)
	}

	// 11) logout
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/auth/logout", nil)
	req.AddCookie(sessCookie)
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("logout status %d", w.Code)
	}

	// 12) session should be unauthenticated again
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
	req, _ := http.NewRequest("POST", "/api/v1/auth/setup", strings.NewReader(`{"password":"StrongPass123!"}`))
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
		req.RemoteAddr = "198.51.100.20:1000"
		srv.router.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("try %d expected 401, got %d", i+1, w.Code)
		}
	}
	// Next should yield 429
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{"username":"admin","password":"bad"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "198.51.100.20:1000"
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", w.Code)
	}
	if got := w.Result().Header.Get("Retry-After"); got == "" {
		t.Fatalf("missing Retry-After")
	}
}
