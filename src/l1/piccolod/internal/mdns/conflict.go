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
	
	// Send probes on all active interfaces
	for _, state := range m.interfaces {
		if !state.Active {
			continue
		}
		
		// Probe both IPv4 and IPv6 if available
		if state.HasIPv4 && state.IPv4Conn != nil {
			if m.sendConflictProbe(state, "IPv4", m.finalName+".local.") {
				conflictFound = true
			}
		}
		
		if state.HasIPv6 && state.IPv6Conn != nil {
			if m.sendConflictProbe(state, "IPv6", m.finalName+".local.") {
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
	time.Sleep(250 * time.Millisecond)
	
	log.Printf("DEBUG: [%s-%s] Sent conflict probe for %s", state.Interface.Name, stack, hostname)
	return false // TODO: Implement response detection
}

// handleConflictDetection processes responses that might indicate conflicts
func (m *Manager) handleConflictDetection(msg *dns.Msg, clientAddr *net.UDPAddr) {
	// Check if this is a response to our hostname query
	for _, answer := range msg.Answer {
		if strings.EqualFold(answer.Header().Name, m.finalName+".local.") {
			// Found a conflict - someone else is responding to our name
			hostKey := clientAddr.IP.String()
			
			conflict, exists := m.conflictDetector.ConflictingSources[hostKey]
			if !exists {
				conflict = ConflictingHost{
					IP:        clientAddr.IP,
					FirstSeen: time.Now(),
					QueryCount: 0,
				}
				log.Printf("CONFLICT: New conflicting host detected: %s for %s.local", 
					clientAddr.IP, m.finalName)
			}
			
			conflict.LastSeen = time.Now()
			conflict.QueryCount++
			m.conflictDetector.ConflictingSources[hostKey] = conflict
			
			if !m.conflictDetector.ConflictDetected {
				m.conflictDetector.ConflictDetected = true
				log.Printf("CONFLICT: Name conflict detected for %s.local!", m.finalName)
				go m.resolveNameConflict()
			}
		}
	}
}

// resolveNameConflict handles name conflicts using deterministic resolution
func (m *Manager) resolveNameConflict() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
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
	go func() {
		// Send multiple announcements to establish the new name quickly
		for i := 0; i < 3; i++ {
			m.sendMultiInterfaceAnnouncements()
			time.Sleep(time.Second)
		}
	}()
	
	// Clear conflict state after resolution
	m.conflictDetector.ConflictDetected = false
	m.conflictDetector.ConflictingSources = make(map[string]ConflictingHost)
}

// probeNameAvailability performs initial conflict detection during startup
func (m *Manager) probeNameAvailability() error {
	log.Printf("CONFLICT: Probing name availability for %s.local", m.finalName)
	
	// Send probes and wait for responses
	if m.detectNameConflicts() {
		log.Printf("CONFLICT: Name conflict detected during startup")
		m.resolveNameConflict()
	}
	
	// Always wait a bit for any late responses
	time.Sleep(time.Second)
	
	if m.conflictDetector.ConflictDetected {
		log.Printf("CONFLICT: Using resolved name: %s.local", m.finalName)
	} else {
		log.Printf("CONFLICT: No conflicts detected, using: %s.local", m.finalName)
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
			m.conflictDetector.LastConflictCheck = time.Now()
		}
	}
}