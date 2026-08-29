package llm

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// CircuitState represents the operational state of the circuit breaker.
type CircuitState int

const (
	StateClosed CircuitState = iota // Normal operations
	StateHalfOpen                   // Probing provider recovery
	StateOpen                       // Failing fast, blocking upstream calls
)

// CircuitBreaker guards AI API calls from cascade failures during provider outages.
type CircuitBreaker struct {
	mu             sync.Mutex
	state          CircuitState
	failureCount   int
	successCount   int
	failureThreshold int
	cooldownDuration time.Duration
	lastStateChange  time.Time
}

// NewCircuitBreaker initializes a circuit breaker.
func NewCircuitBreaker(failureThreshold int, cooldownDuration time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:            StateClosed,
		failureThreshold: failureThreshold,
		cooldownDuration: cooldownDuration,
		lastStateChange:  time.Now(),
	}
}

var ErrCircuitOpen = errors.New("ai provider circuit breaker is OPEN: fast-failing call")

// Execute wraps an external network call with circuit breaker monitoring and exponential backoff.
func (cb *CircuitBreaker) Execute(ctx context.Context, maxRetries int, fn func(ctx context.Context) error) error {
	cb.mu.Lock()
	now := time.Now()

	// Check transition from Open to Half-Open
	if cb.state == StateOpen {
		if now.Sub(cb.lastStateChange) > cb.cooldownDuration {
			cb.state = StateHalfOpen
			cb.successCount = 0
		} else {
			cb.mu.Unlock()
			return ErrCircuitOpen
		}
	}
	cb.mu.Unlock()

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		lastErr = fn(ctx)
		if lastErr == nil {
			cb.recordSuccess()
			return nil
		}

		// Calculate exponential backoff with jitter
		if attempt < maxRetries {
			backoff := time.Duration(1<<attempt)*500*time.Millisecond + time.Duration(rand.Intn(250))*time.Millisecond
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}
	}

	cb.recordFailure()
	return fmt.Errorf("exhausted %d retries: %w", maxRetries, lastErr)
}

func (cb *CircuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == StateHalfOpen {
		cb.successCount++
		if cb.successCount >= 3 {
			cb.state = StateClosed
			cb.failureCount = 0
		}
	} else if cb.state == StateClosed {
		cb.failureCount = 0
	}
}

func (cb *CircuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount++
	if cb.failureCount >= cb.failureThreshold {
		cb.state = StateOpen
		cb.lastStateChange = time.Now()
	}
}

// Allow reports whether a call can proceed through the circuit breaker.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	now := time.Now()

	if cb.state == StateOpen {
		if now.Sub(cb.lastStateChange) > cb.cooldownDuration {
			cb.state = StateHalfOpen
			cb.successCount = 0
			return true
		}
		return false
	}
	return true
}

// RecordSuccess registers a successful invocation.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.recordSuccess()
}

// RecordFailure registers a failed invocation.
func (cb *CircuitBreaker) RecordFailure() {
	cb.recordFailure()
}

