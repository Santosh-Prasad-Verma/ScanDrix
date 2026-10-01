package limiter

import (
	"os"
	"strings"

	"github.com/redis/go-redis/v9"
)

// RedisClient is the subset of the Redis client the limiter needs. It is an
// alias so callers can pass *redis.Client without importing anything extra.
type RedisClient = *redis.Client

// DistributedRequired reports whether this deployment must not fall back to
// per-process rate limiting or lockout tracking.
//
// True for production and staging. False for development and test, where a single
// process is the intended topology and a local bucket gives the same answer a
// shared one would.
//
// Why this distinction decides security behaviour rather than convenience
// (AUDIT_REMEDIATION.md F-28/F-29/F-32): a per-process bucket is not "no limit",
// it is *N times the intended limit* for N replicas, and it is invisible. A
// brute-force budget of 10 attempts per 15 minutes silently becomes 10 per replica
// per 15 minutes. Failing closed outside development means an outage denies
// traffic rather than quietly removing a control that is assumed to be active.
func DistributedRequired() bool {
	for _, key := range []string{"APP_ENV", "ENVIRONMENT", "GO_ENV", "SCANDRIX_ENV"} {
		switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
		case "production", "prod", "staging":
			return true
		}
	}
	return false
}

// RedisTokenBucket returns a limiter for auth-style buckets, honouring the
// distributed requirement.
//
//   - rdb present: a Redis-backed bucket, fail-closed outside development.
//   - rdb absent, distributed required: a limiter that denies, because silently
//     substituting a local bucket is the defect.
//   - rdb absent, single process: a local bucket, which is correct here.
//
// It returns nil when no limiter should be installed, letting the caller keep
// whatever it already had.
func RedisTokenBucket(rdb RedisClient, config RateLimitConfig) RateLimiter {
	if rdb == nil {
		if DistributedRequired() {
			return NewUnavailableLimiter("no shared rate-limit store configured")
		}
		return NewTokenBucketLimiter(config)
	}

	l := NewRedisTokenBucketLimiter(rdb, config)
	if DistributedRequired() {
		l.SetFailClosed(true)
	}
	return l
}
