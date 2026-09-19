package workflow

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/core/domain"
	"github.com/scandrix/backend/internal/core/pipeline"
)

// WorkflowPausedError is an alias to pipeline.WorkflowPausedError for workflow consumers.
type WorkflowPausedError = pipeline.WorkflowPausedError

// Standard timeouts from ScanDrix job-processor-router.service.ts.
const (
	WebhookProcessTimeout       = 9 * time.Minute
	CodeReviewProcessTimeout    = 105 * time.Minute
	CLICodeReviewProcessTimeout = 28 * time.Minute
	CheckImplementationTimeout  = 9 * time.Minute
	ASTGraphBuildTimeout        = 19 * time.Minute
	ASTGraphIncrementalTimeout  = 9 * time.Minute
	DefaultProcessTimeout       = 10 * time.Minute
)

// JobProcessor handles the concrete execution of a specific workflow type.
type JobProcessor interface {
	Process(ctx context.Context, job *WorkflowJobModel) error
}

// JobRepository defines database persistence operations for workflow jobs.
type JobRepository interface {
	FindOne(ctx context.Context, id uuid.UUID) (*WorkflowJobModel, error)
	Update(ctx context.Context, id uuid.UUID, updates map[string]any) error
}

// JobProcessorRouterService mirrors ScanDrix JobProcessorRouterService.
type JobProcessorRouterService struct {
	jobRepo    JobRepository
	processors map[domain.WorkflowType]JobProcessor
	classifier *ErrorClassifierService
}

// NewJobProcessorRouterService instantiates a router service.
func NewJobProcessorRouterService(
	jobRepo JobRepository,
	classifier *ErrorClassifierService,
) *JobProcessorRouterService {
	return &JobProcessorRouterService{
		jobRepo:    jobRepo,
		processors: make(map[domain.WorkflowType]JobProcessor),
		classifier: classifier,
	}
}

// Register registers a processor for a workflow type.
func (r *JobProcessorRouterService) Register(workflowType domain.WorkflowType, processor JobProcessor) {
	r.processors[workflowType] = processor
}

// GetProcessTimeout returns the bounded timeout for the specific workflow type.
func (r *JobProcessorRouterService) GetProcessTimeout(workflowType domain.WorkflowType) time.Duration {
	switch workflowType {
	case domain.WorkflowTypeWebhookProcessing:
		return WebhookProcessTimeout
	case domain.WorkflowTypeCodeReview:
		return CodeReviewProcessTimeout
	case domain.WorkflowTypeCLICodeReview:
		return CLICodeReviewProcessTimeout
	case domain.WorkflowTypeCheckSuggestionVerification:
		return CheckImplementationTimeout
	case domain.WorkflowTypeASTGraphBuild:
		return ASTGraphBuildTimeout
	case domain.WorkflowTypeASTGraphIncremental:
		return ASTGraphIncrementalTimeout
	default:
		return DefaultProcessTimeout
	}
}

// Process routes and executes a job within its bounded timeout window.
func (r *JobProcessorRouterService) Process(ctx context.Context, jobID uuid.UUID) error {
	job, err := r.jobRepo.FindOne(ctx, jobID)
	if err != nil {
		return fmt.Errorf("failed fetching workflow job %s: %w", jobID, err)
	}
	if job == nil {
		return fmt.Errorf("workflow job %s not found", jobID)
	}

	processor, exists := r.processors[job.WorkflowType]
	if !exists {
		return fmt.Errorf("no processor registered for workflow type %s", job.WorkflowType)
	}

	timeout := r.GetProcessTimeout(job.WorkflowType)
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Update startedAt and status to PROCESSING
	now := time.Now().UTC()
	_ = r.jobRepo.Update(ctx, jobID, map[string]any{
		"status":     domain.JobStatusProcessing,
		"started_at": now,
	})

	processErr := processor.Process(timeoutCtx, job)

	if processErr != nil {
		var pausedErr *pipeline.WorkflowPausedError
		if errors.As(processErr, &pausedErr) {
			waitingForEvent := map[string]any{
				"event_type": pausedErr.EventType,
				"event_key":  pausedErr.EventKey,
				"timeout_ms": pausedErr.TimeoutMs,
			}
			if pausedErr.TaskID != "" {
				waitingForEvent["task_id"] = pausedErr.TaskID
			}
			if pausedErr.Metadata != nil {
				waitingForEvent["metadata"] = pausedErr.Metadata
			}

			updates := map[string]any{
				"status":            domain.JobStatusWaitingForEvent,
				"waiting_for_event": waitingForEvent,
			}
			if pausedErr.StageName != "" {
				updates["current_stage"] = pausedErr.StageName
			}

			_ = r.jobRepo.Update(ctx, jobID, updates)
			return nil
		}

		isTimeout := errors.Is(timeoutCtx.Err(), context.DeadlineExceeded)
		classification := r.classifier.Classify(processErr)
		if isTimeout {
			classification = domain.ErrorClassificationRetryable
		}

		errMsg := processErr.Error()
		if isTimeout {
			errMsg = fmt.Sprintf("Workflow job %s timeout after %v", jobID, timeout)
		}

		_ = r.jobRepo.Update(ctx, jobID, map[string]any{
			"status":               domain.JobStatusFailed,
			"error_classification": classification,
			"last_error":           errMsg,
			"completed_at":         time.Now().UTC(),
		})

		return processErr
	}

	// Mark COMPLETED
	completedNow := time.Now().UTC()
	_ = r.jobRepo.Update(ctx, jobID, map[string]any{
		"status":       domain.JobStatusCompleted,
		"completed_at": completedNow,
	})

	return nil
}

// ProcessJob executes a job directly using its UUID.
func (r *JobProcessorRouterService) ProcessJob(ctx context.Context, job *WorkflowJobModel) error {
	if job == nil {
		return fmt.Errorf("cannot process nil workflow job")
	}
	return r.Process(ctx, job.UUID)
}

// GetClassifier returns the error classifier service used by the router.
func (r *JobProcessorRouterService) GetClassifier() *ErrorClassifierService {
	return r.classifier
}
