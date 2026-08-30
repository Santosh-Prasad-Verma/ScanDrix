package cron

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/scandrix/backend/internal/database"
)

// StaleReviewWatchdog periodically reaps pull request reviews stuck in IN_PROGRESS states.
type StaleReviewWatchdog struct {
	repo           *database.Repository
	interval       time.Duration
	timeoutMinutes int
}

// NewStaleReviewWatchdog creates a watchdog cron job (default: checks every 15m for reviews older than 30m).
func NewStaleReviewWatchdog(repo *database.Repository, interval time.Duration, timeoutMinutes int) *StaleReviewWatchdog {
	if interval <= 0 {
		interval = 15 * time.Minute
	}
	if timeoutMinutes <= 0 {
		timeoutMinutes = 30
	}
	return &StaleReviewWatchdog{
		repo:           repo,
		interval:       interval,
		timeoutMinutes: timeoutMinutes,
	}
}

func (w *StaleReviewWatchdog) Name() string {
	return "StaleReviewWatchdog"
}

func (w *StaleReviewWatchdog) Interval() time.Duration {
	return w.interval
}

func (w *StaleReviewWatchdog) Run(ctx context.Context) error {
	if w.repo == nil {
		return nil
	}

	reaped, err := w.repo.TimeoutStaleReviews(ctx, w.timeoutMinutes)
	if err != nil {
		return fmt.Errorf("stale review watchdog failed: %w", err)
	}

	if reaped > 0 {
		slog.Warn("Stale reviews reaped and marked failed", "reaped_count", reaped, "timeout_minutes", w.timeoutMinutes)
	}
	return nil
}
