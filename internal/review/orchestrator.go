package review

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/integrations/github"
	"github.com/scandrix/backend/internal/llm"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/internal/storage"
	"github.com/scandrix/backend/pkg/models"
)

// SCMPublisher publishes inline reviews and updates commit checks on SCM providers.
type SCMPublisher interface {
	SubmitPullRequestReview(ctx context.Context, owner, repo string, pullNumber int, submission github.PullReviewSubmission) error
	UpdateCheckRun(ctx context.Context, owner, repo string, checkRunID int64, req github.UpdateCheckRunRequest) error
}

// Orchestrator coordinates the end-to-end pull request code review pipeline.
type Orchestrator struct {
	repo           *database.Repository
	llmGateway     *llm.Gateway
	artifactClient *storage.ArtifactClient
	rulesEvaluator *rules.Evaluator
	scmPublisher   SCMPublisher
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

// SetSCMPublisher binds an active SCM client for publishing reviews back to GitHub/GitLab.
func (o *Orchestrator) SetSCMPublisher(pub SCMPublisher) {
	o.scmPublisher = pub
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
	CheckRunID    int64
}

// ProcessReview executes all analysis stages, records findings, and uploads scan artifacts.
func (o *Orchestrator) ProcessReview(ctx context.Context, task ExecutionTask) error {
	slog.Info("Starting pull request review execution",
		"review_id", task.ReviewID,
		"repo", task.RepoNamespace,
		"pr", task.PullNumber,
	)

	// Update state to PROCESSING
	if o.repo != nil {
		_ = o.repo.UpdateReviewState(ctx, task.WorkspaceID, task.ReviewID, models.ReviewStateProcessing, 0)
	}

	// Step 1: Parse git unified diff stream
	patches, err := diff.ParseUnifiedDiff(strings.NewReader(task.RawDiff))
	if err != nil {
		slog.Error("Failed to parse unified diff", "error", err)
		if o.repo != nil {
			_ = o.repo.UpdateReviewState(ctx, task.WorkspaceID, task.ReviewID, models.ReviewStateFailed, 0)
		}
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
	if o.repo != nil {
		if err := o.repo.BatchInsertFindings(ctx, task.WorkspaceID, allFindings); err != nil {
			slog.Error("Failed saving review findings", "error", err)
			_ = o.repo.UpdateReviewState(ctx, task.WorkspaceID, task.ReviewID, models.ReviewStateFailed, 0)
			return fmt.Errorf("failed persisting findings: %w", err)
		}

		// Step 6: Transition review state to COMPLETED
		if err := o.repo.UpdateReviewState(ctx, task.WorkspaceID, task.ReviewID, models.ReviewStateCompleted, len(allFindings)); err != nil {
			return fmt.Errorf("failed finalizing review: %w", err)
		}
	}

	// Step 7: Dispatch Inline Review Comments and Check Run update to SCM Provider
	if o.scmPublisher != nil && task.RepoNamespace != "" && task.PullNumber > 0 {
		parts := strings.Split(task.RepoNamespace, "/")
		if len(parts) == 2 {
			owner, repoName := parts[0], parts[1]

			hasCriticalOrHigh := false
			var comments []github.ReviewCommentPayload
			var annotations []github.CheckRunAnnotation

			for _, f := range allFindings {
				if f.Severity == models.SeverityCritical || f.Severity == models.SeverityHigh {
					hasCriticalOrHigh = true
				}

				comments = append(comments, github.ReviewCommentPayload{
					Path: f.FilePath,
					Line: f.EndLine,
					Side: "RIGHT",
					Body: fmt.Sprintf("### 🛡️ [%s] %s\n\n%s\n\n**Remediation:**\n%s", f.Severity, f.Title, f.Description, f.Remediation),
				})

				level := "warning"
				if f.Severity == models.SeverityCritical || f.Severity == models.SeverityHigh {
					level = "failure"
				} else if f.Severity == models.SeverityInfo || f.Severity == models.SeverityLow {
					level = "notice"
				}

				if len(annotations) < 50 {
					annotations = append(annotations, github.CheckRunAnnotation{
						Path:            f.FilePath,
						StartLine:       f.StartLine,
						EndLine:         f.EndLine,
						AnnotationLevel: level,
						Title:           fmt.Sprintf("[%s] %s", f.Severity, f.Title),
						Message:         f.Description,
					})
				}
			}

			event := "APPROVE"
			summaryText := "ScanDrix automated security review passed with zero critical findings."
			conclusion := "success"

			if hasCriticalOrHigh {
				event = "REQUEST_CHANGES"
				summaryText = fmt.Sprintf("ScanDrix detected %d actionable findings requiring resolution.", len(allFindings))
				conclusion = "failure"
			}

			submission := github.PullReviewSubmission{
				CommitID: task.HeadSHA,
				Body:     fmt.Sprintf("## 🚀 ScanDrix Automated Code Assurance Report\n\n%s\n\n- **Total Findings:** %d\n- **Review ID:** `%s`", summaryText, len(allFindings), task.ReviewID.String()),
				Event:    event,
				Comments: comments,
			}

			if err := o.scmPublisher.SubmitPullRequestReview(ctx, owner, repoName, task.PullNumber, submission); err != nil {
				slog.Warn("Failed publishing review comments to SCM", "error", err)
			} else {
				slog.Info("Successfully published review comments to SCM", "owner", owner, "repo", repoName, "pr", task.PullNumber)
			}

			if task.CheckRunID > 0 {
				now := time.Now().UTC()
				updateReq := github.UpdateCheckRunRequest{
					Status:      "completed",
					Conclusion:  conclusion,
					CompletedAt: &now,
					Output: &github.CheckRunOutput{
						Title:       fmt.Sprintf("ScanDrix Review: %s", strings.ToUpper(conclusion)),
						Summary:     summaryText,
						Annotations: annotations,
					},
				}
				if err := o.scmPublisher.UpdateCheckRun(ctx, owner, repoName, task.CheckRunID, updateReq); err != nil {
					slog.Warn("Failed updating SCM check run", "error", err)
				}
			}
		}
	}

	slog.Info("Review execution completed successfully",
		"review_id", task.ReviewID,
		"total_findings", len(allFindings),
	)

	return nil
}
