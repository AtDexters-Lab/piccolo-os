package app

import (
	"context"

	"piccolod/internal/container"
)

type MockContainerManager struct {
	containers  map[string]*mockContainer
	nextID      int
	createError error
	startError  error
	stopError   error
	removeError error
}

type mockContainer struct {
	ID     string
	Status string
	Spec   container.ContainerCreateSpec
}

func NewMockContainerManager() *MockContainerManager {
	return &MockContainerManager{containers: make(map[string]*mockContainer), nextID: 1}
}

func (m *MockContainerManager) CreateContainer(ctx context.Context, spec container.ContainerCreateSpec) (string, error) {
	if m.createError != nil {
		return "", m.createError
	}
	if m.containers == nil {
		m.containers = make(map[string]*mockContainer)
	}
	id := generateMockContainerID(m.nextID)
	m.nextID++
	m.containers[id] = &mockContainer{ID: id, Status: "created", Spec: spec}
	return id, nil
}

func (m *MockContainerManager) StartContainer(ctx context.Context, containerID string) error {
	if m.startError != nil {
		return m.startError
	}
	if c, ok := m.containers[containerID]; ok {
		c.Status = "running"
		return nil
	}
	return container.ErrContainerNotFound(containerID)
}

func (m *MockContainerManager) StopContainer(ctx context.Context, containerID string) error {
	if m.stopError != nil {
		return m.stopError
	}
	if c, ok := m.containers[containerID]; ok {
		c.Status = "stopped"
		return nil
	}
	return container.ErrContainerNotFound(containerID)
}

func (m *MockContainerManager) RemoveContainer(ctx context.Context, containerID string) error {
	if m.removeError != nil {
		return m.removeError
	}
	if _, ok := m.containers[containerID]; ok {
		delete(m.containers, containerID)
		return nil
	}
	return container.ErrContainerNotFound(containerID)
}

func (m *MockContainerManager) PullImage(ctx context.Context, image string) error { return nil }

func (m *MockContainerManager) Logs(ctx context.Context, containerID string, lines int) ([]string, error) {
	if _, ok := m.containers[containerID]; !ok {
		return nil, container.ErrContainerNotFound(containerID)
	}
	if lines <= 0 {
		lines = 3
	}
	out := make([]string, lines)
	for i := range out {
		out[i] = "log line"
	}
	return out, nil
}

func generateMockContainerID(id int) string {
	return "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcd" + string(rune('0'+id%10))
}
