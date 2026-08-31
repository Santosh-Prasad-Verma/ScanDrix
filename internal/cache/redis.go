package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Client encapsulates a thread-safe Redis/Valkey connection pool for distributed locks and caching.
type Client struct {
	rdb *redis.Client
}

// NewClient initializes a connection pool to Redis with health checks and timeout limits.
func NewClient(ctx context.Context, redisURL string) (*Client, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("invalid redis url: %w", err)
	}

	opt.PoolSize = 50
	opt.DialTimeout = 1 * time.Second
	opt.ReadTimeout = 2 * time.Second
	opt.WriteTimeout = 2 * time.Second
	opt.MaxRetries = 1

	rdb := redis.NewClient(opt)

	pingCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	if err := rdb.Ping(pingCtx).Err(); err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}

	return &Client{rdb: rdb}, nil
}

// Close terminates Redis connections.
func (c *Client) Close() error {
	if c.rdb != nil {
		return c.rdb.Close()
	}
	return nil
}

// AcquireLock attempts to acquire a distributed mutex using Redis SETNX with TTL.
func (c *Client) AcquireLock(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	success, err := c.rdb.SetNX(ctx, "lock:"+key, "1", ttl).Result()
	if err != nil {
		return false, fmt.Errorf("failed acquiring lock %s: %w", key, err)
	}
	return success, nil
}

// ReleaseLock removes the distributed mutex.
func (c *Client) ReleaseLock(ctx context.Context, key string) error {
	return c.rdb.Del(ctx, "lock:"+key).Err()
}

// Get retrieves a string cache value.
func (c *Client) Get(ctx context.Context, key string) (string, error) {
	return c.rdb.Get(ctx, key).Result()
}

// Set stores a string cache value with TTL.
func (c *Client) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	return c.rdb.Set(ctx, key, value, ttl).Err()
}
