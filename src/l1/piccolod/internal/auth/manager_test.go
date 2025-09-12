package auth

import (
    "os"
    "testing"
)

func TestManager_SetupAndVerify(t *testing.T) {
    dir, err := os.MkdirTemp("", "authmgr")
    if err != nil { t.Fatalf("temp: %v", err) }
    defer os.RemoveAll(dir)
    m, err := NewManager(dir)
    if err != nil { t.Fatalf("new: %v", err) }
    if m.IsInitialized() { t.Fatalf("unexpected initialized") }
    if err := m.Setup("pw123456"); err != nil { t.Fatalf("setup: %v", err) }
    if !m.Verify("admin", "pw123456") { t.Fatalf("verify failed") }
}

func TestArgon2_HashAndVerify(t *testing.T) {
    ref, err := hashArgon2id("pw123456")
    if err != nil { t.Fatalf("hash: %v", err) }
    if !verifyArgon2id(ref, "pw123456") {
        t.Fatalf("verifyArgon2id failed: %s", ref)
    }
}
