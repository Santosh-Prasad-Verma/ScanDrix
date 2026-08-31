package cron

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/pkg/models"
)

// SpendLimitAlertCron evaluates monthly token and compute spend across all workspaces,
// triggering proactive multi-tier alerts when thresholds (80%, 90%, 100%) are reached.
type SpendLimitAlertCron struct {
	repo     *database.Repository
	interval time.Duration
}

// NewSpendLimitAlertCron creates a new spend limit alerting cron job.
func NewSpendLimitAlertCron(repo *database.Repository, interval time.Duration) *SpendLimitAlertCron {
	if interval <= 0 {
		interval = 1 * time.Hour
	}
	return &SpendLimitAlertCron{
		repo:     repo,
		interval: interval,
	}
}

func (s *SpendLimitAlertCron) Name() string {
	return "SpendLimitAlertCron"
}

func (s *SpendLimitAlertCron) Interval() time.Duration {
	return s.interval
}

func (s *SpendLimitAlertCron) Run(ctx context.Context) error {
	if s.repo == nil {
		return nil
	}

	start := time.Now()
	evaluations, err := s.repo.GetWorkspacesSpendEvaluation(ctx)
	if err != nil {
		slog.Error("SpendLimitAlertCron failed querying spend evaluations", "error", err)
		return err
	}

	if len(evaluations) == 0 {
		return nil
	}

	alertsDispatched := 0
	for _, e := range evaluations {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if e.UsagePercentage >= 80.0 {
			alertLevel := "WARNING_80_PERCENT"
			if e.UsagePercentage >= 100.0 {
				alertLevel = "HARD_LIMIT_100_PERCENT"
			} else if e.UsagePercentage >= 90.0 {
				alertLevel = "CRITICAL_90_PERCENT"
			}

			payload, _ := json.Marshal(map[string]any{
				"workspace_id":       e.WorkspaceID.String(),
				"organization_name":  e.OrganizationName,
				"owner_email":        e.OwnerEmail,
				"alert_level":        alertLevel,
				"spend_limit_usd":    e.MonthlySpendLimit,
				"current_spend_usd":  e.CurrentSpendUSD,
				"usage_percentage":   e.UsagePercentage,
				"total_tokens_used":  e.TotalTokensUsed,
				"evaluated_at":       time.Now().UTC().Format(time.RFC3339),
			})

			outbox := &models.OutboxRecord{
				ID:          uuid.New(),
				WorkspaceID: e.WorkspaceID,
				EventType:   fmt.Sprintf("spend_limit.alert.%s", alertLevel),
				Payload:     payload,
				Status:      models.OutboxPending,
				CreatedAt:   time.Now().UTC(),
			}

			if err := s.repo.InsertOutboxEvent(ctx, outbox); err == nil {
				alertsDispatched++
				slog.Warn("Dispatched spend limit threshold alert",
					"workspace_id", e.WorkspaceID,
					"org", e.OrganizationName,
					"alert_level", alertLevel,
					"usage_pct", fmt.Sprintf("%.1f%%", e.UsagePercentage),
					"spend_usd", fmt.Sprintf("$%.2f", e.CurrentSpendUSD),
					"limit_usd", fmt.Sprintf("$%.2f", e.MonthlySpendLimit),
				)
			}
		}
	}

	slog.Info("Completed spend limit evaluation sweep",
		"workspaces_evaluated", len(evaluations),
		"alerts_dispatched", alertsDispatched,
		"duration", time.Since(start),
	)
	return nil
}
