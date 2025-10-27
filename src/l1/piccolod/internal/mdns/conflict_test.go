package mdns

import (
	"net"
	"os"
	"os/exec"
	"sync"
	"testing"

	"github.com/miekg/dns"
)

func TestConflictDetectorConcurrentMapWrite(t *testing.T) {
	if os.Getenv("MDNS_CONFLICT_PANIC_HELPER") == "1" {
		runConflictDetectorConcurrentScenario()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestConflictDetectorConcurrentMapWrite")
	cmd.Env = append(os.Environ(), "MDNS_CONFLICT_PANIC_HELPER=1")

	if err := cmd.Run(); err != nil {
		t.Fatalf("conflict detector should be concurrency-safe: %v", err)
	}
}

func runConflictDetectorConcurrentScenario() {
	manager := NewManager()

	msgTemplate := dns.Msg{}
	msgTemplate.Response = true
	msgTemplate.Answer = []dns.RR{
		&dns.A{
			Hdr: dns.RR_Header{
				Name:   manager.finalName + ".local.",
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    120,
			},
			A: net.IPv4(10, 0, 0, 1),
		},
	}

	var wg sync.WaitGroup
	start := make(chan struct{})

	const goroutines = 32
	const loops = 5000

	for i := 0; i < goroutines; i++ {
		wg.Add(1)

		go func(idx int) {
			defer wg.Done()
			<-start
			clientAddr := &net.UDPAddr{IP: net.IPv4(10, 0, 0, byte(idx+1))}

			for j := 0; j < loops; j++ {
				msg := msgTemplate
				msg.Answer = append([]dns.RR(nil), msgTemplate.Answer...)
				manager.handleConflictDetection(&msg, clientAddr)
			}
		}(i)
	}

	close(start)
	wg.Wait()
}
