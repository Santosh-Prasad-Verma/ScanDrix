package stages

import (
	"context"

	"github.com/scandrix/backend/internal/review/domain"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/pkg/models"
)

// FinishProcessReviewStage (Stage 16) submits formal review conclusion (APPROVE / REQUEST_CHANGES) to SCM.
type FinishProcessReviewStage struct {
	commentManager domain.ICommentManagerService
}

// NewFinishProcessReviewStage constructs Stage 16.
func NewFinishProcessReviewStage(cm domain.ICommentManagerService) *FinishProcessReviewStage {
	return &FinishProcessReviewStage{commentManager: cm}
}

func (s *FinishProcessReviewStage) Name() string {
	return "FinishProcessReviewStage"
}

func (s *FinishProcessReviewStage) Execute(ctx context.Context, pCtx *pipeline.PipelineContext) error {
	if s.commentManager == nil || pCtx.SkipReview {
		return nil
	}

	repo := models.TrackedRepository{
		ID:            pCtx.RepositoryID,
		WorkspaceID:   pCtx.WorkspaceID,
		Provider:      pCtx.Provider,
		NamespacePath: pCtx.RepoNamespace,
	}

	event := "COMMENT"
	body := "ScanDrix code review completed."

	if !pCtx.PassedReview {
		event = "REQUEST_CHANGES"
		body = "ScanDrix detected security or critical bug findings that must be addressed."
	} else if pCtx.ResolvedConfig.AutoApprove {
		event = "APPROVE"
		body = "ScanDrix verified changes against all security and repository quality policies."
	}

	return s.commentManager.PostPRReviewSubmission(ctx, pCtx.WorkspaceID.String(), repo, pCtx.PullNumber, pCtx.HeadSHA, body, event, nil)
}
