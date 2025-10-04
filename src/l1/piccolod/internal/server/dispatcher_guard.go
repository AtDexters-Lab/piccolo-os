package server

import (
    "context"
    "strings"

    crypt "piccolod/internal/crypt"
    "piccolod/internal/persistence"
    "piccolod/internal/runtime/commands"
)

// guardMiddleware blocks mutating persistence commands when volumes are locked.
func guardMiddleware(c *crypt.Manager) commands.Middleware {
    return func(ctx context.Context, cmd commands.Command, next commands.Handler) (commands.Response, error) {
        name := cmd.Name()
        if strings.HasPrefix(name, "persistence.") {
            switch name {
            case persistence.CommandRecordLockState:
                // Always allow propagation of lock state
            default:
                if c != nil && c.IsInitialized() && c.IsLocked() {
                    return nil, persistence.ErrLocked
                }
            }
        }
        return next.Handle(ctx, cmd)
    }
}

