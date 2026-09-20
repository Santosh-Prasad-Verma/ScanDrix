// Package chaos provides SCM chaos fault injection, resiliency primitives, and benchmark matrices.
package chaos

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SCMPlatform identifies the targeted source code management platform.
type SCMPlatform string

const (
	PlatformGitHub     SCMPlatform = "github"
	PlatformGitLab     SCMPlatform = "gitlab"
	PlatformBitbucket  SCMPlatform = "bitbucket"
	PlatformAzureRepos SCMPlatform = "azure_repos"
	PlatformForgejo    SCMPlatform = "forgejo"
)

// RateLimitPolicy defines retry counts and wait boundaries for an SCM platform.
type RateLimitPolicy struct {
	MaxRetries       int           `json:"max_retries"`
	BaseBackoff      time.Duration `json:"base_backoff"`
	MaxBackoff       time.Duration `json:"max_backoff"`
	JitterFactor     float64       `json:"jitter_factor"` // 0.0 - 0.5
	MinInterval      time.Duration `json:"min_interval"`
	HonorRetryAfter  bool          `json:"honor_retry_after"`
}

// DefaultRateLimitPolicies provides platform-tailored rate limiting configuration.
func DefaultRateLimitPolicies() map[SCMPlatform]RateLimitPolicy {
	return map[SCMPlatform]RateLimitPolicy{
		PlatformGitHub: {
			MaxRetries:      5,
			BaseBackoff:     1 * time.Second,
			MaxBackoff:      30 * time.Second,
			JitterFactor:    0.25,
			MinInterval:     100 * time.Millisecond,
			HonorRetryAfter: true,
		},
		PlatformGitLab: {
			MaxRetries:      10, // GitBeaker default parity
			BaseBackoff:     500 * time.Millisecond,
			MaxBackoff:      20 * time.Second,
			JitterFactor:    0.20,
			MinInterval:     50 * time.Millisecond,
			HonorRetryAfter: true,
		},
		PlatformBitbucket: {
			MaxRetries:      4, // BITBUCKET_RATE_GATE_429_MAX_RETRIES
			BaseBackoff:     1500 * time.Millisecond,
			MaxBackoff:      45 * time.Second,
			JitterFactor:    0.30,
			MinInterval:     250 * time.Millisecond,
			HonorRetryAfter: true,
		},
		PlatformAzureRepos: {
			MaxRetries:      4,
			BaseBackoff:     1 * time.Second,
			MaxBackoff:      30 * time.Second,
			JitterFactor:    0.20,
			MinInterval:     150 * time.Millisecond,
			HonorRetryAfter: true,
		},
		PlatformForgejo: {
			MaxRetries:      3,
			BaseBackoff:     500 * time.Millisecond,
			MaxBackoff:      15 * time.Second,
			JitterFactor:    0.15,
			MinInterval:     50 * time.Millisecond,
			HonorRetryAfter: true,
		},
	}
}

// SCMError wraps platform-specific HTTP response failures.
type SCMError struct {
	Platform   SCMPlatform
	StatusCode int
	Message    string
	RetryAfter time.Duration
	Headers    http.Header
	Err        error
}

func (e *SCMError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("[%s] HTTP %d: %s (retry-after: %v)", e.Platform, e.StatusCode, e.Message, e.RetryAfter)
	}
	return fmt.Sprintf("[%s] HTTP %d: %s", e.Platform, e.StatusCode, e.Message)
}

func (e *SCMError) Unwrap() error {
	return e.Err
}

// IsRateLimit checks if the error represents an HTTP 429 Too Many Requests or secondary limit.
func IsRateLimit(err error) bool {
	if err == nil {
		return false
	}
	var scmErr *SCMError
	if errors.As(err, &scmErr) {
		if scmErr.StatusCode == http.StatusTooManyRequests {
			return true
		}
		// GitHub secondary abuse rate limits often return 403 with specific message
		if scmErr.StatusCode == http.StatusForbidden && strings.Contains(strings.ToLower(scmErr.Message), "rate limit") {
			return true
		}
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "429") || strings.Contains(msg, "rate limit")
}

// ParseRetryAfter extracts duration from HTTP Retry-After header (seconds or RFC1123).
func ParseRetryAfter(headerVal string) time.Duration {
	if headerVal == "" {
		return 0
	}

	// 1. Try seconds integer
	if seconds, err := strconv.Atoi(strings.TrimSpace(headerVal)); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}

	// 2. Try HTTP date header
	if t, err := http.ParseTime(headerVal); err == nil {
		diff := time.Until(t)
		if diff > 0 {
			return diff
		}
	}

	return 0
}

// ResilientSCMCaller executes outbound SCM operations with platform-aware backoff.
type ResilientSCMCaller struct {
	mu       sync.RWMutex
	policies map[SCMPlatform]RateLimitPolicy
	rng      *rand.Rand
	rngMu    sync.Mutex
}

// NewResilientSCMCaller creates a caller configured with default policies.
func NewResilientSCMCaller() *ResilientSCMCaller {
	return &ResilientSCMCaller{
		policies: DefaultRateLimitPolicies(),
		rng:      rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// ExecuteWithRetry wraps an SCM operation with exponential backoff and Retry-After parsing.
func (c *ResilientSCMCaller) ExecuteWithRetry(
	ctx context.Context,
	platform SCMPlatform,
	operationName string,
	fn func(ctx context.Context) error,
) error {
	c.mu.RLock()
	policy, ok := c.policies[platform]
	if !ok {
		policy = DefaultRateLimitPolicies()[PlatformGitHub]
	}
	c.mu.RUnlock()

	var lastErr error

	for attempt := 0; attempt <= policy.MaxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		err := fn(ctx)
		if err == nil {
			return nil
		}

		lastErr = err

		// Only retry on rate limit or transient network error
		if !IsRateLimit(err) && !isTransientTransportError(err) {
			return err
		}

		if attempt == policy.MaxRetries {
			break
		}

		waitDuration := c.calculateWait(attempt, policy, err)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitDuration):
		}
	}

	return fmt.Errorf("%s failed on %s after %d retries: %w", operationName, platform, policy.MaxRetries, lastErr)
}

// CalculateWait computes the wait duration for a platform and attempt, taking into account Retry-After headers and jitter.
func (c *ResilientSCMCaller) CalculateWait(attempt int, platform SCMPlatform, err error) time.Duration {
	c.mu.RLock()
	policy, ok := c.policies[platform]
	if !ok {
		policy = DefaultRateLimitPolicies()[PlatformGitHub]
	}
	c.mu.RUnlock()
	return c.calculateWait(attempt, policy, err)
}

func (c *ResilientSCMCaller) calculateWait(attempt int, policy RateLimitPolicy, err error) time.Duration {
	// 1. Honor explicit Retry-After header if present
	var scmErr *SCMError
	if errors.As(err, &scmErr) && policy.HonorRetryAfter && scmErr.RetryAfter > 0 {
		return scmErr.RetryAfter
	}

	// 2. Exponential backoff: base * 2^attempt
	multiplier := math.Pow(2, float64(attempt))
	backoff := time.Duration(float64(policy.BaseBackoff) * multiplier)
	if backoff > policy.MaxBackoff {
		backoff = policy.MaxBackoff
	}

	// 3. Jitter: +/- (jitterFactor * backoff)
	if policy.JitterFactor > 0 {
		c.rngMu.Lock()
		jitterFraction := (c.rng.Float64()*2.0 - 1.0) * policy.JitterFactor
		c.rngMu.Unlock()

		jitterDelta := time.Duration(float64(backoff) * jitterFraction)
		backoff += jitterDelta
	}

	if backoff < policy.MinInterval {
		backoff = policy.MinInterval
	}

	return backoff
}

func isTransientTransportError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	transientSignals := []string{
		"connection reset",
		"connection refused",
		"eof",
		"timeout",
		"deadline exceeded",
		"broken pipe",
		"handshake failure",
		"transport failure",
	}
	for _, signal := range transientSignals {
		if strings.Contains(msg, signal) {
			return true
		}
	}
	return false
}
