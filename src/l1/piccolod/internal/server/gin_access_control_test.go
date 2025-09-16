package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFirstRunGuardBlocksPublicEndpoints(t *testing.T) {
	server := createGinTestServer(t, t.TempDir())

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/remote/status", nil)
	server.router.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 before setup, got %d", w.Code)
	}

	// Allowed route should still respond
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/api/v1/auth/initialized", nil)
	server.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("initialized endpoint should remain accessible, got %d", w.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("parse json: %v", err)
	}
	if initialized, _ := body["initialized"].(bool); initialized {
		t.Fatalf("expected not initialized")
	}
}

func TestLockedModeGuardBlocksPublicEndpoints(t *testing.T) {
	server := createGinTestServer(t, t.TempDir())
	sessionCookie, csrf := setupTestAdminSession(t, server)

	// Initialize crypto to enter locked mode
	if err := server.cryptoManager.Setup("TestPass123!"); err != nil {
		t.Fatalf("crypto setup: %v", err)
	}

	// Public endpoint should be blocked with 423
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/remote/status", nil)
	server.router.ServeHTTP(w, req)
	if w.Code != http.StatusLocked {
		t.Fatalf("expected 423 when locked, got %d", w.Code)
	}

	// Authenticated session endpoint should still respond
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	req.AddCookie(sessionCookie)
	server.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("auth session blocked unexpectedly: %d", w.Code)
	}

	// Crypto status should remain reachable once CSRF/session provided
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/api/v1/crypto/unlock", strings.NewReader(`{"password":"TestPass123!"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", csrf)
	server.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected unlock to succeed, got %d", w.Code)
	}
}
