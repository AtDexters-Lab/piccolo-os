package events

import (
	"sync"
	"time"

	"piccolod/internal/cluster"
)

// Topic enumerates bus channels shared across piccolod subsystems.
type Topic string

const (
	TopicLockStateChanged      Topic = "lock_state_changed"
	TopicLeadershipRoleChanged Topic = "leadership_role_changed"
	TopicDeviceEvent           Topic = "device_event"
	TopicExportResult          Topic = "export_result"
	TopicControlHealth         Topic = "control_health"
	TopicControlStoreCommit    Topic = "control_store_commit"
	TopicRemoteConfigChanged   Topic = "remote_config_changed"
	TopicVolumeStateChanged    Topic = "volume_state_changed"
	TopicAudit                 Topic = "audit"
)

// Event represents a message broadcast on the event bus.
type Event struct {
	Topic   Topic
	Payload any
}

type VolumeStateChanged struct {
	ID          string
	Desired     string
	Observed    string
	Role        string
	Generation  int
	NeedsRepair bool
	LastError   string
}

// AuditEvent captures operator-visible security events.
type AuditEvent struct {
	Kind     string
	Time     time.Time
	Source   string
	Metadata map[string]any
}

// LeadershipChanged describes a leadership role update for a resource.
type LeadershipChanged struct {
	Resource string
	Role     cluster.Role
}
type LockStateChanged struct {
	Locked bool
}

// ControlStoreCommit announces that the control store has advanced to a new revision.
type ControlStoreCommit struct {
	Revision uint64
	Checksum string
	Role     cluster.Role
}

// Bus is a simple pub/sub dispatcher for intra-process events.
type Bus struct {
	mu     sync.RWMutex
	subs   map[Topic][]chan Event
	closed bool
}

// NewBus constructs an empty event bus.
func NewBus() *Bus {
	return &Bus{subs: make(map[Topic][]chan Event)}
}

// Subscribe registers a buffered channel for a topic.
func (b *Bus) Subscribe(topic Topic, buffer int) <-chan Event {
	ch := make(chan Event, buffer)

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		close(ch)
		return ch
	}
	b.subs[topic] = append(b.subs[topic], ch)
	return ch
}

// Publish broadcasts an event to all subscribers.
func (b *Bus) Publish(evt Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.closed {
		return
	}
	for _, ch := range b.subs[evt.Topic] {
		select {
		case ch <- evt:
		default:
			// Drop when subscriber is saturated; listeners should size buffers appropriately.
		}
	}
}

// Close shuts down the bus and all subscriber channels.
func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for _, chans := range b.subs {
		for _, ch := range chans {
			close(ch)
		}
	}
	b.subs = nil
}
