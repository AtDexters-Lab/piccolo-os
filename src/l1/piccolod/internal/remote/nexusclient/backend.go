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

type backendClient interface {
	Start(context.Context)
	Stop()
}

type clientFactory func(backend.ClientBackendConfig, backend.ConnectHandler, backend.TokenProvider) backendClient

type realBackendClient struct {
	*backend.Client
}

func (c *realBackendClient) Start(ctx context.Context) { c.Client.Start(ctx) }
func (c *realBackendClient) Stop()                     { c.Client.Stop() }

var (
	backendTokenTTL = 15 * time.Minute
	timeNow         = time.Now
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
		factory: func(cfg backend.ClientBackendConfig, handler backend.ConnectHandler, tokenProvider backend.TokenProvider) backendClient {
			return &realBackendClient{
				Client: backend.New(cfg, backend.WithConnectHandler(handler), backend.WithTokenProvider(tokenProvider)),
			}
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
		PortMappings: map[int]backend.PortMapping{
			443: {Default: "127.0.0.1:443"},
			80:  {Default: "127.0.0.1:80"},
		},
	}
	handler := a.connectHandler()
	provider := a.tokenProvider()

	client := a.factory(backendCfg, handler, provider)
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

func (a *BackendAdapter) tokenProvider() backend.TokenProvider {
	return func(ctx context.Context) (backend.Token, error) {
		cfg := a.currentConfig()
		if !configReady(cfg) {
			return backend.Token{}, fmt.Errorf("nexus adapter not configured")
		}
		hosts := buildHostnameList(cfg)
		tokenValue, expires, err := buildBackendToken(cfg.DeviceSecret, hosts)
		if err != nil {
			return backend.Token{}, err
		}
		return backend.Token{Value: tokenValue, Expiry: expires}, nil
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

func buildBackendToken(secret string, hostnames []string) (string, time.Time, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return "", time.Time{}, fmt.Errorf("nexus jwt secret required")
	}
	if len(hostnames) == 0 {
		return "", time.Time{}, fmt.Errorf("at least one hostname required")
	}
	now := timeNow()
	expires := now.Add(backendTokenTTL)
	claims := jwt.MapClaims{
		"hostnames": hostnames,
		"iat":       now.Unix(),
		"exp":       expires.Unix(),
	}
	if len(hostnames) == 1 {
		claims["hostname"] = hostnames[0]
	}
	tkn := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token, err := tkn.SignedString([]byte(secret))
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}
