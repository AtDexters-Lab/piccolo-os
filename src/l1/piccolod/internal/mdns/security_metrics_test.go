package mdns

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestBug6_SecurityMetricsNotUpdated(t *testing.T) {
	t.Log("=== TESTING Bug #6: Security Metrics Not Updated ===")
	
	manager := NewManager()
	
	// Check initial metrics
	initialTotal := atomic.LoadUint64(&manager.securityMetrics.TotalQueries)
	t.Logf("Initial TotalQueries: %d", initialTotal)
	
	// Simulate queries from different clients
	testIPs := []string{
		"192.168.1.100",
		"192.168.1.101", 
		"192.168.1.102",
	}
	
	queryCount := 0
	for _, ip := range testIPs {
		for i := 0; i < 5; i++ {
			manager.isRateLimited(ip)
			queryCount++
		}
	}
	
	// Check metrics after queries
	finalTotal := atomic.LoadUint64(&manager.securityMetrics.TotalQueries)
	t.Logf("After %d queries - TotalQueries: %d", queryCount, finalTotal)
	
	if finalTotal == initialTotal {
		t.Error("BUG CONFIRMED: TotalQueries counter is not being incremented")
		t.Error("ISSUE: isRateLimited() processes queries but doesn't update TotalQueries metric")
		integrationBugs.Add("Security metrics not tracking total queries - TotalQueries always 0")
	} else {
		t.Log("SUCCESS: TotalQueries counter is being updated")
	}
	
	// Expected: TotalQueries should equal the number of calls to isRateLimited()
	if finalTotal != uint64(queryCount) {
		t.Errorf("TotalQueries mismatch: got %d, expected %d", finalTotal, queryCount)
	}
}

func TestBug6_AllSecurityMetrics(t *testing.T) {
	t.Log("=== TESTING All Security Metrics ===")
	
	manager := NewManager()
	attackerIP := "10.0.0.1"
	
	// Track metrics before
	initialMetrics := struct {
		total   uint64
		blocked uint64
		hits    uint64
	}{
		total:   atomic.LoadUint64(&manager.securityMetrics.TotalQueries),
		blocked: atomic.LoadUint64(&manager.securityMetrics.BlockedQueries), 
		hits:    atomic.LoadUint64(&manager.securityMetrics.RateLimitHits),
	}
	
	t.Logf("Initial metrics - Total: %d, Blocked: %d, Hits: %d", 
		initialMetrics.total, initialMetrics.blocked, initialMetrics.hits)
	
	// Generate enough queries to trigger rate limiting
	queriesGenerated := 0
	for i := 0; i < 20; i++ {
		isLimited := manager.isRateLimited(attackerIP)
		queriesGenerated++
		
		if isLimited {
			t.Logf("Query %d: Rate limited", i+1)
		}
		
		// Small delay to avoid hitting per-second limits too quickly
		time.Sleep(time.Millisecond * 10)
	}
	
	// Check final metrics
	finalMetrics := struct {
		total   uint64
		blocked uint64
		hits    uint64
	}{
		total:   atomic.LoadUint64(&manager.securityMetrics.TotalQueries),
		blocked: atomic.LoadUint64(&manager.securityMetrics.BlockedQueries),
		hits:    atomic.LoadUint64(&manager.securityMetrics.RateLimitHits),
	}
	
	t.Logf("Final metrics - Total: %d, Blocked: %d, Hits: %d", 
		finalMetrics.total, finalMetrics.blocked, finalMetrics.hits)
	
	// Test TotalQueries
	if finalMetrics.total == initialMetrics.total {
		t.Error("BUG: TotalQueries not incremented despite processing queries")
	} else if finalMetrics.total != initialMetrics.total + uint64(queriesGenerated) {
		t.Errorf("TotalQueries incorrect: got %d, expected %d", 
			finalMetrics.total, initialMetrics.total + uint64(queriesGenerated))
	} else {
		t.Log("SUCCESS: TotalQueries tracking correctly")
	}
	
	// Test other metrics - these should work if rate limiting occurred
	if finalMetrics.hits > initialMetrics.hits {
		t.Log("SUCCESS: RateLimitHits metric working")
	}
	
	if finalMetrics.blocked > initialMetrics.blocked {
		t.Log("SUCCESS: BlockedQueries metric working")
	}
}

func TestBug6_MetricsConsistency(t *testing.T) {
	t.Log("=== TESTING Security Metrics Consistency ===")
	
	manager := NewManager()
	
	// Normal queries (should not be blocked)
	normalClient := "192.168.1.50"
	for i := 0; i < 5; i++ {
		manager.isRateLimited(normalClient)
		time.Sleep(time.Second) // Wait between queries to avoid rate limiting
	}
	
	// Rapid queries (should trigger rate limiting)
	rapidClient := "192.168.1.51"
	for i := 0; i < 15; i++ {
		manager.isRateLimited(rapidClient)
		// No delay - rapid fire
	}
	
	totalQueries := atomic.LoadUint64(&manager.securityMetrics.TotalQueries)
	blockedQueries := atomic.LoadUint64(&manager.securityMetrics.BlockedQueries)
	rateLimitHits := atomic.LoadUint64(&manager.securityMetrics.RateLimitHits)
	
	t.Logf("Metrics after mixed workload:")
	t.Logf("  TotalQueries: %d", totalQueries)
	t.Logf("  BlockedQueries: %d", blockedQueries)
	t.Logf("  RateLimitHits: %d", rateLimitHits)
	
	// Logical consistency checks
	if totalQueries == 0 {
		t.Error("BUG: TotalQueries should be > 0 after processing queries")
	}
	
	if totalQueries != 20 { // 5 normal + 15 rapid
		t.Errorf("TotalQueries should be 20, got %d", totalQueries)
	}
	
	// BlockedQueries should be <= TotalQueries
	if blockedQueries > totalQueries {
		t.Error("INCONSISTENCY: BlockedQueries cannot exceed TotalQueries")
	}
	
	// If we had rate limiting, RateLimitHits should be > 0
	if rateLimitHits == 0 && blockedQueries > 0 {
		t.Error("INCONSISTENCY: RateLimitHits should be > 0 if queries were blocked")
	}
}

func TestBug6_RealWorldScenario(t *testing.T) {
	t.Log("=== TESTING Real-World Security Metrics Scenario ===")
	
	manager := NewManager()
	
	// Simulate a real-world scenario with multiple clients
	clients := map[string]int{
		"192.168.1.10": 3,   // Normal usage
		"192.168.1.11": 5,   // Heavy usage
		"192.168.1.12": 25,  // Potential attack
		"192.168.1.13": 2,   // Light usage
	}
	
	expectedTotal := 0
	for ip, queryCount := range clients {
		for i := 0; i < queryCount; i++ {
			manager.isRateLimited(ip)
			expectedTotal++
		}
		// Small delay between clients
		time.Sleep(time.Millisecond * 50)
	}
	
	actualTotal := atomic.LoadUint64(&manager.securityMetrics.TotalQueries)
	
	t.Logf("Expected TotalQueries: %d", expectedTotal)
	t.Logf("Actual TotalQueries: %d", actualTotal)
	
	if actualTotal == 0 {
		t.Error("BUG: Security monitoring completely broken - no queries tracked")
		t.Error("IMPACT: Admin dashboards would show 0 queries despite active traffic")
	} else if actualTotal != uint64(expectedTotal) {
		t.Errorf("BUG: Query count mismatch - security monitoring inaccurate")
	} else {
		t.Log("SUCCESS: Security metrics accurately reflect query activity")
	}
}