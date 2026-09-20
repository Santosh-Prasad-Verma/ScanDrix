package stages

import (
	"context"
	"strings"

	"github.com/scandrix/backend/internal/review/pipeline"
)

// ValidateNewCommitsStage (Stage 2) validates that new commits exist and have not already been reviewed.
type ValidateNewCommitsStage struct{}

// NewValidateNewCommitsStage constructs Stage 2.
func NewValidateNewCommitsStage() *ValidateNewCommitsStage {
	return &ValidateNewCommitsStage{}
}

func (s *ValidateNewCommitsStage) Name() string {
	return "ValidateNewCommitsStage"
}

func (s *ValidateNewCommitsStage) Execute(ctx context.Context, pCtx *pipeline.PipelineContext) error {
	// If already flagged to skip, proceed
	if pCtx.SkipReview {
		return nil
	}

	// 1. Verify head SHA is provided
	headSHA := strings.TrimSpace(pCtx.HeadSHA)
	if headSHA == "" {
		pCtx.SkipReview = true
		pCtx.SkipReason = "head commit SHA is missing"
		return nil
	}

	// 2. Prevent redundant review runs on identical commit SHA
	if pCtx.LastAnalyzedSHA != "" && pCtx.LastAnalyzedSHA == headSHA {
		pCtx.SkipReview = true
		pCtx.SkipReason = "commit " + headSHA[:min(7, len(headSHA))] + " has already been reviewed"
		return nil
	}

	// 3. Skip pure merge commits if diff is identical to base
	if pCtx.HeadSHA == pCtx.BaseSHA && pCtx.HeadSHA != "" {
		pCtx.SkipReview = true
		pCtx.SkipReason = "head and base commit SHAs are identical (no changes)"
		return nil
	}

	return nil
}
