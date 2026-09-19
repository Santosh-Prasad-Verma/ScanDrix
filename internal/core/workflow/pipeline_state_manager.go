package workflow

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// PipelineStateSnapshot mirrors ScanDrix pipelineState JSON structure in workflow_jobs.
type PipelineStateSnapshot struct {
	Context      any       `json:"context"`
	CurrentStage string    `json:"currentStage"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// PipelineStateManager mirrors ScanDrix PipelineStateManager: manages state persistence across worker boundaries.
type PipelineStateManager struct {
	jobRepo JobRepository
}

// NewPipelineStateManager instantiates a pipeline state manager.
func NewPipelineStateManager(jobRepo JobRepository) *PipelineStateManager {
	return &PipelineStateManager{jobRepo: jobRepo}
}

// SaveState persists the active pipeline context and stage snapshot to the workflow job.
func (m *PipelineStateManager) SaveState(ctx context.Context, jobID uuid.UUID, pipelineCtx any, currentStage string) error {
	snapshot := PipelineStateSnapshot{
		Context:      pipelineCtx,
		CurrentStage: currentStage,
		UpdatedAt:    time.Now().UTC(),
	}

	updates := map[string]any{
		"current_stage":  currentStage,
		"pipeline_state": snapshot,
		"updated_at":     time.Now().UTC(),
	}

	if err := m.jobRepo.Update(ctx, jobID, updates); err != nil {
		return fmt.Errorf("failed saving pipeline state for job %s: %w", jobID, err)
	}
	return nil
}

// ClearState clears the pipeline state upon successful completion.
func (m *PipelineStateManager) ClearState(ctx context.Context, jobID uuid.UUID) error {
	updates := map[string]any{
		"pipeline_state": nil,
		"updated_at":     time.Now().UTC(),
	}
	return m.jobRepo.Update(ctx, jobID, updates)
}
