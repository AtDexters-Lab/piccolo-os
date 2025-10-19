package services

import (
	"piccolod/internal/api"
	"testing"
)

func TestReconcile_AddRemoveChange(t *testing.T) {
	m := NewServiceManager()
	// Seed existing app endpoints
	eps, err := m.AllocateForApp("app", []api.AppListener{{Name: "a", GuestPort: 80, Flow: api.FlowTCP, Protocol: api.ListenerProtocolRaw}})
	if err != nil {
		t.Fatalf("alloc: %v", err)
	}
	if len(eps) != 1 {
		t.Fatalf("expected 1 endpoint")
	}
	oldHB := eps[0].HostBind
	oldPP := eps[0].PublicPort

	// Reconcile: change guest port for a, add b, remove nothing
	rec, _, err := m.Reconcile("app", []api.AppListener{
		{Name: "a", GuestPort: 8080, Flow: api.FlowTCP, Protocol: api.ListenerProtocolRaw},
		{Name: "b", GuestPort: 22, Flow: api.FlowTCP, Protocol: api.ListenerProtocolRaw},
	})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(rec.GuestPortChanged) != 1 {
		t.Fatalf("want 1 guest change, got %d", len(rec.GuestPortChanged))
	}
	if rec.GuestPortChanged[0].Old.HostBind != oldHB || rec.GuestPortChanged[0].Old.PublicPort != oldPP {
		t.Fatalf("host/public must be preserved for changed listener")
	}
	if len(rec.Added) != 1 {
		t.Fatalf("want 1 added, got %d", len(rec.Added))
	}
}
