// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package operations

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/scandrix/backend/pkg/models"
)

func TestRateLimitCoordinator_ExecuteAndStatus(t *testing.T) {
	coord := NewRateLimitCoordinator()
	ctx := context.Background()

	executed := false
	err := coord.Execute(ctx, "org-1", models.ProviderGitHub, func(ctx context.Context) error {
		executed = true
		return nil
	})
	if err != nil || !executed {
		t.Fatalf("expected successful execution, got err=%v", err)
	}

	st := coord.GetStatus("org-1", models.ProviderGitHub)
	if st.CircuitState != CircuitClosed || st.RemainingTokens >= st.LimitPerHour {
		t.Fatalf("unexpected status: %+v", st)
	}
}

func TestRateLimitCoordinator_CircuitBreakerTrip(t *testing.T) {
	coord := NewRateLimitCoordinator()
	ctx := context.Background()

	// Cause 5 consecutive errors to trip circuit
	customErr := errors.New("github 503 service unavailable")
	for i := 0; i < 5; i++ {
		_ = coord.Execute(ctx, "org-1", models.ProviderGitHub, func(ctx context.Context) error {
			return customErr
		})
	}

	// 6th call should fail immediately via circuit breaker
	err := coord.Execute(ctx, "org-1", models.ProviderGitHub, func(ctx context.Context) error {
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "circuit breaker open") {
		t.Fatalf("expected circuit breaker error, got: %v", err)
	}
}

func TestRateLimitCoordinator_TokenBucketExhaustion(t *testing.T) {
	coord := NewRateLimitCoordinator()
	ctx := context.Background()

	bucket := coord.getOrCreateBucket("org-1", models.ProviderBitbucket)
	bucket.mu.Lock()
	bucket.tokens = 0
	bucket.mu.Unlock()

	err := coord.Execute(ctx, "org-1", models.ProviderBitbucket, func(ctx context.Context) error {
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "token bucket exhausted") {
		t.Fatalf("expected token bucket exhaustion, got: %v", err)
	}
}

func TestRateLimitCoordinator_BackoffWithJitter(t *testing.T) {
	coord := NewRateLimitCoordinator()

	for attempt := 1; attempt <= 5; attempt++ {
		delay := coord.ComputeBackoffWithJitter(attempt, 100*time.Millisecond, 2*time.Second)
		if delay < 100*time.Millisecond || delay > 2*time.Second {
			t.Fatalf("delay out of expected range: %v", delay)
		}
	}
}
