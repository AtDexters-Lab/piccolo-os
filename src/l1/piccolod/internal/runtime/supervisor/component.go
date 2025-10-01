package supervisor

import "context"

// ComponentFunc wraps simple start/stop functions into a Component.
type ComponentFunc struct {
	name  string
	start func(ctx context.Context) error
	stop  func(ctx context.Context) error
}

// NewComponent creates a Component from callbacks.
func NewComponent(name string, start func(ctx context.Context) error, stop func(ctx context.Context) error) Component {
	return &ComponentFunc{name: name, start: start, stop: stop}
}

func (c *ComponentFunc) Name() string { return c.name }

func (c *ComponentFunc) Start(ctx context.Context) error {
	if c.start == nil {
		return nil
	}
	return c.start(ctx)
}

func (c *ComponentFunc) Stop(ctx context.Context) error {
	if c.stop == nil {
		return nil
	}
	return c.stop(ctx)
}
