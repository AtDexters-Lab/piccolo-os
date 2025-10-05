package router

import (
	"log"
	"sync"
	"time"
)

// Mode describes how traffic should be handled for a resource.
type Mode string

const (
	ModeUnknown Mode = "unknown"
	ModeLocal   Mode = "local"
	ModeTunnel  Mode = "tunnel"
)

// Route captures the current routing decision for a resource.
type Route struct {
	Mode       Mode
	LeaderAddr string
	UpdatedAt  time.Time
}

// Manager keeps track of kernel/app routing state and emits debug logs for now.
type Manager struct {
	mu          sync.RWMutex
	kernelRoute Route
	appRoutes   map[string]Route
}

// NewManager constructs a routing manager with default-local routes.
func NewManager() *Manager {
	return &Manager{
		kernelRoute: Route{Mode: ModeLocal, UpdatedAt: time.Now().UTC()},
		appRoutes:   make(map[string]Route),
	}
}

// RegisterKernelRoute updates the kernel routing mode (local or tunnel).
func (m *Manager) RegisterKernelRoute(mode Mode, leaderAddr string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.kernelRoute = Route{Mode: normalizeMode(mode), LeaderAddr: leaderAddr, UpdatedAt: time.Now().UTC()}
	log.Printf("INFO: router kernel route mode=%s leader=%s", m.kernelRoute.Mode, leaderAddr)
}

// RegisterAppRoute updates the routing mode for an app.
func (m *Manager) RegisterAppRoute(app string, mode Mode, leaderAddr string) {
	if app == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.appRoutes == nil {
		m.appRoutes = make(map[string]Route)
	}
	route := Route{Mode: normalizeMode(mode), LeaderAddr: leaderAddr, UpdatedAt: time.Now().UTC()}
	m.appRoutes[app] = route
	log.Printf("INFO: router app=%s mode=%s leader=%s", app, route.Mode, leaderAddr)
}

// KernelRoute returns the latest kernel routing decision.
func (m *Manager) KernelRoute() Route {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.kernelRoute
}

// AppRoute returns the routing decision for the given app (default Local).
func (m *Manager) AppRoute(app string) Route {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if route, ok := m.appRoutes[app]; ok {
		return route
	}
	return Route{Mode: ModeLocal, UpdatedAt: time.Now().UTC()}
}

// DecideAppRoute resolves the effective route for an app and logs it for now.
func (m *Manager) DecideAppRoute(app string) Route {
	route := m.AppRoute(app)
	log.Printf("INFO: router deciding app=%s mode=%s leader=%s", app, route.Mode, route.LeaderAddr)
	return route
}

func normalizeMode(mode Mode) Mode {
	switch mode {
	case ModeLocal, ModeTunnel:
		return mode
	default:
		return ModeLocal
	}
}

// Registrar is the subset of manager behaviour used by other modules.
type Registrar interface {
	RegisterKernelRoute(mode Mode, leaderAddr string)
	RegisterAppRoute(app string, mode Mode, leaderAddr string)
}
