package stages

import (
	"context"

	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/pkg/models"
)

// AggregateResultsStage (Stage 14) consolidates findings, evaluates thresholds, and computes pass/fail verdict.
type AggregateResultsStage struct{}

// NewAggregateResultsStage constructs Stage 14.
func NewAggregateResultsStage() *AggregateResultsStage {
	return &AggregateResultsStage{}
}

func (s *AggregateResultsStage) Name() string {
	return "AggregateResultsStage"
}

func (s *AggregateResultsStage) Execute(ctx context.Context, pCtx *pipeline.PipelineContext) error {
	if pCtx.SkipReview {
		pCtx.PassedReview = true
		return nil
	}

	criticalCount := 0
	highCount := 0

	for _, f := range pCtx.AllFindings {
		switch f.Severity {
		case models.SeverityCritical:
			criticalCount++
		case models.SeverityHigh:
			highCount++
		}
	}

	// Any critical or high severity finding blocks approval
	if criticalCount > 0 || highCount > 0 {
		pCtx.PassedReview = false
	} else {
		pCtx.PassedReview = true
	}

	return nil
}
