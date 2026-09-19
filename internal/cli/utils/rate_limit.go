// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package utils

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"os"
	"os/user"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TrialStatusFetcher abstracts the API call to check trial status.
type TrialStatusFetcher interface {
	GetTrialStatus(ctx context.Context, fingerprint string) (*TrialStatus, error)
}

// TrialIdentifier computes a deterministic 32-character fingerprint for rate limiting and trials.
func TrialIdentifier(workDir string) string {
	devID, err := GetOrCreateDeviceID()
	if err != nil || devID == "" {
		devID = "anonymous-device"
	}

	username := "user"
	if u, err := user.Current(); err == nil && u.Username != "" {
		username = u.Username
	}

	repoPath := workDir
	if repoPath == "" {
		if cwd, err := os.Getwd(); err == nil {
			repoPath = cwd
		}
	}

	raw := fmt.Sprintf("%s:%s:%s", devID, username, repoPath)
	hash := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(hash[:])[:32]
}

// CheckTrialQuota queries for trial quota status using a TrialStatusFetcher.
func CheckTrialQuota(ctx context.Context, fetcher TrialStatusFetcher, workDir string) (*TrialStatus, error) {
	if fetcher == nil {
		return nil, fmt.Errorf("trial status fetcher is required")
	}
	fingerprint := TrialIdentifier(workDir)
	return fetcher.GetTrialStatus(ctx, fingerprint)
}

// ParseRetryAfter extracts delay from an HTTP 429 Retry-After header.
func ParseRetryAfter(headerVal string) time.Duration {
	v := strings.TrimSpace(headerVal)
	if v == "" {
		return 0
	}

	// 1. Try integer seconds
	if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}

	// 2. Try HTTP-date
	if t, err := http.ParseTime(v); err == nil {
		diff := time.Until(t)
		if diff > 0 {
			return diff
		}
	}

	return 0
}

// ComputeBackoffDuration calculates exponential backoff with full jitter.
func ComputeBackoffDuration(attempt int, baseDelay, maxDelay time.Duration) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	if baseDelay <= 0 {
		baseDelay = 100 * time.Millisecond
	}
	if maxDelay <= 0 {
		maxDelay = 30 * time.Second
	}

	multiplier := math.Pow(2, float64(attempt))
	tempDelay := float64(baseDelay) * multiplier

	if tempDelay > float64(maxDelay) {
		tempDelay = float64(maxDelay)
	}

	// Full jitter: uniformly distributed between 0 and tempDelay
	sleep := rand.Float64() * tempDelay
	return time.Duration(sleep)
}

// ClientRateLimiter tracks rate limits and schedules backoff delays.
type ClientRateLimiter struct {
	mu          sync.Mutex
	blockedUntil time.Time
}

// NewClientRateLimiter creates an in-memory client rate limiter.
func NewClientRateLimiter() *ClientRateLimiter {
	return &ClientRateLimiter{}
}

// RecordRateLimit records that a 429 was received with a specified retry-after delay.
func (r *ClientRateLimiter) RecordRateLimit(retryAfter time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if retryAfter <= 0 {
		retryAfter = 5 * time.Second
	}
	r.blockedUntil = time.Now().Add(retryAfter)
}

// WaitIfBlocked blocks until the rate limit expires, or until ctx is cancelled.
func (r *ClientRateLimiter) WaitIfBlocked(ctx context.Context) error {
	r.mu.Lock()
	blocked := r.blockedUntil
	r.mu.Unlock()

	now := time.Now()
	if blocked.After(now) {
		waitDur := blocked.Sub(now)
		select {
		case <-time.After(waitDur):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
