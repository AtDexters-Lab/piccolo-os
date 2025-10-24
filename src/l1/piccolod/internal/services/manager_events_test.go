package services

import (
	"sync"
	"testing"
	"time"

	"piccolod/internal/cluster"
	"piccolod/internal/events"
)

type stubServiceLockReader struct {
	mu     sync.RWMutex
	locked bool
}

func (s *stubServiceLockReader) ControlLocked() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.locked
}

func (s *stubServiceLockReader) set(locked bool) {
	s.mu.Lock()
	s.locked = locked
	s.mu.Unlock()
}

func TestServiceManagerLeadershipTracking(t *testing.T) {
	mgr := NewServiceManager()
	bus := events.NewBus()
	mgr.ObserveRuntimeEvents(bus)
	defer mgr.StopRuntimeEvents()

	bus.Publish(events.Event{Topic: events.TopicLeadershipRoleChanged, Payload: events.LeadershipChanged{Resource: "control", Role: cluster.RoleLeader}})

	deadline := time.Now().Add(100 * time.Millisecond)
	for time.Now().Before(deadline) {
		if mgr.LastObservedRole("control") == cluster.RoleLeader {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("expected control role to become leader, got %s", mgr.LastObservedRole("control"))
}

func TestServiceManagerLockTracking(t *testing.T) {
	mgr := NewServiceManager()
	reader := &stubServiceLockReader{}
	mgr.SetLockReader(reader)
	reader.set(true)
	if !mgr.Locked() {
		t.Fatalf("expected Locked() to reflect reader locked state")
	}

	reader.set(false)
	if mgr.Locked() {
		t.Fatalf("expected Locked() to reflect reader unlocked state")
	}
}
