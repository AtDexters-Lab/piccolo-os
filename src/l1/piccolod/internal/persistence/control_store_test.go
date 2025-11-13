package persistence

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestSQLiteControlStoreLifecycle(t *testing.T) {
	t.Setenv("PICCOLO_ALLOW_UNMOUNTED_TESTS", "1")
	dir := t.TempDir()
	key, _ := hex.DecodeString("7f1c8a6c3b5d7e91aabbccddeeff00112233445566778899aabbccddeeff0011")

	prepareControlCipherDir(t, dir)

	store, err := newSQLiteControlStore(dir, staticKeyProvider{key: key})
	if err != nil {
		t.Fatalf("newSQLiteControlStore: %v", err)
	}
	defer store.Close(context.Background())

	if err := store.Unlock(context.Background()); err != nil {
		t.Fatalf("unlock: %v", err)
	}

	// Locked state should gate reads
	store.Lock()
	if _, err := store.Auth().IsInitialized(context.Background()); err != ErrLocked {
		t.Fatalf("expected ErrLocked before unlock, got %v", err)
	}
	if err := store.Unlock(context.Background()); err != nil {
		t.Fatalf("unlock after lock: %v", err)
	}

	checkRevision := func(expected uint64) {
		rev, checksum, err := store.Revision(context.Background())
		if err != nil {
			t.Fatalf("revision: %v", err)
		}
		if rev != expected {
			t.Fatalf("expected revision %d, got %d", expected, rev)
		}
		if checksum == "" {
			t.Fatalf("expected checksum for revision %d", expected)
		}
	}

	if err := store.Auth().SetInitialized(context.Background()); err != nil {
		t.Fatalf("SetInitialized: %v", err)
	}
	checkRevision(1)

	const hashValue = "argon2id$v=19$m=65536,t=3,p=1$c2FsdHNhbHQ$ZHVtbXlobGFo"
	if err := store.Auth().SavePasswordHash(context.Background(), hashValue); err != nil {
		t.Fatalf("SavePasswordHash: %v", err)
	}
	checkRevision(2)

	remotePayload := []byte(`{"endpoint":"https://example"}`)
	if err := store.Remote().SaveConfig(context.Background(), RemoteConfig{Payload: remotePayload}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	checkRevision(3)

	if err := store.AppState().UpsertApp(context.Background(), AppRecord{Name: "app-alpha"}); err != nil {
		t.Fatalf("UpsertApp: %v", err)
	}
	checkRevision(4)

	apps, err := store.AppState().ListApps(context.Background())
	if err != nil {
		t.Fatalf("ListApps: %v", err)
	}
	if len(apps) != 1 || apps[0].Name != "app-alpha" {
		t.Fatalf("unexpected apps: %#v", apps)
	}

	store.Lock()
	if _, err := store.Remote().CurrentConfig(context.Background()); err != ErrLocked {
		t.Fatalf("expected ErrLocked after lock, got %v", err)
	}
	if err := store.Unlock(context.Background()); err != nil {
		t.Fatalf("unlock after lock: %v", err)
	}

	init, err := store.Auth().IsInitialized(context.Background())
	if err != nil {
		t.Fatalf("IsInitialized: %v", err)
	}
	if !init {
		t.Fatalf("expected initialized to persist")
	}
	storedHash, err := store.Auth().PasswordHash(context.Background())
	if err != nil {
		t.Fatalf("PasswordHash: %v", err)
	}
	if storedHash != hashValue {
		t.Fatalf("expected password hash to persist, got %s", storedHash)
	}
	cfg, err := store.Remote().CurrentConfig(context.Background())
	if err != nil {
		t.Fatalf("CurrentConfig: %v", err)
	}
	if string(cfg.Payload) != string(remotePayload) {
		t.Fatalf("unexpected remote payload: %s", string(cfg.Payload))
	}

	rev, checksum, err := store.Revision(context.Background())
	if err != nil {
		t.Fatalf("revision after writes: %v", err)
	}
	if rev != 4 || checksum == "" {
		t.Fatalf("unexpected revision/checksum: rev=%d checksum=%q", rev, checksum)
	}

	// Rehydrate via a new store to simulate restart.
	store2, err := newSQLiteControlStore(dir, staticKeyProvider{key: key})
	if err != nil {
		t.Fatalf("newSQLiteControlStore (second): %v", err)
	}
	defer store2.Close(context.Background())
	if err := store2.Unlock(context.Background()); err != nil {
		t.Fatalf("unlock second: %v", err)
	}
	init, err = store2.Auth().IsInitialized(context.Background())
	if err != nil {
		t.Fatalf("IsInitialized second: %v", err)
	}
	if !init {
		t.Fatalf("expected initialized to persist across restart")
	}
	storedHash, err = store2.Auth().PasswordHash(context.Background())
	if err != nil {
		t.Fatalf("PasswordHash second: %v", err)
	}
	if storedHash != hashValue {
		t.Fatalf("expected password hash after restart, got %s", storedHash)
	}
	cfg, err = store2.Remote().CurrentConfig(context.Background())
	if err != nil {
		t.Fatalf("CurrentConfig second: %v", err)
	}
	if string(cfg.Payload) != string(remotePayload) {
		t.Fatalf("unexpected remote payload after restart: %s", string(cfg.Payload))
	}
	apps, err = store2.AppState().ListApps(context.Background())
	if err != nil {
		t.Fatalf("ListApps second: %v", err)
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
		t.Fatalf("expected checksum %s after restart, got %s", checksum, restoredChecksum)
	}
}

func TestSQLiteControlStoreBlocksWhenVolumeUnprepared(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PICCOLO_ALLOW_UNMOUNTED_TESTS", "1")
	key, _ := hex.DecodeString("7f1c8a6c3b5d7e91aabbccddeeff00112233445566778899aabbccddeeff0011")

	store, err := newSQLiteControlStore(dir, staticKeyProvider{key: key})
	if err != nil {
		t.Fatalf("newSQLiteControlStore: %v", err)
	}
	if err := store.Unlock(context.Background()); !errors.Is(err, ErrLocked) {
		t.Fatalf("expected ErrLocked before metadata, got %v", err)
	}

	if err := store.Auth().SetInitialized(context.Background()); !errors.Is(err, ErrLocked) {
		t.Fatalf("expected ErrLocked when metadata missing, got %v", err)
	}

	prepareControlCipherDir(t, dir)
	if err := store.Unlock(context.Background()); err != nil {
		t.Fatalf("unlock after metadata: %v", err)
	}
}

func TestSQLiteControlStoreUnlockReadOnly(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PICCOLO_ALLOW_UNMOUNTED_TESTS", "1")
	key, _ := hex.DecodeString("7f1c8a6c3b5d7e91aabbccddeeff00112233445566778899aabbccddeeff0011")
	prepareControlCipherDir(t, dir)

	store, err := newSQLiteControlStore(dir, staticKeyProvider{key: key})
	if err != nil {
		t.Fatalf("newSQLiteControlStore: %v", err)
	}
	if err := store.Unlock(context.Background()); err != nil {
		t.Fatalf("unlock: %v", err)
	}
	if err := store.Auth().SetInitialized(context.Background()); err != nil {
		t.Fatalf("SetInitialized: %v", err)
	}
	const seedHash = "argon2id$v=19$m=32768,t=2,p=1$c2FsdA$Ynl0ZXM"
	if err := store.Auth().SavePasswordHash(context.Background(), seedHash); err != nil {
		t.Fatalf("SavePasswordHash: %v", err)
	}
	if err := store.Close(context.Background()); err != nil {
		t.Fatalf("close store: %v", err)
	}

	origDetector := detectReadOnlyMount
	detectReadOnlyMount = func(string) (bool, error) { return true, nil }
	t.Cleanup(func() { detectReadOnlyMount = origDetector })

	follower, err := newSQLiteControlStore(dir, staticKeyProvider{key: key})
	if err != nil {
		t.Fatalf("newSQLiteControlStore follower: %v", err)
	}
	defer follower.Close(context.Background())

	if err := follower.Unlock(context.Background()); err != nil {
		t.Fatalf("follower unlock: %v", err)
	}
	if !follower.readOnly {
		t.Fatalf("expected follower to mark store read-only")
	}
	init, err := follower.Auth().IsInitialized(context.Background())
	if err != nil {
		t.Fatalf("IsInitialized follower: %v", err)
	}
	if !init {
		t.Fatalf("expected initialization flag to persist for follower")
	}
	hash, err := follower.Auth().PasswordHash(context.Background())
	if err != nil {
		t.Fatalf("PasswordHash follower: %v", err)
	}
	if hash != seedHash {
		t.Fatalf("expected password hash %q, got %q", seedHash, hash)
	}
	if err := follower.Auth().SavePasswordHash(context.Background(), "new-hash"); err != ErrLocked {
		t.Fatalf("expected ErrLocked on follower write, got %v", err)
	}
}

func TestSQLiteControlStoreQuickCheck(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PICCOLO_ALLOW_UNMOUNTED_TESTS", "1")
	key, _ := hex.DecodeString("7f1c8a6c3b5d7e91aabbccddeeff00112233445566778899aabbccddeeff0011")
	prepareControlCipherDir(t, dir)

	store, err := newSQLiteControlStore(dir, staticKeyProvider{key: key})
	if err != nil {
		t.Fatalf("newSQLiteControlStore: %v", err)
	}
	defer store.Close(context.Background())
	if err := store.Unlock(context.Background()); err != nil {
		t.Fatalf("unlock: %v", err)
	}

	report, err := store.QuickCheck(context.Background())
	if err != nil {
		t.Fatalf("QuickCheck: %v", err)
	}
	if report.Status != ControlHealthStatusOK {
		t.Fatalf("expected OK status, got %s (%s)", report.Status, report.Message)
	}
	if report.CheckedAt.IsZero() {
		t.Fatalf("expected CheckedAt to be set")
	}
}

func TestSQLiteControlStoreQuickCheckLocked(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PICCOLO_ALLOW_UNMOUNTED_TESTS", "1")
	key, _ := hex.DecodeString("7f1c8a6c3b5d7e91aabbccddeeff00112233445566778899aabbccddeeff0011")
	store, err := newSQLiteControlStore(dir, staticKeyProvider{key: key})
	if err != nil {
		t.Fatalf("newSQLiteControlStore: %v", err)
	}
	defer store.Close(context.Background())

	report, err := store.QuickCheck(context.Background())
	if err != nil {
		t.Fatalf("QuickCheck (locked): %v", err)
	}
	if report.Status != ControlHealthStatusUnknown {
		t.Fatalf("expected unknown status when locked, got %s", report.Status)
	}
}

func TestSQLiteControlStoreCheckpointInvoked(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PICCOLO_ALLOW_UNMOUNTED_TESTS", "1")
	key, _ := hex.DecodeString("7f1c8a6c3b5d7e91aabbccddeeff00112233445566778899aabbccddeeff0011")
	prepareControlCipherDir(t, dir)

	store, err := newSQLiteControlStore(dir, staticKeyProvider{key: key})
	if err != nil {
		t.Fatalf("newSQLiteControlStore: %v", err)
	}
	defer store.Close(context.Background())
	store.checkpointInterval = 0
	var called int
	store.checkpointFn = func(*sql.DB) error {
		called++
		return nil
	}
	if err := store.Unlock(context.Background()); err != nil {
		t.Fatalf("unlock: %v", err)
	}
	if err := store.Auth().SetInitialized(context.Background()); err != nil {
		t.Fatalf("SetInitialized: %v", err)
	}
	if called == 0 {
		t.Fatalf("expected checkpoint to run at least once")
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
		t.Fatalf("expected sqliteControlStore, got %T", store)
	}
}

func TestSQLiteControlStoreAuthStalenessPersists(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PICCOLO_ALLOW_UNMOUNTED_TESTS", "1")
	key, _ := hex.DecodeString("7f1c8a6c3b5d7e91aabbccddeeff00112233445566778899aabbccddeeff0011")
	prepareControlCipherDir(t, dir)

	store, err := newSQLiteControlStore(dir, staticKeyProvider{key: key})
	if err != nil {
		t.Fatalf("newSQLiteControlStore: %v", err)
	}
	defer store.Close(context.Background())
	if err := store.Unlock(context.Background()); err != nil {
		t.Fatalf("unlock: %v", err)
	}

	st, err := store.Auth().Staleness(context.Background())
	if err != nil {
		t.Fatalf("initial Staleness: %v", err)
	}
	if st.PasswordStale || st.RecoveryStale {
		t.Fatalf("expected flags false initially, got %+v", st)
	}

	tsPassword := time.Date(2025, time.November, 13, 12, 0, 0, 0, time.UTC)
	tsRecovery := tsPassword.Add(1 * time.Hour)
	trueVal := true
	update := AuthStalenessUpdate{
		PasswordStale:   &trueVal,
		PasswordStaleAt: &tsPassword,
		RecoveryStale:   &trueVal,
		RecoveryStaleAt: &tsRecovery,
	}
	if err := store.Auth().UpdateStaleness(context.Background(), update); err != nil {
		t.Fatalf("UpdateStaleness mark: %v", err)
	}
	st, err = store.Auth().Staleness(context.Background())
	if err != nil {
		t.Fatalf("Staleness after mark: %v", err)
	}
	if !st.PasswordStale || !st.RecoveryStale {
		t.Fatalf("expected staleness flags true, got %+v", st)
	}
	if !st.PasswordStaleAt.Equal(tsPassword) || !st.RecoveryStaleAt.Equal(tsRecovery) {
		t.Fatalf("expected timestamps to persist, got %+v", st)
	}

	falseVal := false
	tsAck := tsRecovery.Add(30 * time.Minute)
	ackUpdate := AuthStalenessUpdate{
		PasswordStale: &falseVal,
		PasswordAckAt: &tsAck,
		RecoveryStale: &falseVal,
		RecoveryAckAt: &tsAck,
	}
	if err := store.Auth().UpdateStaleness(context.Background(), ackUpdate); err != nil {
		t.Fatalf("UpdateStaleness ack: %v", err)
	}
	st, err = store.Auth().Staleness(context.Background())
	if err != nil {
		t.Fatalf("Staleness after ack: %v", err)
	}
	if st.PasswordStale || st.RecoveryStale {
		t.Fatalf("expected flags cleared after ack, got %+v", st)
	}
	if !st.PasswordAckAt.Equal(tsAck) || !st.RecoveryAckAt.Equal(tsAck) {
		t.Fatalf("expected ack timestamps recorded, got %+v", st)
	}
	if !st.PasswordStaleAt.Equal(tsPassword) || !st.RecoveryStaleAt.Equal(tsRecovery) {
		t.Fatalf("stale timestamps should remain, got %+v", st)
	}

	store2, err := newSQLiteControlStore(dir, staticKeyProvider{key: key})
	if err != nil {
		t.Fatalf("newSQLiteControlStore restart: %v", err)
	}
	defer store2.Close(context.Background())
	if err := store2.Unlock(context.Background()); err != nil {
		t.Fatalf("unlock restart: %v", err)
	}
	st2, err := store2.Auth().Staleness(context.Background())
	if err != nil {
		t.Fatalf("Staleness restart: %v", err)
	}
	if !st2.PasswordAckAt.Equal(tsAck) || !st2.RecoveryAckAt.Equal(tsAck) {
		t.Fatalf("expected ack timestamps after restart, got %+v", st2)
	}
	if st2.PasswordStale || st2.RecoveryStale {
		t.Fatalf("expected cleared flags after restart, got %+v", st2)
	}
}

func prepareControlCipherDir(t *testing.T, root string) {
	t.Helper()
	cipherDir := filepath.Join(root, "ciphertext", "control")
	if err := os.MkdirAll(cipherDir, 0o700); err != nil {
		t.Fatalf("mkdir ciphertext: %v", err)
	}
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
