package pipeline

import (
	"context"
	"fmt"
)

// IPipelineStrategy configures and supplies the ordered stages of review execution.
type IPipelineStrategy interface {
	GetPipelineName() string
	ConfigureStages() []PipelineStage
}

// CanonicalCodeReviewPipelineStrategy configures the 16-stage code review pipeline matching canonical parity.
type CanonicalCodeReviewPipelineStrategy struct {
	stages []PipelineStage
}

// NewCanonicalPipelineStrategy constructs the review pipeline strategy with configured stages.
func NewCanonicalPipelineStrategy(
	validatePrerequisites PipelineStage,
	validateNewCommits PipelineStage,
	resolveConfig PipelineStage,
	validateConfig PipelineStage,
	fetchChangedFiles PipelineStage,
	loadExternalContext PipelineStage,
	initialComment PipelineStage,
	businessLogicValidation PipelineStage,
	createSandbox PipelineStage,
	agentReview PipelineStage,
	createPrLevelComments PipelineStage,
	validateSuggestions PipelineStage,
	createFileComments PipelineStage,
	aggregateResults PipelineStage,
	finishComments PipelineStage,
	finishProcessReview PipelineStage,
) *CanonicalCodeReviewPipelineStrategy {
	stages := []PipelineStage{
		// Early stages (1-7)
		validatePrerequisites,
		validateNewCommits,
		resolveConfig,
		validateConfig,
		fetchChangedFiles,
		loadExternalContext,
		initialComment,

		// Core Analysis & Agent stages (8-10)
		businessLogicValidation,
		createSandbox,
		agentReview,

		// Post-Processing stages (11-16)
		createPrLevelComments,
		validateSuggestions,
		createFileComments,
		aggregateResults,
		finishComments,
		finishProcessReview,
	}

	return &CanonicalCodeReviewPipelineStrategy{stages: stages}
}

func (s *CanonicalCodeReviewPipelineStrategy) GetPipelineName() string {
	return "CanonicalCodeReviewPipeline"
}

func (s *CanonicalCodeReviewPipelineStrategy) ConfigureStages() []PipelineStage {
	return s.stages
}

// BuildEngine creates a PipelineEngine executing the strategy's configured stages.
func (s *CanonicalCodeReviewPipelineStrategy) BuildEngine() *PipelineEngine {
	return NewPipelineEngine(s.stages...)
}

// ExecuteStrategy runs the pipeline end-to-end with the strategy's stages.
func (s *CanonicalCodeReviewPipelineStrategy) Execute(ctx context.Context, pCtx *PipelineContext) error {
	engine := s.BuildEngine()
	if err := engine.Execute(ctx, pCtx); err != nil {
		return fmt.Errorf("canonical review pipeline execution failed: %w", err)
	}
	return nil
}
