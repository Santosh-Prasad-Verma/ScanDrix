package review

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/llm"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/internal/storage"
	"github.com/scandrix/backend/pkg/models"
)

// Orchestrator coordinates the end-to-end pull request code review pipeline.
type Orchestrator struct {
	repo           *database.Repository
	llmGateway     *llm.Gateway
	artifactClient *storage.ArtifactClient
	rulesEvaluator *rules.Evaluator
}

// NewOrchestrator initializes the review orchestration engine.
func NewOrchestrator(
	repo *database.Repository,
	llmGateway *llm.Gateway,
	artifactClient *storage.ArtifactClient,
	rulesEvaluator *rules.Evaluator,
) *Orchestrator {
	return &Orchestrator{
		repo:           repo,
		llmGateway:     llmGateway,
		artifactClient: artifactClient,
		rulesEvaluator: rulesEvaluator,
	}
}

// ExecutionTask represents the input job submitted to the review pipeline.
type ExecutionTask struct {
	ReviewID      uuid.UUID
	WorkspaceID   uuid.UUID
	RepositoryID  uuid.UUID
	RepoNamespace string
	PullNumber    int
	Title         string
	HeadSHA       string
	BaseSHA       string
	Author        string
	RawDiff       string
}

// ProcessReview executes all analysis stages, records findings, and uploads scan artifacts.
func (o *Orchestrator) ProcessReview(ctx context.Context, task ExecutionTask) error {
	slog.Info("Starting pull request review execution",
		"review_id", task.ReviewID,
		"repo", task.RepoNamespace,
		"pr", task.PullNumber,
	)

	// Update state to PROCESSING
	_ = o.repo.UpdateReviewState(ctx, task.WorkspaceID, task.ReviewID, models.ReviewStateProcessing, 0)

	// Step 1: Parse git unified diff stream
	patches, err := diff.ParseUnifiedDiff(strings.NewReader(task.RawDiff))
	if err != nil {
		slog.Error("Failed to parse unified diff", "error", err)
		_ = o.repo.UpdateReviewState(ctx, task.WorkspaceID, task.ReviewID, models.ReviewStateFailed, 0)
		return fmt.Errorf("diff parse failed: %w", err)
	}

	var allFindings []models.CodeFinding

	// Step 2: Evaluate deterministic custom workspace rules
	if o.rulesEvaluator != nil {
		ruleFindings := o.rulesEvaluator.EvaluatePatches(task.ReviewID, task.WorkspaceID, patches)
		allFindings = append(allFindings, ruleFindings...)
		slog.Info("Rule evaluation finished", "findings", len(ruleFindings))
	}

	// Step 3: Run AI Gateway deep semantic analysis
	if o.llmGateway != nil && len(task.RawDiff) > 0 {
		llmReq := llm.ReviewRequest{
			RepoNamespace: task.RepoNamespace,
			PullTitle:     task.Title,
			DiffContent:   task.RawDiff,
		}

		aiResp, err := o.llmGateway.AnalyzeDiff(ctx, llmReq)
		if err != nil {
			slog.Warn("AI review synthesis warning", "error", err)
		} else {
			for _, f := range aiResp.Findings {
				sev := models.SeverityMedium
				switch strings.ToUpper(f.Severity) {
				case "CRITICAL":
					sev = models.SeverityCritical
				case "HIGH":
					sev = models.SeverityHigh
				case "LOW":
					sev = models.SeverityLow
				case "INFO":
					sev = models.SeverityInfo
				}

				allFindings = append(allFindings, models.CodeFinding{
					ID:            uuid.New(),
					ReviewID:      task.ReviewID,
					WorkspaceID:   task.WorkspaceID,
					FilePath:      f.FilePath,
					StartLine:     f.StartLine,
					EndLine:       f.EndLine,
					Severity:      sev,
					Category:      f.Category,
					Title:         f.Title,
					Description:   f.Description,
					Remediation:   f.Remediation,
					SuggestedDiff: f.SuggestedDiff,
					Fingerprint:   fmt.Sprintf("%x", task.ReviewID.String()+f.FilePath+f.Title),
				})
			}
			slog.Info("AI synthesis completed", "ai_findings", len(aiResp.Findings))
		}
	}

	// Step 4: Archive diff and audit report to Appwrite Storage
	if o.artifactClient != nil {
		artifactID := fmt.Sprintf("diff-%s", task.ReviewID.String())
		_, err := o.artifactClient.UploadArtifact(
			ctx,
			"review-artifacts",
			artifactID,
			fmt.Sprintf("pr-%d.diff", task.PullNumber),
			bytes.NewReader([]byte(task.RawDiff)),
		)
		if err != nil {
			slog.Warn("Appwrite storage artifact upload deferred", "error", err)
		}
	}

	// Step 5: Batch insert findings into PostgreSQL with Row-Level Security
	if err := o.repo.BatchInsertFindings(ctx, task.WorkspaceID, allFindings); err != nil {
		slog.Error("Failed saving review findings", "error", err)
		_ = o.repo.UpdateReviewState(ctx, task.WorkspaceID, task.ReviewID, models.ReviewStateFailed, 0)
		return fmt.Errorf("failed persisting findings: %w", err)
	}

	// Step 6: Transition review state to COMPLETED
	if err := o.repo.UpdateReviewState(ctx, task.WorkspaceID, task.ReviewID, models.ReviewStateCompleted, len(allFindings)); err != nil {
		return fmt.Errorf("failed finalizing review: %w", err)
	}

	slog.Info("Review execution completed successfully",
		"review_id", task.ReviewID,
		"total_findings", len(allFindings),
	)

	return nil
}
