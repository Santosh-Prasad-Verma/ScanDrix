package cache_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/scandrix/backend/internal/cache"
)

func TestCacheIsNotFound(t *testing.T) {
	if !cache.IsNotFound(redis.Nil) {
		t.Fatal("expected IsNotFound(redis.Nil) to be true")
	}
	if !cache.IsNotFound(cache.ErrNotFound) {
		t.Fatal("expected IsNotFound(cache.ErrNotFound) to be true")
	}
	if cache.IsNotFound(errors.New("generic error")) {
		t.Fatal("expected IsNotFound(generic error) to be false")
	}
	if cache.IsNotFound(nil) {
		t.Fatal("expected IsNotFound(nil) to be false")
	}
}

func TestRedisTTLValidation(t *testing.T) {
	ctx := context.Background()

	// Client with dummy pointer for parameter validation tests
	c := &cache.Client{}

	// AcquireLockWithToken must reject non-positive TTL upfront
	_, _, err := c.AcquireLockWithToken(ctx, "test-key", 0)
	if err == nil {
		t.Fatal("expected error when acquiring lock with 0 TTL")
	}

	_, _, errNegative := c.AcquireLockWithToken(ctx, "test-key", -1*time.Minute)
	if errNegative == nil {
		t.Fatal("expected error when acquiring lock with negative TTL")
	}

	// ExtendLock must reject non-positive TTL upfront
	_, errExtend := c.ExtendLock(ctx, "test-key", "dummy-token", 0)
	if errExtend == nil {
		t.Fatal("expected error when extending lock with 0 TTL")
	}
}

func TestNilClientGuards(t *testing.T) {
	var c *cache.Client

	if err := c.Publish(context.Background(), "chan", "msg"); err != nil {
		t.Fatalf("expected nil Publish to return nil, got %v", err)
	}
	if sub := c.Subscribe(context.Background(), "chan"); sub != nil {
		t.Fatal("expected nil Subscribe to return nil")
	}
	if raw := c.RawClient(); raw != nil {
		t.Fatal("expected nil RawClient to return nil")
	}
}
