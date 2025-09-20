package remote

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
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

	m, err := newManagerWithDeps(dir, dial, res, fixedNow(time.Unix(1, 0)))
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

func TestRunPreflightFailures(t *testing.T) {
	dir := t.TempDir()
	dial := &stubDialer{err: errors.New("dial failed")}
	res := &stubResolver{}
	m, err := newManagerWithDeps(dir, dial, res, fixedNow(time.Unix(2, 0)))
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
