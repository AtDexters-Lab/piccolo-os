package services

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"piccolod/internal/api"
)

type connectionHint struct {
	isTLS      bool
	remotePort int
}

type hintContextKey struct{}

// ProxyManager manages TCP listeners and proxies traffic based on ServiceEndpoint
type ProxyManager struct {
	mu        sync.Mutex
	listeners map[int]net.Listener // by public port
	hints     map[int]map[int]connectionHint
	wg        sync.WaitGroup
	acme      http.Handler
}

func NewProxyManager() *ProxyManager {
	return &ProxyManager{listeners: make(map[int]net.Listener)}
}

func (p *ProxyManager) registerHint(listenerPort, sourcePort int, hint connectionHint) {
	if sourcePort <= 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.hints == nil {
		p.hints = make(map[int]map[int]connectionHint)
	}
	m := p.hints[listenerPort]
	if m == nil {
		m = make(map[int]connectionHint)
		p.hints[listenerPort] = m
	}
	m[sourcePort] = hint
}

func (p *ProxyManager) consumeHint(listenerPort, sourcePort int) (connectionHint, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if m := p.hints[listenerPort]; m != nil {
		if hint, ok := m[sourcePort]; ok {
			delete(m, sourcePort)
			if len(m) == 0 {
				delete(p.hints, listenerPort)
			}
			return hint, true
		}
	}
	return connectionHint{}, false
}

// SetAcmeHandler registers a handler to serve HTTP-01 challenges for all HTTP proxies.
func (p *ProxyManager) SetAcmeHandler(h http.Handler) { p.mu.Lock(); p.acme = h; p.mu.Unlock() }

// StartListener starts a TCP proxy for the given endpoint
func (p *ProxyManager) StartListener(ep ServiceEndpoint) {
	addr := net.JoinHostPort("0.0.0.0", strconv.Itoa(ep.PublicPort))
	// Avoid double-start
	p.mu.Lock()
	if _, exists := p.listeners[ep.PublicPort]; exists {
		p.mu.Unlock()
		return
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Printf("WARN: Failed to bind public listener on %s: %v", addr, err)
		p.mu.Unlock()
		return
	}
	p.listeners[ep.PublicPort] = ln
	p.mu.Unlock()

	switch ep.Flow {
	case api.FlowTLS:
		// Raw TCP passthrough
		p.startTCPProxy(ln, ep)
	case api.FlowTCP:
		switch ep.Protocol {
		case api.ListenerProtocolHTTP, api.ListenerProtocolWebsocket:
			p.startHTTPProxy(ln, ep)
		default:
			p.startTCPProxy(ln, ep)
		}
	default:
		p.startTCPProxy(ln, ep)
	}
}

func (p *ProxyManager) handleConn(ep ServiceEndpoint, client net.Conn) {
	defer client.Close()
	backendAddr := net.JoinHostPort("127.0.0.1", strconv.Itoa(ep.HostBind))

	// For v1: passthrough for all flows; framework in place to add protocol handlers
	backend, err := net.DialTimeout("tcp", backendAddr, 5*time.Second)
	if err != nil {
		log.Printf("WARN: Backend connect failed %s: %v", backendAddr, err)
		return
	}
	defer backend.Close()

	// Bi-directional copy
	done := make(chan struct{}, 2)
	go func() { io.Copy(backend, client); backend.(*net.TCPConn).CloseWrite(); done <- struct{}{} }()
	go func() { io.Copy(client, backend); client.(*net.TCPConn).CloseWrite(); done <- struct{}{} }()
	<-done
}

func (p *ProxyManager) startTCPProxy(ln net.Listener, ep ServiceEndpoint) {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		log.Printf("INFO: TCP proxy %s → 127.0.0.1:%d (app=%s listener=%s)", ln.Addr().String(), ep.HostBind, ep.App, ep.Name)
		for {
			conn, err := ln.Accept()
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Temporary() {
					time.Sleep(50 * time.Millisecond)
					continue
				}
				return
			}
			// TODO L0: rate-limit + metrics per IP (stub)
			p.wg.Add(1)
			go func(c net.Conn) {
				defer p.wg.Done()
				p.handleConn(ep, c)
			}(conn)
		}
	}()
}

func (p *ProxyManager) startHTTPProxy(ln net.Listener, ep ServiceEndpoint) {
	target := "http://127.0.0.1:" + strconv.Itoa(ep.HostBind)
	u, err := url.Parse(target)
	if err != nil {
		log.Printf("WARN: invalid reverse proxy target %s: %v", target, err)
		return
	}
	rp := httputil.NewSingleHostReverseProxy(u)
	// Basic transport tuning; defaults are fine for v1

	// Default middleware chain (stubs)
	handler := http.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		applyForwardHeaders(r, ep)
		// Intercept ACME HTTP-01 challenges on HTTP proxies only
		if strings.HasPrefix(r.URL.Path, "/.well-known/acme-challenge/") {
			p.mu.Lock()
			acme := p.acme
			p.mu.Unlock()
			if acme != nil {
				acme.ServeHTTP(w, r)
				return
			}
		}
		rp.ServeHTTP(w, r)
	}))
	handler = securityHeaders(handler)
	handler = requestLogging(handler)
	handler = basicRateLimit(handler) // stub

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		log.Printf("INFO: HTTP proxy %s → %s (app=%s listener=%s protocol=%s)", ln.Addr().String(), target, ep.App, ep.Name, ep.Protocol.String())
		srv := &http.Server{
			Handler: handler,
			ConnContext: func(ctx context.Context, c net.Conn) context.Context {
				if addr, ok := c.RemoteAddr().(*net.TCPAddr); ok {
					if hint, ok := p.consumeHint(ep.PublicPort, addr.Port); ok {
						ctx = context.WithValue(ctx, hintContextKey{}, hint)
					}
				}
				return ctx
			},
		}
		_ = srv.Serve(ln) // returns on ln.Close()
	}()
}

// Middleware stubs
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		next.ServeHTTP(w, r)
	})
}

func requestLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Minimal logging to avoid noise in tests
		next.ServeHTTP(w, r)
	})
}

func basicRateLimit(next http.Handler) http.Handler { // placeholder
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}

func hintFromRequest(r *http.Request) (connectionHint, bool) {
	if hint, ok := r.Context().Value(hintContextKey{}).(connectionHint); ok {
		return hint, true
	}
	return connectionHint{}, false
}

func applyForwardHeaders(r *http.Request, ep ServiceEndpoint) {
	host, hostPort := splitHostPortValue(r.Host)
	if host == "" {
		altHost, altPort := splitHostPortValue(r.URL.Host)
		host, hostPort = altHost, altPort
	}

	proto := resolveProto(r, ep)
	ensureHeader(r, "X-Forwarded-Proto", proto)
	if host != "" {
		forwardHost := host
		if hostPort != "" {
			forwardHost = net.JoinHostPort(host, hostPort)
		}
		ensureHeader(r, "X-Forwarded-Host", forwardHost)
		host = forwardHost
	}

	port := resolvePortHeader(r, proto, hostPort)
	ensureHeader(r, "X-Forwarded-Port", port)

	ip := ensureClientIPHeaders(r)
	appendForwardedHeader(r, proto, host, ip)

	if proto == "https" {
		r.URL.Scheme = "https"
	} else {
		r.URL.Scheme = "http"
	}
}

func splitHostPortValue(value string) (string, string) {
	if value == "" {
		return "", ""
	}
	if strings.Contains(value, ":") {
		if host, port, err := net.SplitHostPort(value); err == nil {
			return host, port
		}
	}
	return value, ""
}

func resolveProto(r *http.Request, ep ServiceEndpoint) string {
	if v := strings.ToLower(r.Header.Get("X-Forwarded-Proto")); v != "" {
		return v
	}
	if shouldRewriteAsHTTPS(ep, r) {
		return "https"
	}
	return "http"
}

func shouldRewriteAsHTTPS(ep ServiceEndpoint, r *http.Request) bool {
	if ep.Flow != api.FlowTCP {
		return false
	}
	switch ep.Protocol {
	case api.ListenerProtocolHTTP, api.ListenerProtocolWebsocket:
		return requestArrivedViaTLS(r)
	default:
		return false
	}
}

func requestArrivedViaTLS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if hint, ok := hintFromRequest(r); ok && hint.isTLS {
		return true
	}
	return false
}

func resolvePortHeader(r *http.Request, proto, hostPort string) string {
	if v := r.Header.Get("X-Forwarded-Port"); v != "" {
		return v
	}
	if hint, ok := hintFromRequest(r); ok && hint.remotePort > 0 {
		return strconv.Itoa(hint.remotePort)
	}
	if hostPort != "" {
		return hostPort
	}
	if proto == "https" {
		return "443"
	}
	return "80"
}

func ensureClientIPHeaders(r *http.Request) string {
	ip := r.RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		ip = host
	}
	if ip == "" {
		return ""
	}
	if prior := r.Header.Get("X-Forwarded-For"); prior != "" {
		r.Header.Set("X-Forwarded-For", prior+", "+ip)
	} else {
		r.Header.Set("X-Forwarded-For", ip)
	}
	ensureHeader(r, "X-Real-Ip", ip)
	return ip
}

func appendForwardedHeader(r *http.Request, proto, host, ip string) {
	parts := []string{fmt.Sprintf("proto=%s", proto)}
	if host != "" {
		parts = append(parts, fmt.Sprintf("host=%s", strings.ToLower(host)))
	}
	if ip != "" {
		parts = append(parts, fmt.Sprintf("for=%s", ip))
	}
	value := strings.Join(parts, ";")
	if prior := r.Header.Get("Forwarded"); prior != "" {
		r.Header.Set("Forwarded", prior+", "+value)
	} else {
		r.Header.Set("Forwarded", value)
	}
}

func ensureHeader(r *http.Request, key, value string) {
	if value == "" {
		return
	}
	if r.Header.Get(key) == "" {
		r.Header.Set(key, value)
	}
}

// no extra helpers

// StopAll stops all listeners
func (p *ProxyManager) StopAll() {
	p.mu.Lock()
	for port, ln := range p.listeners {
		_ = ln.Close()
		delete(p.listeners, port)
		delete(p.hints, port)
	}
	p.mu.Unlock()
	p.wg.Wait()
}

// StopPort stops a specific public listener if running
func (p *ProxyManager) StopPort(port int) {
	p.mu.Lock()
	if ln, ok := p.listeners[port]; ok {
		_ = ln.Close()
		delete(p.listeners, port)
	}
	delete(p.hints, port)
	p.mu.Unlock()
}

// small int→string helper without strconv to keep deps minimal
// no extra helpers
