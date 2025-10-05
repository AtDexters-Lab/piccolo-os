package server

import (
	"os"
	"testing"

	"piccolod/internal/api"
	"piccolod/internal/remote/nexusclient"
	"piccolod/internal/services"
)

func TestServiceRemoteResolver(t *testing.T) {
	oldPort := os.Getenv("PORT")
	os.Setenv("PORT", "8081")
	defer os.Setenv("PORT", oldPort)

	svc := services.NewServiceManager()
	resolver := newServiceRemoteResolver(svc)

	listeners := []api.AppListener{
		{Name: "web", GuestPort: 8080, RemotePorts: []int{80, 443}},
	}
	eps, err := svc.AllocateForApp("demo", listeners)
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	if len(eps) != 1 {
		t.Fatalf("expected one endpoint, got %d", len(eps))
	}
	webEndpoint := eps[0]

	resolver.UpdateConfig(nexusclient.Config{PortalHostname: "portal.example.com", TLD: "example.com"})

	port, ok := resolver.Resolve("portal.example.com", 443)
	if !ok || port != 8081 {
		t.Fatalf("expected portal to map to 8081, got %d (ok=%v)", port, ok)
	}

	port, ok = resolver.Resolve("web.example.com", 443)
	if !ok || port != webEndpoint.PublicPort {
		t.Fatalf("expected web listener to map to %d, got %d (ok=%v)", webEndpoint.PublicPort, port, ok)
	}

	svc.Stop()
}
