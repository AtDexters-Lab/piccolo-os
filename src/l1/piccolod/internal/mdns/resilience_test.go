package mdns

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestUpdateInterfaceHealth_HealthyInterface(t *testing.T) {
	manager := createMockManager()
	state := createMockInterfaceState("test0", true, true)

	// Set healthy metrics
	atomic.StoreUint64(&state.QueryCount, 100)
	atomic.StoreUint64(&state.ErrorCount, 0)
	atomic.StoreUint64(&state.FailureCount, 0)

	manager.updateInterfaceHealth(state)

	if state.HealthScore != 1.0 {
		t.Errorf("Healthy interface HealthScore = %v, want %v", state.HealthScore, 1.0)
	}
}

func TestUpdateInterfaceHealth_ErrorRate(t *testing.T) {
	manager := createMockManager()
	state := createMockInterfaceState("test0", true, true)

	tests := []struct {
		name         string
		queryCount   uint64
		errorCount   uint64
		expectedMin  float64
		expectedMax  float64
	}{
		{
			name:        "Low error rate",
			queryCount:  100,
			errorCount:  5,
			expectedMin: 0.9,  // 100% - (5/100 * 50%) = 97.5%
			expectedMax: 1.0,
		},
		{
			name:        "Medium error rate",
			queryCount:  100,
			errorCount:  20,
			expectedMin: 0.8,  // 100% - (20/100 * 50%) = 90%
			expectedMax: 0.95,
		},
		{
			name:        "High error rate",
			queryCount:  100,
			errorCount:  50,
			expectedMin: 0.6,
			expectedMax: 0.7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atomic.StoreUint64(&state.QueryCount, tt.queryCount)
			atomic.StoreUint64(&state.ErrorCount, tt.errorCount)
			atomic.StoreUint64(&state.FailureCount, 0)

			manager.updateInterfaceHealth(state)

			if state.HealthScore < tt.expectedMin || state.HealthScore > tt.expectedMax {
				t.Errorf("HealthScore = %v, want between %v and %v", 
					state.HealthScore, tt.expectedMin, tt.expectedMax)
			}
		})
	}
}

func TestUpdateInterfaceHealth_FailureImpact(t *testing.T) {
	manager := createMockManager()
	state := createMockInterfaceState("test0", true, true)

	// Set no query errors but some failures
	atomic.StoreUint64(&state.QueryCount, 100)
	atomic.StoreUint64(&state.ErrorCount, 0)
	atomic.StoreUint64(&state.FailureCount, 3)
	state.LastFailure = time.Now()

	manager.updateInterfaceHealth(state)

	// With 3 recent failures, health should be reduced by ~30%
	if state.HealthScore > 0.75 {
		t.Errorf("Interface with recent failures should have reduced health, got %v", 
			state.HealthScore)
	}

	if state.HealthScore < 0.65 {
		t.Errorf("Health reduction seems too severe, got %v", state.HealthScore)
	}
}

func TestUpdateInterfaceHealth_FailureDecay(t *testing.T) {
	manager := createMockManager()
	state := createMockInterfaceState("test0", true, true)

	// Set old failures (should have less impact)
	atomic.StoreUint64(&state.QueryCount, 100)
	atomic.StoreUint64(&state.ErrorCount, 0)
	atomic.StoreUint64(&state.FailureCount, 3)
	state.LastFailure = time.Now().Add(-time.Minute * 15) // Old failure

	manager.updateInterfaceHealth(state)

	// Old failures should have minimal impact due to decay
	if state.HealthScore < 0.95 {
		t.Errorf("Interface with old failures should have high health due to decay, got %v", 
			state.HealthScore)
	}
}

func TestUpdateInterfaceHealth_CombinedFactors(t *testing.T) {
	manager := createMockManager()
	state := createMockInterfaceState("test0", true, true)

	// Set both errors and failures
	atomic.StoreUint64(&state.QueryCount, 100)
	atomic.StoreUint64(&state.ErrorCount, 10)  // 10% error rate
	atomic.StoreUint64(&state.FailureCount, 2) // 2 failures
	state.LastFailure = time.Now()

	manager.updateInterfaceHealth(state)

	// Health should be impacted by both errors and failures
	if state.HealthScore > 0.8 {
		t.Errorf("Interface with both errors and failures should have reduced health, got %v", 
			state.HealthScore)
	}

	if state.HealthScore < 0.6 {
		t.Errorf("Health reduction seems too severe for moderate issues, got %v", 
			state.HealthScore)
	}
}

func TestUpdateInterfaceHealth_HealthScoreBounds(t *testing.T) {
	manager := createMockManager()
	state := createMockInterfaceState("test0", true, true)

	// Test extreme case that would result in negative health
	atomic.StoreUint64(&state.QueryCount, 100)
	atomic.StoreUint64(&state.ErrorCount, 100)  // 100% error rate
	atomic.StoreUint64(&state.FailureCount, 20) // Many failures
	state.LastFailure = time.Now()

	manager.updateInterfaceHealth(state)

	// Health score should be bounded to 0.0
	if state.HealthScore < 0.0 {
		t.Errorf("HealthScore should not go below 0.0, got %v", state.HealthScore)
	}

	if state.HealthScore > 1.0 {
		t.Errorf("HealthScore should not exceed 1.0, got %v", state.HealthScore)
	}
}

func TestCalculateBackoffDuration(t *testing.T) {
	manager := createMockManager()
	config := manager.resilienceConfig

	tests := []struct {
		name           string
		attemptNumber  uint64
		expectedMin    time.Duration
		expectedMax    time.Duration
	}{
		{
			name:          "First attempt",
			attemptNumber: 1,
			expectedMin:   config.InitialBackoff * 2, // 2^1
			expectedMax:   config.InitialBackoff * 2,
		},
		{
			name:          "Second attempt",
			attemptNumber: 2,
			expectedMin:   config.InitialBackoff * 4, // 2^2
			expectedMax:   config.InitialBackoff * 4,
		},
		{
			name:          "Third attempt",
			attemptNumber: 3,
			expectedMin:   config.InitialBackoff * 8, // 2^3
			expectedMax:   config.InitialBackoff * 8,
		},
		{
			name:          "Large attempt number",
			attemptNumber: 10,
			expectedMin:   config.MaxBackoff,
			expectedMax:   config.MaxBackoff,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := createMockInterfaceState("test", true, true)
			atomic.StoreUint64(&state.RecoveryAttempts, tt.attemptNumber)
			
			duration := manager.calculateBackoffDuration(state)

			if duration < tt.expectedMin && tt.attemptNumber <= 3 {
				t.Errorf("Backoff duration = %v, want >= %v", duration, tt.expectedMin)
			}

			if duration > tt.expectedMax && tt.attemptNumber <= 3 {
				t.Errorf("Backoff duration = %v, want <= %v", duration, tt.expectedMax)
			}

			if duration > config.MaxBackoff {
				t.Errorf("Backoff duration = %v, should not exceed MaxBackoff %v", 
					duration, config.MaxBackoff)
			}
		})
	}
}

func TestCalculateBackoffDuration_ExponentialGrowth(t *testing.T) {
	manager := createMockManager()
	
	// Test that backoff grows exponentially
	state1 := createMockInterfaceState("test1", true, true)
	state2 := createMockInterfaceState("test2", true, true)
	state3 := createMockInterfaceState("test3", true, true)
	
	atomic.StoreUint64(&state1.RecoveryAttempts, 1)
	atomic.StoreUint64(&state2.RecoveryAttempts, 2)
	atomic.StoreUint64(&state3.RecoveryAttempts, 3)

	attempt1 := manager.calculateBackoffDuration(state1)
	attempt2 := manager.calculateBackoffDuration(state2)
	attempt3 := manager.calculateBackoffDuration(state3)

	if attempt2 != attempt1*2 {
		t.Errorf("Second attempt should be double first: got %v, %v", attempt2, attempt1*2)
	}

	if attempt3 != attempt2*2 {
		t.Errorf("Third attempt should be double second: got %v, %v", attempt3, attempt2*2)
	}
}

func TestMarkInterfaceFailure(t *testing.T) {
	manager := createMockManager()
	state := createMockInterfaceState("test0", true, true)

	// Initialize counters
	atomic.StoreUint64(&state.FailureCount, 0)
	atomic.StoreUint64(&state.RecoveryAttempts, 0)

	initialTime := state.LastFailure

	manager.markInterfaceFailure(state, fmt.Errorf("Test failure"))

	// Verify failure was recorded
	failureCount := atomic.LoadUint64(&state.FailureCount)
	if failureCount != 1 {
		t.Errorf("FailureCount = %v, want %v", failureCount, 1)
	}

	// Verify LastFailure was updated
	if !state.LastFailure.After(initialTime) {
		t.Error("LastFailure should be updated")
	}

	// Verify backoff was set
	if state.BackoffUntil.IsZero() {
		t.Error("BackoffUntil should be set")
	}

	if !state.BackoffUntil.After(time.Now()) {
		t.Error("BackoffUntil should be in the future")
	}
}

func TestMarkInterfaceFailure_RepeatedFailures(t *testing.T) {
	manager := createMockManager()
	state := createMockInterfaceState("test0", true, true)

	// Mark multiple failures
	for i := 0; i < 3; i++ {
		manager.markInterfaceFailure(state, fmt.Errorf("Test failure"))
		time.Sleep(time.Millisecond) // Small delay to ensure different timestamps
	}

	// Verify failure count increased
	failureCount := atomic.LoadUint64(&state.FailureCount)
	if failureCount != 3 {
		t.Errorf("FailureCount = %v, want %v", failureCount, 3)
	}

	// Verify backoff duration increases with failures
	expectedBackoff := manager.calculateBackoffDuration(state)
	timeDiff := state.BackoffUntil.Sub(state.LastFailure)
	
	// Allow some tolerance for timing
	tolerance := time.Millisecond * 10
	if timeDiff < expectedBackoff-tolerance || timeDiff > expectedBackoff+tolerance {
		t.Errorf("Backoff duration = %v, want ~%v", timeDiff, expectedBackoff)
	}
}

func TestInterfaceHealthEvaluation(t *testing.T) {
	manager := createMockManager()
	
	tests := []struct {
		name        string
		healthScore float64
		expected    bool
	}{
		{
			name:        "Healthy interface",
			healthScore: 0.9,
			expected:    true,
		},
		{
			name:        "Marginally healthy",
			healthScore: 0.7, // At minimum threshold
			expected:    true,
		},
		{
			name:        "Unhealthy interface",
			healthScore: 0.6,
			expected:    false,
		},
		{
			name:        "Very unhealthy interface",
			healthScore: 0.1,
			expected:    false,
		},
	}

	minHealthScore := manager.resilienceConfig.MinHealthScore

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test if interface meets health threshold
			healthy := tt.healthScore >= minHealthScore
			if healthy != tt.expected {
				t.Errorf("Health evaluation = %v, want %v for health score %v (threshold: %v)", 
					healthy, tt.expected, tt.healthScore, minHealthScore)
			}
		})
	}
}

func TestPerformHealthCheck(t *testing.T) {
	manager := createMockManager()

	// Add interfaces with different health scores
	interfaces := map[string]float64{
		"eth0":  1.0,
		"wlan0": 0.8,
		"lo":    0.9,
	}

	manager.mutex.Lock()
	for name, health := range interfaces {
		state := createMockInterfaceState(name, true, true)
		state.HealthScore = health
		manager.interfaces[name] = state
	}
	manager.mutex.Unlock()

	manager.performHealthCheck()

	// Verify overall health was calculated
	overallHealth := manager.healthMonitor.OverallHealth
	if overallHealth < 0.8 || overallHealth > 1.0 {
		t.Errorf("Overall health = %v, seems unreasonable", overallHealth)
	}

	// Verify LastHealthCheck was updated
	if !assertTimestamp(manager.healthMonitor.LastHealthCheck, time.Second) {
		t.Error("LastHealthCheck should be recent")
	}

	// Verify health calculation is approximately correct (average)
	expectedHealth := (1.0 + 0.8 + 0.9) / 3.0
	if abs(overallHealth-expectedHealth) > 0.1 {
		t.Errorf("Overall health = %v, want approximately %v", overallHealth, expectedHealth)
	}
}

// Helper function for floating point comparison
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func TestPerformHealthCheck_NoInterfaces(t *testing.T) {
	manager := createMockManager()

	// Ensure no interfaces
	manager.mutex.Lock()
	manager.interfaces = make(map[string]*InterfaceState)
	manager.mutex.Unlock()

	manager.performHealthCheck()

	// Overall health should be 0.0 when no interfaces
	overallHealth := manager.healthMonitor.OverallHealth
	if overallHealth != 0.0 {
		t.Errorf("Overall health = %v, want 0.0 when no interfaces", overallHealth)
	}

	// Verify LastHealthCheck was updated
	if !assertTimestamp(manager.healthMonitor.LastHealthCheck, time.Second) {
		t.Error("LastHealthCheck should be recent")
	}
}

func TestHealthMonitorInitialization(t *testing.T) {
	manager := createMockManager()
	monitor := manager.healthMonitor

	// Test initial values
	if monitor.OverallHealth != 1.0 {
		t.Errorf("Initial OverallHealth = %v, want %v", monitor.OverallHealth, 1.0)
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

	if monitor.InterfaceHealth == nil {
		t.Error("InterfaceHealth map should be initialized")
	}

	if len(monitor.InterfaceHealth) != 0 {
		t.Error("InterfaceHealth map should be empty initially")
	}
}

func TestHealthMonitorConcurrentMapWrite(t *testing.T) {
	if os.Getenv("MDNS_HEALTH_PANIC_HELPER") == "1" {
		runHealthMonitorConcurrentScenario()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestHealthMonitorConcurrentMapWrite")
	cmd.Env = append(os.Environ(), "MDNS_HEALTH_PANIC_HELPER=1")

	if err := cmd.Run(); err != nil {
		t.Fatalf("health monitor operations should be concurrency-safe: %v", err)
	}
}

func runHealthMonitorConcurrentScenario() {
	manager := NewManager()
	state := createMockInterfaceState("eth0", true, true)

	manager.mutex.Lock()
	manager.interfaces["eth0"] = state
	manager.mutex.Unlock()

	var wg sync.WaitGroup

	// Simulate concurrent failure recording.
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 5000; j++ {
				manager.markInterfaceFailure(state, fmt.Errorf("simulated failure"))
			}
		}()
	}

	// Simultaneously run health checks.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 2000; j++ {
				manager.performHealthCheck()
			}
		}()
	}

	wg.Wait()
}
