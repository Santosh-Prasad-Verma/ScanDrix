package clireview

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// IRateLimitGate validates code host API rate limits before initiating review workers.
type IRateLimitGate interface {
	Check(ctx context.Context, orgAndTeam OrganizationAndTeamData, platform string) error
}

// PassThroughRateLimitGate is a default no-op gate when external rate-limits are non-exhausted.
type PassThroughRateLimitGate struct{}

func (g *PassThroughRateLimitGate) Check(ctx context.Context, orgAndTeam OrganizationAndTeamData, platform string) error {
	return nil
}

// IJobRepository abstracts job persistence for asynchronous workflow processing.
type IJobRepository interface {
	FindOne(ctx context.Context, jobID string) (*JobQueueItem, error)
	Update(ctx context.Context, jobID string, status string, metadata map[string]any, lastError string) error
}

// CliReviewJobProcessorService processes queued jobs by invoking ExecuteCliReviewUseCase.
type CliReviewJobProcessorService struct {
	jobRepository           IJobRepository
	executeCliReviewUseCase *ExecuteCliReviewUseCase
	rateLimitGate           IRateLimitGate
	logger                  *slog.Logger
}

// NewCliReviewJobProcessorService creates an initialized worker processor.
func NewCliReviewJobProcessorService(
	jobRepo IJobRepository,
	executeUseCase *ExecuteCliReviewUseCase,
	gate IRateLimitGate,
) *CliReviewJobProcessorService {
	if gate == nil {
		gate = &PassThroughRateLimitGate{}
	}
	return &CliReviewJobProcessorService{
		jobRepository:           jobRepo,
		executeCliReviewUseCase: executeUseCase,
		rateLimitGate:           gate,
		logger:                  slog.Default().With("service", "CliReviewJobProcessorService"),
	}
}

// Process coordinates single job lifecycle: validation, execution with cancellation race, and state persistence.
func (s *CliReviewJobProcessorService) Process(ctx context.Context, jobID string, abortSignal <-chan struct{}) error {
	if s.jobRepository == nil {
		return errors.New("job repository unavailable")
	}

	job, err := s.jobRepository.FindOne(ctx, jobID)
	if err != nil || job == nil {
		return fmt.Errorf("CLI review job %s not found", jobID)
	}

	select {
	case <-abortSignal:
		return fmt.Errorf("job %s aborted before start", jobID)
	default:
	}

	payload := job.Payload
	if payload == nil {
		return fmt.Errorf("invalid CLI review payload for job %s: missing payload", jobID)
	}

	var input CliReviewInput
	if inVal, ok := payload["input"].(CliReviewInput); ok {
		input = inVal
	} else if inPtr, ok := payload["input"].(*CliReviewInput); ok && inPtr != nil {
		input = *inPtr
	} else if inMap, ok := payload["input"].(map[string]any); ok {
		diff, _ := inMap["diff"].(string)
		input = CliReviewInput{Diff: diff}
	} else {
		return fmt.Errorf("invalid CLI review payload for job %s: missing required fields", jobID)
	}

	var gitContext *GitContext
	if gc, ok := payload["gitContext"].(*GitContext); ok {
		gitContext = gc
	}

	var cliAuth *ExecutionAuthContext
	if ca, ok := payload["cliAuth"].(*ExecutionAuthContext); ok {
		cliAuth = ca
	}

	isTrial, _ := payload["isTrialMode"].(bool)
	userEmail, _ := payload["userEmail"].(string)

	inferredPlatform := ""
	if gitContext != nil {
		inferredPlatform = gitContext.InferredPlatform
	}

	// Check rate limit gate
	if err := s.rateLimitGate.Check(ctx, job.OrganizationAndTeamData, inferredPlatform); err != nil {
		s.logger.Warn("Rate limit gate rejected job", "jobId", jobID, "error", err)
		_ = s.HandleFailure(ctx, jobID, err)
		return err
	}

	_ = s.jobRepository.Update(ctx, jobID, "PROCESSING", nil, "")

	type execResult struct {
		resp *CliReviewResponse
		err  error
	}

	resultChan := make(chan execResult, 1)
	go func() {
		resp, err := s.executeCliReviewUseCase.Execute(ctx, ExecuteCliReviewInput{
			OrganizationAndTeamData: job.OrganizationAndTeamData,
			Input:                   input,
			IsTrialMode:             isTrial,
			UserEmail:               userEmail,
			GitContext:              gitContext,
			CliAuth:                 cliAuth,
		})
		resultChan <- execResult{resp: resp, err: err}
	}()

	select {
	case <-abortSignal:
		abortErr := fmt.Errorf("job %s aborted during execution", jobID)
		_ = s.HandleFailure(ctx, jobID, abortErr)
		return abortErr
	case <-ctx.Done():
		timeoutErr := ctx.Err()
		_ = s.HandleFailure(ctx, jobID, timeoutErr)
		return timeoutErr
	case res := <-resultChan:
		if res.err != nil {
			classifiedErr := s.classifyError(res.err)
			s.logger.Error("CLI review job failed",
				"jobId", jobID,
				"correlationId", job.CorrelationID,
				"error", classifiedErr,
			)
			_ = s.HandleFailure(ctx, jobID, classifiedErr)
			return classifiedErr
		}

		if err := s.MarkCompleted(ctx, jobID, res.resp); err != nil {
			return err
		}
		return nil
	}
}

// HandleFailure transitions job status to FAILED with error notes.
func (s *CliReviewJobProcessorService) HandleFailure(ctx context.Context, jobID string, err error) error {
	errMsg := err.Error()
	return s.jobRepository.Update(ctx, jobID, "FAILED", map[string]any{
		"errorClassification": "PERMANENT",
		"failedAt":            time.Now().UTC(),
	}, errMsg)
}

// MarkCompleted records review output and marks job completed.
func (s *CliReviewJobProcessorService) MarkCompleted(ctx context.Context, jobID string, result *CliReviewResponse) error {
	return s.jobRepository.Update(ctx, jobID, "COMPLETED", map[string]any{
		"result":      result,
		"completedAt": time.Now().UTC(),
	}, "")
}

func (s *CliReviewJobProcessorService) classifyError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if strings.Contains(msg, "403") || strings.Contains(msg, "rate limit") || strings.Contains(msg, "429") {
		return fmt.Errorf("RateLimitError: %w", err)
	}
	return err
}

// InMemoryJobRepository provides repository implementation for local execution and unit tests.
type InMemoryJobRepository struct {
	queueService *InMemoryJobQueueService
}

func NewInMemoryJobRepository(qs *InMemoryJobQueueService) *InMemoryJobRepository {
	return &InMemoryJobRepository{queueService: qs}
}

func (r *InMemoryJobRepository) FindOne(ctx context.Context, jobID string) (*JobQueueItem, error) {
	return r.queueService.GetStatus(ctx, jobID)
}

func (r *InMemoryJobRepository) Update(ctx context.Context, jobID string, status string, metadata map[string]any, lastError string) error {
	var result *CliReviewResponse
	if metadata != nil {
		if res, ok := metadata["result"].(*CliReviewResponse); ok {
			result = res
		}
	}
	return r.queueService.UpdateStatus(ctx, jobID, status, result, lastError)
}
