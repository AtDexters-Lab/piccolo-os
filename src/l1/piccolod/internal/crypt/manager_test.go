package crypt

import (
	"strings"
	"testing"
)

func TestManager_RewrapUnlocked(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := m.Setup("old-secret"); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := m.Unlock("old-secret"); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	if err := m.RewrapUnlocked("new-secret"); err != nil {
		t.Fatalf("RewrapUnlocked: %v", err)
	}
	m.Lock()
	if err := m.Unlock("old-secret"); err == nil {
		t.Fatalf("expected old password to fail after rewrap")
	}
	if err := m.Unlock("new-secret"); err != nil {
		t.Fatalf("Unlock new password: %v", err)
	}
}

func TestManager_GenerateRecoveryKeyRotation(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := m.Setup("admin-pass"); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := m.Unlock("admin-pass"); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	words1, err := m.GenerateRecoveryKey(false)
	if err != nil {
		t.Fatalf("GenerateRecoveryKey first: %v", err)
	}
	if len(words1) != 24 {
		t.Fatalf("expected 24 words, got %d", len(words1))
	}
	if _, err := m.GenerateRecoveryKey(false); err == nil {
		t.Fatalf("expected error when regenerating without force")
	}
	words2, err := m.GenerateRecoveryKey(true)
	if err != nil {
		t.Fatalf("GenerateRecoveryKey force: %v", err)
	}
	if len(words2) != 24 {
		t.Fatalf("expected 24 words on rotation, got %d", len(words2))
	}
	if strings.Join(words1, " ") == strings.Join(words2, " ") {
		t.Fatalf("expected different mnemonic after rotation")
	}
	if !m.HasRecoveryKey() {
		t.Fatalf("expected HasRecoveryKey to remain true after rotation")
	}
}
