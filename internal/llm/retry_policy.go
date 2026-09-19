// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package llm

import (
	"context"
	crand "crypto/rand"
	"math"
	"math/big"
	"time"
)

const (
	RetryBaseDelayMs = 500
	RetryMaxDelayMs  = 10000
)

// IsRetryableForReissue returns true when a one-shot call should re-issue once for this error.
func IsRetryableForReissue(err error) bool {
	if err == nil || IsAbortOrHardTimeout(err) {
		return false
	}
	classified := ClassifyLLMError(err, 0)
	return classified.Category == CategoryTransient
}

// JitteredBackoff calculates exponential backoff with full jitter to avoid thundering herd.
// Uses crypto/rand for unpredictable jitter.
func JitteredBackoff(attempt int, baseMs, capMs int) time.Duration {
	if baseMs <= 0 {
		baseMs = RetryBaseDelayMs
	}
	if capMs <= 0 {
		capMs = RetryMaxDelayMs
	}
	if attempt < 1 {
		attempt = 1
	}

	exp := float64(baseMs) * math.Pow(2, float64(attempt-1))
	if exp > float64(capMs) {
		exp = float64(capMs)
	}

	// Uniform random in [0.5, 1.0] using crypto/rand
	factor := 0.5
	if n, err := crand.Int(crand.Reader, big.NewInt(500)); err == nil && n != nil {
		factor = 0.5 + float64(n.Int64())/1000.0
	}
	delayMs := int(exp * factor)
	return time.Duration(delayMs) * time.Millisecond
}

// SleepContext pauses execution for the given duration or until context cancellation.
func SleepContext(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}
