package stages

import (
	"context"
	"fmt"

	"github.com/scandrix/backend/internal/review/pipeline"
)

// ValidateConfigStage (Stage 4) verifies that review execution is permitted by active configuration.
type ValidateConfigStage struct{}

// NewValidateConfigStage constructs Stage 4.
func NewValidateConfigStage() *ValidateConfigStage {
	return &ValidateConfigStage{}
}

func (s *ValidateConfigStage) Name() string {
	return "ValidateConfigStage"
}

func (s *ValidateConfigStage) Execute(ctx context.Context, pCtx *pipeline.PipelineContext) error {
	if pCtx.SkipReview {
		return nil
	}

	// 1. Verify review feature is enabled
	if !pCtx.ResolvedConfig.Enabled {
		pCtx.SkipReview = true
		pCtx.SkipReason = "code review is disabled in repository configuration"
		return nil
	}

	// 2. Enforce maximum files per review scope to protect context windows
	maxFiles := pCtx.ResolvedConfig.MaxFilesPerReview
	if maxFiles > 0 && len(pCtx.FilteredPatches) > maxFiles {
		pCtx.SkipReview = true
		pCtx.SkipReason = fmt.Sprintf("PR modifies %d files, exceeding configured limit of %d", len(pCtx.FilteredPatches), maxFiles)
		return nil
	}

	return nil
}
