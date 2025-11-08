package server

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"piccolod/internal/remote"
	"piccolod/internal/remote/nexusclient"
)

func TestRemotePortalOverTLSEmitsHSTS(t *testing.T) {
	srv := createGinTestServer(t, t.TempDir())
	defer srv.tlsMux.Stop()

	srv.startSecureLoopback()
	t.Cleanup(srv.stopSecureLoopback)

	const host = "portal.example.com"

	cert := mustSelfSignedCert(t, host)
	srv.tlsMux.SetCertProvider(&staticCertProvider{cert: cert})

	runtimeStatus := remote.Status{Enabled: true, PortalHostname: host, TLD: "example.com"}
	srv.applyRemoteRuntimeFromStatus(runtimeStatus)

	port := srv.tlsMux.Port()
	if port == 0 {
		t.Fatalf("expected tls mux to be listening")
	}

	conn, err := tls.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port), &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: true, // self-signed for test
	})
	if err != nil {
		t.Fatalf("tls dial: %v", err)
	}
	defer conn.Close()

	req := fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", host)
	if _, err := conn.Write([]byte(req)); err != nil {
		t.Fatalf("write request: %v", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
	if hsts := resp.Header.Get("Strict-Transport-Security"); hsts == "" {
		t.Fatalf("expected Strict-Transport-Security header, got none")
	}
}

func TestRemotePortalHTTPRedirectsToHTTPS(t *testing.T) {
	srv := createGinTestServer(t, t.TempDir())
	defer srv.tlsMux.Stop()

	const host = "portal.example.com"

	runtimeStatus := remote.Status{Enabled: true, PortalHostname: host, TLD: "example.com"}
	srv.applyRemoteRuntimeFromStatus(runtimeStatus)
	srv.remoteResolver.UpdateConfig(nexusclient.Config{PortalHostname: host, TLD: "example.com"})

	deadline := time.Now().Add(200 * time.Millisecond)
	for !srv.remoteResolver.IsRemoteHostname(host) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !srv.remoteResolver.IsRemoteHostname(host) {
		t.Fatalf("resolver did not recognize remote host %q", host)
	}
	for srv.tlsMux.Port() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if srv.tlsMux.Port() == 0 {
		t.Fatalf("tls mux did not start")
	}
	// Reapply to ensure resolver has latest view even if background events reset it.
	srv.applyRemoteRuntimeFromStatus(remote.Status{
		Enabled:        true,
		PortalHostname: host,
		TLD:            "example.com",
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://"+host+"/", nil)
	req.Host = host

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusMovedPermanently {
		t.Fatalf("expected 301 redirect, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "https://"+host+"/" {
		t.Fatalf("expected Location https://%s/, got %q", host, loc)
	}
}

func TestLocalHostSkipsHTTPSRedirect(t *testing.T) {
	srv := createGinTestServer(t, t.TempDir())
	defer srv.tlsMux.Stop()

	const host = "piccolo.local"

	runtimeStatus := remote.Status{Enabled: true, PortalHostname: host, TLD: "local"}
	srv.applyRemoteRuntimeFromStatus(runtimeStatus)
	srv.remoteResolver.UpdateConfig(nexusclient.Config{PortalHostname: host, TLD: "local"})

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "http://"+host+"/health/live", nil)
	req.Host = host
	ctx.Request = req

	handler := srv.httpsRedirectMiddleware()
	handler(ctx)

	if ctx.IsAborted() {
		t.Fatalf("expected middleware to continue for %s, but request was aborted with status %d", host, w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "" {
		t.Fatalf("expected no redirect for %s, got Location header %q (status %d)", host, loc, w.Code)
	}
}

func TestLoopbackIpSkipsHTTPSRedirect(t *testing.T) {
	srv := createGinTestServer(t, t.TempDir())
	defer srv.tlsMux.Stop()

	const host = "127.0.0.1"

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "http://"+host+"/health/live", nil)
	req.Host = host
	ctx.Request = req

	handler := srv.httpsRedirectMiddleware()
	handler(ctx)

	if ctx.IsAborted() {
		t.Fatalf("expected middleware to continue for %s, but request was aborted with status %d", host, w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "" {
		t.Fatalf("expected no redirect for %s, got Location header %q (status %d)", host, loc, w.Code)
	}
}

type staticCertProvider struct {
	cert *tls.Certificate
}

func (p *staticCertProvider) GetCertificate(host string) (*tls.Certificate, error) {
	return p.cert, nil
}

func mustSelfSignedCert(t *testing.T, host string) *tls.Certificate {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	now := time.Now().Add(-time.Minute)
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		t.Fatalf("serial: %v", err)
	}
	template := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: host,
		},
		NotBefore:             now,
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{host},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes})
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("x509 keypair: %v", err)
	}
	return &pair
}
