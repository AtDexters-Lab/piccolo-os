package services

import (
    "bufio"
    "net"
    "strconv"
    "testing"
    "time"
)

// startEchoBackend starts a simple TCP echo server on 127.0.0.1:0 and returns its port and a shutdown func
func startEchoBackend(t *testing.T) (int, func()) {
    t.Helper()
    ln, err := net.Listen("tcp", "127.0.0.1:0")
    if err != nil {
        t.Fatalf("failed to start backend: %v", err)
    }
    addr := ln.Addr().(*net.TCPAddr)
    stop := make(chan struct{})
    go func() {
        for {
            conn, err := ln.Accept()
            if err != nil {
                select {
                case <-stop:
                    return
                default:
                    return
                }
            }
            go func(c net.Conn) {
                defer c.Close()
                r := bufio.NewReader(c)
                w := bufio.NewWriter(c)
                for {
                    line, err := r.ReadBytes('\n')
                    if err != nil {
                        return
                    }
                    if _, err := w.Write(line); err != nil {
                        return
                    }
                    _ = w.Flush()
                }
            }(conn)
        }
    }()

    shutdown := func() {
        close(stop)
        _ = ln.Close()
    }
    return addr.Port, shutdown
}

func getFreePort(t *testing.T) int {
    t.Helper()
    ln, err := net.Listen("tcp", "0.0.0.0:0")
    if err != nil {
        t.Fatalf("failed to get free port: %v", err)
    }
    port := ln.Addr().(*net.TCPAddr).Port
    _ = ln.Close()
    return port
}

func TestProxy_PassthroughTCP(t *testing.T) {
    hb, stop := startEchoBackend(t)
    defer stop()

    pm := NewProxyManager()
    public := getFreePort(t)
    ep := ServiceEndpoint{App: "test", Name: "echo", GuestPort: 0, HostBind: hb, PublicPort: public, Flow: "tcp", Protocol: "raw"}
    pm.StartListener(ep)
    defer pm.StopAll()

    // Give the proxy time to bind
    time.Sleep(100 * time.Millisecond)

    conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(public)))
    if err != nil {
        t.Fatalf("failed to dial proxy: %v", err)
    }
    defer conn.Close()

    msg := []byte("hello\n")
    if _, err := conn.Write(msg); err != nil {
        t.Fatalf("write failed: %v", err)
    }

    buf := make([]byte, len(msg))
    if _, err := conn.Read(buf); err != nil {
        t.Fatalf("read failed: %v", err)
    }

    if string(buf) != string(msg) {
        t.Fatalf("unexpected echo: got %q want %q", string(buf), string(msg))
    }
}
