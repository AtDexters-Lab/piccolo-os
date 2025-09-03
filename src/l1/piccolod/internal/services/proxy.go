package services

import (
    "io"
    "log"
    "net"
    "net/http"
    "net/http/httputil"
    "net/url"
    "strconv"
    "sync"
    "time"
)

// ProxyManager manages TCP listeners and proxies traffic based on ServiceEndpoint
type ProxyManager struct {
    mu        sync.Mutex
    listeners map[int]net.Listener // by public port
    wg        sync.WaitGroup
}

func NewProxyManager() *ProxyManager {
    return &ProxyManager{listeners: make(map[int]net.Listener)}
}

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
    case "tls":
        // Raw TCP passthrough
        p.startTCPProxy(ln, ep)
    case "tcp":
        switch ep.Protocol {
        case "http", "websocket":
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
        rp.ServeHTTP(w, r)
    }))
    handler = securityHeaders(handler)
    handler = requestLogging(handler)
    handler = basicRateLimit(handler) // stub

    p.wg.Add(1)
    go func() {
        defer p.wg.Done()
        log.Printf("INFO: HTTP proxy %s → %s (app=%s listener=%s protocol=%s)", ln.Addr().String(), target, ep.App, ep.Name, ep.Protocol)
        srv := &http.Server{Handler: handler}
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

// no extra helpers


// StopAll stops all listeners
func (p *ProxyManager) StopAll() {
    p.mu.Lock()
    for port, ln := range p.listeners {
        _ = ln.Close()
        delete(p.listeners, port)
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
    p.mu.Unlock()
}

// small int→string helper without strconv to keep deps minimal
// no extra helpers
