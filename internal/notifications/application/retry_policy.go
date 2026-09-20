package application

import (
	"math"
	"math/rand"
	"time"

	"github.com/scandrix/backend/internal/notifications/domain/enums"
)

// RetryDecision defines the outcome of evaluating a delivery attempt against the retry policy.
type RetryDecision struct {
	ShouldRetry   bool      `json:"shouldRetry"`
	NextAttemptAt time.Time `json:"nextAttemptAt"`
	MaxAttempts   int       `json:"maxAttempts"`
}

type policyConfig struct {
	maxAttempts      int
	baseDelaySeconds float64
	maxDelaySeconds  float64
}

var criticalPolicy = policyConfig{
	maxAttempts:      8,
	baseDelaySeconds: 5,
	maxDelaySeconds:  300,
}

var defaultPolicy = policyConfig{
	maxAttempts:      5,
	baseDelaySeconds: 10,
	maxDelaySeconds:  300,
}

// DecideRetry determines whether and when a failed notification delivery should be re-attempted.
func DecideRetry(criticality enums.Criticality, attemptsSoFar int, now time.Time) RetryDecision {
	policy := defaultPolicy
	if criticality == enums.CriticalityCritical {
		policy = criticalPolicy
	}

	if attemptsSoFar >= policy.maxAttempts {
		return RetryDecision{
			ShouldRetry:   false,
			NextAttemptAt: now,
			MaxAttempts:   policy.maxAttempts,
		}
	}

	// Exponential backoff: base * 2^(attempts - 1), capped at maxDelaySeconds.
	exponent := math.Pow(2, float64(attemptsSoFar-1))
	delay := math.Min(policy.baseDelaySeconds*exponent, policy.maxDelaySeconds)

	// ±15% jitter to prevent thundering herd spikes against upstream services
	jitterMultiplier := 0.85 + rand.Float64()*0.30
	finalDelaySeconds := delay * jitterMultiplier

	nextAttempt := now.Add(time.Duration(finalDelaySeconds * float64(time.Second)))

	return RetryDecision{
		ShouldRetry:   true,
		NextAttemptAt: nextAttempt,
		MaxAttempts:   policy.maxAttempts,
	}
}
