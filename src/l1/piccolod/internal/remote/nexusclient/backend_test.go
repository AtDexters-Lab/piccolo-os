package nexusclient

import (
	"context"
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

func TestTokenProviderGeneratesFreshJWTs(t *testing.T) {
	adapter := NewBackendAdapter(nil, nil)
	base := time.Unix(1700000000, 0)
	calls := 0
	origNow := timeNow
	timeNow = func() time.Time {
		defer func() { calls++ }()
		return base.Add(time.Duration(calls) * time.Second)
	}
	defer func() { timeNow = origNow }()

	cfg := Config{
		Endpoint:       "wss://nexus.example.com/connect",
		DeviceSecret:   "super-secret",
		PortalHostname: "portal.example.com",
		TLD:            "example.com",
	}
	if err := adapter.Configure(cfg); err != nil {
		t.Fatalf("configure: %v", err)
	}

	provider := adapter.tokenProvider()
	token1, err := provider(context.Background())
	if err != nil {
		t.Fatalf("token1: %v", err)
	}
	token2, err := provider(context.Background())
	if err != nil {
		t.Fatalf("token2: %v", err)
	}
	if token1.Value == "" || token2.Value == "" {
		t.Fatalf("expected non-empty tokens")
	}
	if token1.Value == token2.Value {
		t.Fatalf("expected distinct tokens, got identical values")
	}
	if token1.Expiry.IsZero() || token2.Expiry.IsZero() {
		t.Fatalf("expected expiry metadata")
	}
}

func TestStartPassesTokenProvider(t *testing.T) {
	adapter := NewBackendAdapter(nil, nil)
	cfg := Config{
		Endpoint:       "wss://nexus.example.com/connect",
		DeviceSecret:   "another-secret",
		PortalHostname: "portal.example.com",
	}
	if err := adapter.Configure(cfg); err != nil {
		t.Fatalf("configure: %v", err)
	}

	tokens := make(chan string, 1)

	adapter.factory = func(cfg backend.ClientBackendConfig, handler backend.ConnectHandler, provider backend.TokenProvider) backendClient {
		return &fakeClient{
			start: func(ctx context.Context) {
				tok, err := provider(ctx)
				if err != nil {
					t.Fatalf("provider error: %v", err)
				}
				tokens <- tok.Value
			},
		}
	}

	if err := adapter.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}

	select {
	case tok := <-tokens:
		if tok == "" {
			t.Fatalf("expected non-empty token")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("token provider not invoked")
	}

	if err := adapter.Stop(context.Background()); err != nil {
		t.Fatalf("stop: %v", err)
	}
}
