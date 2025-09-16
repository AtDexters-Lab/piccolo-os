package server

import (
	"math"
	"strconv"
	"sync"
	"time"
)

type loginAttempt struct {
	failures     int
	resetAt      time.Time
	blockedUntil time.Time
}

type loginRateLimiter struct {
	mu          sync.Mutex
	entries     map[string]*loginAttempt
	maxFailures int
	window      time.Duration
	lockout     time.Duration
	now         func() time.Time
}

func newLoginRateLimiter() *loginRateLimiter {
	return &loginRateLimiter{
		entries:     make(map[string]*loginAttempt),
		maxFailures: 5,
		window:      15 * time.Minute,
		lockout:     15 * time.Minute,
		now:         time.Now,
	}
}

// allow reports whether the caller is allowed to attempt a login.
// When false, the returned duration represents the remaining lockout window.
func (l *loginRateLimiter) allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	entry, ok := l.entries[key]
	if !ok {
		return true, 0
	}
	if entry.blockedUntil.After(now) {
		return false, entry.blockedUntil.Sub(now)
	}
	if !entry.resetAt.IsZero() && now.After(entry.resetAt) {
		delete(l.entries, key)
		return true, 0
	}
	return true, 0
}

// recordFailure updates the counter for key and returns a lockout duration when the limit is exceeded.
func (l *loginRateLimiter) recordFailure(key string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	entry := l.entries[key]
	if entry == nil {
		entry = &loginAttempt{}
		l.entries[key] = entry
	}
	if !entry.resetAt.IsZero() && now.After(entry.resetAt) {
		entry.failures = 0
		entry.blockedUntil = time.Time{}
	}
	entry.failures++
	entry.resetAt = now.Add(l.window)
	if entry.failures >= l.maxFailures {
		entry.blockedUntil = now.Add(l.lockout)
		return l.lockout
	}
	return 0
}

func (l *loginRateLimiter) recordSuccess(key string) {
	l.mu.Lock()
	delete(l.entries, key)
	l.mu.Unlock()
}

func retryAfterSeconds(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	secs := int(math.Ceil(d.Seconds()))
	if secs < 1 {
		secs = 1
	}
	return strconv.Itoa(secs)
}
