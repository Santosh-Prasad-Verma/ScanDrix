// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package stages

import (
	"context"
	"log/slog"

	"github.com/scandrix/backend/internal/review/feedback"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/pkg/models"
)

// SemanticSuppressorStage gates candidate review findings against historical
// developer feedback and pgvector false-positive security memory.
type SemanticSuppressorStage struct {
	feedbackService *feedback.SemanticFeedbackService
}

// NewSemanticSuppressorStage constructs a semantic suppressor stage.
func NewSemanticSuppressorStage(fbService *feedback.SemanticFeedbackService) *SemanticSuppressorStage {
	return &SemanticSuppressorStage{
		feedbackService: fbService,
	}
}

func (s *SemanticSuppressorStage) Name() string {
	return "semantic_suppressor"
}

func (s *SemanticSuppressorStage) Execute(ctx context.Context, pCtx *pipeline.PipelineContext) error {
	if s.feedbackService == nil || len(pCtx.AllFindings) == 0 {
		return nil
	}

	var active []models.CodeFinding
	for _, f := range pCtx.AllFindings {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		decision := s.feedbackService.CheckSuppression(ctx, pCtx.WorkspaceID, &f)
		if decision.ShouldSuppress {
			slog.Info("Suppressing code review finding based on developer feedback memory",
				"workspace_id", pCtx.WorkspaceID,
				"title", f.Title,
				"file", f.FilePath,
				"line", f.StartLine,
				"is_exact", decision.IsExactMatch,
				"similarity", decision.SimilarityScore,
				"reason", decision.Reason,
			)
			pCtx.SuppressedFindings = append(pCtx.SuppressedFindings, f)
			continue
		}

		active = append(active, f)
	}

	pCtx.AllFindings = active
	return nil
}
