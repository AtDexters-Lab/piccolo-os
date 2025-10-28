package mdns

import (
	"fmt"
	"log"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"github.com/miekg/dns"
)

// detectNameConflicts probes the network for existing instances of our hostname
func (m *Manager) detectNameConflicts() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	conflictFound := false
	serviceName := m.finalName

	// Send probes on all active interfaces
	for _, state := range m.interfaces {
		if !state.Active {
			continue
		}

		// Probe both IPv4 and IPv6 if available
		if state.HasIPv4 && state.IPv4Conn != nil {
			if m.sendConflictProbe(state, "IPv4", serviceName+".local.") {
				conflictFound = true
			}
		}

		if state.HasIPv6 && state.IPv6Conn != nil {
			if m.sendConflictProbe(state, "IPv6", serviceName+".local.") {
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
	time.Sleep(1000 * time.Millisecond)

	log.Printf("DEBUG: [%s-%s] Sent conflict probe for %s", state.Interface.Name, stack, hostname)
	return false // TODO: Implement response detection
}

// handleConflictDetection processes responses that might indicate conflicts
func (m *Manager) handleConflictDetection(msg *dns.Msg, clientAddr *net.UDPAddr) {
	// Check if this is a response to our hostname query
	for _, answer := range msg.Answer {
		serviceName := m.currentServiceName()

		if !strings.EqualFold(answer.Header().Name, serviceName+".local.") {
			continue
		}

		// Ignore responses from self
		if m.isSelfResponse(clientAddr.IP) {
			log.Printf("DEBUG: Ignoring self-response from %s", clientAddr.IP)
			continue
		}

		// Found a conflict - someone else is responding to our name
		hostKey := clientAddr.IP.String()

		m.conflictDetector.mutex.Lock()

		conflict, exists := m.conflictDetector.ConflictingSources[hostKey]
		if !exists {
			conflict = ConflictingHost{
				IP:         clientAddr.IP,
				FirstSeen:  time.Now(),
				QueryCount: 0,
			}
			log.Printf("CONFLICT: New conflicting host detected: %s for %s.local",
				clientAddr.IP, serviceName)
		}

		conflict.LastSeen = time.Now()
		conflict.QueryCount++
		m.conflictDetector.ConflictingSources[hostKey] = conflict

		shouldResolve := false

		if !m.conflictDetector.ConflictDetected {
			m.conflictDetector.ConflictDetected = true
			log.Printf("CONFLICT: Name conflict detected for %s.local!", serviceName)
			shouldResolve = true
		}

		m.conflictDetector.mutex.Unlock()

		if shouldResolve {
			m.wg.Add(1)
			go func() {
				defer m.wg.Done()
				m.resolveNameConflict()
			}()
		}
	}
}

// isSelfResponse checks if an IP address belongs to any of the local interfaces
func (m *Manager) isSelfResponse(addr net.IP) bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	for _, state := range m.interfaces {
		if !state.Active {
			continue
		}
		addrs, err := state.Interface.Addrs()
		if err != nil {
			log.Printf("ERROR: Failed to get addresses for interface %s: %v", state.Interface.Name, err)
			continue
		}
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok {
				if ipnet.IP.Equal(addr) {
					return true
				}
			}
		}
	}
	return false
}

// resolveNameConflict handles name conflicts using deterministic resolution
func (m *Manager) resolveNameConflict() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.conflictDetector.mutex.Lock()
	defer m.conflictDetector.mutex.Unlock()

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
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		// Send multiple announcements to establish the new name quickly
		for i := 0; i < 3; i++ {
			// Check stop channel to allow early termination
			select {
			case <-m.stopCh:
				return
			default:
				m.sendMultiInterfaceAnnouncements()
				time.Sleep(time.Second)
			}
		}
	}()

	// Clear conflict state after resolution
	m.conflictDetector.ConflictDetected = false
	m.conflictDetector.ConflictingSources = make(map[string]ConflictingHost)
}

// probeNameAvailability performs initial conflict detection during startup
func (m *Manager) probeNameAvailability() error {
	initialName := m.currentServiceName()
	log.Printf("CONFLICT: Probing name availability for %s.local", initialName)

	// Send probes and wait for responses
	if m.detectNameConflicts() {
		log.Printf("CONFLICT: Name conflict detected during startup")
		m.resolveNameConflict()
	}

	// Always wait a bit for any late responses
	time.Sleep(time.Second)

	m.conflictDetector.mutex.RLock()
	conflictDetected := m.conflictDetector.ConflictDetected
	m.conflictDetector.mutex.RUnlock()

	finalName := m.currentServiceName()

	if conflictDetected {
		log.Printf("CONFLICT: Using resolved name: %s.local", finalName)
	} else {
		log.Printf("CONFLICT: No conflicts detected, using: %s.local", finalName)
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
			m.conflictDetector.mutex.Lock()
			m.conflictDetector.LastConflictCheck = time.Now()
			m.conflictDetector.mutex.Unlock()
		}
	}
}
