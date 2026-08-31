package cron

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/scandrix/backend/internal/database"
)

// CheckPRApprovalCron periodically evaluates completed reviews to trigger automated approvals on green PRs.
type CheckPRApprovalCron struct {
	repo       *database.Repository
	interval   time.Duration
	batchLimit int
}

// NewCheckPRApprovalCron creates a new PR automated approval evaluation cron job.
func NewCheckPRApprovalCron(repo *database.Repository, interval time.Duration, batchLimit int) *CheckPRApprovalCron {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	if batchLimit <= 0 {
		batchLimit = 25
	}
	return &CheckPRApprovalCron{
		repo:       repo,
		interval:   interval,
		batchLimit: batchLimit,
	}
}

func (c *CheckPRApprovalCron) Name() string {
	return "CheckPRApprovalCron"
}

func (c *CheckPRApprovalCron) Interval() time.Duration {
	return c.interval
}

func (c *CheckPRApprovalCron) Run(ctx context.Context) error {
	if c.repo == nil {
		return nil
	}

	start := time.Now()
	pending, err := c.repo.GetPendingApprovalReviews(ctx, c.batchLimit)
	if err != nil {
		slog.Error("Failed querying pending approval reviews", "error", err)
		return err
	}

	if len(pending) == 0 {
		return nil
	}

	slog.Info("Evaluating pull requests for automated approval", "candidates", len(pending))

	approvedCount := 0
	for _, pr := range pending {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Security assertion: strictly 0 critical and 0 high findings allowed
		if pr.CriticalCount == 0 && pr.HighCount == 0 {
			reason := fmt.Sprintf("ScanDrix Automated Assurance: All checks passed with %d minor findings", pr.FindingsCount)
			if err := c.repo.RecordReviewApproval(ctx, pr.WorkspaceID, pr.ReviewID, pr.PullNumber, reason); err != nil {
				slog.Warn("Failed recording review approval event", "review_id", pr.ReviewID, "error", err)
				continue
			}
			approvedCount++
			slog.Debug("Automated PR approval recorded",
				"review_id", pr.ReviewID,
				"workspace_id", pr.WorkspaceID,
				"pull_number", pr.PullNumber,
				"head_sha", pr.HeadSHA,
			)
		}
	}

	slog.Info("Completed PR automated approval evaluation",
		"approved", approvedCount,
		"total_evaluated", len(pending),
		"duration", time.Since(start),
	)
	return nil
}
