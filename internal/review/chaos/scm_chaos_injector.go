// Package chaos provides SCM chaos fault injection, resiliency primitives, and benchmark matrices.
package chaos

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// ChaosFaultType specifies the nature of chaos introduced into an SCM operation.
type ChaosFaultType string

const (
	FaultNone             ChaosFaultType = "none"
	FaultRateLimit429     ChaosFaultType = "rate_limit_429"
	FaultConnectionReset  ChaosFaultType = "connection_reset"
	FaultSocketTimeout    ChaosFaultType = "socket_timeout"
	FaultCorruptPayload   ChaosFaultType = "corrupt_payload"
	FaultSecondaryLimit   ChaosFaultType = "secondary_limit_403"
	FaultInternalError500 ChaosFaultType = "internal_error_500"
)

// ChaosFaultRule configures when and how a fault triggers.
type ChaosFaultRule struct {
	FaultType      ChaosFaultType `json:"fault_type"`
	TriggerCount   int            `json:"trigger_count"` // Number of times to trigger before resolving
	InjectedDelay  time.Duration  `json:"injected_delay"`
	RetryAfterWait time.Duration  `json:"retry_after_wait"`
	TargetEndpoint string         `json:"target_endpoint"` // Empty matches all
}

// SCMChaosInjector provides programmatic simulation of SCM failure modes.
type SCMChaosInjector struct {
	mu           sync.RWMutex
	rules        map[string]*ChaosFaultRule // key: operation/endpoint
	invocations  map[string]int64
	failuresSeen int64
	successSeen  int64
}

// NewSCMChaosInjector initializes a new chaos fault injector.
func NewSCMChaosInjector() *SCMChaosInjector {
	return &SCMChaosInjector{
		rules:       make(map[string]*ChaosFaultRule),
		invocations: make(map[string]int64),
	}
}

// InjectFault registers a chaos fault rule for a targeted operation.
func (ci *SCMChaosInjector) InjectFault(operation string, rule ChaosFaultRule) {
	ci.mu.Lock()
	defer ci.mu.Unlock()
	ci.rules[operation] = &rule
}

// ClearFaults removes all active chaos rules.
func (ci *SCMChaosInjector) ClearFaults() {
	ci.mu.Lock()
	defer ci.mu.Unlock()
	ci.rules = make(map[string]*ChaosFaultRule)
}

// Intercept executes before an SCM operation to determine whether a fault should fire.
func (ci *SCMChaosInjector) Intercept(ctx context.Context, platform SCMPlatform, operation string) error {
	ci.mu.Lock()
	ci.invocations[operation]++
	count := ci.invocations[operation]

	rule, exists := ci.rules[operation]
	ci.mu.Unlock()

	if !exists || rule == nil || rule.FaultType == FaultNone {
		atomic.AddInt64(&ci.successSeen, 1)
		return nil
	}

	// If trigger count exceeded, the service has "recovered"
	if rule.TriggerCount > 0 && int(count) > rule.TriggerCount {
		atomic.AddInt64(&ci.successSeen, 1)
		return nil
	}

	atomic.AddInt64(&ci.failuresSeen, 1)

	if rule.InjectedDelay > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(rule.InjectedDelay):
		}
	}

	switch rule.FaultType {
	case FaultRateLimit429:
		retryAfter := rule.RetryAfterWait
		if retryAfter <= 0 {
			retryAfter = 200 * time.Millisecond
		}
		return &SCMError{
			Platform:   platform,
			StatusCode: http.StatusTooManyRequests,
			Message:    "API rate limit exceeded. Please wait before retrying.",
			RetryAfter: retryAfter,
		}

	case FaultSecondaryLimit:
		return &SCMError{
			Platform:   platform,
			StatusCode: http.StatusForbidden,
			Message:    "You have triggered an abuse detection rate limit. Please retry later.",
			RetryAfter: 500 * time.Millisecond,
		}

	case FaultConnectionReset:
		return &SCMError{
			Platform:   platform,
			StatusCode: 0,
			Message:    "read tcp: connection reset by peer",
			Err:        errors.New("read tcp: connection reset by peer"),
		}

	case FaultSocketTimeout:
		return &SCMError{
			Platform:   platform,
			StatusCode: 0,
			Message:    "net/http: request canceled (Client.Timeout exceeded while awaiting headers)",
			Err:        context.DeadlineExceeded,
		}

	case FaultInternalError500:
		return &SCMError{
			Platform:   platform,
			StatusCode: http.StatusInternalServerError,
			Message:    "Internal server error while rendering diff",
		}

	case FaultCorruptPayload:
		return fmt.Errorf("unexpected EOF reading chunked body: corrupt JSON")

	default:
		return nil
	}
}

// Stats returns aggregate metrics on intercepted chaos operations.
func (ci *SCMChaosInjector) Stats() (totalInvocations int64, failures int64, successes int64) {
	ci.mu.RLock()
	defer ci.mu.RUnlock()

	var total int64
	for _, cnt := range ci.invocations {
		total += cnt
	}
	return total, atomic.LoadInt64(&ci.failuresSeen), atomic.LoadInt64(&ci.successSeen)
}
