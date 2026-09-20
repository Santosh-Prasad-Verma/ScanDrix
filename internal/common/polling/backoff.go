// Package polling provides robust exponential backoff, jitter randomization, and retry policies for asynchronous tasks.
package polling

import (
	"context"
	"crypto/rand"
	"errors"
	"math"
	"math/big"
	"time"
)

// BackoffConfig holds settings for exponential backoff calculations.
type BackoffConfig struct {
	InitialInterval time.Duration
	MaxInterval     time.Duration
	Multiplier      float64
	MaxAttempts     int
	JitterPercent   float64 // e.g. 0.15 for ±15%
}

// DefaultBackoffConfig provides sensible defaults.
var DefaultBackoffConfig = BackoffConfig{
	InitialInterval: 500 * time.Millisecond,
	MaxInterval:     30 * time.Second,
	Multiplier:      1.5,
	MaxAttempts:     5,
	JitterPercent:   0.15,
}

// CalculateBackoff computes the delay for the given attempt index (0-based) with randomized jitter.
func CalculateBackoff(cfg BackoffConfig, attempt int) time.Duration {
	if cfg.InitialInterval <= 0 {
		cfg.InitialInterval = 500 * time.Millisecond
	}
	if cfg.Multiplier <= 1.0 {
		cfg.Multiplier = 1.5
	}
	if cfg.MaxInterval <= 0 {
		cfg.MaxInterval = 30 * time.Second
	}

	delayFloat := float64(cfg.InitialInterval) * math.Pow(cfg.Multiplier, float64(attempt))
	if delayFloat > float64(cfg.MaxInterval) {
		delayFloat = float64(cfg.MaxInterval)
	}

	// Apply jitter: delay * (1 + random(-jitter, +jitter))
	if cfg.JitterPercent > 0 {
		jitterRange := cfg.JitterPercent * 2.0
		// Generate random float between 0 and jitterRange
		n, _ := rand.Int(rand.Reader, big.NewInt(10000))
		randomFrac := float64(n.Int64()) / 10000.0
		jitterDelta := (randomFrac * jitterRange) - cfg.JitterPercent
		delayFloat = delayFloat * (1.0 + jitterDelta)
	}

	if delayFloat < float64(cfg.InitialInterval)/2 {
		delayFloat = float64(cfg.InitialInterval) / 2
	}

	return time.Duration(delayFloat)
}

// RetryWithBackoff executes operation fn repeatedly with backoff until it succeeds, returns non-retryable error, or exceeds attempts/context.
func RetryWithBackoff(ctx context.Context, cfg BackoffConfig, fn func(attempt int) error) error {
	var lastErr error
	maxAttempts := cfg.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 5
	}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return errors.Join(ctx.Err(), lastErr)
			}
			return ctx.Err()
		default:
		}

		err := fn(attempt)
		if err == nil {
			return nil
		}
		lastErr = err

		if attempt < maxAttempts-1 {
			delay := CalculateBackoff(cfg, attempt)
			select {
			case <-ctx.Done():
				return errors.Join(ctx.Err(), lastErr)
			case <-time.After(delay):
			}
		}
	}

	return lastErr
}
