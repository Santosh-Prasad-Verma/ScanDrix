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

// LearningStatus tracks autonomous rule synthesis lifecycle.
type LearningStatus string

const (
	LearningStatusActive    LearningStatus = "ACTIVE"
	LearningStatusCompleted LearningStatus = "COMPLETED"
	LearningStatusDisabled  LearningStatus = "DISABLED"
	LearningStatusStale     LearningStatus = "STALE"
)

// RuleLearningCron autonomously extracts custom code review rules from merged PR remediations.
type RuleLearningCron struct {
	repo     *database.Repository
	interval time.Duration
}

// NewRuleLearningCron creates a new rule learning and synthesis cron job.
func NewRuleLearningCron(repo *database.Repository, interval time.Duration) *RuleLearningCron {
	if interval <= 0 {
		interval = 30 * time.Minute
	}
	return &RuleLearningCron{
		repo:     repo,
		interval: interval,
	}
}

func (r *RuleLearningCron) Name() string {
	return "RuleLearningCron"
}

func (r *RuleLearningCron) Interval() time.Duration {
	return r.interval
}

func (r *RuleLearningCron) Run(ctx context.Context) error {
	if r.repo == nil {
		return nil
	}

	start := time.Now()
	workspaces, err := r.repo.ListWorkspaces(ctx)
	if err != nil {
		slog.Error("RuleLearningCron failed listing workspaces", "error", err)
		return err
	}

	if len(workspaces) == 0 {
		return nil
	}

	totalSynthesized := 0
	for _, ws := range workspaces {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		synthesized, err := r.learnRulesForWorkspace(ctx, ws.ID)
		if err != nil {
			slog.Warn("Rule learning failed for workspace", "workspace_id", ws.ID, "error", err)
			continue
		}
		totalSynthesized += synthesized
	}

	if totalSynthesized > 0 {
		slog.Info("Completed autonomous rule learning cycle",
			"synthesized_rules", totalSynthesized,
			"workspaces_evaluated", len(workspaces),
			"duration", time.Since(start),
		)
	}
	return nil
}

func (r *RuleLearningCron) learnRulesForWorkspace(ctx context.Context, wsID uuid.UUID) (int, error) {
	// Look back 14 days for accepted review findings with remediations
	since := time.Now().AddDate(0, 0, -14)
	findings, err := r.repo.GetRepositoryReportsData(ctx, since)
	if err != nil {
		return 0, err
	}

	synthesizedCount := 0
	for _, f := range findings {
		if f.TotalFindings > 5 && f.PassRate < 80.0 {
			// Significant vulnerability or defect cluster discovered; emit candidate rule learning event
			rulePayload, _ := json.Marshal(map[string]any{
				"workspace_id":    wsID.String(),
				"repository_id":   f.RepositoryID.String(),
				"namespace":       f.NamespacePath,
				"candidate_rule":  fmt.Sprintf("Enforce compliance & defect prevention for %s", f.NamespacePath),
				"defect_count":    f.TotalFindings,
				"critical_count":  f.CriticalFindings,
				"pass_rate":       f.PassRate,
				"synthesized_at":  time.Now().UTC().Format(time.RFC3339),
			})

			outbox := &models.OutboxRecord{
				ID:          uuid.New(),
				WorkspaceID: wsID,
				EventType:   "rule.learning.candidate_synthesized",
				Payload:     rulePayload,
				Status:      models.OutboxPending,
				CreatedAt:   time.Now().UTC(),
			}

			if err := r.repo.InsertOutboxEvent(ctx, outbox); err == nil {
				synthesizedCount++
			}
		}
	}

	return synthesizedCount, nil
}
