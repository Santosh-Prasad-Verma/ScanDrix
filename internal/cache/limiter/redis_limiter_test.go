package limiter

import (
	"context"
	"testing"
	"time"
)

func TestRedisTokenBucketLimiterFallback(t *testing.T) {
	config := RateLimitConfig{
		Capacity:          3,
		RefillRatePerSec:  0.01,
		ExpirationTimeout: 1 * time.Minute,
	}

	// Create with nil Redis client -> must gracefully fallback to in-memory limiter
	limiter := NewRedisTokenBucketLimiter(nil, config)

	ctx := context.Background()
	key := "test-client-ip"

	// First 3 requests should be allowed with Degraded = true
	for i := 1; i <= 3; i++ {
		res, err := limiter.Allow(ctx, key, 1)
		if err != nil {
			t.Fatalf("unexpected error on request %d: %v", i, err)
		}
		if !res.Allowed {
			t.Fatalf("expected request %d to be allowed, got denied", i)
		}
		if !res.Degraded {
			t.Fatalf("expected Degraded flag to be true during fallback, got false")
		}
	}

	// 4th request must be denied
	res4, err := limiter.Allow(ctx, key, 1)
	if err != nil {
		t.Fatalf("unexpected error on request 4: %v", err)
	}
	if res4.Allowed {
		t.Fatal("expected request 4 to be throttled, but was allowed")
	}
	if res4.RetryAfter <= 0 {
		t.Fatalf("expected positive RetryAfter duration, got %v", res4.RetryAfter)
	}
	if !res4.Degraded {
		t.Fatal("expected Degraded flag to be true on throttled fallback response")
	}
}

func TestRedisTokenBucketLimiterFailClosed(t *testing.T) {
	config := RateLimitConfig{
		Capacity:          5,
		RefillRatePerSec:  1,
		ExpirationTimeout: 1 * time.Minute,
	}

	limiter := NewRedisTokenBucketLimiter(nil, config)
	limiter.SetFailClosed(true)

	ctx := context.Background()
	res, err := limiter.Allow(ctx, "critical-auth-key", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Allowed {
		t.Fatal("expected FailClosed limiter to deny request when Redis is nil")
	}
	if !res.Degraded {
		t.Fatal("expected Degraded flag to be true on FailClosed response")
	}
}

func TestRedisTokenBucketLimiterInputValidation(t *testing.T) {
	config := RateLimitConfig{
		Capacity:          5,
		RefillRatePerSec:  1,
		ExpirationTimeout: 1 * time.Minute,
	}

	limiter := NewRedisTokenBucketLimiter(nil, config)
	ctx := context.Background()

	// Empty key must return error
	_, errEmpty := limiter.Allow(ctx, "", 1)
	if errEmpty == nil {
		t.Fatal("expected error when rate limit key is empty")
	}

	// Requesting tokens > capacity must return Allowed = false with RetryAfter = -1
	resOver, errOver := limiter.Allow(ctx, "valid-key", 100)
	if errOver != nil {
		t.Fatalf("unexpected error: %v", errOver)
	}
	if resOver.Allowed {
		t.Fatal("expected request exceeding capacity to be denied")
	}
	if resOver.RetryAfter != -1 {
		t.Fatalf("expected RetryAfter = -1 for impossible request, got %v", resOver.RetryAfter)
	}
}
