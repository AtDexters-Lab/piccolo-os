package mdns

import (
	"net"
	"time"
)

// MockInterface creates a mock network interface for testing
func createMockInterface(name, mac string, flags net.Flags) *net.Interface {
	hwAddr, _ := net.ParseMAC(mac)
	return &net.Interface{
		Index:        1,
		MTU:          1500,
		Name:         name,
		HardwareAddr: hwAddr,
		Flags:        flags,
	}
}

// MockInterfaceState creates a mock InterfaceState for testing
func createMockInterfaceState(name string, hasIPv4, hasIPv6 bool) *InterfaceState {
	iface := createMockInterface(name, "00:11:22:33:44:55", net.FlagUp|net.FlagMulticast)
	state := &InterfaceState{
		Interface:   iface,
		Active:      true,
		LastSeen:    time.Now(),
		HasIPv4:     hasIPv4,
		HasIPv6:     hasIPv6,
		HealthScore: 1.0,
	}

	if hasIPv4 {
		state.IPv4 = net.ParseIP("192.168.1.100")
	}
	if hasIPv6 {
		state.IPv6 = net.ParseIP("fe80::1234:5678:9abc:def0")
	}

	return state
}

// MockManager creates a minimal Manager for testing
func createMockManager() *Manager {
	return NewManager()
}

// MockClientState creates a test client state
func createMockClientState(ip string, queryCount uint64, blocked bool) *ClientState {
	return &ClientState{
		IP:         ip,
		QueryCount: queryCount,
		LastQuery:  time.Now(),
		Blocked:    blocked,
	}
}

// MockSecurityConfig creates test security configuration
func createMockSecurityConfig() *SecurityConfig {
	return &SecurityConfig{
		MaxQueriesPerSecond:  5,
		MaxQueriesPerMinute:  50,
		MaxPacketSize:        1024,
		MaxResponseSize:      512,
		MaxConcurrentQueries: 10,
		QueryTimeout:         time.Second,
		ClientBlockDuration:  time.Minute,
		CleanupInterval:      time.Minute * 5,
	}
}

// MockResilienceConfig creates test resilience configuration
func createMockResilienceConfig() *ResilienceConfig {
	return &ResilienceConfig{
		MaxRetries:            2,
		InitialBackoff:        time.Second,
		MaxBackoff:            time.Second * 30,
		BackoffMultiplier:     2.0,
		HealthCheckInterval:   time.Second * 10,
		RecoveryCheckInterval: time.Second * 5,
		MaxFailureRate:        0.5,
		MinHealthScore:        0.6,
		RecoveryTimeout:       time.Second * 30,
	}
}

// assertTimestamp checks if a timestamp is within reasonable bounds
func assertTimestamp(t time.Time, tolerance time.Duration) bool {
	now := time.Now()
	return t.After(now.Add(-tolerance)) && t.Before(now.Add(tolerance))
}
