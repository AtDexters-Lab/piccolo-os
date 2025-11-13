package auth

import (
	"context"
	"os"
	"testing"
)

func TestManager_SetupAndVerify(t *testing.T) {
	dir, err := os.MkdirTemp("", "authmgr")
	if err != nil {
		t.Fatalf("temp: %v", err)
	}
	defer os.RemoveAll(dir)
	m, err := NewManager(dir)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	ctx := context.Background()
	if init, err := m.IsInitialized(ctx); err != nil || init {
		t.Fatalf("unexpected initialized=%v err=%v", init, err)
	}
	if err := m.Setup(ctx, "pw123456"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if ok, err := m.Verify(ctx, "admin", "pw123456"); err != nil || !ok {
		t.Fatalf("verify failed ok=%v err=%v", ok, err)
	}
}

func TestArgon2_HashAndVerify(t *testing.T) {
	ref, err := hashArgon2id("pw123456")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !verifyArgon2id(ref, "pw123456") {
		t.Fatalf("verifyArgon2id failed: %s", ref)
	}
}

func TestManager_ChangePasswordWithRecovery(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	ctx := context.Background()
	if err := m.Setup(ctx, "initial"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := m.ChangePasswordWithRecovery(ctx, "recovered"); err != nil {
		t.Fatalf("ChangePasswordWithRecovery: %v", err)
	}
	if ok, _ := m.Verify(ctx, "admin", "initial"); ok {
		t.Fatalf("expected old password to fail")
	}
	if ok, err := m.Verify(ctx, "admin", "recovered"); err != nil || !ok {
		t.Fatalf("expected recovered password to verify, ok=%v err=%v", ok, err)
	}
}
