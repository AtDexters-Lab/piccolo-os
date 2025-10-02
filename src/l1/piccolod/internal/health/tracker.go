package health

import (
	"fmt"
	"sync"
	"time"
)

type Level int

const (
	LevelOK Level = iota
	LevelWarn
	LevelError
)

func (l Level) String() string {
	switch l {
	case LevelOK:
		return "ok"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	default:
		return fmt.Sprintf("unknown(%d)", int(l))
	}
}

type Status struct {
	Level     Level                  `json:"level"`
	Message   string                 `json:"message"`
	Details   map[string]interface{} `json:"details,omitempty"`
	UpdatedAt time.Time              `json:"updated_at"`
}

func NewStatus(level Level, message string) Status {
	return Status{Level: level, Message: message, UpdatedAt: time.Now().UTC()}
}

// Tracker maintains a thread-safe collection of component health statuses.
type Tracker struct {
	mu       sync.RWMutex
	statuses map[string]Status
}

func NewTracker() *Tracker {
	return &Tracker{statuses: make(map[string]Status)}
}

func (t *Tracker) Set(name string, status Status) {
	if status.UpdatedAt.IsZero() {
		status.UpdatedAt = time.Now().UTC()
	}
	t.mu.Lock()
	t.statuses[name] = status
	t.mu.Unlock()
}

func (t *Tracker) Setf(name string, level Level, msg string) {
	t.Set(name, Status{Level: level, Message: msg, UpdatedAt: time.Now().UTC()})
}

func (t *Tracker) Status(name string) (Status, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	s, ok := t.statuses[name]
	return s, ok
}

func (t *Tracker) Snapshot() map[string]Status {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make(map[string]Status, len(t.statuses))
	for k, v := range t.statuses {
		out[k] = v
	}
	return out
}

func (t *Tracker) Overall() Level {
	t.mu.RLock()
	defer t.mu.RUnlock()
	worst := LevelOK
	for _, st := range t.statuses {
		if st.Level > worst {
			worst = st.Level
		}
	}
	return worst
}

func (t *Tracker) Ready(required ...string) (bool, map[string]Status) {
	snapshot := t.Snapshot()
	if len(required) == 0 {
		return true, snapshot
	}
	ok := true
	for _, name := range required {
		st, exists := snapshot[name]
		if !exists || st.Level > LevelOK {
			ok = false
		}
	}
	return ok, snapshot
}
