package nexusclient

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	backend "github.com/AtDexters-Lab/nexus-proxy-backend-client/client"
)

type fakeClient struct {
	start func(context.Context)
	stop  func()
}

func (f *fakeClient) Start(ctx context.Context) {
	if f.start != nil {
		f.start(ctx)
	}
}

func (f *fakeClient) Stop() {
	if f.stop != nil {
		f.stop()
	}
}

func TestStartConfiguresAttestation(t *testing.T) {
	adapter := NewBackendAdapter(nil, nil)
	cfg := Config{
		Endpoint:       "wss://nexus.example.com/connect",
		DeviceSecret:   "  secret-value  ",
		PortalHostname: "portal.example.com",
		TLD:            "example.com",
	}
	if err := adapter.Configure(cfg); err != nil {
		t.Fatalf("configure: %v", err)
	}

	var captured backend.ClientBackendConfig
	started := make(chan struct{}, 1)

	adapter.factory = func(cfg backend.ClientBackendConfig, handler backend.ConnectHandler) (backendClient, error) {
		captured = cfg
		return &fakeClient{
			start: func(context.Context) {
				started <- struct{}{}
			},
		}, nil
	}

	if err := adapter.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() {
		if err := adapter.Stop(context.Background()); err != nil {
			t.Fatalf("stop: %v", err)
		}
	})

	select {
	case <-started:
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("expected client Start to be invoked")
	}

	expectedHosts := []string{"portal.example.com", "*.example.com"}
	if !reflect.DeepEqual(captured.Hostnames, expectedHosts) {
		t.Fatalf("unexpected hostnames: got %v want %v", captured.Hostnames, expectedHosts)
	}
	if captured.Attestation.HMACSecret != "secret-value" {
		t.Fatalf("expected HMAC secret to be trimmed, got %q", captured.Attestation.HMACSecret)
	}
	if captured.Weight != 1 {
		t.Fatalf("expected default weight 1, got %d", captured.Weight)
	}
	if strings.TrimSpace(captured.Attestation.Command) != "" {
		t.Fatalf("expected no command configured, got %q", captured.Attestation.Command)
	}
	if captured.Attestation.TokenTTL != attestationTokenTTL {
		t.Fatalf("unexpected token TTL: got %v want %v", captured.Attestation.TokenTTL, attestationTokenTTL)
	}
	if captured.Attestation.ReauthIntervalSeconds != attestationReauthIntervalSec {
		t.Fatalf("unexpected reauth interval: got %d want %d", captured.Attestation.ReauthIntervalSeconds, attestationReauthIntervalSec)
	}
}

func TestStartPropagatesFactoryError(t *testing.T) {
	adapter := NewBackendAdapter(nil, nil)
	cfg := Config{
		Endpoint:       "wss://nexus.example.com/connect",
		DeviceSecret:   "secret",
		PortalHostname: "portal.example.com",
	}
	if err := adapter.Configure(cfg); err != nil {
		t.Fatalf("configure: %v", err)
	}

	adapter.factory = func(cfg backend.ClientBackendConfig, handler backend.ConnectHandler) (backendClient, error) {
		return nil, errors.New("construction failed")
	}

	err := adapter.Start(context.Background())
	if err == nil {
		t.Fatalf("expected error from start when factory fails")
	}
	if !strings.Contains(err.Error(), "construction failed") {
		t.Fatalf("expected factory error to propagate, got %v", err)
	}
}
