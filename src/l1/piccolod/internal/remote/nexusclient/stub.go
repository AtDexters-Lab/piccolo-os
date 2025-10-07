package nexusclient

import (
	"context"
	"log"
	"sync"
)

// Stub is a lightweight implementation used until the real nexus client is wired.
type Stub struct {
    mu      sync.Mutex
    cfg     Config
    running bool
}

func NewStub() *Stub {
	return &Stub{}
}

func (s *Stub) Configure(cfg Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg = cfg
	log.Printf("INFO: nexus stub configured endpoint=%s portal=%s", cfg.Endpoint, cfg.PortalHostname)
	return nil
}

func (s *Stub) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	cfg := s.cfg
	s.running = true
	s.mu.Unlock()

	log.Printf("INFO: nexus stub starting endpoint=%s", cfg.Endpoint)
	<-ctx.Done()
	return nil
}

func (s *Stub) Stop(ctx context.Context) error {
    s.mu.Lock()
    if !s.running {
        s.mu.Unlock()
        return nil
    }
    s.running = false
    s.mu.Unlock()
    log.Printf("INFO: nexus stub stopping")
    return nil
}

// UnregisterPublicPort is a no-op for the stub, but keeps logs for visibility.
func (s *Stub) UnregisterPublicPort(port int) {
    log.Printf("INFO: nexus stub unregister port=%d (no-op)", port)
}

// RegisterPublicPort is a no-op for the stub.
func (s *Stub) RegisterPublicPort(port int) {
    log.Printf("INFO: nexus stub register port=%d (no-op)", port)
}
