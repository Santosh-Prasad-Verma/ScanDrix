package limiter_test

import (
	"context"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/cache/limiter"
)

func TestTokenBucketLimiter(t *testing.T) {
	ctx := context.Background()

	cfg := limiter.RateLimitConfig{
		Capacity:          5,
		RefillRatePerSec:  10,
		ExpirationTimeout: 1 * time.Second,
	}
	tb := limiter.NewTokenBucketLimiter(cfg)

	key := "ws:test:reviews"

	// 1. Consume 5 tokens (allowed)
	for i := 0; i < 5; i++ {
		res, err := tb.Allow(ctx, key, 1)
		if err != nil || !res.Allowed {
			t.Fatalf("expected token %d to be allowed", i)
		}
	}

	// 2. 6th token must be rejected (burst exhausted)
	res, err := tb.Allow(ctx, key, 1)
	if err != nil || res.Allowed {
		t.Fatalf("expected rate limit rejection, got allowed=true")
	}
	if res.RetryAfter <= 0 {
		t.Fatalf("expected positive retry-after duration, got %v", res.RetryAfter)
	}

	// 3. Sleep briefly to refill tokens
	time.Sleep(150 * time.Millisecond)

	resAfter, err := tb.Allow(ctx, key, 1)
	if err != nil || !resAfter.Allowed {
		t.Fatalf("expected token to be allowed after refill, got %+v", resAfter)
	}
}

func TestDistributedLockManager(t *testing.T) {
	ctx := context.Background()
	mgr := limiter.NewDistributedLockManager()

	resource := "review:pr:42"
	workerA := "worker-node-1"
	workerB := "worker-node-2"

	// 1. Worker A acquires lock
	lockA, err := mgr.Acquire(ctx, resource, workerA, 50*time.Millisecond)
	if err != nil || lockA.OwnerID != workerA {
		t.Fatalf("failed acquiring lock for worker A: %v", err)
	}
	if lockA.FencingToken != 1 {
		t.Fatalf("expected fencing token 1, got %d", lockA.FencingToken)
	}

	// 2. Worker B fails to acquire locked resource
	_, err = mgr.Acquire(ctx, resource, workerB, 50*time.Millisecond)
	if err == nil {
		t.Fatalf("expected lock conflict error for worker B, got nil")
	}

	// 3. Worker A releases lock
	if err := mgr.Release(ctx, resource, workerA); err != nil {
		t.Fatalf("failed releasing lock: %v", err)
	}

	// 4. Worker B now succeeds
	lockB, err := mgr.Acquire(ctx, resource, workerB, 50*time.Millisecond)
	if err != nil || lockB.OwnerID != workerB {
		t.Fatalf("failed acquiring lock for worker B after release: %v", err)
	}
	if lockB.FencingToken != 2 {
		t.Fatalf("expected monotonically increasing fencing token 2, got %d", lockB.FencingToken)
	}
}

func TestTieredCache(t *testing.T) {
	ctx := context.Background()
	cache := limiter.NewTieredCache()

	key := "repo:meta:scandrix"
	val := "meta_json_data"

	// Miss initially
	_, ok := cache.Get(ctx, key)
	if ok {
		t.Fatalf("expected cache miss initially")
	}

	// Set with TTL
	cache.Set(ctx, key, val, 50*time.Millisecond)

	// Hit immediately
	got, ok := cache.Get(ctx, key)
	if !ok || got.(string) != val {
		t.Fatalf("expected cache hit with %s, got %v", val, got)
	}

	// Sleep until expired
	time.Sleep(60 * time.Millisecond)

	_, ok = cache.Get(ctx, key)
	if ok {
		t.Fatalf("expected cache miss after expiration")
	}

	stats := cache.Stats()
	if stats.Hits < 1 || stats.Misses < 2 {
		t.Fatalf("unexpected cache stats: %+v", stats)
	}
}
