package consensus

import (
	"context"
	"log"

	"piccolod/internal/cluster"
	"piccolod/internal/events"
)

// Stub implements a single-node consensus manager that always assumes leader role.
type Stub struct {
	registry *cluster.Registry
	bus      *events.Bus
	resource string
}

// NewStub constructs a stub consensus manager for the given resource.
func NewStub(registry *cluster.Registry, bus *events.Bus) *Stub {
	return &Stub{registry: registry, bus: bus, resource: cluster.ResourceKernel}
}

// Start marks this node as leader and broadcasts an event.
func (s *Stub) Start(ctx context.Context) error {
	if s.registry != nil {
		s.registry.Set(s.resource, cluster.RoleLeader)
	}
	if s.bus != nil {
		s.bus.Publish(events.Event{
			Topic: events.TopicLeadershipRoleChanged,
			Payload: events.LeadershipChanged{
				Resource: s.resource,
				Role:     cluster.RoleLeader,
			},
		})
	}
	log.Printf("INFO: consensus stub set resource %s role=%s", s.resource, cluster.RoleLeader)
	return nil
}

// Stop performs no action for the stub implementation.
func (s *Stub) Stop(ctx context.Context) error {
	return nil
}

// SetRole allows tests or orchestration to publish a leadership change for any resource.
func (s *Stub) SetRole(resource string, role cluster.Role) {
	if s.registry != nil {
		s.registry.Set(resource, role)
	}
	if s.bus != nil {
		s.bus.Publish(events.Event{Topic: events.TopicLeadershipRoleChanged, Payload: events.LeadershipChanged{Resource: resource, Role: role}})
	}
}
