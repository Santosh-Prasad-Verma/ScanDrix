package operations

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/scandrix/backend/pkg/models"
)

// CircuitState represents the health state of a VCS provider API.
type CircuitState string

const (
	CircuitClosed   CircuitState = "CLOSED"    // Normal operations
	CircuitHalfOpen CircuitState = "HALF_OPEN" // Trial recovery mode
	CircuitOpen     CircuitState = "OPEN"      // Failing fast, blocking outbound calls
)

// ProviderRateLimitStatus conveys token bucket budget and circuit state.
type ProviderRateLimitStatus struct {
	Provider        models.SCMProvider `json:"provider"`
	LimitPerHour    int                `json:"limitPerHour"`
	RemainingTokens int                `json:"remainingTokens"`
	ResetAt         time.Time          `json:"resetAt"`
	CircuitState    CircuitState       `json:"circuitState"`
	ActiveRequests  int                `json:"activeRequests"`
	MaxConcurrency  int                `json:"maxConcurrency"`
}

type tokenBucket struct {
	mu             sync.Mutex
	capacity       int
	tokens         int
	lastRefill     time.Time
	refillInterval time.Duration
	refillRate     int
	resetAt        time.Time
}

func newTokenBucket(capacity int, refillRate int, interval time.Duration) *tokenBucket {
	return &tokenBucket{
		capacity:       capacity,
		tokens:         capacity,
		lastRefill:     time.Now(),
		refillInterval: interval,
		refillRate:     refillRate,
		resetAt:        time.Now().Add(time.Hour),
	}
}

func (tb *tokenBucket) consume(count int) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastRefill)
	if elapsed >= tb.refillInterval {
		refills := int(elapsed / tb.refillInterval)
		tb.tokens += refills * tb.refillRate
		if tb.tokens > tb.capacity {
			tb.tokens = tb.capacity
		}
		tb.lastRefill = now
		if now.After(tb.resetAt) {
			tb.resetAt = now.Add(time.Hour)
		}
	}

	if tb.tokens >= count {
		tb.tokens -= count
		return true
	}
	return false
}

type circuitBreaker struct {
	mu               sync.Mutex
	state            CircuitState
	failureThreshold int
	consecutiveFails int
	recoveryTimeout  time.Duration
	lastFailure      time.Time
	halfOpenSuccess  int
	requiredSuccess  int
}

func newCircuitBreaker(failureThreshold int, recoveryTimeout time.Duration) *circuitBreaker {
	return &circuitBreaker{
		state:            CircuitClosed,
		failureThreshold: failureThreshold,
		recoveryTimeout:  recoveryTimeout,
		requiredSuccess:  3,
	}
}

func (cb *circuitBreaker) allowRequest() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		if time.Since(cb.lastFailure) > cb.recoveryTimeout {
			cb.state = CircuitHalfOpen
			cb.halfOpenSuccess = 0
			return true
		}
		return false
	case CircuitHalfOpen:
		return true
	default:
		return true
	}
}

func (cb *circuitBreaker) recordResult(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.consecutiveFails++
		cb.lastFailure = time.Now()
		if cb.consecutiveFails >= cb.failureThreshold || cb.state == CircuitHalfOpen {
			cb.state = CircuitOpen
		}
	} else {
		if cb.state == CircuitHalfOpen {
			cb.halfOpenSuccess++
			if cb.halfOpenSuccess >= cb.requiredSuccess {
				cb.state = CircuitClosed
				cb.consecutiveFails = 0
			}
		} else if cb.state == CircuitClosed {
			cb.consecutiveFails = 0
		}
	}
}

// RateLimitCoordinator coordinates multi-tenant API budgets and circuit protection across VCS platforms.
type RateLimitCoordinator struct {
	mu          sync.RWMutex
	buckets     map[string]*tokenBucket
	circuits    map[models.SCMProvider]*circuitBreaker
	concurrency map[models.SCMProvider]chan struct{}
}

// NewRateLimitCoordinator constructs a configured rate limit and resilience coordinator.
func NewRateLimitCoordinator() *RateLimitCoordinator {
	c := &RateLimitCoordinator{
		buckets:     make(map[string]*tokenBucket),
		circuits:    make(map[models.SCMProvider]*circuitBreaker),
		concurrency: make(map[models.SCMProvider]chan struct{}),
	}

	providers := []models.SCMProvider{
		models.ProviderGitHub,
		models.ProviderGitLab,
		models.ProviderBitbucket,
		models.ProviderAzure,
		models.ProviderForgejo,
	}

	for _, p := range providers {
		c.circuits[p] = newCircuitBreaker(5, 30*time.Second)
		maxConc := 50
		if p == models.ProviderAzure {
			maxConc = 25
		}
		c.concurrency[p] = make(chan struct{}, maxConc)
	}

	return c
}

func (c *RateLimitCoordinator) getBucketKey(orgID string, provider models.SCMProvider) string {
	return fmt.Sprintf("%s:%s", orgID, string(provider))
}

func (c *RateLimitCoordinator) getOrCreateBucket(orgID string, provider models.SCMProvider) *tokenBucket {
	key := c.getBucketKey(orgID, provider)
	c.mu.Lock()
	defer c.mu.Unlock()

	if b, ok := c.buckets[key]; ok {
		return b
	}

	capacity := 5000
	switch provider {
	case models.ProviderGitLab:
		capacity = 2000
	case models.ProviderBitbucket:
		capacity = 1000
	case models.ProviderAzure:
		capacity = 1500
	case models.ProviderForgejo:
		capacity = 3000
	}

	b := newTokenBucket(capacity, capacity/60, time.Minute)
	c.buckets[key] = b
	return b
}

// Execute executes an outbound VCS operation guarded by token limits, concurrency limits, and circuit breaker.
func (c *RateLimitCoordinator) Execute(
	ctx context.Context,
	orgID string,
	provider models.SCMProvider,
	operation func(ctx context.Context) error,
) error {
	cb, ok := c.circuits[provider]
	if !ok {
		cb = newCircuitBreaker(5, 30*time.Second)
	}

	if !cb.allowRequest() {
		return fmt.Errorf("rate limit coordinator: circuit breaker open for provider %s", provider)
	}

	bucket := c.getOrCreateBucket(orgID, provider)
	if !bucket.consume(1) {
		return errors.New("rate limit coordinator: organization token bucket exhausted, back off")
	}

	sem := c.concurrency[provider]
	select {
	case sem <- struct{}{}:
		defer func() { <-sem }()
	case <-ctx.Done():
		return ctx.Err()
	}

	err := operation(ctx)
	cb.recordResult(err)
	return err
}

// ComputeBackoffWithJitter calculates exponential backoff duration with randomized full jitter.
func (c *RateLimitCoordinator) ComputeBackoffWithJitter(attempt int, baseDelay, maxDelay time.Duration) time.Duration {
	if attempt <= 0 {
		attempt = 1
	}
	multiplier := 1 << uint(attempt-1)
	calculated := time.Duration(multiplier) * baseDelay
	if calculated > maxDelay {
		calculated = maxDelay
	}
	// Full jitter: [0, calculated]
	jittered := time.Duration(rand.Int63n(int64(calculated)))
	if jittered < baseDelay {
		jittered = baseDelay
	}
	return jittered
}

// GetStatus returns the operational status for an organization and provider.
func (c *RateLimitCoordinator) GetStatus(orgID string, provider models.SCMProvider) ProviderRateLimitStatus {
	bucket := c.getOrCreateBucket(orgID, provider)
	cb := c.circuits[provider]
	sem := c.concurrency[provider]

	bucket.mu.Lock()
	rem := bucket.tokens
	capVal := bucket.capacity
	reset := bucket.resetAt
	bucket.mu.Unlock()

	cb.mu.Lock()
	st := cb.state
	cb.mu.Unlock()

	return ProviderRateLimitStatus{
		Provider:        provider,
		LimitPerHour:    capVal,
		RemainingTokens: rem,
		ResetAt:         reset,
		CircuitState:    st,
		ActiveRequests:  len(sem),
		MaxConcurrency:  cap(sem),
	}
}
