package commands

import "context"

// Command represents a typed request routed through the dispatcher.
type Command interface {
	Name() string
}

// Response represents a typed response to a command.
type Response interface{}

// Handler processes a specific command type.
type Handler interface {
	Handle(ctx context.Context, cmd Command) (Response, error)
}

// HandlerFunc adapts a function to the Handler interface.
type HandlerFunc func(ctx context.Context, cmd Command) (Response, error)

// Handle invokes the underlying function.
func (f HandlerFunc) Handle(ctx context.Context, cmd Command) (Response, error) {
	return f(ctx, cmd)
}

// Dispatcher routes commands to registered handlers.
type Dispatcher struct {
	handlers map[string]Handler
}

// NewDispatcher creates an empty dispatcher.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{handlers: make(map[string]Handler)}
}

// Register associates a handler with a command name. Panics if a handler is
// already registered.
func (d *Dispatcher) Register(name string, h Handler) {
	if _, exists := d.handlers[name]; exists {
		panic("commands: handler already registered for " + name)
	}
	d.handlers[name] = h
}

// Dispatch routes the command to the registered handler.
func (d *Dispatcher) Dispatch(ctx context.Context, cmd Command) (Response, error) {
	h, ok := d.handlers[cmd.Name()]
	if !ok {
		return nil, ErrUnknownCommand{name: cmd.Name()}
	}
	return h.Handle(ctx, cmd)
}

// ErrUnknownCommand is returned when no handler exists for a command.
type ErrUnknownCommand struct {
	name string
}

func (e ErrUnknownCommand) Error() string { return "commands: unknown command " + e.name }
