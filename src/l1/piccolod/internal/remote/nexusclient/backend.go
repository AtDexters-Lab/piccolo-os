package nexusclient

import (
	"context"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	backend "github.com/AtDexters-Lab/nexus-proxy-backend-client/client"

	"piccolod/internal/router"
)

type backendClient interface {
	Start(context.Context)
	Stop()
}

type clientFactory func(backend.ClientBackendConfig, backend.ConnectHandler) (backendClient, error)

type realBackendClient struct {
	*backend.Client
}

func (c *realBackendClient) Start(ctx context.Context) { c.Client.Start(ctx) }
func (c *realBackendClient) Stop()                     { c.Client.Stop() }

const (
	attestationTokenTTL           = 60 * time.Second
	attestationHandshakeMaxAgeSec = 5
	attestationReauthIntervalSec  = 300
	attestationReauthGraceSec     = 30
	attestationMaintenanceCapSec  = 600
	attestationCacheHandshake     = 5 * time.Second
)

// BackendAdapter bridges piccolod with the nexus proxy backend client. It now uses
// the upstream token provider hook so that every connection attempt receives a
// freshly minted JWT without custom reconnect loops on our side.
type BackendAdapter struct {
	mu       sync.Mutex
	cfg      Config
	router   *router.Manager
	resolver RemoteResolver

	factory clientFactory
	cancel  context.CancelFunc
	client  backendClient
}

func NewBackendAdapter(r *router.Manager, resolver RemoteResolver) *BackendAdapter {
	return &BackendAdapter{
		router:   r,
		resolver: resolver,
		factory: func(cfg backend.ClientBackendConfig, handler backend.ConnectHandler) (backendClient, error) {
			client, err := backend.New(cfg, backend.WithConnectHandler(handler))
			if err != nil {
				return nil, err
			}
			return &realBackendClient{
				Client: client,
			}, nil
		},
	}
}

func (a *BackendAdapter) Configure(cfg Config) error {
	a.mu.Lock()
	a.cfg = cfg
	a.mu.Unlock()

	if updater, ok := a.resolver.(interface{ UpdateConfig(Config) }); ok {
		updater.UpdateConfig(cfg)
	}
	return nil
}

func (a *BackendAdapter) Start(ctx context.Context) error {
	a.mu.Lock()
	if a.cancel != nil {
		a.mu.Unlock()
		return nil
	}
	cfg := a.cfg
	if !configReady(cfg) {
		a.mu.Unlock()
		log.Printf("WARN: nexus adapter start skipped, missing configuration")
		return nil
	}
	hosts := buildHostnameList(cfg)
	backendCfg := backend.ClientBackendConfig{
		Name:         "piccolo-portal",
		Hostnames:    hosts,
		NexusAddress: cfg.Endpoint,
		Weight:       1,
		PortMappings: map[int]backend.PortMapping{
			443: {Default: "127.0.0.1:443"},
			80:  {Default: "127.0.0.1:80"},
		},
		Attestation: backend.AttestationOptions{
			HMACSecret:                 strings.TrimSpace(cfg.DeviceSecret),
			TokenTTL:                   attestationTokenTTL,
			CacheHandshake:             attestationCacheHandshake,
			HandshakeMaxAgeSeconds:     attestationHandshakeMaxAgeSec,
			ReauthIntervalSeconds:      attestationReauthIntervalSec,
			ReauthGraceSeconds:         attestationReauthGraceSec,
			MaintenanceGraceCapSeconds: attestationMaintenanceCapSec,
		},
	}
	handler := a.connectHandler()

	client, err := a.factory(backendCfg, handler)
	if err != nil {
		a.mu.Unlock()
		return fmt.Errorf("construct backend client: %w", err)
	}
	runCtx, cancel := context.WithCancel(ctx)
	a.client = client
	a.cancel = cancel
	a.mu.Unlock()

	go client.Start(runCtx)
	return nil
}

func (a *BackendAdapter) Stop(ctx context.Context) error {
	a.mu.Lock()
	cancel := a.cancel
	client := a.client
	a.cancel = nil
	a.client = nil
	a.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if client != nil {
		client.Stop()
	}
	return nil
}

func (a *BackendAdapter) connectHandler() backend.ConnectHandler {
	return func(ctx context.Context, req backend.ConnectRequest) (net.Conn, error) {
		if a.router != nil {
			route := a.router.DecideAppRoute(req.Hostname)
			if route.Mode == router.ModeTunnel {
				return nil, backend.ErrNoRoute
			}
		}

		localPort := 0
		if a.resolver != nil {
			if port, ok := a.resolver.Resolve(req.OriginalHostname, req.Port, req.IsTLS); ok {
				localPort = port
			} else if port, ok := a.resolver.Resolve(req.Hostname, req.Port, req.IsTLS); ok {
				localPort = port
			} else {
				return nil, backend.ErrNoRoute
			}
		}
		if localPort == 0 {
			localPort = req.Port
		}
		target := fmt.Sprintf("127.0.0.1:%d", localPort)
		var d net.Dialer
		conn, err := d.DialContext(ctx, "tcp", target)
		if err != nil {
			return nil, err
		}
		if recorder, ok := a.resolver.(interface{ RecordConnectionHint(int, int, int, bool) }); ok {
			if addr, ok := conn.LocalAddr().(*net.TCPAddr); ok {
				recorder.RecordConnectionHint(localPort, addr.Port, req.Port, req.IsTLS)
			}
		}
		return conn, nil
	}
}

func (a *BackendAdapter) currentConfig() Config {
	a.mu.Lock()
	cfg := a.cfg
	a.mu.Unlock()
	return cfg
}

func buildHostnameList(cfg Config) []string {
	hosts := []string{strings.TrimSuffix(strings.ToLower(cfg.PortalHostname), ".")}
	if cfg.TLD != "" {
		trimmed := strings.TrimSuffix(strings.ToLower(cfg.TLD), ".")
		if trimmed != "" {
			hosts = append(hosts, "*."+trimmed)
		}
	}
	return hosts
}

func configReady(cfg Config) bool {
	return strings.TrimSpace(cfg.Endpoint) != "" &&
		strings.TrimSpace(cfg.DeviceSecret) != "" &&
		strings.TrimSpace(cfg.PortalHostname) != ""
}
