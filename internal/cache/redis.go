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
	opt.MinIdleConns = 10
	opt.DialTimeout = 5 * time.Second
	opt.ReadTimeout = 3 * time.Second
	opt.WriteTimeout = 3 * time.Second
	opt.PoolTimeout = 4 * time.Second

	rdb := redis.NewClient(opt)

	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	if err := rdb.Ping(pingCtx).Err(); err != nil {
		rdb.Close()
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

// CheckIdempotency checks if a webhook delivery has already been processed within the window.
// Returns true if event is NEW and should be processed; false if already claimed/processed.
func (c *Client) CheckIdempotency(ctx context.Context, deliveryID string, window time.Duration) (bool, error) {
	isNew, err := c.rdb.SetNX(ctx, "idempotency:"+deliveryID, "claimed", window).Result()
	if err != nil {
		return false, fmt.Errorf("idempotency check error: %w", err)
	}
	return isNew, nil
}

// Get retrieves cached bytes by key.
func (c *Client) Get(ctx context.Context, key string) ([]byte, error) {
	return c.rdb.Get(ctx, key).Bytes()
}

// Set stores a key-value pair with TTL.
func (c *Client) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return c.rdb.Set(ctx, key, value, ttl).Err()
}
