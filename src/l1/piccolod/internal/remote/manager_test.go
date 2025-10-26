package remote

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"piccolod/internal/remote/nexusclient"
)

type stubDialer struct {
	err error
}

func (s *stubDialer) DialTimeout(network, address string, timeout time.Duration) (net.Conn, error) {
	if s.err != nil {
		return nil, s.err
	}
	c1, c2 := net.Pipe()
	_ = c2.Close()
	return c1, nil
}

type stubResolver struct {
	hosts  map[string][]string
	cnames map[string]string
}

func (s *stubResolver) LookupHost(ctx context.Context, host string) ([]string, error) {
	if addresses, ok := s.hosts[host]; ok {
		return addresses, nil
	}
	return nil, errors.New("host not found")
}

func (s *stubResolver) LookupCNAME(ctx context.Context, host string) (string, error) {
	if cname, ok := s.cnames[host]; ok {
		return cname, nil
	}
	return "", errors.New("cname not found")
}

func fixedNow(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

func TestRunPreflightSuccess(t *testing.T) {
	dir := t.TempDir()
	dial := &stubDialer{}
	res := &stubResolver{
		hosts: map[string][]string{
			"portal.example.com": {"1.2.3.4"},
			"app.example.com":    {"1.2.3.4"},
		},
		cnames: map[string]string{
			"portal.example.com": "nexus.example.com.",
		},
	}

	storage, err := newFileStorage(dir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := newManagerWithDeps(storage, dir, dial, res, fixedNow(time.Unix(1, 0)))
	if err != nil {
		t.Fatal(err)
	}

	err = m.Configure(ConfigureRequest{
		Endpoint:       "wss://nexus.example.com/connect",
		DeviceSecret:   "secret",
		Solver:         "http-01",
		TLD:            "example.com",
		PortalHostname: "portal.example.com",
	})
	if err != nil {
		t.Fatalf("configure failed: %v", err)
	}

	result, err := m.RunPreflight()
	if err != nil {
		t.Fatalf("preflight failed: %v", err)
	}
	if len(result.Checks) < 3 {
		t.Fatalf("expected checks, got %v", result.Checks)
	}

	st := m.Status()
	if st.State != "active" && st.State != "warning" {
		t.Fatalf("unexpected state %s", st.State)
	}
	if st.PortalHostname != "portal.example.com" {
		t.Fatalf("unexpected portal host %s", st.PortalHostname)
	}
}

type fakeAdapter struct {
	config  nexusclient.Config
	startCh chan struct{}
	stopCh  chan struct{}
}

func newFakeAdapter() *fakeAdapter {
	return &fakeAdapter{startCh: make(chan struct{}, 1), stopCh: make(chan struct{}, 1)}
}

func (f *fakeAdapter) Configure(cfg nexusclient.Config) error {
	f.config = cfg
	return nil
}

func (f *fakeAdapter) Start(ctx context.Context) error {
	select {
	case f.startCh <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return ctx.Err()
}

func (f *fakeAdapter) Stop(ctx context.Context) error {
	select {
	case f.stopCh <- struct{}{}:
	default:
	}
	return nil
}

func (f *fakeAdapter) awaitStop(timeout time.Duration) error {
	select {
	case <-f.stopCh:
		return nil
	case <-time.After(timeout):
		return errors.New("adapter stop timeout")
	}
}

func TestManager_NexusAdapterLifecycle(t *testing.T) {
	dir := t.TempDir()
	storage, err := newFileStorage(dir)
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	m, err := newManagerWithDeps(storage, dir, &stubDialer{}, &stubResolver{}, fixedNow(time.Unix(3, 0)))
	if err != nil {
		t.Fatalf("manager: %v", err)
	}
	adapter := newFakeAdapter()
	m.SetNexusAdapter(adapter)

	if err := m.Configure(ConfigureRequest{
		Endpoint:       "wss://nexus.example.com/connect",
		DeviceSecret:   "secret",
		Solver:         "http-01",
		TLD:            "example.com",
		PortalHostname: "portal.example.com",
	}); err != nil {
		t.Fatalf("configure: %v", err)
	}
	if adapter.config.TLD != "example.com" {
		t.Fatalf("expected TLD to propagate, got %s", adapter.config.TLD)
	}

	select {
	case <-adapter.startCh:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("expected adapter start")
	}

	if err := m.Disable(); err != nil {
		t.Fatalf("disable: %v", err)
	}

	if err := adapter.awaitStop(500 * time.Millisecond); err != nil {
		t.Fatalf("adapter stop: %v", err)
	}
}

func TestRunPreflightFailures(t *testing.T) {
	dir := t.TempDir()
	dial := &stubDialer{err: errors.New("dial failed")}
	res := &stubResolver{}
	storage, err := newFileStorage(dir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := newManagerWithDeps(storage, dir, dial, res, fixedNow(time.Unix(2, 0)))
	if err != nil {
		t.Fatal(err)
	}

	_ = m.Configure(ConfigureRequest{
		Endpoint:       "wss://nexus.example.com/connect",
		DeviceSecret:   "secret",
		Solver:         "dns-01",
		TLD:            "example.com",
		PortalHostname: "portal.example.com",
		DNSProvider:    "cloudflare",
	})

	result, err := m.RunPreflight()
	if err != nil {
		t.Fatalf("preflight failed: %v", err)
	}

	foundFail := false
	for _, check := range result.Checks {
		if check.Status == "fail" {
			foundFail = true
		}
	}
	if !foundFail {
		t.Fatalf("expected failure check, got %+v", result.Checks)
	}
}
