package try

import (
	"sync"
	"time"
)

// RateLimiter tracks client submissions per fingerprint within a sliding time window.
type RateLimiter struct {
	mu           sync.Mutex
	limit        int
	window       time.Duration
	fingerprints map[string][]time.Time
}

// RateLimitResult reports remaining quota and reset timing.
type RateLimitResult struct {
	Allowed   bool      `json:"allowed"`
	Remaining int       `json:"remaining"`
	Limit     int       `json:"limit"`
	ResetAt   time.Time `json:"resetAt"`
}

// NewRateLimiter initializes the fingerprint rate limiter (default: 2 reviews per 1 hour).
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	if limit <= 0 {
		limit = 2
	}
	if window <= 0 {
		window = 1 * time.Hour
	}
	return &RateLimiter{
		limit:        limit,
		window:       window,
		fingerprints: make(map[string][]time.Time),
	}
}

// CheckAndRecord evaluates whether a fingerprint is within quota and records the submission.
func (r *RateLimiter) CheckAndRecord(fingerprint string) RateLimitResult {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	cutoff := now.Add(-r.window)

	// Clean expired timestamps
	timestamps := r.fingerprints[fingerprint]
	var active []time.Time
	for _, t := range timestamps {
		if t.After(cutoff) {
			active = append(active, t)
		}
	}

	// Evict entries with no active timestamps to prevent unbounded map growth.
	if len(active) == 0 && len(timestamps) > 0 {
		delete(r.fingerprints, fingerprint)
	}

	if len(active) >= r.limit {
		oldest := active[0]
		resetAt := oldest.Add(r.window)
		return RateLimitResult{
			Allowed:   false,
			Remaining: 0,
			Limit:     r.limit,
			ResetAt:   resetAt,
		}
	}

	// Record submission
	active = append(active, now)
	r.fingerprints[fingerprint] = active

	remaining := r.limit - len(active)
	resetAt := active[0].Add(r.window)

	return RateLimitResult{
		Allowed:   true,
		Remaining: remaining,
		Limit:     r.limit,
		ResetAt:   resetAt,
	}
}
