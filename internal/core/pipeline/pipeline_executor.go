package pipeline

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/core/domain"
)

// PipelineMetadata mirrors ScanDrix PipelineMetadata.
type PipelineMetadata struct {
	PipelineID       string `json:"pipelineId"`
	ParentPipelineID string `json:"parentPipelineId,omitempty"`
	RootPipelineID   string `json:"rootPipelineId,omitempty"`
	PipelineName     string `json:"pipelineName"`
}

// StatusInfo mirrors ScanDrix StatusInfo.
type StatusInfo struct {
	Status      domain.AutomationStatus `json:"status"`
	SkipStages  []string                `json:"skipStages,omitempty"`
	JumpToStage string                  `json:"jumpToStage,omitempty"`
}

// PipelineError mirrors ScanDrix PipelineError.
type PipelineError struct {
	PipelineID string                       `json:"pipelineId"`
	Stage      string                       `json:"stage"`
	Substage   string                       `json:"substage"`
	Error      string                       `json:"error"`
	Severity   domain.PipelineErrorSeverity `json:"severity"`
	Metadata   map[string]any               `json:"metadata,omitempty"`
}

// BasePipelineContext provides common pipeline fields matching ScanDrix PipelineContext.
type BasePipelineContext struct {
	PipelineMetadata PipelineMetadata `json:"pipelineMetadata"`
	StatusInfo       StatusInfo       `json:"statusInfo"`
	Errors           []PipelineError  `json:"errors"`
	CorrelationID    string           `json:"correlationId,omitempty"`
}

// StageOptions configures stage visibility and UI labels.
type StageOptions struct {
	Visibility string `json:"visibility,omitempty"`
	Label      string `json:"label,omitempty"`
}

// PipelineObserver mirrors ScanDrix IPipelineObserver.
type PipelineObserver interface {
	OnPipelineStart(ctx context.Context, pCtx *BasePipelineContext)
	OnStageStart(ctx context.Context, stageName string, pCtx *BasePipelineContext, opts StageOptions)
	OnStageCompleted(ctx context.Context, stageName string, pCtx *BasePipelineContext, opts StageOptions)
	OnStageError(ctx context.Context, stageName string, err error, pCtx *BasePipelineContext, opts StageOptions)
	OnPipelineEnd(ctx context.Context, pCtx *BasePipelineContext)
}

// PipelineStage mirrors ScanDrix PipelineStage.
type PipelineStage interface {
	StageName() string
	IsSilent() bool
	Options() StageOptions
	Execute(ctx context.Context, pCtx *BasePipelineContext) error
}

// PipelineExecutor mirrors ScanDrix PipelineExecutor.
type PipelineExecutor struct{}

// NewPipelineExecutor instantiates a pipeline executor.
func NewPipelineExecutor() *PipelineExecutor {
	return &PipelineExecutor{}
}

// Execute executes a slice of pipeline stages sequentially, honoring skipStages, jumps, and observers.
func (e *PipelineExecutor) Execute(
	ctx context.Context,
	pCtx *BasePipelineContext,
	stages []PipelineStage,
	pipelineName string,
	observers ...PipelineObserver,
) (*BasePipelineContext, error) {
	pipelineID := uuid.New().String()
	if pipelineName == "" {
		pipelineName = "UnnamedPipeline"
	}

	pCtx.PipelineMetadata = PipelineMetadata{
		PipelineID:   pipelineID,
		PipelineName: pipelineName,
	}

	// 1. Notify observers of pipeline start
	for _, obs := range observers {
		obs.OnPipelineStart(ctx, pCtx)
	}

	processedErrors := make(map[string]bool)

	for _, stage := range stages {
		stageName := stage.StageName()

		// 2. Per-stage opt-out: skipStages bypasses specific named stages
		if contains(pCtx.StatusInfo.SkipStages, stageName) {
			continue
		}

		// 3. Handle skipped status & jumpToStage fast-forward
		if pCtx.StatusInfo.Status == domain.AutomationStatusSkipped {
			if pCtx.StatusInfo.JumpToStage != "" && pCtx.StatusInfo.JumpToStage == stageName {
				// Resuming at jumpToStage target
				pCtx.StatusInfo.Status = domain.AutomationStatusPending
				pCtx.StatusInfo.JumpToStage = ""
			} else {
				// Still skipped
				continue
			}
		}

		if !stage.IsSilent() {
			for _, obs := range observers {
				obs.OnStageStart(ctx, stageName, pCtx, stage.Options())
			}
		}

		err := stage.Execute(ctx, pCtx)

		if err != nil {
			errKey := fmt.Sprintf("%s:StageExecution:%s", stageName, err.Error())
			if !processedErrors[errKey] {
				processedErrors[errKey] = true
				pCtx.Errors = append(pCtx.Errors, PipelineError{
					PipelineID: pipelineID,
					Stage:      stageName,
					Substage:   "StageExecution",
					Error:      err.Error(),
					Severity:   domain.PipelineSeverityHigh,
					Metadata: map[string]any{
						"pipelineName": pipelineName,
						"timestamp":    time.Now().UTC().Format(time.RFC3339),
					},
				})
			}

			if !stage.IsSilent() {
				for _, obs := range observers {
					obs.OnStageError(ctx, stageName, err, pCtx, stage.Options())
				}
			}

			// Stage failure sets failure status
			pCtx.StatusInfo.Status = domain.AutomationStatusFailure
			break
		}

		if !stage.IsSilent() {
			for _, obs := range observers {
				obs.OnStageCompleted(ctx, stageName, pCtx, stage.Options())
			}
		}
	}

	if pCtx.StatusInfo.Status != domain.AutomationStatusFailure && pCtx.StatusInfo.Status != domain.AutomationStatusSkipped {
		pCtx.StatusInfo.Status = domain.AutomationStatusSuccess
	}

	// 4. Notify observers of pipeline end
	for _, obs := range observers {
		obs.OnPipelineEnd(ctx, pCtx)
	}

	return pCtx, nil
}

func contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}
