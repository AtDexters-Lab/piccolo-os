package services

import (
	"testing"

	"piccolod/internal/api"
)

func TestReserveHostPortPreventsReallocation(t *testing.T) {
	manager := NewServiceManager()

	eps, err := manager.AllocateForApp("app", []api.AppListener{
		{Name: "http", GuestPort: 80, Flow: api.FlowTCP, Protocol: api.ListenerProtocolHTTP},
	})
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	host := eps[0].HostBind

	manager.RemoveApp("app")
	if err := manager.ReserveHostPort(host); err != nil {
		t.Fatalf("reserve host: %v", err)
	}

	eps2, err := manager.AllocateForApp("app2", []api.AppListener{
		{Name: "http", GuestPort: 80, Flow: api.FlowTCP, Protocol: api.ListenerProtocolHTTP},
	})
	if err != nil {
		t.Fatalf("allocate second: %v", err)
	}
	if eps2[0].HostBind == host {
		t.Fatalf("expected allocator to skip reserved port %d, got %d", host, eps2[0].HostBind)
	}
}
