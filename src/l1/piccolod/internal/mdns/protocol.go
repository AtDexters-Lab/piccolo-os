package mdns

import (
	"log"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/miekg/dns"
)

// interfaceResponder handles dual-stack mDNS queries on a specific interface
func (m *Manager) interfaceResponder(state *InterfaceState) {
	defer m.wg.Done()

	interfaceName := state.Interface.Name
	var localWg sync.WaitGroup

	// Start IPv4 responder if available
	if state.IPv4Conn != nil {
		m.wg.Add(1)
		localWg.Add(1)
		go func() {
			defer localWg.Done()
			m.ipv4Responder(state, interfaceName)
		}()
	}

	// Start IPv6 responder if available
	if state.IPv6Conn != nil {
		m.wg.Add(1)
		localWg.Add(1)
		go func() {
			defer localWg.Done()
			m.ipv6Responder(state, interfaceName)
		}()
	}

	// Wait for child responders to finish
	localWg.Wait()
}

// ipv4Responder handles IPv4 mDNS queries
func (m *Manager) ipv4Responder(state *InterfaceState, interfaceName string) {
	defer m.wg.Done()

	buffer := make([]byte, 1500)

	for {
		select {
		case <-m.stopCh:
			return
		default:
			if state.IPv4Conn == nil {
				return
			}

			state.IPv4Conn.SetReadDeadline(time.Now().Add(1 * time.Second))

			n, clientAddr, err := state.IPv4Conn.ReadFromUDP(buffer)
			if err != nil {
				// Check if we should stop before continuing
				select {
				case <-m.stopCh:
					return
				default:
				}

				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				if strings.Contains(err.Error(), "use of closed network connection") {
					// Connection closed - likely during shutdown
					return
				}
				log.Printf("WARN: IPv4 mDNS read error on %s: %v", interfaceName, err)
				m.markInterfaceFailure(state, err)
				continue
			}

			// Handle query in separate goroutine to avoid blocking UDP reader
			m.wg.Add(1)
			go func(data []byte, addr *net.UDPAddr) {
				defer m.wg.Done()
				m.handleDualStackQuery(data, addr, state, "IPv4")
			}(append([]byte(nil), buffer[:n]...), clientAddr)
		}
	}
}

// ipv6Responder handles IPv6 mDNS queries
func (m *Manager) ipv6Responder(state *InterfaceState, interfaceName string) {
	defer m.wg.Done()

	buffer := make([]byte, 1500)

	for {
		select {
		case <-m.stopCh:
			return
		default:
			if state.IPv6Conn == nil {
				return
			}

			state.IPv6Conn.SetReadDeadline(time.Now().Add(1 * time.Second))

			n, clientAddr, err := state.IPv6Conn.ReadFromUDP(buffer)
			if err != nil {
				// Check if we should stop before continuing
				select {
				case <-m.stopCh:
					return
				default:
				}

				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				if strings.Contains(err.Error(), "use of closed network connection") {
					// Connection closed - likely during shutdown
					return
				}
				log.Printf("WARN: IPv6 mDNS read error on %s: %v", interfaceName, err)
				m.markInterfaceFailure(state, err)
				continue
			}

			// Handle query in separate goroutine to avoid blocking UDP reader
			m.wg.Add(1)
			go func(data []byte, addr *net.UDPAddr) {
				defer m.wg.Done()
				m.handleDualStackQuery(data, addr, state, "IPv6")
			}(append([]byte(nil), buffer[:n]...), clientAddr)
		}
	}
}

// handleDualStackQuery processes mDNS queries with dual-stack support and security
func (m *Manager) handleDualStackQuery(data []byte, clientAddr *net.UDPAddr, state *InterfaceState, stack string) {
	// Try to acquire a processing slot
	if !m.acquireQuerySlot() {
		// Too many concurrent queries, drop this one
		return
	}
	defer m.releaseQuerySlot()

	// Set query timeout
	startTime := time.Now()
	defer func() {
		if time.Since(startTime) > m.securityConfig.QueryTimeout {
			log.Printf("SECURITY: Query timeout from %s", clientAddr.IP)
		}
	}()

	// Update interface metrics
	atomic.AddUint64(&state.QueryCount, 1)
	state.LastQuery = time.Now()

	// Validate packet security
	if err := m.validatePacket(data, clientAddr); err != nil {
		atomic.AddUint64(&state.ErrorCount, 1)
		log.Printf("SECURITY: [%s] Rejected packet from %s: %v", state.Interface.Name, clientAddr.IP, err)
		return
	}

	// Parse DNS message with error handling
	var msg dns.Msg
	if err := msg.Unpack(data); err != nil {
		atomic.AddUint64(&m.securityMetrics.MalformedPackets, 1)
		atomic.AddUint64(&state.ErrorCount, 1)
		log.Printf("SECURITY: [%s] Malformed packet from %s: %v", state.Interface.Name, clientAddr.IP, err)
		return
	}

	// Additional DNS message validation
	if err := m.validateDNSMessage(&msg); err != nil {
		atomic.AddUint64(&state.ErrorCount, 1)
		log.Printf("SECURITY: [%s] Invalid DNS message from %s: %v", state.Interface.Name, clientAddr.IP, err)
		return
	}

	// Handle responses for conflict detection
	if msg.Response {
		m.handleConflictDetection(&msg, clientAddr)
		return
	}

	// Only handle queries
	if msg.Opcode != dns.OpcodeQuery {
		return
	}

	// Build response
	response := &dns.Msg{}
	response.SetReply(&msg)
	response.Authoritative = true
	response.RecursionAvailable = false

	// Process each question with dual-stack support
	for _, q := range msg.Question {
		serviceName := m.currentServiceName()

		if q.Qclass == dns.ClassINET && strings.EqualFold(q.Name, serviceName+".local.") {
			// Handle A record requests (IPv4)
			if (q.Qtype == dns.TypeA || q.Qtype == dns.TypeANY) && state.HasIPv4 && state.IPv4 != nil {
				rr := &dns.A{
					Hdr: dns.RR_Header{
						Name:   q.Name,
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    120,
					},
					A: state.IPv4,
				}
				response.Answer = append(response.Answer, rr)
				log.Printf("DEBUG: [%s-%s] Adding A record: %s -> %s",
					state.Interface.Name, stack, serviceName, state.IPv4.String())
			}

			// Handle AAAA record requests (IPv6)
			if (q.Qtype == dns.TypeAAAA || q.Qtype == dns.TypeANY) && state.HasIPv6 && state.IPv6 != nil {
				rr := &dns.AAAA{
					Hdr: dns.RR_Header{
						Name:   q.Name,
						Rrtype: dns.TypeAAAA,
						Class:  dns.ClassINET,
						Ttl:    120,
					},
					AAAA: state.IPv6,
				}
				response.Answer = append(response.Answer, rr)
				log.Printf("DEBUG: [%s-%s] Adding AAAA record: %s -> %s",
					state.Interface.Name, stack, serviceName, state.IPv6.String())
			}
		}
	}

	// Send response if we have answers
	if len(response.Answer) > 0 {
		// Verify name is still current before transmitting
		currentName := m.currentServiceName()
		expectedName := currentName + ".local."

		for _, rr := range response.Answer {
			if !strings.EqualFold(rr.Header().Name, expectedName) {
				// Name changed while we built the response; drop it.
				return
			}
		}

		if responseData, err := response.Pack(); err == nil {
			// Check response size limit
			if len(responseData) > m.securityConfig.MaxResponseSize {
				log.Printf("SECURITY: [%s] Response too large for %s: %d bytes",
					state.Interface.Name, clientAddr.IP, len(responseData))
				return
			}

			// Choose the appropriate connection based on stack
			var conn *net.UDPConn
			if stack == "IPv4" {
				conn = state.IPv4Conn
			} else {
				conn = state.IPv6Conn
			}

			if conn != nil {
				if _, err := conn.WriteToUDP(responseData, clientAddr); err != nil {
					atomic.AddUint64(&state.ErrorCount, 1)
					log.Printf("WARN: [%s-%s] Failed to send response to %s: %v",
						state.Interface.Name, stack, clientAddr.IP, err)
				} else {
					log.Printf("DEBUG: [%s-%s] Responded to query from %s for %s.local",
						state.Interface.Name, stack, clientAddr.IP, currentName)
				}
			}
		}
	}
}

// announcer sends periodic mDNS announcements on all interfaces
func (m *Manager) announcer() {
	defer m.wg.Done()

	// Send initial announcements
	announcements := []time.Duration{0, 1 * time.Second, 2 * time.Second}
	for _, delay := range announcements {
		select {
		case <-m.stopCh:
			return
		case <-time.After(delay):
			m.sendMultiInterfaceAnnouncements()
		}
	}

	// Periodic announcements
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.sendMultiInterfaceAnnouncements()
		}
	}
}

// sendMultiInterfaceAnnouncements sends dual-stack mDNS announcements on all active interfaces
func (m *Manager) sendMultiInterfaceAnnouncements() {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	serviceName := m.finalName

	for name, state := range m.interfaces {
		if !state.Active {
			continue
		}

		// Send IPv4 announcements
		if state.HasIPv4 && state.IPv4Conn != nil && state.IPv4 != nil {
			m.sendIPv4Announcement(name, state, serviceName)
		}

		// Send IPv6 announcements
		if state.HasIPv6 && state.IPv6Conn != nil && state.IPv6 != nil {
			m.sendIPv6Announcement(name, state, serviceName)
		}
	}
}

// sendIPv4Announcement sends IPv4 mDNS announcement
func (m *Manager) sendIPv4Announcement(name string, state *InterfaceState, serviceName string) {
	msg := &dns.Msg{}
	msg.Response = true
	msg.Authoritative = true
	msg.Opcode = dns.OpcodeQuery

	rr := &dns.A{
		Hdr: dns.RR_Header{
			Name:   serviceName + ".local.",
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    120,
		},
		A: state.IPv4,
	}
	msg.Answer = append(msg.Answer, rr)

	if data, err := msg.Pack(); err == nil {
		multicastAddr := &net.UDPAddr{
			IP:   net.IPv4(224, 0, 0, 251),
			Port: 5353,
		}

		if _, err := state.IPv4Conn.WriteToUDP(data, multicastAddr); err == nil {
			log.Printf("DEBUG: [%s-IPv4] Announced %s.local -> %s",
				name, serviceName, state.IPv4.String())
		} else {
			log.Printf("WARN: Failed to send IPv4 announcement on %s: %v", name, err)
			m.markInterfaceFailure(state, err)
		}
	}
}

// sendIPv6Announcement sends IPv6 mDNS announcement
func (m *Manager) sendIPv6Announcement(name string, state *InterfaceState, serviceName string) {
	msg := &dns.Msg{}
	msg.Response = true
	msg.Authoritative = true
	msg.Opcode = dns.OpcodeQuery

	rr := &dns.AAAA{
		Hdr: dns.RR_Header{
			Name:   serviceName + ".local.",
			Rrtype: dns.TypeAAAA,
			Class:  dns.ClassINET,
			Ttl:    120,
		},
		AAAA: state.IPv6,
	}
	msg.Answer = append(msg.Answer, rr)

	if data, err := msg.Pack(); err == nil {
		multicastAddr := &net.UDPAddr{
			IP:   net.ParseIP("ff02::fb"),
			Port: 5353,
		}

		if _, err := state.IPv6Conn.WriteToUDP(data, multicastAddr); err == nil {
			log.Printf("DEBUG: [%s-IPv6] Announced %s.local -> %s",
				name, serviceName, state.IPv6.String())
		} else {
			log.Printf("WARN: Failed to send IPv6 announcement on %s: %v", name, err)
			m.markInterfaceFailure(state, err)
		}
	}
}
