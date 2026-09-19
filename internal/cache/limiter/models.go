package limiter

import (
	"time"
)

// RateLimitConfig defines bucket limits and refill dynamics.
type RateLimitConfig struct {
	Capacity          float64       `json:"capacity"`
	RefillRatePerSec  float64       `json:"refill_rate_per_sec"`
	ExpirationTimeout time.Duration `json:"expiration_timeout"`
}

// RateLimitResult reports whether a request passed the rate limiter.
type RateLimitResult struct {
	Allowed    bool          `json:"allowed"`
	Remaining  int           `json:"remaining"`
	ResetAfter time.Duration `json:"reset_after"`
	RetryAfter time.Duration `json:"retry_after,omitempty"`
	Degraded   bool          `json:"degraded,omitempty"` // true if evaluated via local in-memory fallback during Redis outage
}

// DistributedLock represents an active mutual exclusion lease on a resource.
type DistributedLock struct {
	ResourceKey  string    `json:"resource_key"`
	OwnerID      string    `json:"owner_id"`
	FencingToken int64     `json:"fencing_token"`
	AcquiredAt   time.Time `json:"acquired_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// CacheItem stores a typed or binary value with time-to-live.
type CacheItem struct {
	Value     any       `json:"value"`
	ExpiresAt time.Time `json:"expires_at"`
}
