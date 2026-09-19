package application

import (
	"context"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type memoryEntry struct {
	expiresAt time.Time
}

// NotificationRateLimiter provides per-recipient and per-org rate limiting for notification emits.
type NotificationRateLimiter struct {
	redisClient *redis.Client
	mu          sync.RWMutex
	memoryCache map[string]memoryEntry
}

// NewNotificationRateLimiter creates a rate limiter instance, optionally backed by Redis.
func NewNotificationRateLimiter(rdb *redis.Client) *NotificationRateLimiter {
	limiter := &NotificationRateLimiter{
		redisClient: rdb,
		memoryCache: make(map[string]memoryEntry),
	}
	go limiter.startCleanup(10 * time.Minute)
	return limiter
}

// ShouldEmit returns true if the key has not been seen within the TTL window, false otherwise.
// If storage encounters an error, it fails open (returns true) so critical notifications are never silently dropped.
func (rl *NotificationRateLimiter) ShouldEmit(ctx context.Context, key string, ttlSeconds int) bool {
	if rl.redisClient != nil {
		set, err := rl.redisClient.SetNX(ctx, "notif:ratelimit:"+key, "1", time.Duration(ttlSeconds)*time.Second).Result()
		if err != nil {
			// Fail-open: allow notification when cache fails
			return true
		}
		return set
	}

	// In-memory fallback
	now := time.Now()
	rl.mu.Lock()
	defer rl.mu.Unlock()

	entry, found := rl.memoryCache[key]
	if found && entry.expiresAt.After(now) {
		return false
	}

	rl.memoryCache[key] = memoryEntry{
		expiresAt: now.Add(time.Duration(ttlSeconds) * time.Second),
	}
	return true
}

func (rl *NotificationRateLimiter) startCleanup(interval time.Duration) {
	ticker := time.NewTicker(interval)
	for range ticker.C {
		now := time.Now()
		rl.mu.Lock()
		for k, v := range rl.memoryCache {
			if v.expiresAt.Before(now) {
				delete(rl.memoryCache, k)
			}
		}
		rl.mu.Unlock()
	}
}
