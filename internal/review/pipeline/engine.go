package pipeline

import (
	"context"
	"fmt"
	"time"
)

// PipelineEngine executes an ordered sequence of review stages on a pull request.
type PipelineEngine struct {
	stages []PipelineStage
}

// NewPipelineEngine creates a review pipeline with configured stages.
func NewPipelineEngine(stages ...PipelineStage) *PipelineEngine {
	return &PipelineEngine{stages: stages}
}

// Execute runs each stage in sequence, tracking execution duration and metrics.
func (e *PipelineEngine) Execute(ctx context.Context, pCtx *PipelineContext) error {
	pCtx.StartTime = time.Now().UTC()
	defer func() {
		pCtx.EndTime = time.Now().UTC()
		if pCtx.SandboxHandle != nil {
			_ = pCtx.SandboxHandle.Cleanup(ctx)
		}
	}()

	for _, stage := range e.stages {
		stageStart := time.Now()
		err := stage.Execute(ctx, pCtx)
		duration := time.Since(stageStart)

		success := err == nil
		pCtx.AddMetric(stage.Name(), duration, success, err, len(pCtx.AllFindings))

		if err != nil {
			return fmt.Errorf("stage %s failed: %w", stage.Name(), err)
		}
	}

	return nil
}
