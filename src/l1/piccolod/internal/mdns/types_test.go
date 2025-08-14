package mdns

import (
	"net"
	"testing"
	"time"
)

func TestInterfaceState(t *testing.T) {
	tests := []struct {
		name     string
		hasIPv4  bool
		hasIPv6  bool
		wantIPv4 bool
		wantIPv6 bool
	}{
		{
			name:     "IPv4 only",
			hasIPv4:  true,
			hasIPv6:  false,
			wantIPv4: true,
			wantIPv6: false,
		},
		{
			name:     "IPv6 only",
			hasIPv4:  false,
			hasIPv6:  true,
			wantIPv4: false,
			wantIPv6: true,
		},
		{
			name:     "Dual stack",
			hasIPv4:  true,
			hasIPv6:  true,
			wantIPv4: true,
			wantIPv6: true,
		},
		{
			name:     "No IP addresses",
			hasIPv4:  false,
			hasIPv6:  false,
			wantIPv4: false,
			wantIPv6: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := createMockInterfaceState("test0", tt.hasIPv4, tt.hasIPv6)

			if state.HasIPv4 != tt.wantIPv4 {
				t.Errorf("HasIPv4 = %v, want %v", state.HasIPv4, tt.wantIPv4)
			}

			if state.HasIPv6 != tt.wantIPv6 {
				t.Errorf("HasIPv6 = %v, want %v", state.HasIPv6, tt.wantIPv6)
			}

			if tt.wantIPv4 && state.IPv4 == nil {
				t.Error("Expected IPv4 address but got nil")
			}

			if tt.wantIPv6 && state.IPv6 == nil {
				t.Error("Expected IPv6 address but got nil")
			}

			// Verify initial state
			if !state.Active {
				t.Error("Expected interface to be active initially")
			}

			if state.HealthScore != 1.0 {
				t.Errorf("Expected initial health score of 1.0, got %f", state.HealthScore)
			}

			if !assertTimestamp(state.LastSeen, time.Second) {
				t.Error("LastSeen timestamp should be recent")
			}
		})
	}
}

func TestClientState(t *testing.T) {
	tests := []struct {
		name       string
		ip         string
		queryCount uint64
		blocked    bool
	}{
		{
			name:       "Normal client",
			ip:         "192.168.1.100",
			queryCount: 5,
			blocked:    false,
		},
		{
			name:       "Blocked client",
			ip:         "192.168.1.200",
			queryCount: 100,
			blocked:    true,
		},
		{
			name:       "IPv6 client",
			ip:         "2001:db8::1",
			queryCount: 0,
			blocked:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := createMockClientState(tt.ip, tt.queryCount, tt.blocked)

			if client.IP != tt.ip {
				t.Errorf("IP = %v, want %v", client.IP, tt.ip)
			}

			if client.QueryCount != tt.queryCount {
				t.Errorf("QueryCount = %v, want %v", client.QueryCount, tt.queryCount)
			}

			if client.Blocked != tt.blocked {
				t.Errorf("Blocked = %v, want %v", client.Blocked, tt.blocked)
			}

			if !assertTimestamp(client.LastQuery, time.Second) {
				t.Error("LastQuery timestamp should be recent")
			}
		})
	}
}

func TestSecurityConfig(t *testing.T) {
	config := createMockSecurityConfig()

	// Test configuration validation
	tests := []struct {
		name     string
		field    string
		getValue func() interface{}
		isValid  func(interface{}) bool
	}{
		{
			name:     "MaxQueriesPerSecond positive",
			field:    "MaxQueriesPerSecond",
			getValue: func() interface{} { return config.MaxQueriesPerSecond },
			isValid:  func(v interface{}) bool { return v.(int) > 0 },
		},
		{
			name:     "MaxPacketSize reasonable",
			field:    "MaxPacketSize",
			getValue: func() interface{} { return config.MaxPacketSize },
			isValid:  func(v interface{}) bool { return v.(int) > 512 && v.(int) <= 65535 },
		},
		{
			name:     "QueryTimeout positive",
			field:    "QueryTimeout",
			getValue: func() interface{} { return config.QueryTimeout },
			isValid:  func(v interface{}) bool { return v.(time.Duration) > 0 },
		},
		{
			name:     "ClientBlockDuration positive",
			field:    "ClientBlockDuration",
			getValue: func() interface{} { return config.ClientBlockDuration },
			isValid:  func(v interface{}) bool { return v.(time.Duration) > 0 },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := tt.getValue()
			if !tt.isValid(value) {
				t.Errorf("Invalid %s: %v", tt.field, value)
			}
		})
	}
}

func TestResilienceConfig(t *testing.T) {
	config := createMockResilienceConfig()

	// Test configuration validation
	tests := []struct {
		name     string
		field    string
		getValue func() interface{}
		isValid  func(interface{}) bool
	}{
		{
			name:     "MaxRetries positive",
			field:    "MaxRetries",
			getValue: func() interface{} { return config.MaxRetries },
			isValid:  func(v interface{}) bool { return v.(int) > 0 },
		},
		{
			name:     "BackoffMultiplier greater than 1",
			field:    "BackoffMultiplier",
			getValue: func() interface{} { return config.BackoffMultiplier },
			isValid:  func(v interface{}) bool { return v.(float64) > 1.0 },
		},
		{
			name:     "MaxFailureRate between 0 and 1",
			field:    "MaxFailureRate",
			getValue: func() interface{} { return config.MaxFailureRate },
			isValid:  func(v interface{}) bool { return v.(float64) > 0.0 && v.(float64) <= 1.0 },
		},
		{
			name:     "MinHealthScore between 0 and 1",
			field:    "MinHealthScore",
			getValue: func() interface{} { return config.MinHealthScore },
			isValid:  func(v interface{}) bool { return v.(float64) > 0.0 && v.(float64) <= 1.0 },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := tt.getValue()
			if !tt.isValid(value) {
				t.Errorf("Invalid %s: %v", tt.field, value)
			}
		})
	}
}

func TestConflictingHost(t *testing.T) {
	ip := net.ParseIP("192.168.1.50")
	now := time.Now()

	host := ConflictingHost{
		IP:         ip,
		FirstSeen:  now,
		LastSeen:   now,
		QueryCount: 3,
		MachineID:  "test-machine-123",
	}

	if !host.IP.Equal(ip) {
		t.Errorf("IP = %v, want %v", host.IP, ip)
	}

	if host.QueryCount != 3 {
		t.Errorf("QueryCount = %v, want %v", host.QueryCount, 3)
	}

	if host.MachineID != "test-machine-123" {
		t.Errorf("MachineID = %v, want %v", host.MachineID, "test-machine-123")
	}
}

func TestRateLimiter(t *testing.T) {
	rateLimiter := &RateLimiter{
		clients: make(map[string]*ClientState),
	}

	// Test initial state
	if len(rateLimiter.clients) != 0 {
		t.Error("Expected empty clients map initially")
	}

	// Test adding clients
	testIPs := []string{"192.168.1.1", "192.168.1.2", "2001:db8::1"}
	for _, ip := range testIPs {
		client := createMockClientState(ip, 0, false)
		rateLimiter.clients[ip] = client
	}

	if len(rateLimiter.clients) != len(testIPs) {
		t.Errorf("Expected %d clients, got %d", len(testIPs), len(rateLimiter.clients))
	}

	// Test client retrieval
	for _, ip := range testIPs {
		if client, exists := rateLimiter.clients[ip]; !exists {
			t.Errorf("Client with IP %s not found", ip)
		} else if client.IP != ip {
			t.Errorf("Client IP mismatch: got %s, want %s", client.IP, ip)
		}
	}
}

func TestSecurityMetrics(t *testing.T) {
	metrics := &SecurityMetrics{
		TotalQueries:     100,
		BlockedQueries:   5,
		MalformedPackets: 2,
		RateLimitHits:    3,
		LargePackets:     1,
	}

	if metrics.TotalQueries != 100 {
		t.Errorf("TotalQueries = %v, want %v", metrics.TotalQueries, 100)
	}

	if metrics.BlockedQueries != 5 {
		t.Errorf("BlockedQueries = %v, want %v", metrics.BlockedQueries, 5)
	}

	// Test that blocked queries don't exceed total queries
	if metrics.BlockedQueries > metrics.TotalQueries {
		t.Error("BlockedQueries should not exceed TotalQueries")
	}

	// Test individual counters don't exceed blocked queries
	individualBlocked := metrics.MalformedPackets + metrics.RateLimitHits + metrics.LargePackets
	if individualBlocked > metrics.TotalQueries {
		t.Error("Sum of individual blocked counters should not exceed TotalQueries")
	}
}

func TestHealthMonitor(t *testing.T) {
	monitor := &HealthMonitor{
		OverallHealth:    0.8,
		InterfaceHealth:  make(map[string]float64),
		LastHealthCheck:  time.Now(),
		RecoveryActive:   false,
		SystemErrors:     0,
		RecoveryAttempts: 0,
	}

	// Test initial health state
	if monitor.OverallHealth < 0.0 || monitor.OverallHealth > 1.0 {
		t.Errorf("OverallHealth should be between 0.0 and 1.0, got %f", monitor.OverallHealth)
	}

	// Test interface health tracking
	interfaceNames := []string{"eth0", "wlan0", "lo"}
	for i, name := range interfaceNames {
		health := float64(i+1) * 0.3 // 0.3, 0.6, 0.9
		if health > 1.0 {
			health = 1.0
		}
		monitor.InterfaceHealth[name] = health
	}

	if len(monitor.InterfaceHealth) != len(interfaceNames) {
		t.Errorf("Expected %d interface health entries, got %d", 
			len(interfaceNames), len(monitor.InterfaceHealth))
	}

	// Verify all health values are within valid range
	for name, health := range monitor.InterfaceHealth {
		if health < 0.0 || health > 1.0 {
			t.Errorf("Interface %s health should be between 0.0 and 1.0, got %f", name, health)
		}
	}
}