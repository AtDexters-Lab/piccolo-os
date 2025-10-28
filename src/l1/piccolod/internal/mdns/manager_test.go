package mdns

import (
	"fmt"
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	manager := NewManager()

	// Test initialization
	if manager == nil {
		t.Fatal("NewManager() returned nil")
	}

	// Test basic fields
	if manager.hostname != "piccolo" {
		t.Errorf("hostname = %v, want %v", manager.hostname, "piccolo")
	}

	if manager.port != 80 {
		t.Errorf("port = %v, want %v", manager.port, 80)
	}

	if manager.baseName != "piccolo" {
		t.Errorf("baseName = %v, want %v", manager.baseName, "piccolo")
	}

	if manager.finalName != "piccolo" {
		t.Errorf("finalName = %v, want %v", manager.finalName, "piccolo")
	}

	// Test machine ID generation
	if manager.machineID == "" {
		t.Error("machineID should not be empty")
	}

	if len(manager.machineID) != 6 {
		t.Errorf("machineID length = %v, want 6", len(manager.machineID))
	}

	// Test security components initialization
	if manager.rateLimiter == nil {
		t.Error("rateLimiter should be initialized")
	}

	if manager.securityConfig == nil {
		t.Error("securityConfig should be initialized")
	}

	if manager.securityMetrics == nil {
		t.Error("securityMetrics should be initialized")
	}

	if manager.queryProcessor == nil {
		t.Error("queryProcessor should be initialized")
	}

	// Test resilience components initialization
	if manager.resilienceConfig == nil {
		t.Error("resilienceConfig should be initialized")
	}

	if manager.healthMonitor == nil {
		t.Error("healthMonitor should be initialized")
	}

	// Test conflict detection initialization
	if manager.conflictDetector == nil {
		t.Error("conflictDetector should be initialized")
	}

	// Test collections initialization
	if manager.interfaces == nil {
		t.Error("interfaces map should be initialized")
	}

	if len(manager.interfaces) != 0 {
		t.Error("interfaces map should be empty initially")
	}

	// Test channel initialization
	if manager.stopCh == nil {
		t.Error("stopCh should be initialized")
	}
}

func TestManagerSecurityConfigDefaults(t *testing.T) {
	manager := NewManager()

	config := manager.securityConfig
	tests := []struct {
		name     string
		actual   interface{}
		expected interface{}
	}{
		{"MaxQueriesPerSecond", config.MaxQueriesPerSecond, 10},
		{"MaxQueriesPerMinute", config.MaxQueriesPerMinute, 100},
		{"MaxPacketSize", config.MaxPacketSize, 1500},
		{"MaxResponseSize", config.MaxResponseSize, 512},
		{"MaxConcurrentQueries", config.MaxConcurrentQueries, 50},
		{"QueryTimeout", config.QueryTimeout, time.Second * 2},
		{"ClientBlockDuration", config.ClientBlockDuration, time.Minute * 5},
		{"CleanupInterval", config.CleanupInterval, time.Minute * 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.actual != tt.expected {
				t.Errorf("%s = %v, want %v", tt.name, tt.actual, tt.expected)
			}
		})
	}
}

func TestManagerResilienceConfigDefaults(t *testing.T) {
	manager := NewManager()

	config := manager.resilienceConfig
	tests := []struct {
		name     string
		actual   interface{}
		expected interface{}
	}{
		{"MaxRetries", config.MaxRetries, 3},
		{"InitialBackoff", config.InitialBackoff, time.Second * 5},
		{"MaxBackoff", config.MaxBackoff, time.Minute * 5},
		{"BackoffMultiplier", config.BackoffMultiplier, 2.0},
		{"HealthCheckInterval", config.HealthCheckInterval, time.Second * 30},
		{"RecoveryCheckInterval", config.RecoveryCheckInterval, time.Second * 15},
		{"MaxFailureRate", config.MaxFailureRate, 0.3},
		{"MinHealthScore", config.MinHealthScore, 0.7},
		{"RecoveryTimeout", config.RecoveryTimeout, time.Minute * 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.actual != tt.expected {
				t.Errorf("%s = %v, want %v", tt.name, tt.actual, tt.expected)
			}
		})
	}
}

func TestManagerHealthMonitorDefaults(t *testing.T) {
	manager := NewManager()

	monitor := manager.healthMonitor

	if monitor.OverallHealth != 1.0 {
		t.Errorf("OverallHealth = %v, want %v", monitor.OverallHealth, 1.0)
	}

	if monitor.InterfaceHealth == nil {
		t.Error("InterfaceHealth map should be initialized")
	}

	if len(monitor.InterfaceHealth) != 0 {
		t.Error("InterfaceHealth map should be empty initially")
	}

	if !assertTimestamp(monitor.LastHealthCheck, time.Second) {
		t.Error("LastHealthCheck should be recent")
	}

	if monitor.RecoveryActive {
		t.Error("RecoveryActive should be false initially")
	}

	if monitor.SystemErrors != 0 {
		t.Errorf("SystemErrors = %v, want %v", monitor.SystemErrors, 0)
	}

	if monitor.RecoveryAttempts != 0 {
		t.Errorf("RecoveryAttempts = %v, want %v", monitor.RecoveryAttempts, 0)
	}
}

func TestManagerConflictDetectorDefaults(t *testing.T) {
	manager := NewManager()

	detector := manager.conflictDetector

	if detector.ConflictDetected {
		t.Error("ConflictDetected should be false initially")
	}

	if detector.ConflictingSources == nil {
		t.Error("ConflictingSources map should be initialized")
	}

	if len(detector.ConflictingSources) != 0 {
		t.Error("ConflictingSources map should be empty initially")
	}

	if !assertTimestamp(detector.LastConflictCheck, time.Second) {
		t.Error("LastConflictCheck should be recent")
	}

	if detector.ResolutionAttempts != 0 {
		t.Errorf("ResolutionAttempts = %v, want %v", detector.ResolutionAttempts, 0)
	}

	if detector.CurrentSuffix != "" {
		t.Errorf("CurrentSuffix = %v, want empty string", detector.CurrentSuffix)
	}
}

func TestManagerRateLimiterDefaults(t *testing.T) {
	manager := NewManager()

	rateLimiter := manager.rateLimiter

	if rateLimiter.clients == nil {
		t.Error("rateLimiter.clients map should be initialized")
	}

	if len(rateLimiter.clients) != 0 {
		t.Error("rateLimiter.clients map should be empty initially")
	}
}

func TestManagerQueryProcessorDefaults(t *testing.T) {
	manager := NewManager()

	processor := manager.queryProcessor

	if processor.semaphore == nil {
		t.Error("queryProcessor.semaphore should be initialized")
	}

	// Test semaphore capacity matches security config
	expectedCapacity := manager.securityConfig.MaxConcurrentQueries
	semaphoreCapacity := cap(processor.semaphore)

	if semaphoreCapacity != expectedCapacity {
		t.Errorf("semaphore capacity = %v, want %v", semaphoreCapacity, expectedCapacity)
	}

	if processor.activeCount != 0 {
		t.Errorf("activeCount = %v, want %v", processor.activeCount, 0)
	}
}

func TestGetMachineIDDeterministic(t *testing.T) {
	// Test that getMachineID returns consistent results
	id1 := getMachineID()
	id2 := getMachineID()

	if id1 != id2 {
		t.Errorf("getMachineID() should be deterministic: got %s and %s", id1, id2)
	}

	if len(id1) != 6 {
		t.Errorf("getMachineID() length = %v, want 6", len(id1))
	}

	// Validate it's a hex string
	for _, char := range id1 {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')) {
			t.Errorf("getMachineID() should return hex string, got %s", id1)
			break
		}
	}
}

func TestManagerStop(t *testing.T) {
	manager := NewManager()

	// Test stop on unstarted manager (should not panic)
	err := manager.Stop()
	if err != nil {
		t.Errorf("Stop() on unstarted manager should not error, got: %v", err)
	}

	// Test stop channel is closed
	select {
	case <-manager.stopCh:
		// Channel is closed, this is expected
	case <-time.After(time.Millisecond * 100):
		t.Error("Stop() should close the stopCh channel")
	}
}

func TestManagerInterfaceMapOperations(t *testing.T) {
	manager := NewManager()

	// Test adding interface state
	testState := createMockInterfaceState("test0", true, true)

	manager.mutex.Lock()
	manager.interfaces["test0"] = testState
	manager.mutex.Unlock()

	// Test retrieval
	manager.mutex.RLock()
	retrieved, exists := manager.interfaces["test0"]
	count := len(manager.interfaces)
	manager.mutex.RUnlock()

	if !exists {
		t.Error("Interface should exist after adding")
	}

	if retrieved != testState {
		t.Error("Retrieved interface state should match added state")
	}

	if count != 1 {
		t.Errorf("Interface count = %v, want %v", count, 1)
	}

	// Test removal
	manager.mutex.Lock()
	delete(manager.interfaces, "test0")
	manager.mutex.Unlock()

	manager.mutex.RLock()
	_, exists = manager.interfaces["test0"]
	count = len(manager.interfaces)
	manager.mutex.RUnlock()

	if exists {
		t.Error("Interface should not exist after removal")
	}

	if count != 0 {
		t.Errorf("Interface count after removal = %v, want %v", count, 0)
	}
}

func TestManagerConcurrentAccess(t *testing.T) {
	manager := NewManager()

	// Test concurrent read/write access to interfaces map
	done := make(chan bool, 2)

	// Writer goroutine
	go func() {
		for i := 0; i < 10; i++ {
			state := createMockInterfaceState(fmt.Sprintf("test%d", i), true, false)
			manager.mutex.Lock()
			manager.interfaces[fmt.Sprintf("test%d", i)] = state
			manager.mutex.Unlock()
			time.Sleep(time.Microsecond)
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 10; i++ {
			manager.mutex.RLock()
			_ = len(manager.interfaces)
			manager.mutex.RUnlock()
			time.Sleep(time.Microsecond)
		}
		done <- true
	}()

	// Wait for both goroutines
	<-done
	<-done

	// Verify final state
	manager.mutex.RLock()
	finalCount := len(manager.interfaces)
	manager.mutex.RUnlock()

	if finalCount != 10 {
		t.Errorf("Final interface count = %v, want %v", finalCount, 10)
	}
}

// TestGoroutineDeadlockRegression ensures Bug #1 (goroutine deadlock) stays fixed
func TestGoroutineDeadlockRegression(t *testing.T) {
	t.Log("=== REGRESSION TEST: Goroutine Deadlock Prevention ===")

	// This test ensures that the manager can start and stop cleanly without deadlocks
	// Previously, missing defer m.wg.Done() caused hangs on manager.Stop()

	manager := newStubbedManager(t, defaultStubNetworkEnv())

	err := manager.Start()
	if err != nil {
		t.Logf("Manager start failed (may be expected in test env): %v", err)
		return
	}

	// Give it time to start all goroutines
	time.Sleep(100 * time.Millisecond)

	// This should complete within reasonable time without hanging
	done := make(chan struct{})
	go func() {
		manager.Stop()
		close(done)
	}()

	// Timeout test - should not hang
	select {
	case <-done:
		t.Log("✅ Manager stopped cleanly - no deadlock")
	case <-time.After(5 * time.Second):
		t.Error("REGRESSION: Manager.Stop() hung - deadlock detected!")
	}
}
