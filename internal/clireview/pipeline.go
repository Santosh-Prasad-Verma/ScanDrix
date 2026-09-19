package clireview

import (
	"time"

	"github.com/scandrix/backend/internal/clireview/domain"
	"github.com/scandrix/backend/internal/clireview/pipeline"
	"github.com/scandrix/backend/internal/rules"
)

// PipelineFile represents a prepared file for AST and rule analysis.
type PipelineFile = pipeline.PipelineFile

// PrepareCliFiles processes input diff and files into structured pipeline files.
func PrepareCliFiles(input domain.CliReviewInput) []PipelineFile {
	return pipeline.PrepareCliFiles(input)
}

// EvaluateRulesAgainstFiles evaluates AST rules on prepared files.
func EvaluateRulesAgainstFiles(
	evaluator *rules.Evaluator,
	files []PipelineFile,
	cfg *domain.CliReviewConfig,
) []domain.CliReviewIssue {
	return pipeline.EvaluateRulesAgainstFiles(evaluator, files, cfg)
}

// FormatCliOutput formats findings into the final CliReviewResponse.
func FormatCliOutput(issues []domain.CliReviewIssue, filesCount int, startTime time.Time) domain.CliReviewResponse {
	return pipeline.FormatCliOutput(issues, filesCount, startTime)
}
