package services

import (
	"testing"
	"time"

	"piccolod/internal/cluster"
	"piccolod/internal/events"
)

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
	bus := events.NewBus()
	mgr.ObserveRuntimeEvents(bus)
	defer mgr.StopRuntimeEvents()

	bus.Publish(events.Event{Topic: events.TopicLockStateChanged, Payload: events.LockStateChanged{Locked: true}})

	deadline := time.Now().Add(100 * time.Millisecond)
	for time.Now().Before(deadline) {
		if mgr.Locked() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("expected Locked() to report true")
}
