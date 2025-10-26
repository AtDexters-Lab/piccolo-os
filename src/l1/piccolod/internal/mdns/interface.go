package mdns

import (
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"syscall"
	"time"

	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

var (
	listNetworkInterfaces = net.Interfaces
	interfaceAddrs        = func(iface *net.Interface) ([]net.Addr, error) {
		return iface.Addrs()
	}
)

// discoverInterfaces finds and sets up all suitable network interfaces
func (m *Manager) discoverInterfaces() error {
	interfaces, err := listNetworkInterfaces()
	if err != nil {
		return err
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	activeCount := 0
	for _, iface := range interfaces {
		ifaceCopy := iface
		if err := m.setupInterface(&ifaceCopy); err != nil {
			log.Printf("WARN: Failed to setup interface %s: %v", iface.Name, err)
			// Track interface setup failure for resilience
			if state, exists := m.interfaces[iface.Name]; exists {
				m.markInterfaceFailure(state, err)
			}
			continue
		}
		activeCount++
	}

	if activeCount == 0 {
		log.Printf("WARN: No active network interfaces found during initial discovery")
	} else {
		log.Printf("INFO: Successfully configured %d network interfaces for mDNS", activeCount)
	}

	return nil
}

// setupInterface configures dual-stack mDNS for a specific network interface
func (m *Manager) setupInterface(iface *net.Interface) error {
	// Skip loopback and down interfaces
	if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
		return fmt.Errorf("interface %s not suitable (loopback or down)", iface.Name)
	}

	// Get all addresses for this interface
	addrs, err := interfaceAddrs(iface)
	if err != nil {
		return err
	}

	var ipv4Addr, ipv6Addr net.IP

	// Find IPv4 and IPv6 addresses
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok {
			if ipv4 := ipnet.IP.To4(); ipv4 != nil {
				// IPv4 address - skip link-local
				if !ipnet.IP.IsLinkLocalUnicast() {
					ipv4Addr = ipv4
				}
			} else if ipv6 := ipnet.IP.To16(); ipv6 != nil {
				// IPv6 address - accept link-local (required by RFC 6762), skip only loopback
				// RFC 6762 Section 15: "Multicast DNS operates over link-local scope"
				if !ipnet.IP.IsLoopback() {
					ipv6Addr = ipv6
				}
			}
		}
	}

	// Need at least one IP stack
	if ipv4Addr == nil && ipv6Addr == nil {
		return fmt.Errorf("no suitable IP addresses on interface %s", iface.Name)
	}

	// Create interface state
	state := &InterfaceState{
		Interface:   iface,
		IPv4:        ipv4Addr,
		IPv6:        ipv6Addr,
		HasIPv4:     ipv4Addr != nil,
		HasIPv6:     ipv6Addr != nil,
		Active:      true,
		LastSeen:    time.Now(),
		HealthScore: 1.0, // Start with perfect health
	}

	// Setup IPv4 socket if available
	if state.HasIPv4 {
		factory := m.ipv4SocketFactory
		if factory == nil {
			factory = m.createIPv4Socket
		}
		ipv4Conn, err := factory(iface)
		if err != nil {
			log.Printf("WARN: Failed to create IPv4 socket for %s: %v", iface.Name, err)
		} else {
			state.IPv4Conn = ipv4Conn
		}
	}

	// Setup IPv6 socket if available
	if state.HasIPv6 {
		factory := m.ipv6SocketFactory
		if factory == nil {
			factory = m.createIPv6Socket
		}
		ipv6Conn, err := factory(iface)
		if err != nil {
			log.Printf("WARN: Failed to create IPv6 socket for %s: %v", iface.Name, err)
		} else {
			state.IPv6Conn = ipv6Conn
		}
	}

	// Need at least one working socket
	if state.IPv4Conn == nil && state.IPv6Conn == nil {
		return fmt.Errorf("failed to create any sockets for interface %s", iface.Name)
	}

	// Store in manager
	m.interfaces[iface.Name] = state

	// Start responder for this interface
	m.wg.Add(1)
	go m.interfaceResponder(state)

	var addrInfo []string
	if state.HasIPv4 {
		addrInfo = append(addrInfo, fmt.Sprintf("IPv4:%s", ipv4Addr.String()))
	}
	if state.HasIPv6 {
		addrInfo = append(addrInfo, fmt.Sprintf("IPv6:%s", ipv6Addr.String()))
	}

	log.Printf("INFO: Interface %s ready - %s", iface.Name, strings.Join(addrInfo, ", "))
	return nil
}

// createIPv4Socket creates an IPv4 UDP socket bound to a specific interface
func (m *Manager) createIPv4Socket(iface *net.Interface) (*net.UDPConn, error) {
	// Create raw socket with SO_REUSEPORT
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, syscall.IPPROTO_UDP)
	if err != nil {
		return nil, fmt.Errorf("failed to create IPv4 socket: %w", err)
	}

	// Set socket options
	if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1); err != nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("failed to set SO_REUSEADDR: %w", err)
	}

	// SO_REUSEADDR is sufficient for single-daemon mDNS (no need for SO_REUSEPORT)
	// SO_REUSEADDR already set above - no additional port sharing needed

	// Bind to specific interface using SO_BINDTODEVICE
	if err := syscall.SetsockoptString(fd, syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, iface.Name); err != nil {
		log.Printf("WARN: Failed to bind IPv4 to device %s: %v", iface.Name, err)
	}

	// Bind to mDNS port
	addr := &syscall.SockaddrInet4{Port: 5353}
	copy(addr.Addr[:], net.IPv4zero.To4())

	if err := syscall.Bind(fd, addr); err != nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("failed to bind IPv4 to :5353: %w", err)
	}

	// Convert to net.UDPConn
	file := os.NewFile(uintptr(fd), fmt.Sprintf("mdns4-%s", iface.Name))
	if file == nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("failed to create file from IPv4 socket")
	}
	defer file.Close()

	fileConn, err := net.FileConn(file)
	if err != nil {
		return nil, fmt.Errorf("failed to create IPv4 connection: %w", err)
	}

	conn, ok := fileConn.(*net.UDPConn)
	if !ok {
		fileConn.Close()
		return nil, fmt.Errorf("failed to convert to IPv4 UDPConn")
	}

	// Join IPv4 multicast group on this interface
	pc := ipv4.NewPacketConn(conn)
	group := &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251)}
	if err := pc.JoinGroup(iface, group); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to join IPv4 multicast group on %s: %w", iface.Name, err)
	}

	// Set multicast interface
	if err := pc.SetMulticastInterface(iface); err != nil {
		log.Printf("WARN: Failed to set IPv4 multicast interface %s: %v", iface.Name, err)
	}

	return conn, nil
}

// createIPv6Socket creates an IPv6 UDP socket bound to a specific interface
func (m *Manager) createIPv6Socket(iface *net.Interface) (*net.UDPConn, error) {
	// Create raw socket with SO_REUSEPORT
	fd, err := syscall.Socket(syscall.AF_INET6, syscall.SOCK_DGRAM, syscall.IPPROTO_UDP)
	if err != nil {
		return nil, fmt.Errorf("failed to create IPv6 socket: %w", err)
	}

	// Set socket options
	if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1); err != nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("failed to set SO_REUSEADDR on IPv6: %w", err)
	}

	// SO_REUSEADDR is sufficient for single-daemon mDNS (no need for SO_REUSEPORT)
	// SO_REUSEADDR already set above - no additional port sharing needed

	// Disable IPv6 only to allow dual-stack
	if err := syscall.SetsockoptInt(fd, syscall.IPPROTO_IPV6, syscall.IPV6_V6ONLY, 0); err != nil {
		log.Printf("WARN: Failed to disable IPv6-only mode on %s: %v", iface.Name, err)
	}

	// Bind to specific interface using SO_BINDTODEVICE
	if err := syscall.SetsockoptString(fd, syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, iface.Name); err != nil {
		log.Printf("WARN: Failed to bind IPv6 to device %s: %v", iface.Name, err)
	}

	// Bind to mDNS port on IPv6
	addr := &syscall.SockaddrInet6{Port: 5353}
	copy(addr.Addr[:], net.IPv6zero.To16())

	if err := syscall.Bind(fd, addr); err != nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("failed to bind IPv6 to :5353: %w", err)
	}

	// Convert to net.UDPConn
	file := os.NewFile(uintptr(fd), fmt.Sprintf("mdns6-%s", iface.Name))
	if file == nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("failed to create file from IPv6 socket")
	}
	defer file.Close()

	fileConn, err := net.FileConn(file)
	if err != nil {
		return nil, fmt.Errorf("failed to create IPv6 connection: %w", err)
	}

	conn, ok := fileConn.(*net.UDPConn)
	if !ok {
		fileConn.Close()
		return nil, fmt.Errorf("failed to convert to IPv6 UDPConn")
	}

	// Join IPv6 multicast group on this interface
	pc := ipv6.NewPacketConn(conn)
	group := &net.UDPAddr{IP: net.ParseIP("ff02::fb")}
	if err := pc.JoinGroup(iface, group); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to join IPv6 multicast group on %s: %w", iface.Name, err)
	}

	// Set multicast interface
	if err := pc.SetMulticastInterface(iface); err != nil {
		log.Printf("WARN: Failed to set IPv6 multicast interface %s: %v", iface.Name, err)
	}

	return conn, nil
}

// networkMonitor continuously monitors network interface changes
func (m *Manager) networkMonitor() {
	defer m.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.checkInterfaceChanges()
		}
	}
}

// checkInterfaceChanges detects and handles interface changes
func (m *Manager) checkInterfaceChanges() {
	interfaces, err := listNetworkInterfaces()
	if err != nil {
		log.Printf("WARN: Failed to check interfaces: %v", err)
		return
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	seenInterfaces := make(map[string]bool)

	// Check each interface
	for _, iface := range interfaces {
		ifaceCopy := iface
		seenInterfaces[ifaceCopy.Name] = true
		if existing, exists := m.interfaces[ifaceCopy.Name]; exists {
			// Interface still exists - check if IP changed
			if m.hasIPChanged(&ifaceCopy, existing) {
				log.Printf("INFO: IP changed on interface %s, reconfiguring", ifaceCopy.Name)
				if existing.IPv4Conn != nil {
					existing.IPv4Conn.Close()
				}
				if existing.IPv6Conn != nil {
					existing.IPv6Conn.Close()
				}
				m.setupInterface(&ifaceCopy)
			} else {
				existing.Active = true
				existing.LastSeen = time.Now()
			}
		} else {
			// New interface detected
			log.Printf("INFO: New interface detected: %s", ifaceCopy.Name)
			m.setupInterface(&ifaceCopy)
		}
	}

	// Remove interfaces that no longer exist
	for name, state := range m.interfaces {
		if !seenInterfaces[name] {
			log.Printf("INFO: Interface %s no longer available, removing", name)
			if state.IPv4Conn != nil {
				state.IPv4Conn.Close()
			}
			if state.IPv6Conn != nil {
				state.IPv6Conn.Close()
			}
			delete(m.interfaces, name)
		}
	}
}

// hasIPChanged checks if an interface's IPv4 or IPv6 addresses have changed
func (m *Manager) hasIPChanged(iface *net.Interface, state *InterfaceState) bool {
	addrs, err := interfaceAddrs(iface)
	if err != nil {
		return true // Assume changed if we can't check
	}

	var newIPv4, newIPv6 net.IP

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok {
			if ipv4 := ipnet.IP.To4(); ipv4 != nil {
				if !ipnet.IP.IsLinkLocalUnicast() {
					newIPv4 = ipv4
				}
			} else if ipv6 := ipnet.IP.To16(); ipv6 != nil {
				if !ipnet.IP.IsLoopback() {
					newIPv6 = ipv6
				}
			}
		}
	}

	ipv4Changed := !state.IPv4.Equal(newIPv4)
	ipv6Changed := !state.IPv6.Equal(newIPv6)

	return ipv4Changed || ipv6Changed
}
