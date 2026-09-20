package adapters

import (
	"context"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/clireview/domain"
)

// TrialRateLimiter manages trial review rate limits per fingerprint.
type TrialRateLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time
	limit    int
	window   time.Duration
}

// NewTrialRateLimiter creates a rate limiter for unauthenticated trial reviews.
func NewTrialRateLimiter(limit int, window time.Duration) *TrialRateLimiter {
	if limit <= 0 {
		limit = 2
	}
	if window <= 0 {
		window = 1 * time.Hour
	}
	return &TrialRateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
}

// TrialRateLimitResult contains rate limit assessment.
type TrialRateLimitResult struct {
	Allowed   bool
	Remaining int
	Limit     int
	ResetAt   time.Time
}

// CheckRateLimit verifies if a fingerprint is permitted to execute a trial review.
func (rl *TrialRateLimiter) CheckRateLimit(fingerprint string) TrialRateLimitResult {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now().UTC()
	cutoff := now.Add(-rl.window)

	var valid []time.Time
	for _, t := range rl.requests[fingerprint] {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= rl.limit {
		var oldest time.Time
		if len(valid) > 0 {
			oldest = valid[0]
		} else {
			oldest = now
		}
		resetAt := oldest.Add(rl.window)
		rl.requests[fingerprint] = valid
		return TrialRateLimitResult{
			Allowed:   false,
			Remaining: 0,
			Limit:     rl.limit,
			ResetAt:   resetAt,
		}
	}

	valid = append(valid, now)
	rl.requests[fingerprint] = valid

	resetAt := now.Add(rl.window)
	return TrialRateLimitResult{
		Allowed:   true,
		Remaining: rl.limit - len(valid),
		Limit:     rl.limit,
		ResetAt:   resetAt,
	}
}

// CheckRateLimitWithContext implements ITrialRateLimiterService.
func (rl *TrialRateLimiter) CheckRateLimitWithContext(ctx context.Context, fingerprint string) (*domain.RateLimitResult, error) {
	res := rl.CheckRateLimit(fingerprint)
	return &domain.RateLimitResult{
		Allowed:   res.Allowed,
		Remaining: res.Remaining,
		ResetAt:   &res.ResetAt,
	}, nil
}

// InspectRateLimit inspects the current rate limit status for a fingerprint without consuming a request.
func (rl *TrialRateLimiter) InspectRateLimit(fingerprint string) TrialRateLimitResult {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now().UTC()
	cutoff := now.Add(-rl.window)

	var valid []time.Time
	for _, t := range rl.requests[fingerprint] {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	remaining := rl.limit - len(valid)
	if remaining < 0 {
		remaining = 0
	}

	var resetAt time.Time
	if len(valid) > 0 {
		resetAt = valid[0].Add(rl.window)
	} else {
		resetAt = now.Add(rl.window)
	}

	return TrialRateLimitResult{
		Allowed:   remaining > 0,
		Remaining: remaining,
		Limit:     rl.limit,
		ResetAt:   resetAt,
	}
}

// AuthenticatedRateLimiter tracks team-level review concurrency and limits.
type AuthenticatedRateLimiter struct {
	mu         sync.Mutex
	activeJobs map[string]int // teamId -> active concurrent reviews
	maxPerTeam int
}

// NewAuthenticatedRateLimiter creates a concurrency gate for authenticated reviews.
func NewAuthenticatedRateLimiter(maxPerTeam int) *AuthenticatedRateLimiter {
	if maxPerTeam <= 0 {
		maxPerTeam = 10
	}
	return &AuthenticatedRateLimiter{
		activeJobs: make(map[string]int),
		maxPerTeam: maxPerTeam,
	}
}

// Acquire attempts to increment active review count for a team.
func (ar *AuthenticatedRateLimiter) Acquire(teamID string) bool {
	ar.mu.Lock()
	defer ar.mu.Unlock()

	curr := ar.activeJobs[teamID]
	if curr >= ar.maxPerTeam {
		return false
	}
	ar.activeJobs[teamID] = curr + 1
	return true
}

// Release decrements active review count for a team.
func (ar *AuthenticatedRateLimiter) Release(teamID string) {
	ar.mu.Lock()
	defer ar.mu.Unlock()

	curr := ar.activeJobs[teamID]
	if curr > 1 {
		ar.activeJobs[teamID] = curr - 1
	} else {
		delete(ar.activeJobs, teamID)
	}
}

// AuthenticatedRateLimiterService provides sliding-window rate limiting for authenticated teams (1000 req/hr).
type AuthenticatedRateLimiterService struct {
	mu            sync.Mutex
	store         map[string][]time.Time
	limit         int
	window        time.Duration
	fallbackLimit int
}

// NewAuthenticatedRateLimiterService creates a new AuthenticatedRateLimiterService.
func NewAuthenticatedRateLimiterService() *AuthenticatedRateLimiterService {
	return &AuthenticatedRateLimiterService{
		store:         make(map[string][]time.Time),
		limit:         1000,
		window:        1 * time.Hour,
		fallbackLimit: 100,
	}
}

// CheckRateLimit implements IAuthenticatedRateLimiterService.
func (s *AuthenticatedRateLimiterService) CheckRateLimit(ctx context.Context, teamID string) (*domain.AuthenticatedRateLimitResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	cutoff := now.Add(-s.window)

	var valid []time.Time
	for _, t := range s.store[teamID] {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= s.limit {
		var oldest time.Time
		if len(valid) > 0 {
			oldest = valid[0]
		} else {
			oldest = now
		}
		resetAt := oldest.Add(s.window)
		s.store[teamID] = valid
		return &domain.AuthenticatedRateLimitResult{
			Allowed:   false,
			Remaining: 0,
			ResetAt:   &resetAt,
		}, nil
	}

	valid = append(valid, now)
	s.store[teamID] = valid
	resetAt := now.Add(s.window)

	return &domain.AuthenticatedRateLimitResult{
		Allowed:   true,
		Remaining: s.limit - len(valid),
		ResetAt:   &resetAt,
	}, nil
}
