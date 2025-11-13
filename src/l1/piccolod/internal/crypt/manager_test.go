package crypt

import (
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
