package mdns

import (
	"net"
	"sync"
	"time"
)

// InterfaceState tracks the state of a network interface for mDNS
type InterfaceState struct {
	Interface *net.Interface

	// Dual-stack IP support
	IPv4     net.IP
	IPv6     net.IP
	IPv4Conn *net.UDPConn
	IPv6Conn *net.UDPConn

	Active   bool
	LastSeen time.Time

	// Stack capabilities
	HasIPv4 bool
	HasIPv6 bool

	// Security metrics
	QueryCount uint64
	LastQuery  time.Time
	ErrorCount uint64

	// Resilience tracking
	FailureCount     uint64
	LastFailure      time.Time
	RecoveryAttempts uint64
	BackoffUntil     time.Time
	HealthScore      float64 // 0.0 (unhealthy) to 1.0 (healthy)
	resilienceMu     sync.RWMutex
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
	MaxQueriesPerSecond  int
	MaxQueriesPerMinute  int
	MaxPacketSize        int
	MaxResponseSize      int
	MaxConcurrentQueries int
	QueryTimeout         time.Duration
	ClientBlockDuration  time.Duration
	CleanupInterval      time.Duration
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
	MaxRetries            int
	InitialBackoff        time.Duration
	MaxBackoff            time.Duration
	BackoffMultiplier     float64
	HealthCheckInterval   time.Duration
	RecoveryCheckInterval time.Duration
	MaxFailureRate        float64
	MinHealthScore        float64
	RecoveryTimeout       time.Duration
}

// HealthMonitor tracks system health and triggers recovery
type HealthMonitor struct {
	OverallHealth    float64
	InterfaceHealth  map[string]float64
	LastHealthCheck  time.Time
	RecoveryActive   bool
	SystemErrors     uint64
	RecoveryAttempts uint64
	mutex            sync.RWMutex
}

// ConflictDetector manages DNS name conflicts and resolution
type ConflictDetector struct {
	ConflictDetected   bool
	ConflictingSources map[string]ConflictingHost
	LastConflictCheck  time.Time
	ResolutionAttempts uint64
	CurrentSuffix      string
	mutex              sync.RWMutex
}

// ConflictingHost represents a host that conflicts with our name
type ConflictingHost struct {
	IP         net.IP
	FirstSeen  time.Time
	LastSeen   time.Time
	QueryCount uint64
	MachineID  string // Derived from responses if available
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
	baseName  string
	machineID string
	finalName string

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

	// Socket factories (overrideable for tests)
	ipv4SocketFactory func(*net.Interface) (*net.UDPConn, error)
	ipv6SocketFactory func(*net.Interface) (*net.UDPConn, error)
}
