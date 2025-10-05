package nexusclient

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"

	backend "github.com/AtDexters-Lab/nexus-proxy-backend-client/client"
	"piccolod/internal/router"
)

// BackendAdapter wraps the real nexus backend client.
type BackendAdapter struct {
	mu       sync.Mutex
	cfg      Config
	client   *backend.Client
	cancel   context.CancelFunc
	router   *router.Manager
	resolver RemoteResolver
}

func NewBackendAdapter(r *router.Manager, resolver RemoteResolver) *BackendAdapter {
	return &BackendAdapter{router: r, resolver: resolver}
}

func (a *BackendAdapter) Configure(cfg Config) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cfg = cfg
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
	if cfg.Endpoint == "" || cfg.DeviceSecret == "" || cfg.PortalHostname == "" {
		a.mu.Unlock()
		log.Printf("WARN: nexus adapter start skipped, missing configuration")
		return nil
	}

	backendCfg := backend.ClientBackendConfig{
		Name:         "piccolo-portal",
		Hostnames:    []string{cfg.PortalHostname},
		NexusAddress: cfg.Endpoint,
		AuthToken:    cfg.DeviceSecret,
		PortMappings: map[int]backend.PortMapping{
			443: {Default: "127.0.0.1:443"},
			80:  {Default: "127.0.0.1:80"},
		},
	}

	client := backend.New(backendCfg, backend.WithConnectHandler(a.connectHandler()))
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
			if port, ok := a.resolver.Resolve(req.OriginalHostname, req.Port); ok {
				localPort = port
			} else if port, ok := a.resolver.Resolve(req.Hostname, req.Port); ok {
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
		return d.DialContext(ctx, "tcp", target)
	}
}
