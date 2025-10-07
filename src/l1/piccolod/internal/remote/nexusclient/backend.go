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
	"github.com/golang-jwt/jwt/v5"
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

	hosts := []string{strings.TrimSuffix(strings.ToLower(cfg.PortalHostname), ".")}
	if cfg.TLD != "" {
		trimmed := strings.TrimSuffix(strings.ToLower(cfg.TLD), ".")
		if trimmed != "" {
			hosts = append(hosts, "*."+trimmed)
		}
	}
	token, err := buildBackendToken(cfg.DeviceSecret, hosts)
	if err != nil {
		a.mu.Unlock()
		return fmt.Errorf("failed to build backend token: %w", err)
	}
	backendCfg := backend.ClientBackendConfig{
		Name:         "piccolo-portal",
		Hostnames:    hosts,
		NexusAddress: cfg.Endpoint,
		AuthToken:    token,
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

// UnregisterPublicPort marks a local public port as unavailable so new inbound
// streams are refused immediately at the adapter boundary. Existing client
// streams are naturally torn down when the proxy listener closes.
// No-op lifecycle hooks for dynamic port publish/unpublish are intentionally omitted for now.

func buildBackendToken(secret string, hostnames []string) (string, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return "", fmt.Errorf("nexus jwt secret required")
	}
	if len(hostnames) == 0 {
		return "", fmt.Errorf("at least one hostname required")
	}
	expires := time.Now().Add(15 * time.Minute)
	claims := jwt.MapClaims{
		"hostnames": hostnames,
		"iat":       time.Now().Unix(),
		"exp":       expires.Unix(),
	}
	if len(hostnames) == 1 {
		claims["hostname"] = hostnames[0]
	}
	tkn := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tkn.SignedString([]byte(secret))
}
