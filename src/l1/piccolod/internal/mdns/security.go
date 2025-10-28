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

// isRateLimited checks if a client IP is rate limited
func (m *Manager) isRateLimited(clientIP string) bool {
	m.rateLimiter.mutex.Lock()
	defer m.rateLimiter.mutex.Unlock()

	now := time.Now()
	client, exists := m.rateLimiter.clients[clientIP]

	if !exists {
		// New client
		m.rateLimiter.clients[clientIP] = &ClientState{
			IP:        clientIP,
			LastQuery: now,
		}
		atomic.AddUint64(&m.securityMetrics.TotalQueries, 1)
		return false
	}

	// Check if client is currently blocked
	if client.Blocked && now.Before(client.BlockedUntil) {
		atomic.AddUint64(&m.securityMetrics.BlockedQueries, 1)
		atomic.AddUint64(&m.securityMetrics.TotalQueries, 1)
		return true
	}

	// Reset block status if expired
	if client.Blocked && now.After(client.BlockedUntil) {
		client.Blocked = false
		client.QueryCount = 0
	}

	// Check rate limits
	timeSinceLastQuery := now.Sub(client.LastQuery)

	// Reset counter if more than a minute has passed
	if timeSinceLastQuery > time.Minute {
		client.QueryCount = 0
	}

	// Increment query count and update security metrics
	client.QueryCount++
	client.LastQuery = now
	atomic.AddUint64(&m.securityMetrics.TotalQueries, 1)

	// Check per-second rate limit
	if timeSinceLastQuery < time.Second && client.QueryCount > uint64(m.securityConfig.MaxQueriesPerSecond) {
		m.blockClient(client, now)
		atomic.AddUint64(&m.securityMetrics.RateLimitHits, 1)
		return true
	}

	// Check per-minute rate limit
	if client.QueryCount > uint64(m.securityConfig.MaxQueriesPerMinute) {
		m.blockClient(client, now)
		atomic.AddUint64(&m.securityMetrics.RateLimitHits, 1)
		return true
	}

	return false
}

// blockClient blocks a client for the configured duration
func (m *Manager) blockClient(client *ClientState, now time.Time) {
	client.Blocked = true
	client.BlockedUntil = now.Add(m.securityConfig.ClientBlockDuration)
	log.Printf("SECURITY: Blocked client %s for %v due to rate limiting",
		client.IP, m.securityConfig.ClientBlockDuration)
}

// validatePacket performs security validation on incoming packets
func (m *Manager) validatePacket(data []byte, clientAddr *net.UDPAddr) error {
	// Check packet size
	if len(data) > m.securityConfig.MaxPacketSize {
		atomic.AddUint64(&m.securityMetrics.TotalQueries, 1)
		atomic.AddUint64(&m.securityMetrics.LargePackets, 1)
		return fmt.Errorf("packet too large: %d bytes", len(data))
	}

	if len(data) < 12 { // Minimum DNS header size
		atomic.AddUint64(&m.securityMetrics.TotalQueries, 1)
		atomic.AddUint64(&m.securityMetrics.MalformedPackets, 1)
		return fmt.Errorf("packet too small: %d bytes", len(data))
	}

	// Check rate limiting
	if m.isRateLimited(clientAddr.IP.String()) {
		return fmt.Errorf("client rate limited: %s", clientAddr.IP.String())
	}

	return nil
}

// validateDNSMessage performs DNS-specific validation
func (m *Manager) validateDNSMessage(msg *dns.Msg) error {
	// Check for DNS query bombs
	if len(msg.Question) > 10 {
		return fmt.Errorf("too many questions: %d", len(msg.Question))
	}

	// RFC 6762 Section 8.1: Probing queries legitimately contain answer sections
	// Only reject queries with answers if they look like obvious self-loops or malformed packets
	// Allow probing queries (which have both questions and answers) as they are valid mDNS
	if len(msg.Answer) > 0 && len(msg.Question) == 0 {
		// Responses should not be processed as queries
		return fmt.Errorf("message has answers but no questions (not a valid query)")
	}

	// Allow reasonable number of answers in probing queries
	if len(msg.Answer) > 10 {
		return fmt.Errorf("too many answer records in query: %d", len(msg.Answer))
	}

	if len(msg.Extra) > 100 {
		return fmt.Errorf("too many extra records: %d", len(msg.Extra))
	}

	// Validate question types
	for _, q := range msg.Question {
		if q.Qclass != dns.ClassINET {
			return fmt.Errorf("unsupported query class: %d", q.Qclass)
		}

		if q.Qtype != dns.TypeA && q.Qtype != dns.TypeAAAA && q.Qtype != dns.TypeANY {
			return fmt.Errorf("unsupported query type: %d", q.Qtype)
		}

		// Validate hostname
		if !strings.HasSuffix(q.Name, ".local.") {
			return fmt.Errorf("non-local query: %s", q.Name)
		}

		if len(q.Name) > 253 { // DNS name length limit
			return fmt.Errorf("hostname too long: %d", len(q.Name))
		}
	}

	return nil
}

// acquireQuerySlot tries to acquire a processing slot for concurrent queries
func (m *Manager) acquireQuerySlot() bool {
	select {
	case m.queryProcessor.semaphore <- struct{}{}:
		atomic.AddInt64(&m.queryProcessor.activeCount, 1)
		return true
	default:
		// No slot available
		return false
	}
}

// releaseQuerySlot releases a processing slot
func (m *Manager) releaseQuerySlot() {
	<-m.queryProcessor.semaphore
	atomic.AddInt64(&m.queryProcessor.activeCount, -1)
}

// cleanupSecurityState periodically cleans up old client states
func (m *Manager) cleanupSecurityState() {
	defer m.wg.Done()
	ticker := time.NewTicker(m.securityConfig.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.performSecurityCleanup()
		}
	}
}

// performSecurityCleanup removes old client states and logs security metrics
func (m *Manager) performSecurityCleanup() {
	m.rateLimiter.mutex.Lock()
	defer m.rateLimiter.mutex.Unlock()

	now := time.Now()
	cleanupThreshold := now.Add(-time.Minute * 15) // Remove clients inactive for 15 minutes

	for ip, client := range m.rateLimiter.clients {
		if client.LastQuery.Before(cleanupThreshold) && !client.Blocked {
			delete(m.rateLimiter.clients, ip)
		}
	}

	// Log security metrics
	log.Printf("SECURITY: Metrics - Total: %d, Blocked: %d, Malformed: %d, RateLimit: %d, Large: %d, Active: %d",
		atomic.LoadUint64(&m.securityMetrics.TotalQueries),
		atomic.LoadUint64(&m.securityMetrics.BlockedQueries),
		atomic.LoadUint64(&m.securityMetrics.MalformedPackets),
		atomic.LoadUint64(&m.securityMetrics.RateLimitHits),
		atomic.LoadUint64(&m.securityMetrics.LargePackets),
		atomic.LoadInt64(&m.queryProcessor.activeCount))
}
