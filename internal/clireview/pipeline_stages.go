package clireview

import (
	"github.com/scandrix/backend/internal/clireview/pipeline"
)

// CliReviewPipelineContext carries state throughout CLI review pipeline stages.
type CliReviewPipelineContext = pipeline.CliReviewPipelineContext

// IPipelineStage represents a discrete step in pipeline execution.
type IPipelineStage = pipeline.IPipelineStage

// PrepareCliFilesStage prepares and validates file change entries for analysis.
type PrepareCliFilesStage = pipeline.PrepareCliFilesStage

// NewPrepareCliFilesStage creates an initialized PrepareCliFilesStage.
func NewPrepareCliFilesStage() *PrepareCliFilesStage {
	return pipeline.NewPrepareCliFilesStage()
}

// FormatCliOutputStage formats findings into the standard CLI response payload.
type FormatCliOutputStage = pipeline.FormatCliOutputStage

// NewFormatCliOutputStage creates an initialized FormatCliOutputStage.
func NewFormatCliOutputStage(converter *CliInputConverter) *FormatCliOutputStage {
	return pipeline.NewFormatCliOutputStage(converter)
}

// CliReviewPipelineStrategy defines the sequential ordering of CLI review stages.
type CliReviewPipelineStrategy = pipeline.CliReviewPipelineStrategy

// NewCliReviewPipelineStrategy initializes the strategy with requisite stages.
func NewCliReviewPipelineStrategy(
	prepareFilesStage *PrepareCliFilesStage,
	formatOutputStage *FormatCliOutputStage,
) *CliReviewPipelineStrategy {
	return pipeline.NewCliReviewPipelineStrategy(prepareFilesStage, formatOutputStage)
}

// PipelineExecutor runs stages sequentially over a pipeline context.
type PipelineExecutor = pipeline.PipelineExecutor

// NewPipelineExecutor creates an executor instance.
func NewPipelineExecutor() *PipelineExecutor {
	return pipeline.NewPipelineExecutor()
}
