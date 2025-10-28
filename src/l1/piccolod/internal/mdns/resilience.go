package mdns

import (
	"fmt"
	"log"
	"math"
	"sync/atomic"
	"time"
)

// updateInterfaceHealth calculates and updates interface health score
func (m *Manager) updateInterfaceHealth(state *InterfaceState) {
	now := time.Now()

	// Calculate failure rate over recent period
	totalQueries := atomic.LoadUint64(&state.QueryCount)
	errorCount := atomic.LoadUint64(&state.ErrorCount)
	failureCount := atomic.LoadUint64(&state.FailureCount)

	state.resilienceMu.Lock()
	defer state.resilienceMu.Unlock()

	// Start from existing health to accumulate degradation over time
	healthScore := state.HealthScore

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
	m.healthMonitor.mutex.Lock()
	m.healthMonitor.InterfaceHealth[state.Interface.Name] = healthScore
	m.healthMonitor.mutex.Unlock()

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
	state.resilienceMu.RLock()
	backoffUntil := state.BackoffUntil
	state.resilienceMu.RUnlock()
	return time.Now().Before(backoffUntil)
}

// markInterfaceFailure records a failure and updates resilience tracking
func (m *Manager) markInterfaceFailure(state *InterfaceState, err error) {
	now := time.Now()

	atomic.AddUint64(&state.FailureCount, 1)
	atomic.AddUint64(&state.RecoveryAttempts, 1) // Increment for exponential backoff
	// Calculate and set backoff period
	backoff := m.calculateBackoffDuration(state)

	state.resilienceMu.Lock()
	state.LastFailure = now
	state.BackoffUntil = now.Add(backoff)
	state.resilienceMu.Unlock()

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

	// Note: RecoveryAttempts is already incremented in markInterfaceFailure()
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
	state.resilienceMu.Lock()
	state.BackoffUntil = time.Time{} // Clear backoff
	state.resilienceMu.Unlock()

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

		stateActive := state.Active

		state.resilienceMu.Lock()
		healthScore := state.HealthScore
		if !stateActive {
			healthScore = 0.0
			state.HealthScore = 0.0
		}
		state.resilienceMu.Unlock()

		m.healthMonitor.mutex.Lock()
		m.healthMonitor.InterfaceHealth[name] = healthScore
		m.healthMonitor.mutex.Unlock()

		if healthScore >= m.resilienceConfig.MinHealthScore {
			healthyInterfaces++
		}
		totalHealth += healthScore

		// Attempt recovery for unhealthy interfaces
		if !stateActive || healthScore < m.resilienceConfig.MinHealthScore {
			m.attemptInterfaceRecovery(name, state)
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
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
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
