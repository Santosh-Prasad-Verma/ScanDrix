package limiter

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// RateLimiter is a common interface for in-memory and distributed token bucket limiters.
type RateLimiter interface {
	Allow(ctx context.Context, key string, tokens float64) (*RateLimitResult, error)
}

// Token bucket atomic Lua script executed on Redis clusters.
// Uses Redis server monotonic TIME to prevent cross-instance clock drift.
var tokenBucketLuaScript = redis.NewScript(`
local key = KEYS[1]
local capacity = tonumber(ARGV[1])
local refill_rate = tonumber(ARGV[2])
local requested = tonumber(ARGV[3])
local ttl = tonumber(ARGV[4])

local time_result = redis.call("TIME")
local now = tonumber(time_result[1]) + (tonumber(time_result[2]) / 1e6)

local data = redis.call("HMGET", key, "tokens", "last_refill")
local tokens = tonumber(data[1])
local last_refill = tonumber(data[2])

if not tokens then
    tokens = capacity
    last_refill = now
else
    local elapsed = math.max(0, now - last_refill)
    tokens = math.min(capacity, tokens + (elapsed * refill_rate))
    last_refill = now
end

local allowed = 0
local retry_after_ms = 0

if tokens >= requested then
    tokens = tokens - requested
    allowed = 1
else
    local needed = requested - tokens
    retry_after_ms = math.ceil((needed / refill_rate) * 1000)
end

redis.call("HSET", key, "tokens", tokens, "last_refill", last_refill)
redis.call("EXPIRE", key, ttl)

return {allowed, math.floor(tokens), retry_after_ms}
`)

const (
	maxConsecutiveFailures = 5
	circuitCooldown        = 5 * time.Second
	warnLogThrottle        = 5 * time.Second
)

// RedisTokenBucketLimiter implements a multi-instance, horizontally-scaled distributed rate limiter.
type RedisTokenBucketLimiter struct {
	rdb        *redis.Client
	config     RateLimitConfig
	fallback   *TokenBucketLimiter
	failClosed bool

	// Circuit breaker state
	failureCount     atomic.Int32
	circuitOpenUntil atomic.Int64 // Unix nanoseconds
	lastWarnLog      atomic.Int64 // Unix nanoseconds
}

// NewRedisTokenBucketLimiter constructs a distributed rate limiter with automatic in-memory fallback.
func NewRedisTokenBucketLimiter(rdb *redis.Client, config RateLimitConfig) *RedisTokenBucketLimiter {
	if config.Capacity <= 0 {
		config.Capacity = 60
	}
	if config.RefillRatePerSec <= 0 {
		config.RefillRatePerSec = 10
	}
	if config.ExpirationTimeout <= 0 {
		config.ExpirationTimeout = 10 * time.Minute
	}

	return &RedisTokenBucketLimiter{
		rdb:      rdb,
		config:   config,
		fallback: NewTokenBucketLimiter(config),
	}
}

// SetFailClosed configures whether outages reject all requests (fail closed) or degrade to local memory (fail open).
func (l *RedisTokenBucketLimiter) SetFailClosed(failClosed bool) {
	l.failClosed = failClosed
}

// isCircuitOpen checks if Redis is currently tripped due to repeated failures.
func (l *RedisTokenBucketLimiter) isCircuitOpen() bool {
	openUntil := l.circuitOpenUntil.Load()
	if openUntil == 0 {
		return false
	}
	now := time.Now().UnixNano()
	if now < openUntil {
		return true
	}
	// Cooldown expired, permit half-open trial
	return false
}

func (l *RedisTokenBucketLimiter) recordSuccess() {
	l.failureCount.Store(0)
	l.circuitOpenUntil.Store(0)
}

func (l *RedisTokenBucketLimiter) recordFailure() {
	fails := l.failureCount.Add(1)
	if fails >= maxConsecutiveFailures {
		now := time.Now()
		l.circuitOpenUntil.Store(now.Add(circuitCooldown).UnixNano())
	}
}

func (l *RedisTokenBucketLimiter) logThrottledWarning(key string, err error) {
	now := time.Now().UnixNano()
	last := l.lastWarnLog.Load()
	if now-last > int64(warnLogThrottle) {
		if l.lastWarnLog.CompareAndSwap(last, now) {
			slog.Warn("Distributed Redis rate limiter degraded; falling back",
				"key", key,
				"error", err,
				"fail_closed", l.failClosed,
				"circuit_open", l.isCircuitOpen(),
			)
		}
	}
}

// Allow atomically draws tokens from the distributed Redis bucket using server-side monotonic time.
func (l *RedisTokenBucketLimiter) Allow(ctx context.Context, key string, tokens float64) (*RateLimitResult, error) {
	if key == "" {
		return nil, errors.New("rate limit key cannot be empty")
	}

	if tokens <= 0 {
		tokens = 1
	}

	// Requests larger than maximum capacity can never succeed
	if tokens > l.config.Capacity {
		return &RateLimitResult{
			Allowed:    false,
			Remaining:  0,
			RetryAfter: -1,
		}, nil
	}

	// If client is uninitialized or circuit breaker is tripped, execute fallback or fail closed immediately
	if l.rdb == nil || l.isCircuitOpen() {
		if l.failClosed {
			return &RateLimitResult{
				Allowed:    false,
				Degraded:   true,
				Remaining:  0,
				RetryAfter: circuitCooldown,
			}, nil
		}
		res, err := l.fallback.Allow(ctx, key, tokens)
		if res != nil {
			res.Degraded = true
		}
		return res, err
	}

	redisKey := fmt.Sprintf("ratelimit:%s", key)
	ttlSec := int(l.config.ExpirationTimeout.Seconds())
	if ttlSec <= 0 {
		ttlSec = 60
	}

	evalCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	val, err := tokenBucketLuaScript.Run(evalCtx, l.rdb, []string{redisKey},
		l.config.Capacity,
		l.config.RefillRatePerSec,
		tokens,
		ttlSec,
	).Result()

	if err != nil {
		l.recordFailure()
		l.logThrottledWarning(key, err)

		if l.failClosed {
			return &RateLimitResult{
				Allowed:    false,
				Degraded:   true,
				Remaining:  0,
				RetryAfter: circuitCooldown,
			}, nil
		}

		res, fallbackErr := l.fallback.Allow(ctx, key, tokens)
		if res != nil {
			res.Degraded = true
		}
		return res, fallbackErr
	}

	// Successful Redis response
	l.recordSuccess()

	results, ok := val.([]any)
	if !ok || len(results) < 3 {
		l.recordFailure()
		l.logThrottledWarning(key, errors.New("malformed redis limiter response"))
		res, fallbackErr := l.fallback.Allow(ctx, key, tokens)
		if res != nil {
			res.Degraded = true
		}
		return res, fallbackErr
	}

	allowedInt, _ := results[0].(int64)
	remainingInt, _ := results[1].(int64)
	retryAfterMs, _ := results[2].(int64)

	allowed := allowedInt == 1
	res := &RateLimitResult{
		Allowed:    allowed,
		Remaining:  int(remainingInt),
		ResetAfter: time.Duration((l.config.Capacity-float64(remainingInt))/l.config.RefillRatePerSec) * time.Second,
		Degraded:   false,
	}

	if !allowed {
		retryDuration := time.Duration(retryAfterMs) * time.Millisecond
		if retryDuration < time.Second {
			retryDuration = time.Second
		}
		res.RetryAfter = retryDuration
	}

	return res, nil
}
