package stages

import (
	"context"

	"github.com/scandrix/backend/internal/review/domain"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/pkg/models"
)

// InitialCommentStage (Stage 7) posts an initial acknowledgment comment to the pull request.
type InitialCommentStage struct {
	commentManager domain.ICommentManagerService
}

// NewInitialCommentStage constructs Stage 7.
func NewInitialCommentStage(cm domain.ICommentManagerService) *InitialCommentStage {
	return &InitialCommentStage{commentManager: cm}
}

func (s *InitialCommentStage) Name() string {
	return "InitialCommentStage"
}

func (s *InitialCommentStage) Execute(ctx context.Context, pCtx *pipeline.PipelineContext) error {
	if pCtx.SkipReview || s.commentManager == nil {
		return nil
	}

	template := ""
	if pCtx.PullRequestMessages != nil && pCtx.PullRequestMessages.StartReviewMessage != nil {
		if pCtx.PullRequestMessages.StartReviewMessage.Status == domain.MessageStatusOff {
			return nil
		}
		template = pCtx.PullRequestMessages.StartReviewMessage.Content
	}

	repo := models.TrackedRepository{
		ID:            pCtx.RepositoryID,
		WorkspaceID:   pCtx.WorkspaceID,
		Provider:      pCtx.Provider,
		NamespacePath: pCtx.RepoNamespace,
	}

	commentID, err := s.commentManager.CreateInitialComment(ctx, pCtx.WorkspaceID.String(), repo, pCtx.PullNumber, template)
	if err == nil {
		pCtx.InitialCommentID = commentID
	}

	return nil
}
