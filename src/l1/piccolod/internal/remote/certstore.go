package remote

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"piccolod/internal/services"
	"piccolod/internal/state/paths"
)

// FileCertProvider loads certificates from an on-disk store under the encrypted
// control volume. It implements services.CertProvider.
type FileCertProvider struct {
	base       string
	mu         sync.RWMutex
	cache      map[string]*tls.Certificate
	portalHost string
}

// NewFileCertProvider constructs a provider rooted at <control>/remote/certs when
// base is empty, or at the provided base otherwise.
func NewFileCertProvider(base string) *FileCertProvider {
	if strings.TrimSpace(base) == "" {
		base = filepath.Join(paths.ControlDir(), "remote", "certs")
	}
	return &FileCertProvider{base: base, cache: make(map[string]*tls.Certificate)}
}

func (p *FileCertProvider) GetCertificate(host string) (*tls.Certificate, error) {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "" {
		return nil, services.ErrNoCert
	}
	p.mu.RLock()
	portal := p.portalHost
	p.mu.RUnlock()
	if portal != "" && host == portal {
		if cert := p.tryLoad("portal"); cert != nil {
			p.toCache("portal", cert)
			return cert, nil
		}
		if cert := p.fromCache("portal"); cert != nil {
			return cert, nil
		}
	}
	// Always prefer fresh load from disk, then fall back to cache.
	if cert := p.tryLoad(host); cert != nil {
		p.toCache(host, cert)
		return cert, nil
	}
	if cert := p.fromCache(host); cert != nil {
		return cert, nil
	}
	// Wildcard fallback: *.domain
	if i := strings.Index(host, "."); i != -1 {
		domain := host[i+1:]
		if domain != "" {
			star := "*." + domain
			if cert := p.tryLoad(star); cert != nil {
				p.toCache(star, cert)
				return cert, nil
			}
			if cert := p.fromCache(star); cert != nil {
				return cert, nil
			}
		}
	}
	return nil, services.ErrNoCert
}

func (p *FileCertProvider) fromCache(key string) *tls.Certificate {
	p.mu.RLock()
	c := p.cache[key]
	p.mu.RUnlock()
	return c
}

func (p *FileCertProvider) toCache(key string, cert *tls.Certificate) {
	p.mu.Lock()
	p.cache[key] = cert
	p.mu.Unlock()
}

func (p *FileCertProvider) SetPortalHostname(host string) {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	p.mu.Lock()
	p.portalHost = host
	p.mu.Unlock()
}

func (p *FileCertProvider) tryLoad(name string) *tls.Certificate {
	// Prefer separate CRT/KEY pair
	crt := filepath.Join(p.base, name+".crt")
	key := filepath.Join(p.base, name+".key")
	if fileExists(crt) && fileExists(key) {
		if c, err := tls.LoadX509KeyPair(crt, key); err == nil {
			return &c
		}
	}
	// Fallback to PEM bundle (cert + key in one file)
	pemPath := filepath.Join(p.base, name+".pem")
	if fileExists(pemPath) {
		if c, err := loadPEMBundle(pemPath); err == nil {
			return c
		}
	}
	return nil
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func loadPEMBundle(path string) (*tls.Certificate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var certs [][]byte
	var keyBlock *pem.Block
	rest := data
	for {
		var b *pem.Block
		b, rest = pem.Decode(rest)
		if b == nil {
			break
		}
		switch b.Type {
		case "CERTIFICATE":
			certs = append(certs, pem.EncodeToMemory(b))
		case "PRIVATE KEY", "RSA PRIVATE KEY", "EC PRIVATE KEY":
			if keyBlock == nil {
				keyBlock = b
			}
		}
	}
	if len(certs) == 0 || keyBlock == nil {
		return nil, errors.New("pem bundle missing cert or key")
	}
	// Parse key to validate
	if _, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes); err != nil {
		// pkcs1/rsa or ec? try generic parse
		if _, err2 := x509.ParsePKCS1PrivateKey(keyBlock.Bytes); err2 != nil {
			if _, err3 := x509.ParseECPrivateKey(keyBlock.Bytes); err3 != nil {
				// key is still acceptable for tls.Certificate as raw PEM; continue
			}
		}
	}
	// Concatenate certs
	var certPEM []byte
	for _, c := range certs {
		certPEM = append(certPEM, c...)
	}
	keyPEM := pem.EncodeToMemory(keyBlock)
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("x509 keypair: %w", err)
	}
	return &pair, nil
}

var _ services.CertProvider = (*FileCertProvider)(nil)
