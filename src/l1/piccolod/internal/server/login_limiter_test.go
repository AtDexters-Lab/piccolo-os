package server

import (
	"testing"
	"time"
)

func TestLoginRateLimiter(t *testing.T) {
	limiter := newLoginRateLimiter()
	base := time.Now()
	limiter.now = func() time.Time { return base }
	key := "user"

	for i := 0; i < limiter.maxFailures; i++ {
		allowed, _ := limiter.allow(key)
		if !allowed {
			t.Fatalf("expected attempt %d to be allowed", i+1)
		}
		dur := limiter.recordFailure(key)
		if i < limiter.maxFailures-1 && dur != 0 {
			t.Fatalf("unexpected lockout before limit on attempt %d", i+1)
		}
		if i == limiter.maxFailures-1 && dur <= 0 {
			t.Fatalf("expected lockout on attempt %d", i+1)
		}
	}

	allowed, retry := limiter.allow(key)
	if allowed || retry <= 0 {
		t.Fatalf("expected lockout with retry=%v", retry)
	}

	base = base.Add(limiter.lockout + time.Second)
	allowed, _ = limiter.allow(key)
	if !allowed {
		t.Fatalf("expected attempt allowed after lockout")
	}
	limiter.recordSuccess(key)
	allowed, _ = limiter.allow(key)
	if !allowed {
		t.Fatalf("expected clean state after success")
	}
}
