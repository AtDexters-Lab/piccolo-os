package container

import (
	"context"
	"log"

	"piccolod/internal/api"
)

// Manager handles container lifecycle using Podman
type Manager struct {
	// Future: Podman client or direct podman CLI integration
	// podmanClient *podman.Client
}

func NewManager() (*Manager, error) {
	// In a real implementation, we would use Podman client/CLI
	log.Println("INFO: Container Manager initialized with Podman backend (placeholder)")
	return &Manager{}, nil
}

// --- Lifecycle methods ---
func (m *Manager) Create(ctx context.Context, req api.CreateContainerRequest) (*api.Container, error) {
	log.Printf("INFO: Placeholder: Creating container '%s' with resources %+v", req.Name, req.Resources)
	return &api.Container{ID: "new-dummy-id", Name: req.Name, Image: req.Image, State: "created"}, nil
}
func (m *Manager) Start(ctx context.Context, id string) error                   { return nil }
func (m *Manager) Stop(ctx context.Context, id string) error                    { return nil }
func (m *Manager) Restart(ctx context.Context, id string) error                 { return nil }
func (m *Manager) Delete(ctx context.Context, id string) error                  { return nil }
func (m *Manager) Update(ctx context.Context, id string, newImage string) error { return nil }

// --- Information methods ---
func (m *Manager) List(ctx context.Context, filter string) ([]api.Container, error) { return nil, nil }
func (m *Manager) Get(ctx context.Context, id string) (*api.Container, error)       { return nil, nil }
