package mdns

import (
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/miekg/dns"
	"golang.org/x/net/ipv4"
)

// Manager handles mDNS advertising for the Piccolo service
type Manager struct {
	conn     *net.UDPConn
	hostname string
	ip       net.IP
	port     int
	stopCh   chan struct{}
}

// NewManager creates a new mDNS manager
func NewManager() *Manager {
	return &Manager{
		hostname: "piccolo",
		port:     8080,
		stopCh:   make(chan struct{}),
	}
}

// Start begins advertising the service via mDNS
func (m *Manager) Start() error {
	// Get the primary local IP address
	ip, err := getPrimaryLocalIP()
	if err != nil {
		return fmt.Errorf("failed to get local IP: %w", err)
	}
	m.ip = ip
	log.Printf("INFO: Starting mDNS server for %s.local -> %s", m.hostname, ip.String())

	// Create mDNS socket with proper configuration
	conn, err := m.createMDNSSocket()
	if err != nil {
		return fmt.Errorf("failed to create mDNS socket: %w", err)
	}
	m.conn = conn

	// Start the mDNS responder
	go m.responder()
	
	// Start the announcer
	go m.announcer()

	log.Printf("INFO: mDNS server started - %s.local should resolve to %s", m.hostname, ip.String())
	return nil
}

// createMDNSSocket creates and configures a UDP socket for mDNS
func (m *Manager) createMDNSSocket() (*net.UDPConn, error) {
	// Create raw socket with proper options for port sharing
	conn, err := m.createRawSocket()
	if err != nil {
		return nil, err
	}

	// Join the mDNS multicast group
	if err := m.joinMulticastGroup(conn); err != nil {
		conn.Close()
		return nil, err
	}

	return conn, nil
}

// createRawSocket creates a UDP socket with SO_REUSEPORT for mDNS
func (m *Manager) createRawSocket() (*net.UDPConn, error) {
	// Create raw socket
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, syscall.IPPROTO_UDP)
	if err != nil {
		return nil, fmt.Errorf("failed to create socket: %w", err)
	}

	// Set SO_REUSEADDR
	if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1); err != nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("failed to set SO_REUSEADDR: %w", err)
	}

	// Set SO_REUSEPORT (15 is the Linux value for SO_REUSEPORT)
	const SO_REUSEPORT = 15
	if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, SO_REUSEPORT, 1); err != nil {
		log.Printf("WARN: Failed to set SO_REUSEPORT: %v", err)
		// Continue without SO_REUSEPORT - might still work
	}

	// Bind to mDNS port
	addr := &syscall.SockaddrInet4{Port: 5353}
	copy(addr.Addr[:], net.IPv4zero.To4())
	
	if err := syscall.Bind(fd, addr); err != nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("failed to bind to :5353: %w", err)
	}

	// Convert to net.UDPConn
	file := os.NewFile(uintptr(fd), "mdns")
	if file == nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("failed to create file from socket")
	}
	defer file.Close()
	
	fileConn, err := net.FileConn(file)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection: %w", err)
	}
	
	conn, ok := fileConn.(*net.UDPConn)
	if !ok {
		fileConn.Close()
		return nil, fmt.Errorf("failed to convert to UDPConn")
	}

	return conn, nil
}

// joinMulticastGroup joins the mDNS multicast group
func (m *Manager) joinMulticastGroup(conn *net.UDPConn) error {
	pc := ipv4.NewPacketConn(conn)
	
	// Find a suitable network interface
	intf := m.findBestInterface()
	
	// Join the mDNS multicast group (224.0.0.251)
	group := &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251)}
	if err := pc.JoinGroup(intf, group); err != nil {
		return fmt.Errorf("failed to join multicast group: %w", err)
	}

	// Set multicast loop to ensure we can receive our own packets if needed
	if err := pc.SetMulticastLoopback(true); err != nil {
		log.Printf("WARN: Could not set multicast loopback: %v", err)
	}

	return nil
}

// findBestInterface finds the best network interface for mDNS
func (m *Manager) findBestInterface() *net.Interface {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	for _, intf := range interfaces {
		// Skip loopback and down interfaces
		if intf.Flags&net.FlagLoopback != 0 || intf.Flags&net.FlagUp == 0 {
			continue
		}

		// Check if this interface has our target IP
		addrs, err := intf.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok {
				if ipnet.IP.Equal(m.ip) {
					return &intf
				}
			}
		}
	}

	return nil // Use default interface
}

// Stop shuts down the mDNS server
func (m *Manager) Stop() error {
	close(m.stopCh)
	if m.conn != nil {
		m.conn.Close()
	}
	log.Printf("INFO: Stopped mDNS server")
	return nil
}

// responder handles incoming mDNS queries
func (m *Manager) responder() {
	buffer := make([]byte, 1500)
	
	for {
		select {
		case <-m.stopCh:
			return
		default:
			// Set a read timeout to allow checking for stop signal
			m.conn.SetReadDeadline(time.Now().Add(1 * time.Second))
			
			n, clientAddr, err := m.conn.ReadFromUDP(buffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				if !strings.Contains(err.Error(), "use of closed network connection") {
					log.Printf("WARN: mDNS read error: %v", err)
				}
				continue
			}

			// Handle the query in a separate goroutine
			go m.handleQuery(buffer[:n], clientAddr)
		}
	}
}

// handleQuery processes an incoming mDNS query
func (m *Manager) handleQuery(data []byte, clientAddr *net.UDPAddr) {
	var msg dns.Msg
	if err := msg.Unpack(data); err != nil {
		return
	}

	// Skip responses and non-queries
	if msg.Response || msg.Opcode != dns.OpcodeQuery {
		return
	}

	response := &dns.Msg{}
	response.SetReply(&msg)
	response.Authoritative = true
	response.RecursionAvailable = false

	// Process each question
	for _, q := range msg.Question {
		if q.Qclass == dns.ClassINET && strings.EqualFold(q.Name, m.hostname+".local.") {
			if q.Qtype == dns.TypeA || q.Qtype == dns.TypeANY {
				// Create A record
				rr := &dns.A{
					Hdr: dns.RR_Header{
						Name:   q.Name,
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    120,
					},
					A: m.ip,
				}
				response.Answer = append(response.Answer, rr)
			}
		}
	}

	// Send response if we have answers
	if len(response.Answer) > 0 {
		if responseData, err := response.Pack(); err == nil {
			m.conn.WriteToUDP(responseData, clientAddr)
			log.Printf("DEBUG: Responded to mDNS query from %s for %s.local", clientAddr.IP, m.hostname)
		}
	}
}

// announcer sends periodic mDNS announcements
func (m *Manager) announcer() {
	// Send initial announcements immediately and then at intervals
	announcements := []time.Duration{0, 1 * time.Second, 2 * time.Second}
	
	// Send initial announcements
	for _, delay := range announcements {
		select {
		case <-m.stopCh:
			return
		case <-time.After(delay):
			m.sendAnnouncement()
		}
	}

	// Continue with periodic announcements
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.sendAnnouncement()
		}
	}
}

// sendAnnouncement sends an unsolicited mDNS announcement
func (m *Manager) sendAnnouncement() {
	msg := &dns.Msg{}
	msg.Response = true
	msg.Authoritative = true
	msg.Opcode = dns.OpcodeQuery

	// Create A record announcement
	rr := &dns.A{
		Hdr: dns.RR_Header{
			Name:   m.hostname + ".local.",
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    120,
		},
		A: m.ip,
	}
	msg.Answer = append(msg.Answer, rr)

	// Pack and send
	if data, err := msg.Pack(); err == nil {
		multicastAddr := &net.UDPAddr{
			IP:   net.IPv4(224, 0, 0, 251),
			Port: 5353,
		}
		
		if _, err := m.conn.WriteToUDP(data, multicastAddr); err == nil {
			log.Printf("DEBUG: Sent mDNS announcement: %s.local -> %s", m.hostname, m.ip.String())
		} else {
			log.Printf("WARN: Failed to send mDNS announcement: %v", err)
		}
	}
}

// getPrimaryLocalIP returns the primary non-loopback local IP of the host
func getPrimaryLocalIP() (net.IP, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP, nil
}