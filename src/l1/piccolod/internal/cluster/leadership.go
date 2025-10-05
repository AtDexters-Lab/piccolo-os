package cluster

import "sync"

// Role represents the leadership state of a logical resource on this node.
type Role string

const (
	RoleUnknown  Role = "unknown"
	RoleLeader   Role = "leader"
	RoleFollower Role = "follower"
)

const (
	ResourceControlPlane = "control-plane"
	ResourceKernel       = ResourceControlPlane
	ResourceAppPrefix    = "app:"
)

func ResourceForApp(name string) string {
	return ResourceAppPrefix + name
}

// Registry tracks the current leadership role per resource ID.
type Registry struct {
	mu    sync.RWMutex
	roles map[string]Role
}

// NewRegistry constructs an empty leadership registry.
func NewRegistry() *Registry {
	return &Registry{roles: make(map[string]Role)}
}

// Set records the role for the provided resource identifier.
func (r *Registry) Set(id string, role Role) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.roles[id] = role
}

// Current returns the role currently registered for the identifier.
func (r *Registry) Current(id string) Role {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if role, ok := r.roles[id]; ok {
		return role
	}
	return RoleUnknown
}

// Snapshot copies all known roles for diagnostics or metrics.
func (r *Registry) Snapshot() map[string]Role {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]Role, len(r.roles))
	for k, v := range r.roles {
		out[k] = v
	}
	return out
}
