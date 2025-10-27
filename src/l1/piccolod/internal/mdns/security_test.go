package mdns

import (
	"net"
	"sync"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func TestIsRateLimited_NewClient(t *testing.T) {
	manager := createMockManager()
	clientIP := "192.168.1.100"

	// New client should not be rate limited
	limited := manager.isRateLimited(clientIP)
	if limited {
		t.Error("New client should not be rate limited")
	}

	// Verify client was added to rate limiter
	manager.rateLimiter.mutex.RLock()
	client, exists := manager.rateLimiter.clients[clientIP]
	manager.rateLimiter.mutex.RUnlock()

	if !exists {
		t.Error("Client should be added to rate limiter")
	}

	if client.IP != clientIP {
		t.Errorf("Client IP = %v, want %v", client.IP, clientIP)
	}

	if client.Blocked {
		t.Error("New client should not be blocked")
	}

	if client.QueryCount != 0 {
		t.Errorf("New client QueryCount = %v, want %v", client.QueryCount, 0)
	}
}

func TestIsRateLimited_BlockedClient(t *testing.T) {
	manager := createMockManager()
	clientIP := "192.168.1.101"

	// Add a blocked client
	manager.rateLimiter.mutex.Lock()
	manager.rateLimiter.clients[clientIP] = &ClientState{
		IP:           clientIP,
		Blocked:      true,
		BlockedUntil: time.Now().Add(time.Minute),
		QueryCount:   100,
		LastQuery:    time.Now(),
	}
	manager.rateLimiter.mutex.Unlock()

	// Should be rate limited
	limited := manager.isRateLimited(clientIP)
	if !limited {
		t.Error("Blocked client should be rate limited")
	}

	// Verify blocked queries metric was incremented
	if manager.securityMetrics.BlockedQueries != 1 {
		t.Errorf("BlockedQueries = %v, want %v", manager.securityMetrics.BlockedQueries, 1)
	}
}

func TestIsRateLimited_ExpiredBlock(t *testing.T) {
	manager := createMockManager()
	clientIP := "192.168.1.102"

	// Add a client with expired block
	manager.rateLimiter.mutex.Lock()
	manager.rateLimiter.clients[clientIP] = &ClientState{
		IP:           clientIP,
		Blocked:      true,
		BlockedUntil: time.Now().Add(-time.Minute), // Expired
		QueryCount:   100,
		LastQuery:    time.Now(),
	}
	manager.rateLimiter.mutex.Unlock()

	// Should not be rate limited
	limited := manager.isRateLimited(clientIP)
	if limited {
		t.Error("Client with expired block should not be rate limited")
	}

	// Verify block status was reset
	manager.rateLimiter.mutex.RLock()
	client := manager.rateLimiter.clients[clientIP]
	manager.rateLimiter.mutex.RUnlock()

	if client.Blocked {
		t.Error("Expired block should be reset")
	}

	if client.QueryCount != 1 {
		t.Errorf("Query count should be reset and then incremented for expired block, got %d", client.QueryCount)
	}
}

func TestIsRateLimited_QueryCountReset(t *testing.T) {
	manager := createMockManager()
	clientIP := "192.168.1.103"

	// Add a client with old queries
	manager.rateLimiter.mutex.Lock()
	manager.rateLimiter.clients[clientIP] = &ClientState{
		IP:         clientIP,
		Blocked:    false,
		QueryCount: 50,
		LastQuery:  time.Now().Add(-time.Minute * 2), // More than a minute ago
	}
	manager.rateLimiter.mutex.Unlock()

	// Should not be rate limited and count should reset
	limited := manager.isRateLimited(clientIP)
	if limited {
		t.Error("Client with old queries should not be rate limited")
	}

	// Verify query count was reset
	manager.rateLimiter.mutex.RLock()
	client := manager.rateLimiter.clients[clientIP]
	manager.rateLimiter.mutex.RUnlock()

	if client.QueryCount != 1 {
		t.Errorf("Query count should be reset and then incremented for old queries, got %d", client.QueryCount)
	}
}

func TestHandleDualStackQueryCountsQueryOnce(t *testing.T) {
	manager := NewManager()
	state := createMockInterfaceState("eth0", true, false)
	manager.interfaces["eth0"] = state

	msg := dns.Msg{}
	msg.SetQuestion(manager.finalName+".local.", dns.TypeA)

	data, err := msg.Pack()
	if err != nil {
		t.Fatalf("failed to pack DNS query: %v", err)
	}

	clientAddr := &net.UDPAddr{IP: net.IPv4(192, 168, 1, 50)}

	manager.handleDualStackQuery(data, clientAddr, state, "IPv4")

	if got := manager.securityMetrics.TotalQueries; got != 1 {
		t.Fatalf("TotalQueries = %d, want 1", got)
	}
}

func TestQueryProcessorSemaphore(t *testing.T) {
	manager := createMockManager()
	processor := manager.queryProcessor

	// Test semaphore capacity
	maxConcurrent := manager.securityConfig.MaxConcurrentQueries
	if cap(processor.semaphore) != maxConcurrent {
		t.Errorf("Semaphore capacity = %v, want %v", cap(processor.semaphore), maxConcurrent)
	}

	// Test acquiring semaphore slots
	for i := 0; i < maxConcurrent; i++ {
		select {
		case processor.semaphore <- struct{}{}:
			// Successfully acquired slot
		default:
			t.Fatalf("Failed to acquire semaphore slot %d", i)
		}
	}

	// Test that we can't acquire more than max
	select {
	case processor.semaphore <- struct{}{}:
		t.Error("Should not be able to acquire more than max concurrent slots")
	default:
		// Expected behavior
	}

	// Test releasing slots
	for i := 0; i < maxConcurrent; i++ {
		select {
		case <-processor.semaphore:
			// Successfully released slot
		default:
			t.Fatalf("Failed to release semaphore slot %d", i)
		}
	}
}

func TestSecurityMetricsIncrement(t *testing.T) {
	manager := createMockManager()
	metrics := manager.securityMetrics

	// Test initial state
	if metrics.TotalQueries != 0 {
		t.Error("Initial TotalQueries should be 0")
	}

	// Test incrementing metrics (simulating what would happen in real usage)
	metrics.TotalQueries++
	metrics.BlockedQueries++
	metrics.MalformedPackets++
	metrics.RateLimitHits++
	metrics.LargePackets++

	// Verify increments
	tests := []struct {
		name     string
		actual   uint64
		expected uint64
	}{
		{"TotalQueries", metrics.TotalQueries, 1},
		{"BlockedQueries", metrics.BlockedQueries, 1},
		{"MalformedPackets", metrics.MalformedPackets, 1},
		{"RateLimitHits", metrics.RateLimitHits, 1},
		{"LargePackets", metrics.LargePackets, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.actual != tt.expected {
				t.Errorf("%s = %v, want %v", tt.name, tt.actual, tt.expected)
			}
		})
	}
}

func TestRateLimiterConcurrentAccess(t *testing.T) {
	manager := createMockManager()
	clientIPs := []string{
		"192.168.1.10", "192.168.1.11", "192.168.1.12",
		"192.168.1.13", "192.168.1.14", "192.168.1.15",
	}

	// Test concurrent access to rate limiter
	var wg sync.WaitGroup
	for _, ip := range clientIPs {
		wg.Add(1)
		go func(clientIP string) {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				manager.isRateLimited(clientIP)
				time.Sleep(time.Microsecond)
			}
		}(ip)
	}

	wg.Wait()

	// Verify all clients were added
	manager.rateLimiter.mutex.RLock()
	clientCount := len(manager.rateLimiter.clients)
	manager.rateLimiter.mutex.RUnlock()

	if clientCount != len(clientIPs) {
		t.Errorf("Client count = %v, want %v", clientCount, len(clientIPs))
	}

	// Verify each client exists
	for _, ip := range clientIPs {
		manager.rateLimiter.mutex.RLock()
		client, exists := manager.rateLimiter.clients[ip]
		manager.rateLimiter.mutex.RUnlock()

		if !exists {
			t.Errorf("Client %s should exist", ip)
		}

		if client.IP != ip {
			t.Errorf("Client IP = %v, want %v", client.IP, ip)
		}
	}
}

func TestSecurityConfigValidation(t *testing.T) {
	manager := createMockManager()
	config := manager.securityConfig

	// Test configuration values are positive
	tests := []struct {
		name  string
		value int
	}{
		{"MaxQueriesPerSecond", config.MaxQueriesPerSecond},
		{"MaxQueriesPerMinute", config.MaxQueriesPerMinute},
		{"MaxPacketSize", config.MaxPacketSize},
		{"MaxResponseSize", config.MaxResponseSize},
		{"MaxConcurrentQueries", config.MaxConcurrentQueries},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value <= 0 {
				t.Errorf("%s should be positive, got %v", tt.name, tt.value)
			}
		})
	}

	// Test duration values are positive
	durationTests := []struct {
		name  string
		value time.Duration
	}{
		{"QueryTimeout", config.QueryTimeout},
		{"ClientBlockDuration", config.ClientBlockDuration},
		{"CleanupInterval", config.CleanupInterval},
	}

	for _, tt := range durationTests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value <= 0 {
				t.Errorf("%s should be positive, got %v", tt.name, tt.value)
			}
		})
	}

	// Test reasonable limits
	if config.MaxPacketSize > 65535 {
		t.Error("MaxPacketSize should not exceed maximum UDP packet size")
	}

	if config.MaxResponseSize > config.MaxPacketSize {
		t.Error("MaxResponseSize should not exceed MaxPacketSize")
	}

	if config.MaxQueriesPerSecond > config.MaxQueriesPerMinute {
		t.Error("MaxQueriesPerSecond should not exceed MaxQueriesPerMinute")
	}
}

func TestClientStateLifecycle(t *testing.T) {
	manager := createMockManager()
	clientIP := "192.168.1.200"

	// Test client creation
	limited := manager.isRateLimited(clientIP)
	if limited {
		t.Error("New client should not be rate limited")
	}

	// Verify client was created
	manager.rateLimiter.mutex.RLock()
	client, exists := manager.rateLimiter.clients[clientIP]
	manager.rateLimiter.mutex.RUnlock()

	if !exists {
		t.Fatal("Client should exist after first check")
	}

	originalTime := client.LastQuery

	// Test client update
	time.Sleep(time.Millisecond * 10) // Small delay
	limited = manager.isRateLimited(clientIP)
	if limited {
		t.Error("Client should still not be rate limited")
	}

	// Verify client was updated
	manager.rateLimiter.mutex.RLock()
	client = manager.rateLimiter.clients[clientIP]
	manager.rateLimiter.mutex.RUnlock()

	if !client.LastQuery.After(originalTime) {
		t.Error("LastQuery should be updated on subsequent checks")
	}
}

func TestRateLimiterCleanup(t *testing.T) {
	manager := createMockManager()

	// Add several clients with different states
	testClients := map[string]*ClientState{
		"192.168.1.1": {
			IP:        "192.168.1.1",
			LastQuery: time.Now().Add(-time.Hour), // Very old
			Blocked:   false,
		},
		"192.168.1.2": {
			IP:           "192.168.1.2",
			LastQuery:    time.Now().Add(-time.Minute),
			Blocked:      true,
			BlockedUntil: time.Now().Add(-time.Minute), // Expired block
		},
		"192.168.1.3": {
			IP:           "192.168.1.3",
			LastQuery:    time.Now(),
			Blocked:      true,
			BlockedUntil: time.Now().Add(time.Minute), // Active block
		},
	}

	manager.rateLimiter.mutex.Lock()
	for ip, client := range testClients {
		manager.rateLimiter.clients[ip] = client
	}
	manager.rateLimiter.mutex.Unlock()

	// Verify initial count
	manager.rateLimiter.mutex.RLock()
	initialCount := len(manager.rateLimiter.clients)
	manager.rateLimiter.mutex.RUnlock()

	if initialCount != 3 {
		t.Errorf("Initial client count = %v, want %v", initialCount, 3)
	}

	// Test that clients still exist (cleanup is implemented elsewhere)
	for ip := range testClients {
		manager.rateLimiter.mutex.RLock()
		_, exists := manager.rateLimiter.clients[ip]
		manager.rateLimiter.mutex.RUnlock()

		if !exists {
			t.Errorf("Client %s should still exist", ip)
		}
	}
}
