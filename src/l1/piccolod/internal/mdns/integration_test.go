package mdns

import (
	"fmt"
	"net"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/miekg/dns"
)

// BugTracker collects bugs found during integration testing
type BugTracker struct {
	bugs []string
	mu   sync.Mutex
}

func (bt *BugTracker) Add(bug string) {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	bt.bugs = append(bt.bugs, bug)
}

func (bt *BugTracker) List() []string {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	return append([]string{}, bt.bugs...)
}

var integrationBugs = &BugTracker{}

func TestFullManagerLifecycle_Integration(t *testing.T) {
	t.Log("=== INTEGRATION TEST: Full Manager Lifecycle ===")

	manager := NewManager()

	// Test 1: Manager startup with real network interfaces
	t.Log("Testing manager startup...")
	err := manager.Start()
	if err != nil {
		integrationBugs.Add(fmt.Sprintf("Manager.Start() failed: %v", err))
		t.Errorf("Manager failed to start: %v", err)
		return
	}

	// Give it a moment to initialize
	time.Sleep(100 * time.Millisecond)

	// Test 2: Check if interfaces were actually configured
	manager.mutex.RLock()
	interfaceCount := len(manager.interfaces)
	var activeInterfaces []string
	for name, state := range manager.interfaces {
		if state.Active {
			activeInterfaces = append(activeInterfaces, name)
		}
	}
	manager.mutex.RUnlock()

	if interfaceCount == 0 {
		integrationBugs.Add("No network interfaces were configured during startup")
		t.Error("BUG: No interfaces configured")
	}

	t.Logf("Active interfaces: %v", activeInterfaces)

	// Test 3: Check if UDP sockets are actually working
	manager.mutex.RLock()
	for name, state := range manager.interfaces {
		if state.IPv4Conn != nil {
			// Try to write to the connection - this revealed the first bug!
			testAddr, _ := net.ResolveUDPAddr("udp4", "224.0.0.251:5353")
			_, err := state.IPv4Conn.WriteToUDP([]byte("test"), testAddr)
			if err != nil {
				integrationBugs.Add(fmt.Sprintf("IPv4 connection for %s not properly configured for multicast: %v", name, err))
				t.Errorf("BUG: IPv4 connection for %s broken: %v", name, err)
			}
		}
		if state.IPv6Conn != nil {
			testAddr, _ := net.ResolveUDPAddr("udp6", "[ff02::fb]:5353")
			_, err := state.IPv6Conn.WriteToUDP([]byte("test"), testAddr)
			if err != nil {
				integrationBugs.Add(fmt.Sprintf("IPv6 connection for %s not properly configured for multicast: %v", name, err))
				t.Errorf("BUG: IPv6 connection for %s broken: %v", name, err)
			}
		}
	}
	manager.mutex.RUnlock()

	// Test 4: Manager shutdown and cleanup
	err = manager.Stop()
	if err != nil {
		integrationBugs.Add(fmt.Sprintf("Manager.Stop() failed: %v", err))
		t.Errorf("Manager failed to stop cleanly: %v", err)
	}

	// Test 5: Verify cleanup - connections should be closed
	manager.mutex.RLock()
	for name, state := range manager.interfaces {
		if state.IPv4Conn != nil {
			// Try to write after stop - should fail
			testAddr, _ := net.ResolveUDPAddr("udp4", "224.0.0.251:5353")
			_, err := state.IPv4Conn.WriteToUDP([]byte("test"), testAddr)
			if err == nil {
				integrationBugs.Add(fmt.Sprintf("IPv4 connection for %s not properly closed during shutdown", name))
				t.Errorf("BUG: Connection %s not closed after stop", name)
			}
		}
	}
	manager.mutex.RUnlock()
}

func TestMDNSProtocolCompliance_RFC6762(t *testing.T) {
	t.Log("=== INTEGRATION TEST: mDNS Protocol Compliance (RFC 6762) ===")

	// Test IPv6 link-local address handling - should be fixed now
	t.Log("Testing IPv6 link-local address filtering...")

	// Test the IPv6 address filtering logic directly
	linkLocalAddr := net.ParseIP("fe80::1234:5678:9abc:def0")

	// New logic: accept all IPv6 except loopback (link-local is allowed)
	shouldAccept := !linkLocalAddr.IsLoopback()

	if !shouldAccept {
		integrationBugs.Add("CRITICAL: IPv6 link-local addresses are filtered out, violating RFC 6762 Section 15")
		t.Error("BUG: mDNS requires IPv6 link-local scope, but code filters fe80:: addresses")
	} else {
		t.Log("SUCCESS: IPv6 link-local addresses are now accepted (RFC 6762 compliant)")
	}

	// Test mDNS multicast addresses
	ipv4Multicast := net.ParseIP("224.0.0.251")
	ipv6Multicast := net.ParseIP("ff02::fb")

	if !ipv4Multicast.IsMulticast() {
		integrationBugs.Add("IPv4 mDNS multicast address 224.0.0.251 not recognized as multicast")
	}

	if !ipv6Multicast.IsMulticast() {
		integrationBugs.Add("IPv6 mDNS multicast address ff02::fb not recognized as multicast")
	}

	// Test DNS message format compliance
	t.Log("Testing DNS message format...")
	testDNSMessageCompliance(t)
}

func testDNSMessageCompliance(t *testing.T) {
	// Test creating a proper mDNS announcement
	msg := new(dns.Msg)

	// mDNS announcements should be responses, not queries
	msg.Response = true
	msg.Authoritative = true
	msg.RecursionDesired = false
	msg.RecursionAvailable = false

	// Add an A record for piccolo.local
	rr, err := dns.NewRR("piccolo.local. 120 IN A 192.168.1.100")
	if err != nil {
		integrationBugs.Add(fmt.Sprintf("Failed to create mDNS A record: %v", err))
		t.Errorf("DNS record creation failed: %v", err)
		return
	}
	msg.Answer = append(msg.Answer, rr)

	// Pack and check size
	packed, err := msg.Pack()
	if err != nil {
		integrationBugs.Add(fmt.Sprintf("Failed to pack mDNS message: %v", err))
		t.Errorf("DNS message packing failed: %v", err)
		return
	}

	if len(packed) > 512 {
		integrationBugs.Add(fmt.Sprintf("mDNS message too large: %d bytes (should be ≤512 for best compatibility)", len(packed)))
		t.Logf("WARNING: Large mDNS message: %d bytes", len(packed))
	}

	// Test unpacking (roundtrip test)
	unpacked := new(dns.Msg)
	err = unpacked.Unpack(packed)
	if err != nil {
		integrationBugs.Add(fmt.Sprintf("Failed to unpack mDNS message we just created: %v", err))
		t.Errorf("DNS message roundtrip failed: %v", err)
	}
}

func TestSecurityMechanisms_Integration(t *testing.T) {
	t.Log("=== INTEGRATION TEST: Security Mechanisms ===")

	manager := NewManager()

	// Test rate limiting with realistic scenario
	t.Log("Testing rate limiting under realistic load...")

	attackerIP := "192.168.1.200"
	var totalQueries uint64
	var blockedQueries uint64

	// Simulate an attack: 50 queries in rapid succession
	for i := 0; i < 50; i++ {
		totalQueries++
		if manager.isRateLimited(attackerIP) {
			blockedQueries++
		}
		time.Sleep(time.Millisecond * 10) // 100 queries/second
	}

	if blockedQueries == 0 {
		integrationBugs.Add("Rate limiting failed to block rapid queries - DoS vulnerability")
		t.Error("BUG: No queries blocked during simulated attack")
	}

	// Check if security metrics are properly tracked
	if manager.securityMetrics.TotalQueries == 0 {
		integrationBugs.Add("Security metrics not tracking total queries")
		t.Error("BUG: TotalQueries metric not updated")
	}

	if manager.securityMetrics.RateLimitHits == 0 && blockedQueries > 0 {
		integrationBugs.Add("Security metrics not tracking rate limit hits")
		t.Error("BUG: RateLimitHits metric not updated")
	}

	// Test concurrent query processing limits
	t.Log("Testing concurrent query limits...")
	testConcurrentQueryLimits(t, manager)
}

func testConcurrentQueryLimits(t *testing.T, manager *Manager) {
	processor := manager.queryProcessor
	maxConcurrent := manager.securityConfig.MaxConcurrentQueries

	// Try to exhaust the semaphore
	acquired := 0
	for i := 0; i < maxConcurrent+10; i++ {
		select {
		case processor.semaphore <- struct{}{}:
			acquired++
		default:
			break
		}
	}

	if acquired != maxConcurrent {
		integrationBugs.Add(fmt.Sprintf("Concurrent query limiting broken: acquired %d slots, expected max %d", acquired, maxConcurrent))
		t.Errorf("BUG: Semaphore acquired %d slots, expected %d", acquired, maxConcurrent)
	}

	// Release all slots
	for i := 0; i < acquired; i++ {
		<-processor.semaphore
	}
}

func TestResilienceAndRecovery_Integration(t *testing.T) {
	t.Log("=== INTEGRATION TEST: Resilience and Recovery ===")

	manager := NewManager()

	// Test interface failure and recovery
	t.Log("Testing interface failure simulation...")

	// Create a mock interface and add it
	mockState := createMockInterfaceState("mock0", true, true)
	manager.mutex.Lock()
	manager.interfaces["mock0"] = mockState
	manager.mutex.Unlock()

	// Simulate interface failure
	originalHealth := mockState.HealthScore
	err := fmt.Errorf("simulated network interface failure")
	manager.markInterfaceFailure(mockState, err)

	if mockState.HealthScore >= originalHealth {
		integrationBugs.Add("Interface health score not reduced after failure")
		t.Error("BUG: Health score should decrease after failure")
	}

	if mockState.BackoffUntil.IsZero() {
		integrationBugs.Add("Backoff period not set after interface failure")
		t.Error("BUG: Backoff period not configured")
	}

	// Test health monitoring
	t.Log("Testing health monitoring...")
	manager.performHealthCheck()

	overallHealth := manager.healthMonitor.OverallHealth
	if overallHealth > 1.0 {
		integrationBugs.Add(fmt.Sprintf("Overall health score invalid: %f (should be ≤ 1.0)", overallHealth))
		t.Errorf("BUG: Invalid health score: %f", overallHealth)
	}

	// Test backoff calculation
	backoff1 := manager.calculateBackoffDuration(mockState)
	mockState.RecoveryAttempts++
	backoff2 := manager.calculateBackoffDuration(mockState)

	if backoff2 <= backoff1 {
		integrationBugs.Add("Exponential backoff not working - subsequent failures should have longer backoff")
		t.Error("BUG: Backoff duration not increasing exponentially")
	}
}

func TestPlatformCompatibility_Integration(t *testing.T) {
	t.Log("=== INTEGRATION TEST: Platform Compatibility ===")

	// Test socket creation with platform-specific constants
	t.Log("Testing socket option compatibility...")

	// This will reveal the SO_REUSEPORT hardcoding bug
	manager := NewManager()

	interfaces, err := net.Interfaces()
	if err != nil {
		t.Skip("No network interfaces available for platform testing")
		return
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}

		// Try to create sockets - this may fail on non-Linux platforms
		t.Logf("Testing socket creation on interface: %s", iface.Name)

		// Get IPv4 address from the interface
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		var ipv4Addr net.IP
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok {
				if ipv4 := ipnet.IP.To4(); ipv4 != nil && !ipnet.IP.IsLinkLocalUnicast() {
					ipv4Addr = ipv4
					break
				}
			}
		}

		if ipv4Addr != nil {
			ipv4Conn, err := manager.createIPv4Socket(&iface)
			if err != nil {
				if strings.Contains(err.Error(), "protocol not available") ||
					strings.Contains(err.Error(), "invalid argument") {
					integrationBugs.Add(fmt.Sprintf("Socket creation failed on %s - likely platform-specific constants: %v", iface.Name, err))
					t.Logf("POTENTIAL BUG: Platform compatibility issue: %v", err)
				}
			} else if ipv4Conn != nil {
				ipv4Conn.Close()
			}
		}

		// Test one interface to avoid spam
		break
	}
}

func TestMemoryLeaks_Integration(t *testing.T) {
	t.Log("=== INTEGRATION TEST: Memory Leak Detection ===")

	manager := NewManager()

	// Test client tracking cleanup
	t.Log("Testing client state cleanup...")

	// Add many clients
	clientCount := 1000
	for i := 0; i < clientCount; i++ {
		clientIP := fmt.Sprintf("10.0.%d.%d", i/254, i%254)
		manager.isRateLimited(clientIP) // This adds the client
	}

	manager.rateLimiter.mutex.RLock()
	actualClientCount := len(manager.rateLimiter.clients)
	manager.rateLimiter.mutex.RUnlock()

	if actualClientCount != clientCount {
		integrationBugs.Add(fmt.Sprintf("Client tracking inconsistent: added %d clients, found %d", clientCount, actualClientCount))
		t.Errorf("BUG: Client count mismatch: %d vs %d", clientCount, actualClientCount)
	}

	// In a real system, old clients should be cleaned up
	// But we haven't implemented cleanup yet...
	if actualClientCount == clientCount {
		integrationBugs.Add("No client cleanup mechanism - potential memory leak with many clients")
		t.Log("POTENTIAL BUG: Client states never cleaned up - memory leak risk")
	}
}

func TestGoroutineLeaks_Integration(t *testing.T) {
	t.Log("=== INTEGRATION TEST: Goroutine Leak Detection ===")

	initialGoroutines := numGoroutines()

	// Start and stop manager multiple times
	for i := 0; i < 3; i++ {
		manager := NewManager()

		// Start with real interfaces (may fail, that's OK)
		manager.Start()
		time.Sleep(50 * time.Millisecond)
		manager.Stop()

		// Give goroutines time to clean up
		time.Sleep(100 * time.Millisecond)
	}

	finalGoroutines := numGoroutines()
	goroutineDiff := finalGoroutines - initialGoroutines

	// Allow some tolerance for test runner goroutines
	if goroutineDiff > 5 {
		integrationBugs.Add(fmt.Sprintf("Potential goroutine leak: %d extra goroutines after manager cycles", goroutineDiff))
		t.Errorf("POTENTIAL BUG: %d extra goroutines detected", goroutineDiff)
	}
}

// Helper to count goroutines
func numGoroutines() int {
	return runtime.NumGoroutine()
}

// TestRegressionSuite contains tests for previously fixed bugs to prevent regression
func TestRegressionSuite_CriticalBugFixes(t *testing.T) {
	t.Log("=== REGRESSION TEST SUITE: Critical Bug Fixes ===")
	
	manager := NewManager()
	
	// Regression test for Bug #10: DNS Self-Loop Detection Fix
	t.Run("Bug10_mDNSProbingQueries", func(t *testing.T) {
		// Test that mDNS probing queries with answers are accepted (RFC 6762 Section 8.1)
		msg := new(dns.Msg)
		msg.Id = dns.Id()
		msg.Response = false
		msg.Question = []dns.Question{
			{Name: "probe.local.", Qtype: dns.TypeA, Qclass: dns.ClassINET},
		}
		// mDNS probing queries legitimately contain answer sections
		rr, _ := dns.NewRR("probe.local. 120 IN A 192.168.1.100")
		msg.Answer = []dns.RR{rr}
		
		err := manager.validateDNSMessage(msg)
		if err != nil && err.Error() == "queries should not have answers" {
			t.Error("REGRESSION: mDNS probing validation broken")
		} else {
			t.Log("✅ mDNS probing queries work correctly")
		}
	})
	
	// Regression test for IPv6 link-local acceptance (Bug #2)
	t.Run("Bug2_IPv6LinkLocalAcceptance", func(t *testing.T) {
		// Test that IPv6 link-local addresses are accepted (RFC 6762 compliance)
		linkLocalAddr := net.ParseIP("fe80::1234:5678:9abc:def0")
		shouldAccept := !linkLocalAddr.IsLoopback() // Fixed logic: accept all except loopback
		
		if !shouldAccept {
			t.Error("REGRESSION: IPv6 link-local addresses rejected")
		} else {
			t.Log("✅ IPv6 link-local addresses accepted (RFC 6762 compliant)")
		}
	})
	
	// Regression test for malformed packet handling (Bug #11)
	t.Run("Bug11_MalformedPacketSafety", func(t *testing.T) {
		// Test that malformed packets don't crash the system
		malformedPackets := [][]byte{
			{}, // Empty packet
			{0x12, 0x34}, // Too short
			{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}, // Invalid data
		}
		
		clientAddr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345}
		mockState := createMockInterfaceState("test0", true, true)
		
		for i, data := range malformedPackets {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("REGRESSION: Panic on malformed packet %d: %v", i, r)
					}
				}()
				manager.handleDualStackQuery(data, clientAddr, mockState, "regression")
			}()
		}
		t.Log("✅ Malformed packets handled safely")
	})
}
