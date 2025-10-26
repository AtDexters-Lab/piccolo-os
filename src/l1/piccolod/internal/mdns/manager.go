package mdns

import (
	"crypto/sha256"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"
)

// NewManager creates a new mDNS manager
func NewManager() *Manager {
	machineID := getMachineID()

	// Initialize security configuration with safe defaults
	securityConfig := &SecurityConfig{
		MaxQueriesPerSecond:  10,   // Max 10 queries per second per client
		MaxQueriesPerMinute:  100,  // Max 100 queries per minute per client
		MaxPacketSize:        1500, // Standard MTU limit
		MaxResponseSize:      512,  // DNS standard response limit
		MaxConcurrentQueries: 50,   // Max concurrent query processing
		QueryTimeout:         time.Second * 2,
		ClientBlockDuration:  time.Minute * 5,
		CleanupInterval:      time.Minute * 5,
	}

	// Initialize resilience configuration with recovery defaults
	resilienceConfig := &ResilienceConfig{
		MaxRetries:            3,
		InitialBackoff:        time.Second * 5,
		MaxBackoff:            time.Minute * 5,
		BackoffMultiplier:     2.0,
		HealthCheckInterval:   time.Second * 30,
		RecoveryCheckInterval: time.Second * 15,
		MaxFailureRate:        0.3, // 30% failure rate threshold
		MinHealthScore:        0.7, // Minimum health score to be considered healthy
		RecoveryTimeout:       time.Minute * 2,
	}

	manager := &Manager{
		interfaces: make(map[string]*InterfaceState),
		hostname:   "piccolo",
		port:       80,
		stopCh:     make(chan struct{}),
		baseName:   "piccolo",
		machineID:  machineID,
		finalName:  "piccolo", // Will be updated if conflicts detected

		// Security components
		rateLimiter: &RateLimiter{
			clients: make(map[string]*ClientState),
		},
		securityConfig:  securityConfig,
		securityMetrics: &SecurityMetrics{},
		queryProcessor: &QueryProcessor{
			semaphore: make(chan struct{}, securityConfig.MaxConcurrentQueries),
		},

		// Resilience components
		resilienceConfig: resilienceConfig,
		healthMonitor: &HealthMonitor{
			OverallHealth:   1.0,
			InterfaceHealth: make(map[string]float64),
			LastHealthCheck: time.Now(),
		},

		// Conflict detection
		conflictDetector: &ConflictDetector{
			ConflictingSources: make(map[string]ConflictingHost),
			LastConflictCheck:  time.Now(),
		},
	}

	manager.ipv4SocketFactory = manager.createIPv4Socket
	manager.ipv6SocketFactory = manager.createIPv6Socket

	return manager
}

// Start begins advertising the service via mDNS
func (m *Manager) Start() error {
	log.Printf("INFO: Starting multi-interface mDNS manager (machine ID: %s)", m.machineID)

	// Discover and setup all network interfaces
	if err := m.discoverInterfaces(); err != nil {
		return fmt.Errorf("failed to discover network interfaces: %w", err)
	}

	// Start network monitor for interface changes
	m.wg.Add(1)
	go m.networkMonitor()

	// Start announcement routine
	m.wg.Add(1)
	go m.announcer()

	// Start security cleanup routine
	m.wg.Add(1)
	go m.cleanupSecurityState()

	// Start health monitoring routine
	m.wg.Add(1)
	go m.healthMonitorLoop()

	// Perform initial conflict detection
	if err := m.probeNameAvailability(); err != nil {
		return fmt.Errorf("conflict detection failed: %w", err)
	}

	// Start conflict monitoring routine
	m.wg.Add(1)
	go m.conflictMonitor()

	m.mutex.RLock()
	interfaceCount := len(m.interfaces)
	m.mutex.RUnlock()

	serviceName := m.currentServiceName()

	log.Printf("INFO: Secured dual-stack mDNS server started - advertising %s.local on %d interfaces",
		serviceName, interfaceCount)
	log.Printf("INFO: Security limits - %d queries/sec, %d concurrent, %d packet size",
		m.securityConfig.MaxQueriesPerSecond, m.securityConfig.MaxConcurrentQueries, m.securityConfig.MaxPacketSize)

	return nil
}

// Stop shuts down the mDNS server
func (m *Manager) Stop() error {
	close(m.stopCh)

	// Close all interface connections
	m.mutex.Lock()
	for name, state := range m.interfaces {
		if state.IPv4Conn != nil {
			state.IPv4Conn.Close()
			log.Printf("INFO: Closed IPv4 connection for interface %s", name)
		}
		if state.IPv6Conn != nil {
			state.IPv6Conn.Close()
			log.Printf("INFO: Closed IPv6 connection for interface %s", name)
		}
	}
	m.mutex.Unlock()

	// Wait for all goroutines to finish
	m.wg.Wait()

	log.Printf("INFO: Multi-interface mDNS manager stopped")
	return nil
}

// getMachineID generates a deterministic machine identifier
func getMachineID() string {
	// Try multiple sources for machine ID
	sources := []func() string{
		getMachineIDFromFile,
		getMachineIDFromMAC,
		getMachineIDFromHostname,
	}

	for _, source := range sources {
		if id := source(); id != "" {
			// Generate a short, deterministic suffix from the full ID
			hash := sha256.Sum256([]byte(id))
			return fmt.Sprintf("%x", hash[:3]) // 6 character hex
		}
	}

	// Fallback to timestamp-based (not ideal but deterministic per boot)
	return fmt.Sprintf("%06d", time.Now().Unix()%1000000)
}

// getMachineIDFromFile tries to read system machine ID
func getMachineIDFromFile() string {
	paths := []string{
		"/etc/machine-id",
		"/var/lib/dbus/machine-id",
		"/etc/hostid",
	}

	for _, path := range paths {
		if data, err := os.ReadFile(path); err == nil {
			return strings.TrimSpace(string(data))
		}
	}
	return ""
}

// getMachineIDFromMAC generates ID from MAC addresses
func getMachineIDFromMAC() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	var macs []string
	for _, iface := range interfaces {
		if iface.Flags&net.FlagLoopback == 0 && len(iface.HardwareAddr) > 0 {
			macs = append(macs, iface.HardwareAddr.String())
		}
	}

	if len(macs) > 0 {
		// Use first non-loopback MAC as base
		return strings.ReplaceAll(macs[0], ":", "")
	}
	return ""
}

// getMachineIDFromHostname uses hostname as fallback
func getMachineIDFromHostname() string {
	if hostname, err := os.Hostname(); err == nil {
		return hostname
	}
	return ""
}

// currentServiceName returns the currently advertised service name.
func (m *Manager) currentServiceName() string {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.finalName
}
