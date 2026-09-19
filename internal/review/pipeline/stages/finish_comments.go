package stages

import (
	"context"
	"time"

	"github.com/scandrix/backend/internal/review/domain"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/pkg/models"
)

// FinishCommentsStage (Stage 15) updates top-level review summary and resolves outdated comments.
type FinishCommentsStage struct {
	commentManager domain.ICommentManagerService
	templates      domain.IMessageTemplateProcessor
}

// NewFinishCommentsStage constructs Stage 15.
func NewFinishCommentsStage(cm domain.ICommentManagerService, tpl domain.IMessageTemplateProcessor) *FinishCommentsStage {
	return &FinishCommentsStage{
		commentManager: cm,
		templates:      tpl,
	}
}

func (s *FinishCommentsStage) Name() string {
	return "FinishCommentsStage"
}

func (s *FinishCommentsStage) Execute(ctx context.Context, pCtx *pipeline.PipelineContext) error {
	if s.commentManager == nil {
		return nil
	}

	repo := models.TrackedRepository{
		ID:            pCtx.RepositoryID,
		WorkspaceID:   pCtx.WorkspaceID,
		Provider:      pCtx.Provider,
		NamespacePath: pCtx.RepoNamespace,
	}

	// 1. If review was skipped, post/update skip notice
	if pCtx.SkipReview {
		body := "### ⏸️ ScanDrix Review Skipped\n\n> " + pCtx.SkipReason + "\n\n*Powered by [ScanDrix](https://scandrix.dev)*"
		_ = s.commentManager.UpdateOverallSummaryComment(ctx, pCtx.WorkspaceID.String(), repo, pCtx.PullNumber, pCtx.InitialCommentID, body)
		return nil
	}

	// 2. Build summary body with template processor
	template := domain.DefaultEndReviewTemplate().Content
	if pCtx.PullRequestMessages != nil && pCtx.PullRequestMessages.EndReviewMessage != nil && pCtx.PullRequestMessages.EndReviewMessage.Content != "" {
		template = pCtx.PullRequestMessages.EndReviewMessage.Content
	}

	duration := time.Since(pCtx.StartTime).Seconds()
	vars := domain.TemplateVariables{
		Author:        pCtx.Author,
		PRNumber:      pCtx.PullNumber,
		RepoName:      pCtx.RepoNamespace,
		Summary:       pCtx.PRSummaryBody,
		FindingsCount: len(pCtx.AllFindings),
		FindingsList:  "",
		RulesChecked:  len(pCtx.ActiveRules),
		DurationSecs:  duration,
	}

	finalBody := s.templates.Process(template, vars)
	return s.commentManager.UpdateOverallSummaryComment(ctx, pCtx.WorkspaceID.String(), repo, pCtx.PullNumber, pCtx.InitialCommentID, finalBody)
}
