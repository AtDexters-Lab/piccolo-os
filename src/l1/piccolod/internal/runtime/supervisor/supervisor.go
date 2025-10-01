package supervisor

import (
	"context"
	"sync"
)

// Component represents a unit of work managed by the supervisor.
type Component interface {
	Name() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// Supervisor coordinates the lifecycle of registered components.
type Supervisor struct {
	mu         sync.Mutex
	components []Component
	started    bool
}

// New creates an empty supervisor.
func New() *Supervisor {
	return &Supervisor{}
}

// Register adds a component to the supervisor. Registration is only allowed
// before Start is called.
func (s *Supervisor) Register(c Component) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		panic("supervisor: cannot register component after start")
	}
	s.components = append(s.components, c)
}

// Start iterates components in registration order and invokes Start on each.
// If any component fails, previously started components are stopped in reverse
// order and the error is returned.
func (s *Supervisor) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return nil
	}
	s.started = true
	comps := append([]Component(nil), s.components...)
	s.mu.Unlock()

	started := make([]Component, 0, len(comps))
	for _, c := range comps {
		if err := c.Start(ctx); err != nil {
			for i := len(started) - 1; i >= 0; i-- {
				_ = started[i].Stop(ctx)
			}
			return err
		}
		started = append(started, c)
	}
	return nil
}

// Stop stops all components in reverse registration order. It is safe to call
// even if Start was never invoked.
func (s *Supervisor) Stop(ctx context.Context) error {
	s.mu.Lock()
	comps := append([]Component(nil), s.components...)
	s.started = false
	s.mu.Unlock()

	var firstErr error
	for i := len(comps) - 1; i >= 0; i-- {
		if err := comps[i].Stop(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
