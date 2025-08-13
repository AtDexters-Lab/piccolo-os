package mdns

import (
	"crypto/sha256"
	"fmt"
	"log"
	"math"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/miekg/dns"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

// InterfaceState tracks the state of a network interface for mDNS
type InterfaceState struct {
	Interface *net.Interface
	
	// Dual-stack IP support
	IPv4      net.IP
	IPv6      net.IP
	IPv4Conn  *net.UDPConn
	IPv6Conn  *net.UDPConn
	
	Active    bool
	LastSeen  time.Time
	
	// Stack capabilities
	HasIPv4   bool
	HasIPv6   bool
	
	// Security metrics
	QueryCount    uint64
	LastQuery     time.Time
	ErrorCount    uint64
	
	// Resilience tracking
	FailureCount     uint64
	LastFailure      time.Time
	RecoveryAttempts uint64
	BackoffUntil     time.Time
	HealthScore      float64 // 0.0 (unhealthy) to 1.0 (healthy)
}

// RateLimiter tracks query rates per client IP
type RateLimiter struct {
	clients map[string]*ClientState
	mutex   sync.RWMutex
}

// ClientState tracks per-client security metrics
type ClientState struct {
	IP           string
	QueryCount   uint64
	LastQuery    time.Time
	Blocked      bool
	BlockedUntil time.Time
}

// SecurityConfig defines security limits and thresholds
type SecurityConfig struct {
	MaxQueriesPerSecond    int
	MaxQueriesPerMinute    int
	MaxPacketSize          int
	MaxResponseSize        int
	MaxConcurrentQueries   int
	QueryTimeout           time.Duration
	ClientBlockDuration    time.Duration
	CleanupInterval        time.Duration
}

// SecurityMetrics tracks overall security statistics
type SecurityMetrics struct {
	TotalQueries     uint64
	BlockedQueries   uint64
	MalformedPackets uint64
	RateLimitHits    uint64
	LargePackets     uint64
}

// ResilienceConfig defines error recovery and resilience parameters
type ResilienceConfig struct {
	MaxRetries              int
	InitialBackoff          time.Duration
	MaxBackoff              time.Duration
	BackoffMultiplier       float64
	HealthCheckInterval     time.Duration
	RecoveryCheckInterval   time.Duration
	InterfaceRetryThreshold uint64
	MaxFailureRate          float64
	MinHealthScore          float64
	RecoveryTimeout         time.Duration
}

// HealthMonitor tracks system health and triggers recovery
type HealthMonitor struct {
	OverallHealth    float64
	InterfaceHealth  map[string]float64
	LastHealthCheck  time.Time
	RecoveryActive   bool
	SystemErrors     uint64
	RecoveryAttempts uint64
}

// ConflictDetector manages DNS name conflicts and resolution
type ConflictDetector struct {
	ConflictDetected  bool
	ConflictingSources map[string]ConflictingHost
	LastConflictCheck time.Time
	ResolutionAttempts uint64
	CurrentSuffix     string
}

// ConflictingHost represents a host that conflicts with our name
type ConflictingHost struct {
	IP           net.IP
	FirstSeen    time.Time
	LastSeen     time.Time
	QueryCount   uint64
	MachineID    string // Derived from responses if available
}

// QueryProcessor manages concurrent query processing with limits
type QueryProcessor struct {
	semaphore   chan struct{}
	activeCount int64
}

// Manager handles mDNS advertising for the Piccolo service
type Manager struct {
	// Multi-interface support
	interfaces map[string]*InterfaceState
	mutex      sync.RWMutex
	
	// Original fields
	hostname string
	port     int
	stopCh   chan struct{}
	wg       sync.WaitGroup
	
	// Deterministic naming support
	baseName    string
	machineID   string
	finalName   string
	
	// Security components
	rateLimiter     *RateLimiter
	securityConfig  *SecurityConfig
	securityMetrics *SecurityMetrics
	queryProcessor  *QueryProcessor
	
	// Resilience components
	resilienceConfig *ResilienceConfig
	healthMonitor    *HealthMonitor
	
	// Conflict detection and resolution
	conflictDetector *ConflictDetector
}

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
		CleanupInterval:      time.Minute * 10,
	}
	
	// Initialize resilience configuration with recovery defaults
	resilienceConfig := &ResilienceConfig{
		MaxRetries:              3,
		InitialBackoff:          time.Second * 5,
		MaxBackoff:              time.Minute * 5,
		BackoffMultiplier:       2.0,
		HealthCheckInterval:     time.Second * 30,
		RecoveryCheckInterval:   time.Second * 15,
		InterfaceRetryThreshold: 5,
		MaxFailureRate:          0.3,  // 30% failure rate threshold
		MinHealthScore:          0.7,  // Minimum health score to be considered healthy
		RecoveryTimeout:         time.Minute * 2,
	}
	
	return &Manager{
		interfaces: make(map[string]*InterfaceState),
		hostname:   "piccolo",
		port:       8080,
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

// Security Methods

// isRateLimited checks if a client IP is rate limited
func (m *Manager) isRateLimited(clientIP string) bool {
	m.rateLimiter.mutex.Lock()
	defer m.rateLimiter.mutex.Unlock()
	
	now := time.Now()
	client, exists := m.rateLimiter.clients[clientIP]
	
	if !exists {
		// New client
		m.rateLimiter.clients[clientIP] = &ClientState{
			IP:        clientIP,
			LastQuery: now,
		}
		return false
	}
	
	// Check if client is currently blocked
	if client.Blocked && now.Before(client.BlockedUntil) {
		atomic.AddUint64(&m.securityMetrics.BlockedQueries, 1)
		return true
	}
	
	// Reset block status if expired
	if client.Blocked && now.After(client.BlockedUntil) {
		client.Blocked = false
		client.QueryCount = 0
	}
	
	// Check rate limits
	timeSinceLastQuery := now.Sub(client.LastQuery)
	
	// Reset counter if more than a minute has passed
	if timeSinceLastQuery > time.Minute {
		client.QueryCount = 0
	}
	
	// Increment query count
	client.QueryCount++
	client.LastQuery = now
	
	// Check per-second rate limit
	if timeSinceLastQuery < time.Second && client.QueryCount > uint64(m.securityConfig.MaxQueriesPerSecond) {
		m.blockClient(client, now)
		atomic.AddUint64(&m.securityMetrics.RateLimitHits, 1)
		return true
	}
	
	// Check per-minute rate limit
	if client.QueryCount > uint64(m.securityConfig.MaxQueriesPerMinute) {
		m.blockClient(client, now)
		atomic.AddUint64(&m.securityMetrics.RateLimitHits, 1)
		return true
	}
	
	return false
}

// blockClient blocks a client for the configured duration
func (m *Manager) blockClient(client *ClientState, now time.Time) {
	client.Blocked = true
	client.BlockedUntil = now.Add(m.securityConfig.ClientBlockDuration)
	log.Printf("SECURITY: Blocked client %s for %v due to rate limiting", 
		client.IP, m.securityConfig.ClientBlockDuration)
}

// validatePacket performs security validation on incoming packets
func (m *Manager) validatePacket(data []byte, clientAddr *net.UDPAddr) error {
	// Check packet size
	if len(data) > m.securityConfig.MaxPacketSize {
		atomic.AddUint64(&m.securityMetrics.LargePackets, 1)
		return fmt.Errorf("packet too large: %d bytes", len(data))
	}
	
	if len(data) < 12 { // Minimum DNS header size
		atomic.AddUint64(&m.securityMetrics.MalformedPackets, 1)
		return fmt.Errorf("packet too small: %d bytes", len(data))
	}
	
	// Check rate limiting
	if m.isRateLimited(clientAddr.IP.String()) {
		return fmt.Errorf("client rate limited: %s", clientAddr.IP.String())
	}
	
	return nil
}

// validateDNSMessage performs DNS-specific validation
func (m *Manager) validateDNSMessage(msg *dns.Msg) error {
	// Check for DNS query bombs
	if len(msg.Question) > 10 {
		return fmt.Errorf("too many questions: %d", len(msg.Question))
	}
	
	if len(msg.Answer) > 0 {
		return fmt.Errorf("queries should not have answers")
	}
	
	if len(msg.Extra) > 100 {
		return fmt.Errorf("too many extra records: %d", len(msg.Extra))
	}
	
	// Validate question types
	for _, q := range msg.Question {
		if q.Qclass != dns.ClassINET {
			return fmt.Errorf("unsupported query class: %d", q.Qclass)
		}
		
		if q.Qtype != dns.TypeA && q.Qtype != dns.TypeAAAA && q.Qtype != dns.TypeANY {
			return fmt.Errorf("unsupported query type: %d", q.Qtype)
		}
		
		// Validate hostname
		if !strings.HasSuffix(q.Name, ".local.") {
			return fmt.Errorf("non-local query: %s", q.Name)
		}
		
		if len(q.Name) > 253 { // DNS name length limit
			return fmt.Errorf("hostname too long: %d", len(q.Name))
		}
	}
	
	return nil
}

// acquireQuerySlot tries to acquire a processing slot for concurrent queries
func (m *Manager) acquireQuerySlot() bool {
	select {
	case m.queryProcessor.semaphore <- struct{}{}:
		atomic.AddInt64(&m.queryProcessor.activeCount, 1)
		return true
	default:
		// No slot available
		return false
	}
}

// releaseQuerySlot releases a processing slot
func (m *Manager) releaseQuerySlot() {
	<-m.queryProcessor.semaphore
	atomic.AddInt64(&m.queryProcessor.activeCount, -1)
}

// cleanupSecurityState periodically cleans up old client states
func (m *Manager) cleanupSecurityState() {
	ticker := time.NewTicker(m.securityConfig.CleanupInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.performSecurityCleanup()
		}
	}
}

// performSecurityCleanup removes old client states and logs security metrics
func (m *Manager) performSecurityCleanup() {
	m.rateLimiter.mutex.Lock()
	defer m.rateLimiter.mutex.Unlock()
	
	now := time.Now()
	cleanupThreshold := now.Add(-time.Hour) // Remove clients inactive for 1 hour
	
	for ip, client := range m.rateLimiter.clients {
		if client.LastQuery.Before(cleanupThreshold) && !client.Blocked {
			delete(m.rateLimiter.clients, ip)
		}
	}
	
	// Log security metrics
	log.Printf("SECURITY: Metrics - Total: %d, Blocked: %d, Malformed: %d, RateLimit: %d, Large: %d, Active: %d",
		atomic.LoadUint64(&m.securityMetrics.TotalQueries),
		atomic.LoadUint64(&m.securityMetrics.BlockedQueries),
		atomic.LoadUint64(&m.securityMetrics.MalformedPackets),
		atomic.LoadUint64(&m.securityMetrics.RateLimitHits),
		atomic.LoadUint64(&m.securityMetrics.LargePackets),
		atomic.LoadInt64(&m.queryProcessor.activeCount))
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
	
	log.Printf("INFO: Secured dual-stack mDNS server started - advertising %s.local on %d interfaces", 
		m.finalName, interfaceCount)
	log.Printf("INFO: Security limits - %d queries/sec, %d concurrent, %d packet size", 
		m.securityConfig.MaxQueriesPerSecond, m.securityConfig.MaxConcurrentQueries, m.securityConfig.MaxPacketSize)
	
	return nil
}

// discoverInterfaces finds and sets up all suitable network interfaces
func (m *Manager) discoverInterfaces() error {
	interfaces, err := net.Interfaces()
	if err != nil {
		return err
	}
	
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	activeCount := 0
	for _, iface := range interfaces {
		if err := m.setupInterface(&iface); err != nil {
			log.Printf("WARN: Failed to setup interface %s: %v", iface.Name, err)
			// Track interface setup failure for resilience
			if state, exists := m.interfaces[iface.Name]; exists {
				m.markInterfaceFailure(state, err)
			}
			continue
		}
		activeCount++
	}
	
	if activeCount == 0 {
		return fmt.Errorf("no suitable network interfaces found")
	}
	
	log.Printf("INFO: Successfully configured %d network interfaces for mDNS", activeCount)
	return nil
}

// setupInterface configures dual-stack mDNS for a specific network interface
func (m *Manager) setupInterface(iface *net.Interface) error {
	// Skip loopback and down interfaces
	if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
		return fmt.Errorf("interface %s not suitable (loopback or down)", iface.Name)
	}
	
	// Get all addresses for this interface
	addrs, err := iface.Addrs()
	if err != nil {
		return err
	}
	
	var ipv4Addr, ipv6Addr net.IP
	
	// Find IPv4 and IPv6 addresses
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok {
			if ipv4 := ipnet.IP.To4(); ipv4 != nil {
				// IPv4 address - skip link-local
				if !ipnet.IP.IsLinkLocalUnicast() {
					ipv4Addr = ipv4
				}
			} else if ipv6 := ipnet.IP.To16(); ipv6 != nil {
				// IPv6 address - skip link-local and loopback
				if !ipnet.IP.IsLinkLocalUnicast() && !ipnet.IP.IsLoopback() {
					ipv6Addr = ipv6
				}
			}
		}
	}
	
	// Need at least one IP stack
	if ipv4Addr == nil && ipv6Addr == nil {
		return fmt.Errorf("no suitable IP addresses on interface %s", iface.Name)
	}
	
	// Create interface state
	state := &InterfaceState{
		Interface:   iface,
		IPv4:        ipv4Addr,
		IPv6:        ipv6Addr,
		HasIPv4:     ipv4Addr != nil,
		HasIPv6:     ipv6Addr != nil,
		Active:      true,
		LastSeen:    time.Now(),
		HealthScore: 1.0, // Start with perfect health
	}
	
	// Setup IPv4 socket if available
	if state.HasIPv4 {
		ipv4Conn, err := m.createIPv4Socket(iface)
		if err != nil {
			log.Printf("WARN: Failed to create IPv4 socket for %s: %v", iface.Name, err)
		} else {
			state.IPv4Conn = ipv4Conn
		}
	}
	
	// Setup IPv6 socket if available
	if state.HasIPv6 {
		ipv6Conn, err := m.createIPv6Socket(iface)
		if err != nil {
			log.Printf("WARN: Failed to create IPv6 socket for %s: %v", iface.Name, err)
		} else {
			state.IPv6Conn = ipv6Conn
		}
	}
	
	// Need at least one working socket
	if state.IPv4Conn == nil && state.IPv6Conn == nil {
		return fmt.Errorf("failed to create any sockets for interface %s", iface.Name)
	}
	
	// Store in manager
	m.interfaces[iface.Name] = state
	
	// Start responder for this interface
	m.wg.Add(1)
	go m.interfaceResponder(state)
	
	var addrInfo []string
	if state.HasIPv4 {
		addrInfo = append(addrInfo, fmt.Sprintf("IPv4:%s", ipv4Addr.String()))
	}
	if state.HasIPv6 {
		addrInfo = append(addrInfo, fmt.Sprintf("IPv6:%s", ipv6Addr.String()))
	}
	
	log.Printf("INFO: Interface %s ready - %s", iface.Name, strings.Join(addrInfo, ", "))
	return nil
}

// createIPv4Socket creates an IPv4 UDP socket bound to a specific interface
func (m *Manager) createIPv4Socket(iface *net.Interface) (*net.UDPConn, error) {
	// Create raw socket with SO_REUSEPORT
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, syscall.IPPROTO_UDP)
	if err != nil {
		return nil, fmt.Errorf("failed to create IPv4 socket: %w", err)
	}
	
	// Set socket options
	if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1); err != nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("failed to set SO_REUSEADDR: %w", err)
	}
	
	// SO_REUSEPORT for port sharing
	const SO_REUSEPORT = 15
	if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, SO_REUSEPORT, 1); err != nil {
		log.Printf("WARN: Failed to set SO_REUSEPORT on IPv4 %s: %v", iface.Name, err)
	}
	
	// Bind to specific interface using SO_BINDTODEVICE
	if err := syscall.SetsockoptString(fd, syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, iface.Name); err != nil {
		log.Printf("WARN: Failed to bind IPv4 to device %s: %v", iface.Name, err)
	}
	
	// Bind to mDNS port
	addr := &syscall.SockaddrInet4{Port: 5353}
	copy(addr.Addr[:], net.IPv4zero.To4())
	
	if err := syscall.Bind(fd, addr); err != nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("failed to bind IPv4 to :5353: %w", err)
	}
	
	// Convert to net.UDPConn
	file := os.NewFile(uintptr(fd), fmt.Sprintf("mdns4-%s", iface.Name))
	if file == nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("failed to create file from IPv4 socket")
	}
	defer file.Close()
	
	fileConn, err := net.FileConn(file)
	if err != nil {
		return nil, fmt.Errorf("failed to create IPv4 connection: %w", err)
	}
	
	conn, ok := fileConn.(*net.UDPConn)
	if !ok {
		fileConn.Close()
		return nil, fmt.Errorf("failed to convert to IPv4 UDPConn")
	}
	
	// Join IPv4 multicast group on this interface
	pc := ipv4.NewPacketConn(conn)
	group := &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251)}
	if err := pc.JoinGroup(iface, group); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to join IPv4 multicast group on %s: %w", iface.Name, err)
	}
	
	// Set multicast interface
	if err := pc.SetMulticastInterface(iface); err != nil {
		log.Printf("WARN: Failed to set IPv4 multicast interface %s: %v", iface.Name, err)
	}
	
	return conn, nil
}

// createIPv6Socket creates an IPv6 UDP socket bound to a specific interface
func (m *Manager) createIPv6Socket(iface *net.Interface) (*net.UDPConn, error) {
	// Create raw socket with SO_REUSEPORT
	fd, err := syscall.Socket(syscall.AF_INET6, syscall.SOCK_DGRAM, syscall.IPPROTO_UDP)
	if err != nil {
		return nil, fmt.Errorf("failed to create IPv6 socket: %w", err)
	}
	
	// Set socket options
	if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1); err != nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("failed to set SO_REUSEADDR on IPv6: %w", err)
	}
	
	// SO_REUSEPORT for port sharing
	const SO_REUSEPORT = 15
	if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, SO_REUSEPORT, 1); err != nil {
		log.Printf("WARN: Failed to set SO_REUSEPORT on IPv6 %s: %v", iface.Name, err)
	}
	
	// Disable IPv6 only to allow dual-stack
	if err := syscall.SetsockoptInt(fd, syscall.IPPROTO_IPV6, syscall.IPV6_V6ONLY, 0); err != nil {
		log.Printf("WARN: Failed to disable IPv6-only mode on %s: %v", iface.Name, err)
	}
	
	// Bind to specific interface using SO_BINDTODEVICE
	if err := syscall.SetsockoptString(fd, syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, iface.Name); err != nil {
		log.Printf("WARN: Failed to bind IPv6 to device %s: %v", iface.Name, err)
	}
	
	// Bind to mDNS port on IPv6
	addr := &syscall.SockaddrInet6{Port: 5353}
	copy(addr.Addr[:], net.IPv6zero.To16())
	
	if err := syscall.Bind(fd, addr); err != nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("failed to bind IPv6 to :5353: %w", err)
	}
	
	// Convert to net.UDPConn
	file := os.NewFile(uintptr(fd), fmt.Sprintf("mdns6-%s", iface.Name))
	if file == nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("failed to create file from IPv6 socket")
	}
	defer file.Close()
	
	fileConn, err := net.FileConn(file)
	if err != nil {
		return nil, fmt.Errorf("failed to create IPv6 connection: %w", err)
	}
	
	conn, ok := fileConn.(*net.UDPConn)
	if !ok {
		fileConn.Close()
		return nil, fmt.Errorf("failed to convert to IPv6 UDPConn")
	}
	
	// Join IPv6 multicast group on this interface
	pc := ipv6.NewPacketConn(conn)
	group := &net.UDPAddr{IP: net.ParseIP("ff02::fb")}
	if err := pc.JoinGroup(iface, group); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to join IPv6 multicast group on %s: %w", iface.Name, err)
	}
	
	// Set multicast interface
	if err := pc.SetMulticastInterface(iface); err != nil {
		log.Printf("WARN: Failed to set IPv6 multicast interface %s: %v", iface.Name, err)
	}
	
	return conn, nil
}

// networkMonitor continuously monitors network interface changes
func (m *Manager) networkMonitor() {
	defer m.wg.Done()
	
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.checkInterfaceChanges()
		}
	}
}

// checkInterfaceChanges detects and handles interface changes
func (m *Manager) checkInterfaceChanges() {
	interfaces, err := net.Interfaces()
	if err != nil {
		log.Printf("WARN: Failed to check interfaces: %v", err)
		return
	}
	
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	// Mark all current interfaces as not seen
	for _, state := range m.interfaces {
		state.Active = false
	}
	
	// Check each interface
	for _, iface := range interfaces {
		if existing, exists := m.interfaces[iface.Name]; exists {
			// Interface still exists - check if IP changed
			if m.hasIPChanged(&iface, existing) {
				log.Printf("INFO: IP changed on interface %s, reconfiguring", iface.Name)
				if existing.IPv4Conn != nil {
					existing.IPv4Conn.Close()
				}
				if existing.IPv6Conn != nil {
					existing.IPv6Conn.Close()
				}
				delete(m.interfaces, iface.Name)
				m.setupInterface(&iface)
			} else {
				existing.Active = true
				existing.LastSeen = time.Now()
			}
		} else {
			// New interface detected
			log.Printf("INFO: New interface detected: %s", iface.Name)
			m.setupInterface(&iface)
		}
	}
	
	// Remove interfaces that no longer exist
	for name, state := range m.interfaces {
		if !state.Active {
			log.Printf("INFO: Interface %s no longer available, removing", name)
			if state.IPv4Conn != nil {
				state.IPv4Conn.Close()
			}
			if state.IPv6Conn != nil {
				state.IPv6Conn.Close()
			}
			delete(m.interfaces, name)
		}
	}
}

// hasIPChanged checks if an interface's IPv4 or IPv6 addresses have changed
func (m *Manager) hasIPChanged(iface *net.Interface, state *InterfaceState) bool {
	addrs, err := iface.Addrs()
	if err != nil {
		return true // Assume changed if we can't check
	}
	
	var foundIPv4, foundIPv6 bool
	
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok {
			if ipv4 := ipnet.IP.To4(); ipv4 != nil {
				// IPv4 address found
				if !ipnet.IP.IsLinkLocalUnicast() && state.IPv4 != nil && ipnet.IP.Equal(state.IPv4) {
					foundIPv4 = true
				}
			} else if ipv6 := ipnet.IP.To16(); ipv6 != nil {
				// IPv6 address found
				if !ipnet.IP.IsLinkLocalUnicast() && !ipnet.IP.IsLoopback() && 
				   state.IPv6 != nil && ipnet.IP.Equal(state.IPv6) {
					foundIPv6 = true
				}
			}
		}
	}
	
	// Check if expected IPs are still present
	ipv4Changed := state.HasIPv4 && !foundIPv4
	ipv6Changed := state.HasIPv6 && !foundIPv6
	
	return ipv4Changed || ipv6Changed
}

// interfaceResponder handles dual-stack mDNS queries on a specific interface
func (m *Manager) interfaceResponder(state *InterfaceState) {
	defer m.wg.Done()
	
	interfaceName := state.Interface.Name
	
	// Start IPv4 responder if available
	if state.IPv4Conn != nil {
		m.wg.Add(1)
		go m.ipv4Responder(state, interfaceName)
	}
	
	// Start IPv6 responder if available
	if state.IPv6Conn != nil {
		m.wg.Add(1)
		go m.ipv6Responder(state, interfaceName)
	}
}

// ipv4Responder handles IPv4 mDNS queries
func (m *Manager) ipv4Responder(state *InterfaceState, interfaceName string) {
	defer m.wg.Done()
	
	buffer := make([]byte, 1500)
	
	for {
		select {
		case <-m.stopCh:
			return
		default:
			if state.IPv4Conn == nil {
				return
			}
			
			state.IPv4Conn.SetReadDeadline(time.Now().Add(1 * time.Second))
			
			n, clientAddr, err := state.IPv4Conn.ReadFromUDP(buffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				if !strings.Contains(err.Error(), "use of closed network connection") {
					log.Printf("WARN: IPv4 mDNS read error on %s: %v", interfaceName, err)
					m.markInterfaceFailure(state, err)
				}
				continue
			}
			
			go m.handleDualStackQuery(buffer[:n], clientAddr, state, "IPv4")
		}
	}
}

// ipv6Responder handles IPv6 mDNS queries
func (m *Manager) ipv6Responder(state *InterfaceState, interfaceName string) {
	defer m.wg.Done()
	
	buffer := make([]byte, 1500)
	
	for {
		select {
		case <-m.stopCh:
			return
		default:
			if state.IPv6Conn == nil {
				return
			}
			
			state.IPv6Conn.SetReadDeadline(time.Now().Add(1 * time.Second))
			
			n, clientAddr, err := state.IPv6Conn.ReadFromUDP(buffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				if !strings.Contains(err.Error(), "use of closed network connection") {
					log.Printf("WARN: IPv6 mDNS read error on %s: %v", interfaceName, err)
					m.markInterfaceFailure(state, err)
				}
				continue
			}
			
			go m.handleDualStackQuery(buffer[:n], clientAddr, state, "IPv6")
		}
	}
}

// handleDualStackQuery processes mDNS queries with dual-stack support and security
func (m *Manager) handleDualStackQuery(data []byte, clientAddr *net.UDPAddr, state *InterfaceState, stack string) {
	// Try to acquire a processing slot
	if !m.acquireQuerySlot() {
		// Too many concurrent queries, drop this one
		return
	}
	defer m.releaseQuerySlot()
	
	// Set query timeout
	startTime := time.Now()
	defer func() {
		if time.Since(startTime) > m.securityConfig.QueryTimeout {
			log.Printf("SECURITY: Query timeout from %s", clientAddr.IP)
		}
	}()
	
	// Update interface metrics
	atomic.AddUint64(&state.QueryCount, 1)
	state.LastQuery = time.Now()
	
	// Increment global query counter
	atomic.AddUint64(&m.securityMetrics.TotalQueries, 1)
	
	// Validate packet security
	if err := m.validatePacket(data, clientAddr); err != nil {
		atomic.AddUint64(&state.ErrorCount, 1)
		log.Printf("SECURITY: [%s] Rejected packet from %s: %v", state.Interface.Name, clientAddr.IP, err)
		return
	}
	
	// Parse DNS message with error handling
	var msg dns.Msg
	if err := msg.Unpack(data); err != nil {
		atomic.AddUint64(&m.securityMetrics.MalformedPackets, 1)
		atomic.AddUint64(&state.ErrorCount, 1)
		log.Printf("SECURITY: [%s] Malformed packet from %s: %v", state.Interface.Name, clientAddr.IP, err)
		return
	}
	
	// Additional DNS message validation
	if err := m.validateDNSMessage(&msg); err != nil {
		atomic.AddUint64(&state.ErrorCount, 1)
		log.Printf("SECURITY: [%s] Invalid DNS message from %s: %v", state.Interface.Name, clientAddr.IP, err)
		return
	}
	
	// Handle responses for conflict detection
	if msg.Response {
		m.handleConflictDetection(&msg, clientAddr)
		return
	}
	
	// Only handle queries
	if msg.Opcode != dns.OpcodeQuery {
		return
	}
	
	// Build response
	response := &dns.Msg{}
	response.SetReply(&msg)
	response.Authoritative = true
	response.RecursionAvailable = false
	
	// Process each question with dual-stack support
	for _, q := range msg.Question {
		if q.Qclass == dns.ClassINET && strings.EqualFold(q.Name, m.finalName+".local.") {
			// Handle A record requests (IPv4)
			if (q.Qtype == dns.TypeA || q.Qtype == dns.TypeANY) && state.HasIPv4 && state.IPv4 != nil {
				rr := &dns.A{
					Hdr: dns.RR_Header{
						Name:   q.Name,
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    120,
					},
					A: state.IPv4,
				}
				response.Answer = append(response.Answer, rr)
				log.Printf("DEBUG: [%s-%s] Adding A record: %s -> %s", 
					state.Interface.Name, stack, m.finalName, state.IPv4.String())
			}
			
			// Handle AAAA record requests (IPv6)
			if (q.Qtype == dns.TypeAAAA || q.Qtype == dns.TypeANY) && state.HasIPv6 && state.IPv6 != nil {
				rr := &dns.AAAA{
					Hdr: dns.RR_Header{
						Name:   q.Name,
						Rrtype: dns.TypeAAAA,
						Class:  dns.ClassINET,
						Ttl:    120,
					},
					AAAA: state.IPv6,
				}
				response.Answer = append(response.Answer, rr)
				log.Printf("DEBUG: [%s-%s] Adding AAAA record: %s -> %s", 
					state.Interface.Name, stack, m.finalName, state.IPv6.String())
			}
		}
	}
	
	// Send response if we have answers
	if len(response.Answer) > 0 {
		if responseData, err := response.Pack(); err == nil {
			// Check response size limit
			if len(responseData) > m.securityConfig.MaxResponseSize {
				log.Printf("SECURITY: [%s] Response too large for %s: %d bytes", 
					state.Interface.Name, clientAddr.IP, len(responseData))
				return
			}
			
			// Choose the appropriate connection based on stack
			var conn *net.UDPConn
			if stack == "IPv4" {
				conn = state.IPv4Conn
			} else {
				conn = state.IPv6Conn
			}
			
			if conn != nil {
				if _, err := conn.WriteToUDP(responseData, clientAddr); err != nil {
					atomic.AddUint64(&state.ErrorCount, 1)
					log.Printf("WARN: [%s-%s] Failed to send response to %s: %v", 
						state.Interface.Name, stack, clientAddr.IP, err)
				} else {
					log.Printf("DEBUG: [%s-%s] Responded to query from %s for %s.local", 
						state.Interface.Name, stack, clientAddr.IP, m.finalName)
				}
			}
		}
	}
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

// announcer sends periodic mDNS announcements on all interfaces
func (m *Manager) announcer() {
	defer m.wg.Done()
	
	// Send initial announcements
	announcements := []time.Duration{0, 1 * time.Second, 2 * time.Second}
	for _, delay := range announcements {
		select {
		case <-m.stopCh:
			return
		case <-time.After(delay):
			m.sendMultiInterfaceAnnouncements()
		}
	}
	
	// Periodic announcements
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.sendMultiInterfaceAnnouncements()
		}
	}
}

// sendMultiInterfaceAnnouncements sends dual-stack mDNS announcements on all active interfaces
func (m *Manager) sendMultiInterfaceAnnouncements() {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	for name, state := range m.interfaces {
		if !state.Active {
			continue
		}
		
		// Send IPv4 announcements
		if state.HasIPv4 && state.IPv4Conn != nil && state.IPv4 != nil {
			m.sendIPv4Announcement(name, state)
		}
		
		// Send IPv6 announcements
		if state.HasIPv6 && state.IPv6Conn != nil && state.IPv6 != nil {
			m.sendIPv6Announcement(name, state)
		}
	}
}

// sendIPv4Announcement sends IPv4 mDNS announcement
func (m *Manager) sendIPv4Announcement(name string, state *InterfaceState) {
	msg := &dns.Msg{}
	msg.Response = true
	msg.Authoritative = true
	msg.Opcode = dns.OpcodeQuery
	
	rr := &dns.A{
		Hdr: dns.RR_Header{
			Name:   m.finalName + ".local.",
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    120,
		},
		A: state.IPv4,
	}
	msg.Answer = append(msg.Answer, rr)
	
	if data, err := msg.Pack(); err == nil {
		multicastAddr := &net.UDPAddr{
			IP:   net.IPv4(224, 0, 0, 251),
			Port: 5353,
		}
		
		if _, err := state.IPv4Conn.WriteToUDP(data, multicastAddr); err == nil {
			log.Printf("DEBUG: [%s-IPv4] Announced %s.local -> %s", 
				name, m.finalName, state.IPv4.String())
		} else {
			log.Printf("WARN: Failed to send IPv4 announcement on %s: %v", name, err)
			m.markInterfaceFailure(state, err)
		}
	}
}

// sendIPv6Announcement sends IPv6 mDNS announcement
func (m *Manager) sendIPv6Announcement(name string, state *InterfaceState) {
	msg := &dns.Msg{}
	msg.Response = true
	msg.Authoritative = true
	msg.Opcode = dns.OpcodeQuery
	
	rr := &dns.AAAA{
		Hdr: dns.RR_Header{
			Name:   m.finalName + ".local.",
			Rrtype: dns.TypeAAAA,
			Class:  dns.ClassINET,
			Ttl:    120,
		},
		AAAA: state.IPv6,
	}
	msg.Answer = append(msg.Answer, rr)
	
	if data, err := msg.Pack(); err == nil {
		multicastAddr := &net.UDPAddr{
			IP:   net.ParseIP("ff02::fb"),
			Port: 5353,
		}
		
		if _, err := state.IPv6Conn.WriteToUDP(data, multicastAddr); err == nil {
			log.Printf("DEBUG: [%s-IPv6] Announced %s.local -> %s", 
				name, m.finalName, state.IPv6.String())
		} else {
			log.Printf("WARN: Failed to send IPv6 announcement on %s: %v", name, err)
			m.markInterfaceFailure(state, err)
		}
	}
}

// Resilience Methods

// updateInterfaceHealth calculates and updates interface health score
func (m *Manager) updateInterfaceHealth(state *InterfaceState) {
	now := time.Now()
	
	// Calculate failure rate over recent period
	totalQueries := atomic.LoadUint64(&state.QueryCount)
	errorCount := atomic.LoadUint64(&state.ErrorCount)
	failureCount := atomic.LoadUint64(&state.FailureCount)
	
	// Base health calculation
	healthScore := 1.0
	
	// Factor in error rate
	if totalQueries > 0 {
		errorRate := float64(errorCount) / float64(totalQueries)
		healthScore -= errorRate * 0.5 // Errors reduce health by up to 50%
	}
	
	// Factor in failure history with time decay
	if failureCount > 0 {
		timeSinceFailure := now.Sub(state.LastFailure)
		failureImpact := float64(failureCount) * 0.1 // Each failure reduces health by 10%
		
		// Decay failure impact over time (recover over 10 minutes)
		decayFactor := timeSinceFailure.Seconds() / (10 * 60)
		if decayFactor > 1.0 {
			decayFactor = 1.0
		}
		failureImpact *= (1.0 - decayFactor)
		
		healthScore -= failureImpact
	}
	
	// Ensure health score is between 0.0 and 1.0
	if healthScore < 0.0 {
		healthScore = 0.0
	}
	if healthScore > 1.0 {
		healthScore = 1.0
	}
	
	// Update interface health
	state.HealthScore = healthScore
	m.healthMonitor.InterfaceHealth[state.Interface.Name] = healthScore
	
	log.Printf("DEBUG: Interface %s health updated to %.2f (errors: %d/%d, failures: %d)", 
		state.Interface.Name, healthScore, errorCount, totalQueries, failureCount)
}

// calculateBackoffDuration determines how long to wait before retrying a failed interface
func (m *Manager) calculateBackoffDuration(state *InterfaceState) time.Duration {
	attempts := atomic.LoadUint64(&state.RecoveryAttempts)
	
	// Exponential backoff: initial * (multiplier ^ attempts)
	backoff := float64(m.resilienceConfig.InitialBackoff) * 
		math.Pow(m.resilienceConfig.BackoffMultiplier, float64(attempts))
	
	// Cap at maximum backoff
	if backoff > float64(m.resilienceConfig.MaxBackoff) {
		backoff = float64(m.resilienceConfig.MaxBackoff)
	}
	
	return time.Duration(backoff)
}

// isInterfaceInBackoff checks if an interface is currently in backoff period
func (m *Manager) isInterfaceInBackoff(state *InterfaceState) bool {
	return time.Now().Before(state.BackoffUntil)
}

// markInterfaceFailure records a failure and updates resilience tracking
func (m *Manager) markInterfaceFailure(state *InterfaceState, err error) {
	now := time.Now()
	
	atomic.AddUint64(&state.FailureCount, 1)
	state.LastFailure = now
	
	// Calculate and set backoff period
	backoff := m.calculateBackoffDuration(state)
	state.BackoffUntil = now.Add(backoff)
	
	log.Printf("RESILIENCE: Interface %s failed (attempt %d), backing off for %v: %v",
		state.Interface.Name, atomic.LoadUint64(&state.FailureCount), backoff, err)
	
	// Update health score
	m.updateInterfaceHealth(state)
	
	// Update system error counter
	atomic.AddUint64(&m.healthMonitor.SystemErrors, 1)
}

// attemptInterfaceRecovery tries to recover a failed interface
func (m *Manager) attemptInterfaceRecovery(name string, state *InterfaceState) bool {
	// Check if still in backoff period
	if m.isInterfaceInBackoff(state) {
		return false
	}
	
	atomic.AddUint64(&state.RecoveryAttempts, 1)
	atomic.AddUint64(&m.healthMonitor.RecoveryAttempts, 1)
	
	log.Printf("RESILIENCE: Attempting recovery of interface %s (attempt %d)",
		name, atomic.LoadUint64(&state.RecoveryAttempts))
	
	// Close old connections if they exist
	if state.IPv4Conn != nil {
		state.IPv4Conn.Close()
		state.IPv4Conn = nil
	}
	if state.IPv6Conn != nil {
		state.IPv6Conn.Close()
		state.IPv6Conn = nil
	}
	
	// Try to recreate the interface connection
	if err := m.setupInterface(state.Interface); err != nil {
		m.markInterfaceFailure(state, fmt.Errorf("recovery failed: %w", err))
		return false
	}
	
	// Reset failure tracking on successful recovery
	atomic.StoreUint64(&state.FailureCount, 0)
	atomic.StoreUint64(&state.RecoveryAttempts, 0)
	state.BackoffUntil = time.Time{} // Clear backoff
	
	log.Printf("RESILIENCE: Successfully recovered interface %s", name)
	m.updateInterfaceHealth(state)
	
	return true
}

// performHealthCheck runs comprehensive health monitoring
func (m *Manager) performHealthCheck() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	now := time.Now()
	m.healthMonitor.LastHealthCheck = now
	
	totalInterfaces := len(m.interfaces)
	healthyInterfaces := 0
	totalHealth := 0.0
	
	// Check each interface
	for name, state := range m.interfaces {
		m.updateInterfaceHealth(state)
		
		if state.HealthScore >= m.resilienceConfig.MinHealthScore {
			healthyInterfaces++
		}
		totalHealth += state.HealthScore
		
		// Attempt recovery for unhealthy interfaces
		if !state.Active || state.HealthScore < m.resilienceConfig.MinHealthScore {
			if atomic.LoadUint64(&state.FailureCount) < m.resilienceConfig.InterfaceRetryThreshold {
				m.attemptInterfaceRecovery(name, state)
			} else {
				log.Printf("RESILIENCE: Interface %s exceeded retry threshold, giving up", name)
			}
		}
	}
	
	// Calculate overall system health
	if totalInterfaces > 0 {
		m.healthMonitor.OverallHealth = totalHealth / float64(totalInterfaces)
	} else {
		m.healthMonitor.OverallHealth = 0.0
	}
	
	// Log health summary
	log.Printf("RESILIENCE: Health check - Overall: %.2f, Healthy: %d/%d, System errors: %d",
		m.healthMonitor.OverallHealth, healthyInterfaces, totalInterfaces,
		atomic.LoadUint64(&m.healthMonitor.SystemErrors))
	
	// Trigger recovery mode if health is critically low
	if m.healthMonitor.OverallHealth < 0.3 && !m.healthMonitor.RecoveryActive {
		m.enterRecoveryMode()
	} else if m.healthMonitor.OverallHealth > 0.8 && m.healthMonitor.RecoveryActive {
		m.exitRecoveryMode()
	}
}

// enterRecoveryMode activates aggressive recovery measures
func (m *Manager) enterRecoveryMode() {
	m.healthMonitor.RecoveryActive = true
	log.Printf("RESILIENCE: ENTERING RECOVERY MODE - System health critically low (%.2f)",
		m.healthMonitor.OverallHealth)
	
	// Trigger immediate interface rediscovery
	go func() {
		if err := m.discoverInterfaces(); err != nil {
			log.Printf("RESILIENCE: Emergency interface discovery failed: %v", err)
		}
	}()
}

// exitRecoveryMode deactivates recovery mode when health improves
func (m *Manager) exitRecoveryMode() {
	m.healthMonitor.RecoveryActive = false
	log.Printf("RESILIENCE: EXITING RECOVERY MODE - System health restored (%.2f)",
		m.healthMonitor.OverallHealth)
}

// healthMonitorLoop runs periodic health checks and recovery operations
func (m *Manager) healthMonitorLoop() {
	defer m.wg.Done()
	
	ticker := time.NewTicker(m.resilienceConfig.HealthCheckInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.performHealthCheck()
		}
	}
}

// Conflict Detection Methods

// detectNameConflicts probes the network for existing instances of our hostname
func (m *Manager) detectNameConflicts() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	conflictFound := false
	
	// Send probes on all active interfaces
	for _, state := range m.interfaces {
		if !state.Active {
			continue
		}
		
		// Probe both IPv4 and IPv6 if available
		if state.HasIPv4 && state.IPv4Conn != nil {
			if m.sendConflictProbe(state, "IPv4", m.finalName+".local.") {
				conflictFound = true
			}
		}
		
		if state.HasIPv6 && state.IPv6Conn != nil {
			if m.sendConflictProbe(state, "IPv6", m.finalName+".local.") {
				conflictFound = true
			}
		}
	}
	
	return conflictFound
}

// sendConflictProbe sends a DNS query to detect name conflicts
func (m *Manager) sendConflictProbe(state *InterfaceState, stack, hostname string) bool {
	// Create probe query
	msg := &dns.Msg{}
	msg.SetQuestion(hostname, dns.TypeANY)
	msg.RecursionDesired = false
	
	data, err := msg.Pack()
	if err != nil {
		log.Printf("CONFLICT: Failed to pack probe query: %v", err)
		return false
	}
	
	var multicastAddr *net.UDPAddr
	var conn *net.UDPConn
	
	if stack == "IPv4" {
		multicastAddr = &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}
		conn = state.IPv4Conn
	} else {
		multicastAddr = &net.UDPAddr{IP: net.ParseIP("ff02::fb"), Port: 5353}
		conn = state.IPv6Conn
	}
	
	if conn == nil {
		return false
	}
	
	// Send probe query
	if _, err := conn.WriteToUDP(data, multicastAddr); err != nil {
		log.Printf("CONFLICT: Failed to send probe on %s-%s: %v", state.Interface.Name, stack, err)
		return false
	}
	
	// Wait briefly for responses (this is a simplified approach)
	// In production, this should be handled asynchronously
	time.Sleep(250 * time.Millisecond)
	
	log.Printf("DEBUG: [%s-%s] Sent conflict probe for %s", state.Interface.Name, stack, hostname)
	return false // TODO: Implement response detection
}

// handleConflictDetection processes responses that might indicate conflicts
func (m *Manager) handleConflictDetection(msg *dns.Msg, clientAddr *net.UDPAddr) {
	// Check if this is a response to our hostname query
	for _, answer := range msg.Answer {
		if strings.EqualFold(answer.Header().Name, m.finalName+".local.") {
			// Found a conflict - someone else is responding to our name
			hostKey := clientAddr.IP.String()
			
			conflict, exists := m.conflictDetector.ConflictingSources[hostKey]
			if !exists {
				conflict = ConflictingHost{
					IP:        clientAddr.IP,
					FirstSeen: time.Now(),
					QueryCount: 0,
				}
				log.Printf("CONFLICT: New conflicting host detected: %s for %s.local", 
					clientAddr.IP, m.finalName)
			}
			
			conflict.LastSeen = time.Now()
			conflict.QueryCount++
			m.conflictDetector.ConflictingSources[hostKey] = conflict
			
			if !m.conflictDetector.ConflictDetected {
				m.conflictDetector.ConflictDetected = true
				log.Printf("CONFLICT: Name conflict detected for %s.local!", m.finalName)
				go m.resolveNameConflict()
			}
		}
	}
}

// resolveNameConflict handles name conflicts using deterministic resolution
func (m *Manager) resolveNameConflict() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	atomic.AddUint64(&m.conflictDetector.ResolutionAttempts, 1)
	
	// Use our deterministic machine ID as suffix
	newName := fmt.Sprintf("%s-%s", m.baseName, m.machineID)
	
	// Check if we've already applied this suffix
	if m.finalName == newName {
		log.Printf("CONFLICT: Already using deterministic name %s, conflict resolution complete", newName)
		return
	}
	
	// Update to deterministic name
	oldName := m.finalName
	m.finalName = newName
	m.conflictDetector.CurrentSuffix = m.machineID
	
	log.Printf("CONFLICT: Resolved conflict - renamed from %s.local to %s.local", oldName, m.finalName)
	
	// Send immediate announcements with new name
	go func() {
		// Send multiple announcements to establish the new name quickly
		for i := 0; i < 3; i++ {
			m.sendMultiInterfaceAnnouncements()
			time.Sleep(time.Second)
		}
	}()
	
	// Clear conflict state after resolution
	m.conflictDetector.ConflictDetected = false
	m.conflictDetector.ConflictingSources = make(map[string]ConflictingHost)
}

// probeNameAvailability performs initial conflict detection during startup
func (m *Manager) probeNameAvailability() error {
	log.Printf("CONFLICT: Probing name availability for %s.local", m.finalName)
	
	// Send probes and wait for responses
	if m.detectNameConflicts() {
		log.Printf("CONFLICT: Name conflict detected during startup")
		m.resolveNameConflict()
	}
	
	// Always wait a bit for any late responses
	time.Sleep(time.Second)
	
	if m.conflictDetector.ConflictDetected {
		log.Printf("CONFLICT: Using resolved name: %s.local", m.finalName)
	} else {
		log.Printf("CONFLICT: No conflicts detected, using: %s.local", m.finalName)
	}
	
	return nil
}

// conflictMonitor periodically checks for name conflicts
func (m *Manager) conflictMonitor() {
	defer m.wg.Done()
	
	ticker := time.NewTicker(5 * time.Minute) // Check every 5 minutes
	defer ticker.Stop()
	
	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			if m.detectNameConflicts() {
				log.Printf("CONFLICT: Periodic conflict check detected issues")
			}
			m.conflictDetector.LastConflictCheck = time.Now()
		}
	}
}

// getPrimaryLocalIP returns the primary non-loopback local IP of the host
func getPrimaryLocalIP() (net.IP, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP, nil
}