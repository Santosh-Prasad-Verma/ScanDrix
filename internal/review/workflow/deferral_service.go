package workflow

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/review/domain"
)

// PrReviewDeferral holds delay duration and retry attempt number.
type PrReviewDeferral struct {
	DelayMs       int
	DeferredCount int
}

// PrReviewDeferralService manages debounce windows and concurrency locks per PR.
type PrReviewDeferralService struct {
	mu            sync.Mutex
	activeReviews map[string]time.Time // prKey -> leaseExpiry
	deferralCount map[string]int       // prKey -> count
	baseDelay     time.Duration
	maxDelay      time.Duration
	maxDeferrals  int
	leaseDuration time.Duration
}

// NewPrReviewDeferralService constructs a deferral service.
func NewPrReviewDeferralService() *PrReviewDeferralService {
	return &PrReviewDeferralService{
		activeReviews: make(map[string]time.Time),
		deferralCount: make(map[string]int),
		baseDelay:     15 * time.Second,
		maxDelay:      60 * time.Second,
		maxDeferrals:  26,
		leaseDuration: 15 * time.Minute,
	}
}

func (s *PrReviewDeferralService) prKey(repoID string, prNumber int) string {
	return fmt.Sprintf("%s:%d", repoID, prNumber)
}

// ShouldDefer returns true if another review is actively running on this pull request.
func (s *PrReviewDeferralService) ShouldDefer(ctx context.Context, repoID string, prNumber int, headSHA string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := s.prKey(repoID, prNumber)
	expiry, exists := s.activeReviews[key]
	if exists && time.Now().Before(expiry) {
		s.deferralCount[key]++
		return true, nil
	}

	return false, nil
}

// MarkStarted locks the pull request for review execution.
func (s *PrReviewDeferralService) MarkStarted(ctx context.Context, repoID string, prNumber int, headSHA string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := s.prKey(repoID, prNumber)
	s.activeReviews[key] = time.Now().Add(s.leaseDuration)
	return nil
}

// MarkCompleted releases the PR review lock and resets deferral counters.
func (s *PrReviewDeferralService) MarkCompleted(ctx context.Context, repoID string, prNumber int, headSHA string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := s.prKey(repoID, prNumber)
	delete(s.activeReviews, key)
	delete(s.deferralCount, key)
	return nil
}

// CalculateDelay computes exponential backoff for a colliding review attempt.
func (s *PrReviewDeferralService) CalculateDelay(repoID string, prNumber int) (time.Duration, int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := s.prKey(repoID, prNumber)
	count := s.deferralCount[key]
	if count > s.maxDeferrals {
		return 0, count, false // exceeded max deferrals
	}

	shift := count - 1
	if shift < 0 {
		shift = 0
	}
	if shift > 10 {
		shift = 10
	}

	delay := s.baseDelay * (1 << shift)
	if delay > s.maxDelay {
		delay = s.maxDelay
	}

	return delay, count, true
}

// Verify PrReviewDeferralService satisfies domain.IPrReviewDeferralService.
var _ domain.IPrReviewDeferralService = (*PrReviewDeferralService)(nil)
