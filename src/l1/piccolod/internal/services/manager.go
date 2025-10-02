package services

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"piccolod/internal/api"
	"piccolod/internal/events"
)

// ServiceManager coordinates listener allocation, registry, and proxy startup
type ServiceManager struct {
	allocator    *PortAllocator
	registry     map[string]map[string]ServiceEndpoint // app -> name -> endpoint
	proxyManager *ProxyManager
	mu           sync.RWMutex
	stopCh       chan struct{}
	wg           sync.WaitGroup
	containerIDs map[string]string // app -> containerID (optional)
	eventsMu     sync.Mutex
	eventCancel  context.CancelFunc
}

func NewServiceManager() *ServiceManager {
	allocator := NewPortAllocator(
		PortRange{Start: 15000, End: 25000},
		PortRange{Start: 35000, End: 45000},
	)
	return &ServiceManager{
		allocator:    allocator,
		registry:     make(map[string]map[string]ServiceEndpoint),
		proxyManager: NewProxyManager(),
		stopCh:       make(chan struct{}),
		containerIDs: make(map[string]string),
	}
}

func defaultRemotePorts(listener api.AppListener) []int {
	if len(listener.RemotePorts) == 0 {
		return []int{80, 443}
	}
	return append([]int(nil), listener.RemotePorts...)
}

// ObserveRuntimeEvents subscribes to leadership and lock-state events for logging.
func (m *ServiceManager) ObserveRuntimeEvents(bus *events.Bus) {
	if bus == nil {
		return
	}
	m.eventsMu.Lock()
	if m.eventCancel != nil {
		m.eventCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.eventCancel = cancel
	m.eventsMu.Unlock()

	leaders := bus.Subscribe(events.TopicLeadershipRoleChanged, 16)
	locks := bus.Subscribe(events.TopicLockStateChanged, 8)

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		for {
			select {
			case evt, ok := <-leaders:
				if !ok {
					leaders = nil
					if leaders == nil && locks == nil {
						return
					}
					continue
				}
				payload, ok := evt.Payload.(events.LeadershipChanged)
				if !ok {
					log.Printf("WARN: service-manager received unexpected leadership payload: %#v", evt.Payload)
					continue
				}
				log.Printf("INFO: service-manager observed leadership change resource=%s role=%s", payload.Resource, payload.Role)
			case evt, ok := <-locks:
				if !ok {
					locks = nil
					if leaders == nil && locks == nil {
						return
					}
					continue
				}
				payload, ok := evt.Payload.(events.LockStateChanged)
				if !ok {
					log.Printf("WARN: service-manager received unexpected lock payload: %#v", evt.Payload)
					continue
				}
				state := "unlocked"
				if payload.Locked {
					state = "locked"
				}
				log.Printf("INFO: service-manager observed control lock state=%s", state)
			case <-ctx.Done():
				return
			case <-m.stopCh:
				return
			}
		}
	}()
}

func (m *ServiceManager) stopEventObservers() {
	m.eventsMu.Lock()
	if m.eventCancel != nil {
		m.eventCancel()
		m.eventCancel = nil
	}
	m.eventsMu.Unlock()
}

// RestoreFromPodman rebuilds proxies for an app using existing host-bind ports.
func (m *ServiceManager) RestoreFromPodman(appName string, listeners []api.AppListener, hostByGuest map[int]int) ([]ServiceEndpoint, error) {
	// Stop any existing proxies first
	m.RemoveApp(appName)

	m.mu.Lock()
	defer m.mu.Unlock()

	endpoints := make([]ServiceEndpoint, 0, len(listeners))
	if len(listeners) == 0 {
		return endpoints, nil
	}

	registry := make(map[string]ServiceEndpoint)
	for _, l := range listeners {
		host, ok := hostByGuest[l.GuestPort]
		if !ok {
			continue
		}
		if err := m.allocator.ReserveHost(host); err != nil {
			continue
		}
		public, err := m.allocator.AllocatePublic()
		if err != nil {
			m.allocator.freeHost(host)
			return endpoints, err
		}
		ep := ServiceEndpoint{
			App:         appName,
			Name:        l.Name,
			GuestPort:   l.GuestPort,
			HostBind:    host,
			PublicPort:  public,
			Flow:        l.Flow,
			Protocol:    l.Protocol,
			Middleware:  l.Middleware,
			RemotePorts: defaultRemotePorts(l),
		}
		registry[l.Name] = ep
		endpoints = append(endpoints, ep)
		m.proxyManager.StartListener(ep)
	}

	if len(registry) > 0 {
		m.registry[appName] = registry
	}
	return endpoints, nil
}

// AllocateForApp allocates ports for all listeners of an app and starts proxies
func (m *ServiceManager) AllocateForApp(appName string, listeners []api.AppListener) ([]ServiceEndpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	endpoints := make([]ServiceEndpoint, 0, len(listeners))

	for _, l := range listeners {
		hb, pp, err := m.allocator.AllocatePair()
		if err != nil {
			return nil, err
		}
		ep := ServiceEndpoint{
			App:         appName,
			Name:        l.Name,
			GuestPort:   l.GuestPort,
			HostBind:    hb,
			PublicPort:  pp,
			Flow:        l.Flow,
			Protocol:    l.Protocol,
			Middleware:  l.Middleware,
			RemotePorts: defaultRemotePorts(l),
		}
		endpoints = append(endpoints, ep)
		if _, ok := m.registry[appName]; !ok {
			m.registry[appName] = make(map[string]ServiceEndpoint)
		}
		m.registry[appName][l.Name] = ep
	}

	// Start proxies after registration
	for _, ep := range endpoints {
		m.proxyManager.StartListener(ep)
	}

	return endpoints, nil
}

// GetAll returns all service endpoints across all apps
func (m *ServiceManager) GetAll() []ServiceEndpoint {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []ServiceEndpoint
	for _, mapp := range m.registry {
		for _, ep := range mapp {
			out = append(out, ep)
		}
	}
	return out
}

// GetByApp returns endpoints for a single app
func (m *ServiceManager) GetByApp(appName string) ([]ServiceEndpoint, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	mapp, ok := m.registry[appName]
	if !ok {
		return nil, fmt.Errorf("app not found: %s", appName)
	}
	var out []ServiceEndpoint
	for _, ep := range mapp {
		out = append(out, ep)
	}
	return out, nil
}

// GetAppListener returns a specific listener endpoint
func (m *ServiceManager) GetAppListener(appName, listener string) (ServiceEndpoint, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	mapp, ok := m.registry[appName]
	if !ok {
		return ServiceEndpoint{}, false
	}
	ep, ok := mapp[listener]
	return ep, ok
}

// StopAll stops all proxy listeners
func (m *ServiceManager) StopAll() {
	m.proxyManager.StopAll()
}

// StartBackground starts health checks for backends (connectivity to hostBind)
func (m *ServiceManager) StartBackground() {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-m.stopCh:
				return
			case <-ticker.C:
				m.checkBackends()
			}
		}
	}()
}

// Stop stops background tasks and proxies
func (m *ServiceManager) Stop() {
	m.stopEventObservers()
	close(m.stopCh)
	m.wg.Wait()
	m.StopAll()
}

func (m *ServiceManager) checkBackends() {
	// Snapshot under lock
	m.mu.RLock()
	snap := make(map[string]map[string]ServiceEndpoint)
	for app, mapp := range m.registry {
		mm := make(map[string]ServiceEndpoint)
		for name, ep := range mapp {
			mm[name] = ep
		}
		snap[app] = mm
	}
	ids := make(map[string]string)
	for app, id := range m.containerIDs {
		ids[app] = id
	}
	m.mu.RUnlock()

	// TCP connectivity check per endpoint
	for _, mapp := range snap {
		for _, ep := range mapp {
			addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(ep.HostBind))
			conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
			if err != nil {
				log.Printf("WARN: Backend not reachable for %s/%s at %s: %v", ep.App, ep.Name, addr, err)
				continue
			}
			_ = conn.Close()
		}
	}

	// Podman publish mapping check per app (best-effort)
	for app, id := range ids {
		if id == "" {
			continue
		}
		if err := verifyPodmanPorts(id, snap[app]); err != nil {
			log.Printf("WARN: Podman port mapping mismatch for app %s (cid=%s): %v", app, id, err)
		}
	}
}

// verifyPodmanPorts compares published ports via `podman port` with registry endpoints
func verifyPodmanPorts(containerID string, eps map[string]ServiceEndpoint) error {
	if len(eps) == 0 {
		return nil
	}
	cmd := exec.Command("podman", "port", containerID)
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("podman port failed: %w", err)
	}

	published := make(map[int]int) // hostBind -> guest
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Expected: "80/tcp -> 127.0.0.1:15001"
		parts := strings.Split(line, "->")
		if len(parts) != 2 {
			continue
		}
		left := strings.TrimSpace(parts[0])
		right := strings.TrimSpace(parts[1])
		// left like "80/tcp"
		guestStr := strings.Split(left, "/")[0]
		gp, _ := strconv.Atoi(strings.TrimSpace(guestStr))
		hostStr := strings.Split(right, ":")
		if len(hostStr) < 2 {
			continue
		}
		hb, _ := strconv.Atoi(strings.TrimSpace(hostStr[len(hostStr)-1]))
		if hb > 0 && gp > 0 {
			published[hb] = gp
		}
	}
	// Compare
	for _, ep := range eps {
		if gp, ok := published[ep.HostBind]; !ok || gp != ep.GuestPort {
			return fmt.Errorf("expected mapping 127.0.0.1:%d:%d missing or mismatched (have %d)", ep.HostBind, ep.GuestPort, published[ep.HostBind])
		}
	}
	return nil
}

// SetAppContainerID records the container ID for an app (used by watcher reconciliation)
func (m *ServiceManager) SetAppContainerID(appName, containerID string) {
	m.mu.Lock()
	m.containerIDs[appName] = containerID
	m.mu.Unlock()
}

// GetAppContainerID returns the container ID for an app if known
func (m *ServiceManager) GetAppContainerID(appName string) (string, bool) {
	m.mu.RLock()
	id, ok := m.containerIDs[appName]
	m.mu.RUnlock()
	return id, ok
}

// ReconcileResult contains details of changes detected
type ReconcileResult struct {
	Endpoints        []ServiceEndpoint
	Added            []ServiceEndpoint
	Removed          []ServiceEndpoint
	GuestPortChanged []struct{ Old, New ServiceEndpoint }
	ProxyOnlyChanged []ServiceEndpoint
}

// Reconcile synchronizes listeners for an app in-place. Returns final endpoints and whether container changes are required.
func (m *ServiceManager) Reconcile(appName string, listeners []api.AppListener) (ReconcileResult, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing := m.registry[appName]
	if existing == nil {
		existing = make(map[string]ServiceEndpoint)
	}

	newMap := make(map[string]ServiceEndpoint)
	containerChange := false
	result := ReconcileResult{}

	// Index new by name
	for _, l := range listeners {
		if old, ok := existing[l.Name]; ok {
			// Reuse ports; update config
			ep := old
			// Detect guest port change
			if ep.GuestPort != l.GuestPort {
				containerChange = true
				result.GuestPortChanged = append(result.GuestPortChanged, struct{ Old, New ServiceEndpoint }{Old: ep, New: ServiceEndpoint{App: appName, Name: l.Name, GuestPort: l.GuestPort, HostBind: ep.HostBind, PublicPort: ep.PublicPort, Flow: l.Flow, Protocol: l.Protocol, Middleware: l.Middleware, RemotePorts: defaultRemotePorts(l)}})
			}
			ep.GuestPort = l.GuestPort
			// Only restart proxy if proxy-related fields changed
			proxyChanged := ep.Flow != l.Flow || ep.Protocol != l.Protocol || !middlewareEqual(ep.Middleware, l.Middleware)
			ep.Flow = l.Flow
			ep.Protocol = l.Protocol
			ep.Middleware = l.Middleware
			ep.RemotePorts = defaultRemotePorts(l)
			newMap[l.Name] = ep
			if proxyChanged {
				m.proxyManager.StopPort(ep.PublicPort)
				m.proxyManager.StartListener(ep)
				result.ProxyOnlyChanged = append(result.ProxyOnlyChanged, ep)
			}
		} else {
			// New listener: allocate ports, start proxy, mark container change
			hb, pp, err := m.allocator.AllocatePair()
			if err != nil {
				return ReconcileResult{}, false, err
			}
			ep := ServiceEndpoint{
				App:         appName,
				Name:        l.Name,
				GuestPort:   l.GuestPort,
				HostBind:    hb,
				PublicPort:  pp,
				Flow:        l.Flow,
				Protocol:    l.Protocol,
				Middleware:  l.Middleware,
				RemotePorts: defaultRemotePorts(l),
			}
			newMap[l.Name] = ep
			m.proxyManager.StartListener(ep)
			containerChange = true
			result.Added = append(result.Added, ep)
		}
	}

	// Removed listeners
	for name, ep := range existing {
		if _, ok := newMap[name]; !ok {
			m.proxyManager.StopPort(ep.PublicPort)
			containerChange = true
			result.Removed = append(result.Removed, ep)
		}
	}

	// Save
	m.registry[appName] = newMap

	// Return endpoints slice
	var eps []ServiceEndpoint
	for _, ep := range newMap {
		eps = append(eps, ep)
	}
	result.Endpoints = eps
	return result, containerChange, nil
}

func middlewareEqual(a, b []api.AppProtocolMiddleware) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name {
			return false
		}
		// Params equality elided for v1
	}
	return true
}

// RemoveApp stops and removes all listeners for an app
func (m *ServiceManager) RemoveApp(appName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if mapp, ok := m.registry[appName]; ok {
		for _, ep := range mapp {
			m.proxyManager.StopPort(ep.PublicPort)
			m.allocator.Release(ep.HostBind, ep.PublicPort)
		}
		delete(m.registry, appName)
	}
	delete(m.containerIDs, appName)
}
