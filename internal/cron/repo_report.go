package cron

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/pkg/models"
)

// RepoReportCron generates periodic engineering quality, security health,
// and DORA performance digests for repository maintainers.
type RepoReportCron struct {
	repo       *database.Repository
	interval   time.Duration
	windowDays int
}

// NewRepoReportCron creates a periodic repository digest report generation cron job.
func NewRepoReportCron(repo *database.Repository, interval time.Duration, windowDays int) *RepoReportCron {
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	if windowDays <= 0 {
		windowDays = 15
	}
	return &RepoReportCron{
		repo:       repo,
		interval:   interval,
		windowDays: windowDays,
	}
}

func (r *RepoReportCron) Name() string {
	return "RepoReportCron"
}

func (r *RepoReportCron) Interval() time.Duration {
	return r.interval
}

func (r *RepoReportCron) Run(ctx context.Context) error {
	if r.repo == nil {
		return nil
	}

	start := time.Now()
	since := time.Now().AddDate(0, 0, -r.windowDays)

	reports, err := r.repo.GetRepositoryReportsData(ctx, since)
	if err != nil {
		slog.Error("RepoReportCron failed compiling repository data", "error", err)
		return err
	}

	if len(reports) == 0 {
		return nil
	}

	dispatchedCount := 0
	for _, rd := range reports {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		payload, _ := json.Marshal(map[string]any{
			"workspace_id":         rd.WorkspaceID.String(),
			"repository_id":        rd.RepositoryID.String(),
			"namespace":            rd.NamespacePath,
			"window_days":          r.windowDays,
			"window_since":         since.Format(time.RFC3339),
			"total_reviews":        rd.TotalReviews,
			"clean_reviews_count":  rd.CleanReviewsCount,
			"pass_rate":            rd.PassRate,
			"total_findings":       rd.TotalFindings,
			"critical_findings":    rd.CriticalFindings,
			"high_findings":        rd.HighFindings,
			"active_contributors":  rd.ActiveContributors,
			"generated_at":         time.Now().UTC().Format(time.RFC3339),
		})

		outbox := &models.OutboxRecord{
			ID:          uuid.New(),
			WorkspaceID: rd.WorkspaceID,
			EventType:   "repo.digest.generated",
			Payload:     payload,
			Status:      models.OutboxPending,
			CreatedAt:   time.Now().UTC(),
		}

		if err := r.repo.InsertOutboxEvent(ctx, outbox); err == nil {
			dispatchedCount++
			slog.Debug("Generated repository health digest",
				"repo", rd.NamespacePath,
				"pass_rate", rd.PassRate,
				"reviews", rd.TotalReviews,
			)
		}
	}

	slog.Info("Completed periodic repository report sweep",
		"repositories_evaluated", len(reports),
		"reports_dispatched", dispatchedCount,
		"window_days", r.windowDays,
		"duration", time.Since(start),
	)
	return nil
}
