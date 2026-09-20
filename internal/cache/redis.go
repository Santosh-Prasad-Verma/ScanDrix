package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

var (
	// ErrNotFound indicates that a requested key was not found in Redis.
	ErrNotFound = redis.Nil

	// Precompiled Lua scripts using EVALSHA transparently to minimize bandwidth and execution overhead.
	releaseLockWithTokenScript = redis.NewScript(`
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("del", KEYS[1])
		else
			return 0
		end
	`)

	extendLockScript = redis.NewScript(`
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("pexpire", KEYS[1], ARGV[2])
		else
			return 0
		end
	`)
)

// IsNotFound checks whether an error indicates a cache miss.
func IsNotFound(err error) bool {
	return errors.Is(err, redis.Nil) || errors.Is(err, ErrNotFound)
}

func lockKey(key string) string {
	return "lock:" + key
}

// RedisOptions configures connection pooling and timeouts for the Redis client.
type RedisOptions struct {
	PoolSize     int
	MinIdleConns int
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	MaxRetries   int
}

// DefaultRedisOptions returns production-tested default connection pool options.
func DefaultRedisOptions() RedisOptions {
	return RedisOptions{
		PoolSize:     50,
		MinIdleConns: 5,
		DialTimeout:  1 * time.Second,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
		MaxRetries:   1,
	}
}

// Client encapsulates a thread-safe Redis/Valkey connection pool for distributed locks and caching.
type Client struct {
	rdb *redis.Client
}

// NewClient initializes a connection pool to Redis with default options and ping health checks.
func NewClient(ctx context.Context, redisURL string) (*Client, error) {
	return NewClientWithOptions(ctx, redisURL, DefaultRedisOptions())
}

// NewClientWithOptions initializes a Redis client with customizable pool and timeout settings.
func NewClientWithOptions(ctx context.Context, redisURL string, opts RedisOptions) (*Client, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("invalid redis url: %w", err)
	}

	if opts.PoolSize > 0 {
		opt.PoolSize = opts.PoolSize
	}
	if opts.MinIdleConns > 0 {
		opt.MinIdleConns = opts.MinIdleConns
	}
	if opts.DialTimeout > 0 {
		opt.DialTimeout = opts.DialTimeout
	}
	if opts.ReadTimeout > 0 {
		opt.ReadTimeout = opts.ReadTimeout
	}
	if opts.WriteTimeout > 0 {
		opt.WriteTimeout = opts.WriteTimeout
	}
	opt.MaxRetries = opts.MaxRetries

	rdb := redis.NewClient(opt)

	pingTimeout := opts.DialTimeout
	if pingTimeout <= 0 {
		pingTimeout = 1 * time.Second
	}
	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
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

// Raw returns the underlying go-redis Client instance for custom commands and scripts.
func (c *Client) Raw() *redis.Client {
	if c == nil {
		return nil
	}
	return c.rdb
}

// AcquireLock attempts to acquire a distributed mutex using Redis SETNX with TTL.
func (c *Client) AcquireLock(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	_, ok, err := c.AcquireLockWithToken(ctx, key, ttl)
	return ok, err
}

// AcquireLockWithToken acquires a distributed mutex and returns the unique ownership token.
// Guarantees ttl > 0 to prevent permanent deadlocks.
func (c *Client) AcquireLockWithToken(ctx context.Context, key string, ttl time.Duration) (string, bool, error) {
	if ttl <= 0 {
		return "", false, errors.New("ttl must be positive to prevent permanent deadlocks")
	}

	token := uuid.New().String()
	success, err := c.rdb.SetNX(ctx, lockKey(key), token, ttl).Result()
	if err != nil {
		return "", false, fmt.Errorf("failed acquiring lock %s: %w", key, err)
	}
	if !success {
		return "", false, nil
	}
	return token, true, nil
}

// ExtendLock extends the TTL of an active lock only if the caller owns the lock token (heartbeat watchdog).
func (c *Client) ExtendLock(ctx context.Context, key, token string, ttl time.Duration) (bool, error) {
	if ttl <= 0 {
		return false, errors.New("ttl must be positive")
	}

	res, err := extendLockScript.Run(ctx, c.rdb, []string{lockKey(key)}, token, ttl.Milliseconds()).Int64()
	if err != nil {
		return false, fmt.Errorf("failed extending lock %s: %w", key, err)
	}
	return res == 1, nil
}

// ReleaseLockWithToken safely removes the distributed mutex only if the ownership token matches.
// Returns (true, nil) if successfully released, or (false, nil) if the lock already expired or was claimed by another worker.
func (c *Client) ReleaseLockWithToken(ctx context.Context, key, token string) (bool, error) {
	res, err := releaseLockWithTokenScript.Run(ctx, c.rdb, []string{lockKey(key)}, token).Int64()
	if err != nil {
		return false, fmt.Errorf("failed releasing lock %s: %w", key, err)
	}
	return res == 1, nil
}

// ForceReleaseLock forcefully deletes the lock key regardless of ownership token.
// DANGEROUS: This bypasses token ownership verification and can steal an active lock
// held by another worker. Use ReleaseLockWithToken for safe operation.
func (c *Client) ForceReleaseLock(ctx context.Context, key string) error {
	return c.rdb.Del(ctx, lockKey(key)).Err()
}

// ReleaseLock is an alias for ForceReleaseLock.
// Deprecated: Use ReleaseLockWithToken to avoid distributed race conditions.
func (c *Client) ReleaseLock(ctx context.Context, key string) error {
	return c.ForceReleaseLock(ctx, key)
}

// Get retrieves a string cache value with ErrNotFound error wrapping.
func (c *Client) Get(ctx context.Context, key string) (string, error) {
	val, err := c.rdb.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("redis get failed for key %s: %w", key, err)
	}
	return val, nil
}

// Set stores a string cache value with TTL.
func (c *Client) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	if err := c.rdb.Set(ctx, key, value, ttl).Err(); err != nil {
		return fmt.Errorf("redis set failed for key %s: %w", key, err)
	}
	return nil
}

// Publish publishes a message payload to a Redis Pub/Sub channel.
func (c *Client) Publish(ctx context.Context, channel string, message any) error {
	if c == nil || c.rdb == nil {
		return nil
	}
	return c.rdb.Publish(ctx, channel, message).Err()
}

// Subscribe returns a Redis PubSub handle for the given channels.
func (c *Client) Subscribe(ctx context.Context, channels ...string) *redis.PubSub {
	if c == nil || c.rdb == nil {
		return nil
	}
	return c.rdb.Subscribe(ctx, channels...)
}

// RawClient returns the underlying *redis.Client for advanced scripts or custom limiters.
func (c *Client) RawClient() *redis.Client {
	if c == nil {
		return nil
	}
	return c.rdb
}
