package limiter

import (
	"context"
	"testing"
)

// F-28/F-29/F-32: a per-process bucket is not "no limit", it is N times the
// intended limit for N replicas, and it is invisible in the logs. Outside
// development these tests pin that a missing or broken shared store denies
// rather than silently weakening the control.

func TestDistributedRequiredByEnvironment(t *testing.T) {
	for _, env := range []string{"production", "PRODUCTION", "staging", "Staging"} {
		t.Run(env+" required", func(t *testing.T) {
			t.Setenv("APP_ENV", env)
			if !DistributedRequired() {
				t.Errorf("%s must require distributed limiting", env)
			}
		})
	}
	for _, env := range []string{"development", "dev", "test", "local", ""} {
		t.Run(env+" not required", func(t *testing.T) {
			t.Setenv("APP_ENV", env)
			t.Setenv("ENVIRONMENT", "")
			t.Setenv("GO_ENV", "")
			t.Setenv("SCANDRIX_ENV", "")
			if DistributedRequired() {
				t.Errorf("%q must not require distributed limiting", env)
			}
		})
	}
}

// Production with no shared store must deny, not fall back to a local bucket.
func TestMissingStoreDeniesOutsideDevelopment(t *testing.T) {
	t.Setenv("APP_ENV", "production")

	l := RedisTokenBucket(nil, RateLimitConfig{Capacity: 10, RefillRatePerSec: 1})
	ul, ok := l.(*UnavailableLimiter)
	if !ok {
		t.Fatalf("production without a shared store must produce an UnavailableLimiter, got %T", l)
	}
	if ul.Reason() == "" {
		t.Error("the denying limiter must explain itself for logs and health output")
	}

	res, err := ul.Allow(context.Background(), "any-key", 1)
	if err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if res.Allowed {
		t.Error("a request must be denied when no shared rate-limit store exists")
	}
	if !res.Degraded {
		t.Error("the result must be marked degraded so it is distinguishable from a normal 429")
	}
}

// Single-process environments legitimately keep the local bucket.
func TestSingleProcessKeepsLocalBucket(t *testing.T) {
	t.Setenv("APP_ENV", "development")

	l := RedisTokenBucket(nil, RateLimitConfig{Capacity: 2, RefillRatePerSec: 0.001})
	if _, ok := l.(*TokenBucketLimiter); !ok {
		t.Fatalf("development should keep a local bucket, got %T", l)
	}
}

// The degradation branch is the one that matters: when the shared store cannot
// be reached, fail-closed must deny and fail-open must serve from the local
// bucket. Tested with a nil client, which puts the limiter in exactly the state
// an unreachable store produces, so no Redis server is needed.
func TestDegradedBranchHonoursFailClosed(t *testing.T) {
	newLimiter := func(failClosed bool) *RedisTokenBucketLimiter {
		l := NewRedisTokenBucketLimiter(nil, RateLimitConfig{Capacity: 10, RefillRatePerSec: 1})
		l.SetFailClosed(failClosed)
		return l
	}

	t.Run("fail closed denies", func(t *testing.T) {
		res, err := newLimiter(true).Allow(context.Background(), "k", 1)
		if err != nil {
			t.Fatalf("Allow: %v", err)
		}
		if res.Allowed {
			t.Error("fail-closed must deny when the store is unreachable")
		}
		if !res.Degraded {
			t.Error("the denial must be marked degraded")
		}
		if !newLimiter(true).Degraded() {
			t.Error("Degraded() must report the outage")
		}
	})

	t.Run("fail open serves from the local bucket", func(t *testing.T) {
		l := newLimiter(false)
		// Degraded() reports that the shared store is unusable, which is true in
		// both branches; what differs is whether the request is admitted.
		if !l.Degraded() {
			t.Error("Degraded() must report the unreachable store regardless of fail-closed")
		}
		res, err := l.Allow(context.Background(), "k", 1)
		if err != nil {
			t.Fatalf("Allow: %v", err)
		}
		if !res.Allowed {
			t.Error("fail-open should still serve from the local bucket in development")
		}
	})
}
