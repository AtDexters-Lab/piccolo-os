package persistence

import (
	"context"
	"encoding/hex"
	"testing"
)

func TestGuardedControlStore_LeaderEnforcement(t *testing.T) {
	dir := t.TempDir()
	key, _ := hex.DecodeString("7f1c8a6c3b5d7e91aabbccddeeff00112233445566778899aabbccddeeff0011")

	store, err := newEncryptedControlStore(dir, staticKeyProvider{key: key})
	if err != nil {
		t.Fatalf("newEncryptedControlStore: %v", err)
	}
	if err := store.Unlock(context.Background()); err != nil {
		t.Fatalf("unlock: %v", err)
	}
	prepareControlCipherDir(t, dir)

	// Follower: expect ErrNotLeader on writes
	follower := newGuardedControlStore(store, func() bool { return false }, nil)
	if err := follower.Auth().SetInitialized(context.Background()); err != ErrNotLeader {
		t.Fatalf("expected ErrNotLeader on SetInitialized, got %v", err)
	}
	if err := follower.Auth().SavePasswordHash(context.Background(), "h"); err != ErrNotLeader {
		t.Fatalf("expected ErrNotLeader on SavePasswordHash, got %v", err)
	}
	if err := follower.Remote().SaveConfig(context.Background(), RemoteConfig{Payload: []byte("{}")}); err != ErrNotLeader {
		t.Fatalf("expected ErrNotLeader on SaveConfig, got %v", err)
	}
	if err := follower.AppState().UpsertApp(context.Background(), AppRecord{Name: "a"}); err != ErrNotLeader {
		t.Fatalf("expected ErrNotLeader on UpsertApp, got %v", err)
	}

	// Leader: writes succeed
	leader := newGuardedControlStore(store, func() bool { return true }, nil)
	if err := leader.Auth().SetInitialized(context.Background()); err != nil {
		t.Fatalf("leader SetInitialized: %v", err)
	}
	if err := leader.Auth().SavePasswordHash(context.Background(), "h"); err != nil {
		t.Fatalf("leader SavePasswordHash: %v", err)
	}
	if err := leader.Remote().SaveConfig(context.Background(), RemoteConfig{Payload: []byte("{}")}); err != nil {
		t.Fatalf("leader SaveConfig: %v", err)
	}
	if err := leader.AppState().UpsertApp(context.Background(), AppRecord{Name: "a"}); err != nil {
		t.Fatalf("leader UpsertApp: %v", err)
	}
}

func TestGuardedControlStore_LockUnlockPassthrough(t *testing.T) {
	dir := t.TempDir()
	key, _ := hex.DecodeString("7f1c8a6c3b5d7e91aabbccddeeff00112233445566778899aabbccddeeff0011")

	store, err := newEncryptedControlStore(dir, staticKeyProvider{key: key})
	if err != nil {
		t.Fatalf("newEncryptedControlStore: %v", err)
	}
	guard := newGuardedControlStore(store, func() bool { return true }, nil).(*guardedControlStore)

	if err := guard.Unlock(context.Background()); err != nil {
		t.Fatalf("unlock via guard: %v", err)
	}
	prepareControlCipherDir(t, dir)
	if err := guard.Auth().SetInitialized(context.Background()); err != nil {
		t.Fatalf("set initialized: %v", err)
	}

	guard.Lock()
	if _, err := guard.Auth().IsInitialized(context.Background()); err != ErrLocked {
		t.Fatalf("expected ErrLocked after lock, got %v", err)
	}

	if err := guard.Unlock(context.Background()); err != nil {
		t.Fatalf("unlock after lock: %v", err)
	}
	init, err := guard.Auth().IsInitialized(context.Background())
	if err != nil {
		t.Fatalf("is initialized: %v", err)
	}
	if !init {
		t.Fatalf("expected initialized to persist")
	}
}

func TestGuardedControlStore_CommitCallback(t *testing.T) {
	dir := t.TempDir()
	key, _ := hex.DecodeString("7f1c8a6c3b5d7e91aabbccddeeff00112233445566778899aabbccddeeff0011")

	store, err := newEncryptedControlStore(dir, staticKeyProvider{key: key})
	if err != nil {
		t.Fatalf("newEncryptedControlStore: %v", err)
	}
	if err := store.Unlock(context.Background()); err != nil {
		t.Fatalf("unlock: %v", err)
	}
	prepareControlCipherDir(t, dir)
	called := 0
	guard := newGuardedControlStore(store, func() bool { return true }, func(context.Context) { called++ })
	if err := guard.Auth().SetInitialized(context.Background()); err != nil {
		t.Fatalf("SetInitialized: %v", err)
	}
	if called == 0 {
		t.Fatalf("expected commit callback")
	}
}
