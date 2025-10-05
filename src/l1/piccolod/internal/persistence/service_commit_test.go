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
	key, _ := hex.DecodeString("7f1c8a6c3b5d7e91aabbccddeeff00112233445566778899aabbccddeeff0011")
	store, err := newEncryptedControlStore(dir, staticKeyProvider{key: key})
	if err != nil {
		t.Fatalf("newEncryptedControlStore: %v", err)
	}
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
