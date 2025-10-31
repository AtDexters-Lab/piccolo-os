package persistence

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
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

	prepareControlCipherDir(t, dir)

	if err := store.Auth().SetInitialized(context.Background()); err != nil {
		t.Fatalf("set initialized: %v", err)
	}
	rev, checksum, err := store.Revision(context.Background())
	if err != nil {
		t.Fatalf("revision after init: %v", err)
	}
	if rev != 1 {
		t.Fatalf("expected revision 1 after init, got %d", rev)
	}
	if checksum == "" {
		t.Fatalf("expected checksum after init")
	}

	const hashValue = "argon2id$v=19$m=65536,t=3,p=1$c2FsdHNhbHQ$ZHVtbXlobGFo"
	if err := store.Auth().SavePasswordHash(context.Background(), hashValue); err != nil {
		t.Fatalf("save password hash: %v", err)
	}
	rev, checksum, err = store.Revision(context.Background())
	if err != nil {
		t.Fatalf("revision after password hash: %v", err)
	}
	if rev != 2 {
		t.Fatalf("expected revision 2 after password hash, got %d", rev)
	}
	if checksum == "" {
		t.Fatalf("expected checksum non-empty after password hash")
	}

	storedHash, err := store.Auth().PasswordHash(context.Background())
	if err != nil {
		t.Fatalf("password hash: %v", err)
	}
	if storedHash != hashValue {
		t.Fatalf("expected password hash to persist, got %s", storedHash)
	}

	remotePayload := []byte(`{"endpoint":"https://example"}`)
	if err := store.Remote().SaveConfig(context.Background(), RemoteConfig{Payload: remotePayload}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	rev, checksum, err = store.Revision(context.Background())
	if err != nil {
		t.Fatalf("revision after remote config: %v", err)
	}
	if rev != 3 {
		t.Fatalf("expected revision 3 after remote config, got %d", rev)
	}

	if err := store.AppState().UpsertApp(context.Background(), AppRecord{Name: "app-alpha"}); err != nil {
		t.Fatalf("upsert app: %v", err)
	}
	rev, checksum, err = store.Revision(context.Background())
	if err != nil {
		t.Fatalf("revision after app upsert: %v", err)
	}
	if rev != 4 {
		t.Fatalf("expected revision 4 after app upsert, got %d", rev)
	}
	if checksum == "" {
		t.Fatalf("expected checksum non-empty after app upsert")
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

	prepareControlCipherDir(t, dir)

	init, err := store2.Auth().IsInitialized(context.Background())
	if err != nil {
		t.Fatalf("is initialized: %v", err)
	}
	if !init {
		t.Fatalf("expected auth initialized to persist")
	}

	storedHash, err = store2.Auth().PasswordHash(context.Background())
	if err != nil {
		t.Fatalf("password hash after restart: %v", err)
	}
	if storedHash != hashValue {
		t.Fatalf("expected password hash to persist after restart, got %s", storedHash)
	}

	cfg, err := store2.Remote().CurrentConfig(context.Background())
	if err != nil {
		t.Fatalf("current config: %v", err)
	}
	if string(cfg.Payload) != string(remotePayload) {
		t.Fatalf("unexpected remote config payload: %s", string(cfg.Payload))
	}

	apps, err = store2.AppState().ListApps(context.Background())
	if err != nil {
		t.Fatalf("list apps after restart: %v", err)
	}
	if len(apps) != 1 || apps[0].Name != "app-alpha" {
		t.Fatalf("unexpected apps after restart: %#v", apps)
	}
	restoredRev, restoredChecksum, err := store2.Revision(context.Background())
	if err != nil {
		t.Fatalf("revision after restart: %v", err)
	}
	if restoredRev != rev {
		t.Fatalf("expected revision %d after restart, got %d", rev, restoredRev)
	}
	if restoredChecksum != checksum {
		t.Fatalf("expected checksum to persist, got %s", restoredChecksum)
	}

	// Ensure file does not contain plaintext values
	data, err := os.ReadFile(filepath.Join(dir, "ciphertext", "control", "control.enc"))
	if err != nil {
		t.Fatalf("read encrypted file: %v", err)
	}
	if strings.Contains(string(data), "app-alpha") || strings.Contains(string(data), "https://example") || strings.Contains(string(data), hashValue) {
		t.Fatalf("encrypted file leaked plaintext: %s", data)
	}
}

func TestEncryptedControlStoreBlocksWhenVolumeUnprepared(t *testing.T) {
	dir := t.TempDir()
	key, _ := hex.DecodeString("7f1c8a6c3b5d7e91aabbccddeeff00112233445566778899aabbccddeeff0011")

	store, err := newEncryptedControlStore(dir, staticKeyProvider{key: key})
	if err != nil {
		t.Fatalf("newEncryptedControlStore: %v", err)
	}
	if err := store.Unlock(context.Background()); err != nil {
		t.Fatalf("unlock: %v", err)
	}

	if err := store.Auth().SetInitialized(context.Background()); err != ErrLocked {
		t.Fatalf("expected ErrLocked when volume metadata missing, got %v", err)
	}

	prepareControlCipherDir(t, dir)

	if err := store.Auth().SetInitialized(context.Background()); err != nil {
		t.Fatalf("set initialized after metadata: %v", err)
	}

	payload := []byte(`{"endpoint":"https://example"}`)
	if err := store.Remote().SaveConfig(context.Background(), RemoteConfig{Payload: payload}); err != nil {
		t.Fatalf("save config after metadata: %v", err)
	}
}

func prepareControlCipherDir(t *testing.T, root string) {
	t.Helper()
	cipherDir := filepath.Join(root, "ciphertext", "control")
	if err := os.WriteFile(filepath.Join(cipherDir, gocryptfsConfigName), []byte("stub"), 0o600); err != nil {
		t.Fatalf("write gocryptfs.conf: %v", err)
	}
	meta := volumeMetadata{
		Version:    metadataVersion,
		WrappedKey: "stub",
		Nonce:      base64.RawStdEncoding.EncodeToString([]byte("nonce")),
	}
	data, err := json.Marshal(&meta)
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cipherDir, controlVolumeMetadataName), data, 0o600); err != nil {
		t.Fatalf("write metadata: %v", err)
	}
}

func TestSQLiteControlStoreLifecycle(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PICCOLO_ALLOW_UNMOUNTED_TESTS", "1")
	key, _ := hex.DecodeString("7f1c8a6c3b5d7e91aabbccddeeff00112233445566778899aabbccddeeff0011")

	store, err := newSQLiteControlStore(dir, staticKeyProvider{key: key})
	if err != nil {
		t.Fatalf("newSQLiteControlStore: %v", err)
	}
	defer store.Close(context.Background())

	prepareControlCipherDir(t, dir)

	if err := store.Unlock(context.Background()); err != nil {
		t.Fatalf("unlock: %v", err)
	}

	const hashValue = "argon2id$v=19$m=65536,t=3,p=1$c2FsdHNhbHQ$ZHVtbXlobGFo"
	if err := store.Auth().SavePasswordHash(context.Background(), hashValue); err != nil {
		t.Fatalf("save password hash: %v", err)
	}
	if err := store.Auth().SetInitialized(context.Background()); err != nil {
		t.Fatalf("set initialized: %v", err)
	}

	remotePayload := []byte(`{"endpoint":"https://example"}`)
	if err := store.Remote().SaveConfig(context.Background(), RemoteConfig{Payload: remotePayload}); err != nil {
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

	store.Lock()
	if _, err := store.Remote().CurrentConfig(context.Background()); err != ErrLocked {
		t.Fatalf("expected ErrLocked after lock, got %v", err)
	}

	if err := store.Unlock(context.Background()); err != nil {
		t.Fatalf("unlock second: %v", err)
	}

	init, err := store.Auth().IsInitialized(context.Background())
	if err != nil {
		t.Fatalf("is initialized: %v", err)
	}
	if !init {
		t.Fatalf("expected auth initialized to persist")
	}

	storedHash, err := store.Auth().PasswordHash(context.Background())
	if err != nil {
		t.Fatalf("password hash after restart: %v", err)
	}
	if storedHash != hashValue {
		t.Fatalf("expected password hash to persist after restart, got %s", storedHash)
	}

	cfg, err := store.Remote().CurrentConfig(context.Background())
	if err != nil {
		t.Fatalf("current config: %v", err)
	}
	if string(cfg.Payload) != string(remotePayload) {
		t.Fatalf("unexpected remote config payload: %s", string(cfg.Payload))
	}

	rev, checksum, err := store.Revision(context.Background())
	if err != nil {
		t.Fatalf("revision: %v", err)
	}
	if rev == 0 || checksum == "" {
		t.Fatalf("expected revision/checksum after writes, got rev=%d checksum=%q", rev, checksum)
	}

	store2, err := newSQLiteControlStore(dir, staticKeyProvider{key: key})
	if err != nil {
		t.Fatalf("newSQLiteControlStore (second): %v", err)
	}
	defer store2.Close(context.Background())
	if err := store2.Unlock(context.Background()); err != nil {
		t.Fatalf("unlock second store: %v", err)
	}
	apps2, err := store2.AppState().ListApps(context.Background())
	if err != nil {
		t.Fatalf("list apps second: %v", err)
	}
	if len(apps2) != 1 || apps2[0].Name != "app-alpha" {
		t.Fatalf("unexpected apps after reopen: %#v", apps2)
	}
}

func TestNewControlStoreReturnsSQLiteImplementation(t *testing.T) {
	dir := t.TempDir()
	key, _ := hex.DecodeString("7f1c8a6c3b5d7e91aabbccddeeff00112233445566778899aabbccddeeff0011")

	store, err := newControlStore(dir, staticKeyProvider{key: key})
	if err != nil {
		t.Fatalf("newControlStore: %v", err)
	}
	defer store.Close(context.Background())

	if _, ok := store.(*sqliteControlStore); !ok {
		t.Fatalf("expected sqliteControlStore when flag enabled, got %T", store)
	}
}

func TestSQLiteControlStoreBlocksWhenVolumeUnprepared(t *testing.T) {
	t.Setenv("PICCOLO_ALLOW_UNMOUNTED_TESTS", "1")
	dir := t.TempDir()
	key, _ := hex.DecodeString("7f1c8a6c3b5d7e91aabbccddeeff00112233445566778899aabbccddeeff0011")

	store, err := newControlStore(dir, staticKeyProvider{key: key})
	if err != nil {
		t.Fatalf("newControlStore: %v", err)
	}
	sqlStore, ok := store.(*sqliteControlStore)
	if !ok {
		t.Fatalf("expected sqlite control store, got %T", store)
	}

	if err := sqlStore.Unlock(context.Background()); !errors.Is(err, ErrLocked) {
		t.Fatalf("expected ErrLocked before metadata, got %v", err)
	}

	err = sqlStore.Auth().SetInitialized(context.Background())
	if !errors.Is(err, ErrLocked) {
		t.Fatalf("expected ErrLocked when volume metadata missing, got %v", err)
	}

	prepareControlCipherDir(t, dir)

	if err := sqlStore.Unlock(context.Background()); err != nil {
		t.Fatalf("unlock after metadata: %v", err)
	}
}
