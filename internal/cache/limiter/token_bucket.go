package limiter

import (
	"context"
	"math"
	"sync"
	"time"
)

type bucket struct {
	tokens     float64
	lastRefill time.Time
}

// TokenBucketLimiter controls throughput using the token bucket algorithm with tenant isolation.
type TokenBucketLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	config  RateLimitConfig
}

// NewTokenBucketLimiter creates a rate limiter with the specified bucket capacity and refill rate.
func NewTokenBucketLimiter(config RateLimitConfig) *TokenBucketLimiter {
	if config.Capacity <= 0 {
		config.Capacity = 60
	}
	if config.RefillRatePerSec <= 0 {
		config.RefillRatePerSec = 10
	}
	if config.ExpirationTimeout <= 0 {
		config.ExpirationTimeout = 10 * time.Minute
	}

	return &TokenBucketLimiter{
		buckets: make(map[string]*bucket),
		config:  config,
	}
}

// Allow evaluates if tokens can be drawn for the given key.
func (l *TokenBucketLimiter) Allow(ctx context.Context, key string, tokens float64) (*RateLimitResult, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, exists := l.buckets[key]

	if !exists {
		b = &bucket{
			tokens:     l.config.Capacity,
			lastRefill: now,
		}
		l.buckets[key] = b
	} else {
		// Refill tokens based on elapsed duration
		elapsed := now.Sub(b.lastRefill).Seconds()
		b.tokens = math.Min(l.config.Capacity, b.tokens+(elapsed*l.config.RefillRatePerSec))
		b.lastRefill = now
	}

	res := &RateLimitResult{
		Remaining:  int(math.Max(0, b.tokens-tokens)),
		ResetAfter: time.Duration((l.config.Capacity-b.tokens)/l.config.RefillRatePerSec) * time.Second,
	}

	if b.tokens >= tokens {
		b.tokens -= tokens
		res.Allowed = true
		return res, nil
	}

	// Rate limit exceeded: calculate retry-after duration
	needed := tokens - b.tokens
	retrySec := needed / l.config.RefillRatePerSec
	res.Allowed = false
	res.RetryAfter = time.Duration(retrySec * float64(time.Second))
	return res, nil
}

// PruneInactive cleans up buckets that have not been accessed within the timeout window.
func (l *TokenBucketLimiter) PruneInactive() int {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	pruned := 0

	for k, b := range l.buckets {
		if now.Sub(b.lastRefill) > l.config.ExpirationTimeout {
			delete(l.buckets, k)
			pruned++
		}
	}
	return pruned
}
