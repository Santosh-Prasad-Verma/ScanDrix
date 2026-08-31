package cron

import (
	"context"
	"log/slog"
	"time"

	"github.com/scandrix/backend/internal/database"
)

// ReviewFeedbackCron synchronizes developer feedback (thumbs up/down, accepted fixes, dismissals)
// into rule confidence weights and fine-tuning datasets.
type ReviewFeedbackCron struct {
	repo     *database.Repository
	interval time.Duration
}

// NewReviewFeedbackCron creates a new feedback synchronization cron job.
func NewReviewFeedbackCron(repo *database.Repository, interval time.Duration) *ReviewFeedbackCron {
	if interval <= 0 {
		interval = 10 * time.Minute
	}
	return &ReviewFeedbackCron{
		repo:     repo,
		interval: interval,
	}
}

func (r *ReviewFeedbackCron) Name() string {
	return "ReviewFeedbackCron"
}

func (r *ReviewFeedbackCron) Interval() time.Duration {
	return r.interval
}

func (r *ReviewFeedbackCron) Run(ctx context.Context) error {
	if r.repo == nil {
		return nil
	}

	start := time.Now()
	workspaces, err := r.repo.ListWorkspaces(ctx)
	if err != nil {
		slog.Error("ReviewFeedbackCron failed listing workspaces", "error", err)
		return err
	}

	if len(workspaces) == 0 {
		return nil
	}

	totalSynced := int64(0)
	for _, ws := range workspaces {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		synced, err := r.repo.SyncFindingFeedbackSentiment(ctx, ws.ID)
		if err != nil {
			slog.Warn("Failed syncing feedback sentiment for workspace", "workspace_id", ws.ID, "error", err)
			continue
		}
		totalSynced += synced
	}

	if totalSynced > 0 {
		slog.Info("Completed review feedback synchronization",
			"events_synced", totalSynced,
			"workspaces_count", len(workspaces),
			"duration", time.Since(start),
		)
	}
	return nil
}
