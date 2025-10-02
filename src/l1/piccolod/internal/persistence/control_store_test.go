package persistence

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type staticKeyProvider struct {
	key []byte
}

func (s staticKeyProvider) WithSDEK(fn func([]byte) error) error {
	if len(s.key) == 0 {
		return ErrCryptoUnavailable
	}
	dup := make([]byte, len(s.key))
	copy(dup, s.key)
	return fn(dup)
}

func TestEncryptedControlStoreLifecycle(t *testing.T) {
	dir := t.TempDir()
	key, _ := hex.DecodeString("7f1c8a6c3b5d7e91aabbccddeeff00112233445566778899aabbccddeeff0011")

	store, err := newEncryptedControlStore(dir, staticKeyProvider{key: key})
	if err != nil {
		t.Fatalf("newEncryptedControlStore: %v", err)
	}

	// Locked by default
	if _, err := store.Auth().IsInitialized(context.Background()); err != ErrLocked {
		t.Fatalf("expected ErrLocked before unlock, got %v", err)
	}

	if err := store.Unlock(context.Background()); err != nil {
		t.Fatalf("unlock: %v", err)
	}

	if err := store.Auth().SetInitialized(context.Background()); err != nil {
		t.Fatalf("set initialized: %v", err)
	}

	if err := store.Remote().SaveConfig(context.Background(), RemoteConfig{Endpoint: "https://example"}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	if err := store.AppState().UpsertApp(context.Background(), AppRecord{Name: "app-alpha"}); err != nil {
		t.Fatalf("upsert app: %v", err)
	}

	apps, err := store.AppState().ListApps(context.Background())
	if err != nil {
		t.Fatalf("list apps: %v", err)
	}
	if len(apps) != 1 || apps[0].Name != "app-alpha" {
		t.Fatalf("unexpected apps list: %#v", apps)
	}

	// Lock and ensure reads block
	store.Lock()
	if _, err := store.Remote().CurrentConfig(context.Background()); err != ErrLocked {
		t.Fatalf("expected ErrLocked after lock, got %v", err)
	}

	// Recreate store to simulate restart
	store2, err := newEncryptedControlStore(dir, staticKeyProvider{key: key})
	if err != nil {
		t.Fatalf("newEncryptedControlStore (second): %v", err)
	}
	if err := store2.Unlock(context.Background()); err != nil {
		t.Fatalf("unlock second: %v", err)
	}

	init, err := store2.Auth().IsInitialized(context.Background())
	if err != nil {
		t.Fatalf("is initialized: %v", err)
	}
	if !init {
		t.Fatalf("expected auth initialized to persist")
	}

	cfg, err := store2.Remote().CurrentConfig(context.Background())
	if err != nil {
		t.Fatalf("current config: %v", err)
	}
	if cfg.Endpoint != "https://example" {
		t.Fatalf("unexpected remote config: %#v", cfg)
	}

	apps, err = store2.AppState().ListApps(context.Background())
	if err != nil {
		t.Fatalf("list apps after restart: %v", err)
	}
	if len(apps) != 1 || apps[0].Name != "app-alpha" {
		t.Fatalf("unexpected apps after restart: %#v", apps)
	}

	// Ensure file does not contain plaintext values
	data, err := os.ReadFile(filepath.Join(dir, "control", "control.enc"))
	if err != nil {
		t.Fatalf("read encrypted file: %v", err)
	}
	if strings.Contains(string(data), "app-alpha") || strings.Contains(string(data), "https://example") {
		t.Fatalf("encrypted file leaked plaintext: %s", data)
	}
}
