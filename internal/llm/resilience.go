package llm

import (
	"context"
	crand "crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"
)

// CircuitState represents the operational state of the circuit breaker.
type CircuitState int

const (
	StateClosed   CircuitState = iota // Normal operations
	StateHalfOpen                     // Probing provider recovery
	StateOpen                         // Failing fast, blocking upstream calls
)

// CircuitBreaker guards AI API calls from cascade failures during provider outages.
type CircuitBreaker struct {
	mu               sync.Mutex
	name             string
	state            CircuitState
	failureCount     int
	successCount     int
	failureThreshold int
	cooldownDuration time.Duration
	lastStateChange  time.Time
}

// NewCircuitBreaker initializes a circuit breaker with threshold and cooldown.
func NewCircuitBreaker(failureThreshold int, cooldownDuration time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		name:             "default",
		state:            StateClosed,
		failureThreshold: failureThreshold,
		cooldownDuration: cooldownDuration,
		lastStateChange:  time.Now(),
	}
}

// NewNamedCircuitBreaker initializes a named circuit breaker.
func NewNamedCircuitBreaker(name string, failureThreshold int, cooldownDuration time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		name:             name,
		state:            StateClosed,
		failureThreshold: failureThreshold,
		cooldownDuration: cooldownDuration,
		lastStateChange:  time.Now(),
	}
}

var ErrCircuitOpen = errors.New("ai provider circuit breaker is OPEN: fast-failing call")

// State returns the current circuit state safely.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

// Name returns the identifier of the circuit breaker.
func (cb *CircuitBreaker) Name() string {
	return cb.name
}

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
			return fmt.Errorf("%w for provider '%s'", ErrCircuitOpen, cb.name)
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
			jitter := int64(0)
			if n, err := crand.Int(crand.Reader, big.NewInt(250)); err == nil && n != nil {
				jitter = n.Int64()
			}
			backoff := time.Duration(1<<attempt)*500*time.Millisecond + time.Duration(jitter)*time.Millisecond
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}
	}

	cb.recordFailure()
	return fmt.Errorf("exhausted %d retries for provider '%s': %w", maxRetries, cb.name, lastErr)
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

// Reset restores the circuit breaker to StateClosed.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = StateClosed
	cb.failureCount = 0
	cb.successCount = 0
	cb.lastStateChange = time.Now()
}

// ProviderBreakerRegistry manages per-upstream-provider circuit breakers with independent failure domains.
type ProviderBreakerRegistry struct {
	mu               sync.RWMutex
	breakers         map[string]*CircuitBreaker
	defaultThreshold int
	defaultCooldown  time.Duration
}

// NewProviderBreakerRegistry creates a thread-safe registry of per-provider circuit breakers.
func NewProviderBreakerRegistry(defaultThreshold int, defaultCooldown time.Duration) *ProviderBreakerRegistry {
	return &ProviderBreakerRegistry{
		breakers:         make(map[string]*CircuitBreaker),
		defaultThreshold: defaultThreshold,
		defaultCooldown:  defaultCooldown,
	}
}

// GetOrCreate returns the isolated circuit breaker for the given provider.
func (r *ProviderBreakerRegistry) GetOrCreate(providerName string) *CircuitBreaker {
	r.mu.RLock()
	cb, exists := r.breakers[providerName]
	r.mu.RUnlock()
	if exists {
		return cb
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if cb, exists = r.breakers[providerName]; exists {
		return cb
	}

	cb = NewNamedCircuitBreaker(providerName, r.defaultThreshold, r.defaultCooldown)
	r.breakers[providerName] = cb
	return cb
}
