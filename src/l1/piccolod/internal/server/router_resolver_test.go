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
		{Name: "web", GuestPort: 8080, RemotePorts: []int{80, 443, 8000}},
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

	resolver.SetTlsMuxPort(9090)

	port, ok := resolver.Resolve("portal.example.com", 443, true)
	if !ok || port != 9090 {
		t.Fatalf("expected portal TLS traffic to map to tls mux (9090), got %d (ok=%v)", port, ok)
	}

	port, ok = resolver.Resolve("web.example.com", 443, true)
	if !ok || port != 9090 {
		t.Fatalf("expected tls mux to terminate tls traffic on 443, got %d (ok=%v)", port, ok)
	}

	port, ok = resolver.Resolve("web.example.com", 443, false)
	if !ok || port != webEndpoint.PublicPort {
		t.Fatalf("expected plain traffic on 443 to map to %d, got %d (ok=%v)", webEndpoint.PublicPort, port, ok)
	}

	port, ok = resolver.Resolve("portal.example.com", 80, false)
	if !ok || port != 8081 {
		t.Fatalf("expected portal HTTP traffic to map to 8081, got %d (ok=%v)", port, ok)
	}

	port, ok = resolver.Resolve("web.example.com", 8000, true)
	if !ok || port != 9090 {
		t.Fatalf("expected tls mux to handle tls requests on 8000, got %d (ok=%v)", port, ok)
	}

	port, ok = resolver.Resolve("web.example.com", 8000, false)
	if !ok || port != webEndpoint.PublicPort {
		t.Fatalf("expected plain requests on 8000 to route to %d, got %d (ok=%v)", webEndpoint.PublicPort, port, ok)
	}

	svc.Stop()
}
