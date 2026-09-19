package clireview

import (
	"time"

	"github.com/scandrix/backend/internal/clireview/infrastructure/adapters"
)

// TrialRateLimiter manages trial review rate limits per fingerprint.
type TrialRateLimiter = adapters.TrialRateLimiter

// TrialRateLimitResult contains rate limit assessment.
type TrialRateLimitResult = adapters.TrialRateLimitResult

// NewTrialRateLimiter creates a rate limiter for unauthenticated trial reviews.
func NewTrialRateLimiter(limit int, window time.Duration) *TrialRateLimiter {
	return adapters.NewTrialRateLimiter(limit, window)
}

// AuthenticatedRateLimiter tracks team-level review concurrency and limits.
type AuthenticatedRateLimiter = adapters.AuthenticatedRateLimiter

// NewAuthenticatedRateLimiter creates a concurrency gate for authenticated reviews.
func NewAuthenticatedRateLimiter(maxPerTeam int) *AuthenticatedRateLimiter {
	return adapters.NewAuthenticatedRateLimiter(maxPerTeam)
}

// AuthenticatedRateLimiterService provides sliding-window rate limiting for authenticated teams (1000 req/hr).
type AuthenticatedRateLimiterService = adapters.AuthenticatedRateLimiterService

// NewAuthenticatedRateLimiterService creates a new AuthenticatedRateLimiterService.
func NewAuthenticatedRateLimiterService() *AuthenticatedRateLimiterService {
	return adapters.NewAuthenticatedRateLimiterService()
}
