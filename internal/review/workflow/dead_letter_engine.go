package workflow

import (
	"context"
	"math"
	"math/rand"
	"strings"
	"sync"
	"time"
)

// FailureClassification categorizes error conditions for intelligent retry policies.
type FailureClassification string

const (
	FailureTransientNetwork   FailureClassification = "TRANSIENT_NETWORK"
	FailureRateLimited429     FailureClassification = "RATE_LIMITED_429"
	FailureSCMAuthExpired     FailureClassification = "SCM_AUTH_EXPIRED"
	FailureSandboxExhaustion  FailureClassification = "SANDBOX_EXHAUSTION"
	FailureFatalMalformedData FailureClassification = "FATAL_MALFORMED_DATA"
	FailureInternalBug        FailureClassification = "INTERNAL_BUG"
)

// RetryPolicy defines backoff parameters and maximum attempts.
type RetryPolicy struct {
	MaxRetries        int           `json:"max_retries"`        // Default: 5
	BaseDelay         time.Duration `json:"base_delay"`         // Default: 1s
	MaxDelay          time.Duration `json:"max_delay"`          // Default: 30s
	BackoffMultiplier float64       `json:"backoff_multiplier"` // Default: 2.0
	JitterFraction    float64       `json:"jitter_fraction"`    // Default: 0.25 (+/- 25%)
}

// DefaultRetryPolicy returns standard production exponential backoff policy.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries:        5,
		BaseDelay:         1 * time.Second,
		MaxDelay:          30 * time.Second,
		BackoffMultiplier: 2.0,
		JitterFraction:    0.25,
	}
}

// DeadLetterEnvelope records the context and history of a failed message.
type DeadLetterEnvelope struct {
	MessageID      string                `json:"message_id"`
	OriginalQueue  string                `json:"original_queue"`
	RoutingKey     string                `json:"routing_key"`
	Payload        []byte                `json:"payload"`
	Attempt        int                   `json:"attempt"`
	LastError      string                `json:"last_error"`
	Classification FailureClassification `json:"classification"`
	IsDeadLettered bool                  `json:"is_dead_lettered"`
	NextRetryAt    time.Time             `json:"next_retry_at,omitempty"`
	DeadLetteredAt time.Time             `json:"dead_lettered_at,omitempty"`
	CreatedAt      time.Time             `json:"created_at"`
}

// DeadLetterEngine handles error categorization, jittered backoff calculation, and dead letter queue routing.
type DeadLetterEngine struct {
	mu           sync.RWMutex
	policy       RetryPolicy
	dlqStore     map[string]DeadLetterEnvelope
	retryQueue   map[string]DeadLetterEnvelope
	rng          *rand.Rand
}

// NewDeadLetterEngine creates a dead letter engine.
func NewDeadLetterEngine(policy ...RetryPolicy) *DeadLetterEngine {
	p := DefaultRetryPolicy()
	if len(policy) > 0 {
		p = policy[0]
	}
	return &DeadLetterEngine{
		policy:     p,
		dlqStore:   make(map[string]DeadLetterEnvelope),
		retryQueue: make(map[string]DeadLetterEnvelope),
		rng:        rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// ClassifyError inspects an error message to determine if it is transient or fatal.
func (e *DeadLetterEngine) ClassifyError(err error) FailureClassification {
	if err == nil {
		return FailureInternalBug
	}
	msg := strings.ToLower(err.Error())

	// 1. Rate limits
	if strings.Contains(msg, "429") || strings.Contains(msg, "rate limit") || strings.Contains(msg, "too many requests") {
		return FailureRateLimited429
	}

	// 2. Network & timeouts
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "connection refused") || strings.Contains(msg, "temporary failure") ||
		strings.Contains(msg, "broken pipe") || strings.Contains(msg, "eof") {
		return FailureTransientNetwork
	}

	// 3. Auth expired
	if strings.Contains(msg, "401") || strings.Contains(msg, "unauthorized") ||
		strings.Contains(msg, "token expired") || strings.Contains(msg, "bad credentials") {
		return FailureSCMAuthExpired
	}

	// 4. Sandbox out of capacity
	if strings.Contains(msg, "out of capacity") || strings.Contains(msg, "lease superseded") ||
		strings.Contains(msg, "sandbox") {
		return FailureSandboxExhaustion
	}

	// 5. Malformed payload
	if strings.Contains(msg, "invalid json") || strings.Contains(msg, "unmarshal") ||
		strings.Contains(msg, "missing required field") {
		return FailureFatalMalformedData
	}

	return FailureInternalBug
}

// IsRetryable determines if a failure should be re-attempted.
func (e *DeadLetterEngine) IsRetryable(classification FailureClassification, attempt int) bool {
	if attempt >= e.policy.MaxRetries {
		return false
	}

	switch classification {
	case FailureFatalMalformedData, FailureSCMAuthExpired:
		// Do not retry permanently broken inputs or invalid credentials
		return false
	case FailureTransientNetwork, FailureRateLimited429, FailureSandboxExhaustion, FailureInternalBug:
		return true
	default:
		return false
	}
}

// CalculateBackoff computes the jittered exponential delay for a given attempt.
func (e *DeadLetterEngine) CalculateBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		attempt = 1
	}

	// base * (multiplier ^ (attempt - 1))
	multiplierFactor := math.Pow(e.policy.BackoffMultiplier, float64(attempt-1))
	delaySecs := e.policy.BaseDelay.Seconds() * multiplierFactor

	maxSecs := e.policy.MaxDelay.Seconds()
	if delaySecs > maxSecs {
		delaySecs = maxSecs
	}

	// Apply jitter (+/- fraction)
	jitterRange := delaySecs * e.policy.JitterFraction
	jitter := (e.rng.Float64()*2.0 - 1.0) * jitterRange // Between -jitterRange and +jitterRange
	finalSecs := delaySecs + jitter
	if finalSecs < 0.1 {
		finalSecs = 0.1
	}

	return time.Duration(finalSecs * float64(time.Second))
}

// HandleFailure evaluates the failed execution and routes the message to either retry queue or DLQ.
func (e *DeadLetterEngine) HandleFailure(
	ctx context.Context,
	msgID, queue, routingKey string,
	payload []byte,
	attempt int,
	execErr error,
) DeadLetterEnvelope {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now().UTC()
	classification := e.ClassifyError(execErr)
	retryable := e.IsRetryable(classification, attempt)

	env := DeadLetterEnvelope{
		MessageID:      msgID,
		OriginalQueue:  queue,
		RoutingKey:     routingKey,
		Payload:        payload,
		Attempt:        attempt,
		LastError:      execErr.Error(),
		Classification: classification,
		CreatedAt:      now,
	}

	if retryable {
		backoff := e.CalculateBackoff(attempt)
		env.NextRetryAt = now.Add(backoff)
		env.IsDeadLettered = false
		e.retryQueue[msgID] = env
		delete(e.dlqStore, msgID)
	} else {
		env.IsDeadLettered = true
		env.DeadLetteredAt = now
		e.dlqStore[msgID] = env
		delete(e.retryQueue, msgID)
	}

	return env
}

// GetDLQMessages returns all dead-lettered messages awaiting administrative inspection.
func (e *DeadLetterEngine) GetDLQMessages() []DeadLetterEnvelope {
	e.mu.RLock()
	defer e.mu.RUnlock()
	var res []DeadLetterEnvelope
	for _, env := range e.dlqStore {
		res = append(res, env)
	}
	return res
}

// GetReadyRetries returns messages whose NextRetryAt has arrived.
func (e *DeadLetterEngine) GetReadyRetries(now time.Time) []DeadLetterEnvelope {
	e.mu.RLock()
	defer e.mu.RUnlock()
	var ready []DeadLetterEnvelope
	for _, env := range e.retryQueue {
		if now.After(env.NextRetryAt) || now.Equal(env.NextRetryAt) {
			ready = append(ready, env)
		}
	}
	return ready
}

// AcknowledgeSuccess clears retry tracking upon successful completion.
func (e *DeadLetterEngine) AcknowledgeSuccess(msgID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.retryQueue, msgID)
	delete(e.dlqStore, msgID)
}
