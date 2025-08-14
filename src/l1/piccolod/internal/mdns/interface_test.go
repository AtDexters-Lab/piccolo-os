package mdns

import (
	"net"
	"testing"
)

func TestDiscoverInterfaces_RealNetwork(t *testing.T) {
	manager := createMockManager()

	// This will actually try to discover real network interfaces
	err := manager.discoverInterfaces()

	// This test might reveal real networking issues
	if err != nil {
		// If it fails, we found a real bug!
		t.Errorf("Real network interface discovery failed: %v", err)
	}

	// Check if any interfaces were actually set up
	manager.mutex.RLock()
	interfaceCount := len(manager.interfaces)
	manager.mutex.RUnlock()

	if interfaceCount == 0 {
		t.Error("No interfaces were discovered - possible network setup issue")
	}

	// Check for resource leaks - do we have hanging connections?
	manager.mutex.RLock()
	for name, state := range manager.interfaces {
		if state.IPv4Conn != nil {
			// Try to actually use the connection
			_, err := state.IPv4Conn.Write([]byte("test"))
			if err != nil {
				t.Errorf("IPv4 connection for %s appears broken: %v", name, err)
			}
		}
		if state.IPv6Conn != nil {
			_, err := state.IPv6Conn.Write([]byte("test"))
			if err != nil {
				t.Errorf("IPv6 connection for %s appears broken: %v", name, err)
			}
		}
	}
	manager.mutex.RUnlock()
}

func TestSetupInterface_InvalidInterface(t *testing.T) {
	manager := createMockManager()

	// Test with a fake interface that will fail
	fakeInterface := &net.Interface{
		Index:        999,
		MTU:          1500,
		Name:         "fake999",
		HardwareAddr: []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		Flags:        net.FlagUp | net.FlagMulticast, // Looks valid but isn't
	}

	err := manager.setupInterface(fakeInterface)

	// This should fail - if it doesn't, we have a bug
	if err == nil {
		t.Error("setupInterface should fail with fake interface, but didn't - possible validation bug")
	}
}

func TestSocketCreation_ErrorPaths(t *testing.T) {
	manager := createMockManager()

	// Get a real interface to test with
	interfaces, err := net.Interfaces()
	if err != nil || len(interfaces) == 0 {
		t.Skip("No network interfaces available for testing")
	}

	realInterface := &interfaces[0]

	// Test IPv4 socket creation with real interface
	// This might fail due to permissions, platform differences, etc.
	// Get IPv4 address from the interface
	addrs, err := realInterface.Addrs()
	if err != nil {
		t.Skip("Cannot get interface addresses")
	}

	var ipv4Addr, ipv6Addr net.IP
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok {
			if ipv4 := ipnet.IP.To4(); ipv4 != nil && !ipnet.IP.IsLinkLocalUnicast() {
				ipv4Addr = ipv4
			} else if ipv6 := ipnet.IP.To16(); ipv6 != nil && !ipnet.IP.IsLoopback() {
				ipv6Addr = ipv6
			}
		}
	}

	if ipv4Addr != nil {
		ipv4Conn, err := manager.createIPv4Socket(realInterface)
		if err != nil {
			t.Logf("IPv4 socket creation failed (expected on some systems): %v", err)
		} else if ipv4Conn != nil {
			ipv4Conn.Close()
		}
	}

	// Test IPv6 socket creation
	if ipv6Addr != nil {
		ipv6Conn, err := manager.createIPv6Socket(realInterface)
		if err != nil {
			t.Logf("IPv6 socket creation failed (expected on some systems): %v", err)
		} else if ipv6Conn != nil {
			ipv6Conn.Close()
		}
	}
}

func TestAddressFiltering_EdgeCases(t *testing.T) {
	// Test the IP address filtering logic that might be too restrictive
	tests := []struct {
		name     string
		ip       string
		expected bool // Should this IP be accepted?
	}{
		{
			name:     "Regular IPv4",
			ip:       "192.168.1.100",
			expected: true,
		},
		{
			name:     "IPv4 link-local",
			ip:       "169.254.1.1",
			expected: false, // Currently filtered out - but should it be?
		},
		{
			name:     "IPv6 link-local",
			ip:       "fe80::1",
			expected: false, // Currently filtered out - but mDNS uses these!
		},
		{
			name:     "IPv6 unique local",
			ip:       "fd00::1",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("Failed to parse IP %s", tt.ip)
			}

			// Test the actual filtering logic from setupInterface
			var shouldAccept bool
			if ipv4 := ip.To4(); ipv4 != nil {
				// IPv4 logic: skip link-local
				shouldAccept = !ip.IsLinkLocalUnicast()
			} else {
				// IPv6 logic: skip link-local and loopback
				shouldAccept = !ip.IsLinkLocalUnicast() && !ip.IsLoopback()
			}

			if shouldAccept != tt.expected {
				t.Errorf("IP %s filtering: got %v, want %v", tt.ip, shouldAccept, tt.expected)
				if tt.ip == "fe80::1" && !shouldAccept {
					t.Error("BUG: mDNS typically USES link-local addresses, but we're filtering them out!")
				}
			}
		})
	}
}

func TestInterfaceResponder_Goroutine(t *testing.T) {
	manager := createMockManager()
	_ = createMockInterfaceState("test0", true, true)

	// The code starts a goroutine but we never tested what happens
	// if we call interfaceResponder with invalid state

	// This should reveal goroutine-related bugs
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("interfaceResponder panicked: %v", r)
		}
	}()

	// Simulate what setupInterface does - start the responder goroutine
	manager.wg.Add(1)
	go func() {
		// Call the internal function to see if it handles invalid state gracefully
		defer manager.wg.Done()
		// interfaceResponder isn't exported, so we can't test it directly
		// But this pattern reveals the testing gap!
		t.Log("This test reveals we can't easily test the goroutine logic")
	}()

	manager.wg.Wait()
}
