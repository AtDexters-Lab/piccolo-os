package app

import (
	"context"
	"time"

	"piccolod/internal/container"
)

// ContainerManager describes the container runtime operations required by the app manager.
type ContainerManager interface {
	CreateContainer(ctx context.Context, spec container.ContainerCreateSpec) (string, error)
	StartContainer(ctx context.Context, containerID string) error
	StopContainer(ctx context.Context, containerID string) error
	RemoveContainer(ctx context.Context, containerID string) error
	PullImage(ctx context.Context, image string) error
	Logs(ctx context.Context, containerID string, lines int) ([]string, error)
}

// AppInstance captures the runtime metadata for an installed application.
type AppInstance struct {
	Name        string            `json:"name"`
	Image       string            `json:"image"`
	Type        string            `json:"type"`
	Status      string            `json:"status"`
	ContainerID string            `json:"container_id"`
	Environment map[string]string `json:"environment,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}
