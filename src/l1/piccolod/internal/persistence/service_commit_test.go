package persistence

import (
	"context"
	"encoding/hex"
	"testing"
	"time"

	"piccolod/internal/cluster"
	"piccolod/internal/events"
)

func TestModulePublishesControlCommit(t *testing.T) {
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

	bus := events.NewBus()
	mod := &Module{events: bus, leadership: cluster.NewRegistry()}
	mod.control = newGuardedControlStore(store, func() bool { return true }, mod.onControlCommit)

	ch := bus.Subscribe(events.TopicControlStoreCommit, 1)
	if err := mod.Control().Auth().SetInitialized(context.Background()); err != nil {
		t.Fatalf("SetInitialized: %v", err)
	}

	select {
	case evt := <-ch:
		payload, ok := evt.Payload.(events.ControlStoreCommit)
		if !ok {
			t.Fatalf("unexpected payload type: %#v", evt.Payload)
		}
		if payload.Revision != 1 {
			t.Fatalf("expected revision 1, got %d", payload.Revision)
		}
		if payload.Role != cluster.RoleLeader {
			t.Fatalf("expected leader role, got %s", payload.Role)
		}
		if payload.Checksum == "" {
			t.Fatalf("expected checksum in commit event")
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for commit event")
	}

	// Duplicate revision should not re-emit
	mod.publishControlCommit(cluster.RoleLeader, mod.lastCommitRevision, "duplicate")
	select {
	case <-ch:
		t.Fatalf("did not expect second event for same revision")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestModulePublishesControlHealth(t *testing.T) {
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
	if err := store.Auth().SetInitialized(context.Background()); err != nil {
		t.Fatalf("SetInitialized: %v", err)
	}

	bus := events.NewBus()
	mod := &Module{
		events:         bus,
		leadership:     cluster.NewRegistry(),
		healthInterval: 10 * time.Millisecond,
	}
	mod.control = newGuardedControlStore(store, func() bool { return true }, nil)
	ch := bus.Subscribe(events.TopicControlHealth, 1)

	mod.startControlHealthMonitor()
	defer mod.Shutdown(context.Background())

	select {
	case evt := <-ch:
		report, ok := evt.Payload.(ControlHealthReport)
		if !ok {
			t.Fatalf("unexpected payload type: %#v", evt.Payload)
		}
		if report.Status != ControlHealthStatusOK {
			t.Fatalf("expected OK status, got %s (%s)", report.Status, report.Message)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for control health event")
	}
}
