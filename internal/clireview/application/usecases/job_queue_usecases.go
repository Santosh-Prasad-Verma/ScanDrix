package usecases

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/clireview/domain"
)

// Standard error sentinels for job operations.
var (
	ErrJobNotFound = errors.New("CLI review job not found")
	ErrJobTimeout  = errors.New("CLI review job timed out")
)

// JobQueueItem represents an enqueued job in the queue service.
type JobQueueItem struct {
	ID                      string                         `json:"id"`
	CorrelationID           string                         `json:"correlationId"`
	WorkflowType            string                         `json:"workflowType"`
	HandlerType             string                         `json:"handlerType"`
	Status                  string                         `json:"status"` // "PENDING", "PROCESSING", "COMPLETED", "FAILED"
	Priority                int                            `json:"priority"`
	RetryCount              int                            `json:"retryCount"`
	MaxRetries              int                            `json:"maxRetries"`
	OrganizationAndTeamData domain.OrganizationAndTeamData `json:"organizationAndTeamData"`
	Payload                 map[string]any                 `json:"payload"`
	Result                  *domain.CliReviewResponse      `json:"result,omitempty"`
	LastError               string                         `json:"lastError,omitempty"`
	CreatedAt               time.Time                      `json:"createdAt"`
	StartedAt               *time.Time                     `json:"startedAt,omitempty"`
	CompletedAt             *time.Time                     `json:"completedAt,omitempty"`
}

// IJobQueueService abstracts background worker queue management.
type IJobQueueService interface {
	Enqueue(ctx context.Context, item *JobQueueItem) (string, error)
	GetStatus(ctx context.Context, jobID string) (*JobQueueItem, error)
	UpdateStatus(ctx context.Context, jobID string, status string, result *domain.CliReviewResponse, errorMsg string) error
}

// EnqueueCliReviewUseCase enqueues CLI reviews into the asynchronous job queue.
type EnqueueCliReviewUseCase struct {
	queueService IJobQueueService
}

// NewEnqueueCliReviewUseCase creates an initialized EnqueueCliReviewUseCase.
func NewEnqueueCliReviewUseCase(queueService IJobQueueService) *EnqueueCliReviewUseCase {
	return &EnqueueCliReviewUseCase{
		queueService: queueService,
	}
}

// Execute enqueues a CLI review workflow.
func (uc *EnqueueCliReviewUseCase) Execute(ctx context.Context, input domain.EnqueueCliReviewInput) (*domain.EnqueueCliReviewResult, error) {
	corrID := input.CorrelationID
	if corrID == "" {
		buf := make([]byte, 8)
		_, _ = rand.Read(buf)
		corrID = fmt.Sprintf("corr_%s", hex.EncodeToString(buf))
	}

	jobID := uuid.New().String()
	payload := map[string]any{
		"input":       input.Input,
		"isTrialMode": input.IsTrialMode,
		"userEmail":   input.UserEmail,
		"gitContext":  input.GitContext,
		"cliAuth":     input.CliAuth,
		"publicPr":    input.PublicPR,
		"publicDiff":  input.PublicDiff,
	}

	item := &JobQueueItem{
		ID:            jobID,
		CorrelationID: corrID,
		WorkflowType:  "CLI_CODE_REVIEW",
		HandlerType:   "PIPELINE_ASYNC",
		Status:        "PENDING",
		Priority:      0,
		RetryCount:    0,
		MaxRetries:    1,
		OrganizationAndTeamData: domain.OrganizationAndTeamData{
			OrganizationID: input.OrganizationID,
			TeamID:         input.TeamID,
		},
		Payload:   payload,
		CreatedAt: time.Now().UTC(),
	}

	if uc.queueService != nil {
		enqueuedID, err := uc.queueService.Enqueue(ctx, item)
		if err != nil {
			return nil, fmt.Errorf("failed to enqueue review job: %w", err)
		}
		if enqueuedID != "" {
			jobID = enqueuedID
		}
	}

	parsedUUID, _ := uuid.Parse(jobID)
	if parsedUUID == uuid.Nil {
		parsedUUID = uuid.New()
	}

	return &domain.EnqueueCliReviewResult{
		JobID:         parsedUUID,
		CorrelationID: corrID,
	}, nil
}

// WaitForCliReviewJobUseCase polls for completion with exponential backoff.
type WaitForCliReviewJobUseCase struct {
	queueService IJobQueueService
	minDelay     time.Duration
	maxDelay     time.Duration
	maxWait      time.Duration
}

// NewWaitForCliReviewJobUseCase creates an initialized WaitForCliReviewJobUseCase.
func NewWaitForCliReviewJobUseCase(queueService IJobQueueService) *WaitForCliReviewJobUseCase {
	return &WaitForCliReviewJobUseCase{
		queueService: queueService,
		minDelay:     500 * time.Millisecond,
		maxDelay:     5 * time.Second,
		maxWait:      30 * time.Minute,
	}
}

// Execute polls until the job transitions to terminal state.
func (uc *WaitForCliReviewJobUseCase) Execute(ctx context.Context, jobID string) (*domain.CliReviewResponse, error) {
	if uc.queueService == nil {
		return nil, errors.New("job queue service unconfigured")
	}

	startedAt := time.Now()
	delay := uc.minDelay

	for time.Since(startedAt) < uc.maxWait {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		job, err := uc.queueService.GetStatus(ctx, jobID)
		if err != nil || job == nil {
			return nil, fmt.Errorf("%w: %s", ErrJobNotFound, jobID)
		}

		if job.Status == "COMPLETED" {
			if job.Result != nil {
				return job.Result, nil
			}
			return nil, errors.New("CLI review completed but result is missing")
		}

		if job.Status == "FAILED" {
			errMsg := job.LastError
			if errMsg == "" {
				errMsg = "CLI review job failed"
			}
			return nil, errors.New(errMsg)
		}

		time.Sleep(delay)
		delay = delay * 2
		if delay > uc.maxDelay {
			delay = uc.maxDelay
		}
	}

	return nil, fmt.Errorf("%w: %s did not finish within %s", ErrJobTimeout, jobID, uc.maxWait)
}

// GetCliReviewJobStatusInput defines parameters to query status.
type GetCliReviewJobStatusInput struct {
	JobID          string `json:"jobId"`
	OrganizationID string `json:"organizationId"`
	OmitPayload    bool   `json:"omitPayload"`
}

// CliReviewJobStatusResponse conveys job progress and output.
type CliReviewJobStatusResponse struct {
	JobID       string                    `json:"jobId"`
	Status      string                    `json:"status"` // "PENDING", "PROCESSING", "COMPLETED", "FAILED"
	Result      *domain.CliReviewResponse `json:"result,omitempty"`
	Error       string                    `json:"error,omitempty"`
	CreatedAt   time.Time                 `json:"createdAt"`
	StartedAt   *time.Time                `json:"startedAt,omitempty"`
	CompletedAt *time.Time                `json:"completedAt,omitempty"`
	PublicPR    any                       `json:"publicPr,omitempty"`
	PublicDiff  string                    `json:"publicDiff,omitempty"`
}

// GetCliReviewJobStatusUseCase safely queries job status verifying organizational tenancy.
type GetCliReviewJobStatusUseCase struct {
	queueService IJobQueueService
}

// NewGetCliReviewJobStatusUseCase creates an initialized GetCliReviewJobStatusUseCase.
func NewGetCliReviewJobStatusUseCase(queueService IJobQueueService) *GetCliReviewJobStatusUseCase {
	return &GetCliReviewJobStatusUseCase{
		queueService: queueService,
	}
}

// Execute retrieves job status ensuring caller belongs to the tenant.
func (uc *GetCliReviewJobStatusUseCase) Execute(ctx context.Context, input GetCliReviewJobStatusInput) (*CliReviewJobStatusResponse, error) {
	if uc.queueService == nil {
		return nil, errors.New("job queue service unconfigured")
	}

	job, err := uc.queueService.GetStatus(ctx, input.JobID)
	if err != nil || job == nil {
		return nil, fmt.Errorf("%w: %s", ErrJobNotFound, input.JobID)
	}

	if job.WorkflowType != "CLI_CODE_REVIEW" {
		return nil, fmt.Errorf("%w: %s", ErrJobNotFound, input.JobID)
	}

	// Multi-tenant check
	jobOrgID := job.OrganizationAndTeamData.OrganizationID
	if jobOrgID != "" && input.OrganizationID != "" && jobOrgID != input.OrganizationID {
		return nil, fmt.Errorf("%w: %s", ErrJobNotFound, input.JobID)
	}

	resp := &CliReviewJobStatusResponse{
		JobID:       job.ID,
		Status:      job.Status,
		Result:      job.Result,
		Error:       job.LastError,
		CreatedAt:   job.CreatedAt,
		StartedAt:   job.StartedAt,
		CompletedAt: job.CompletedAt,
	}

	if !input.OmitPayload && job.Payload != nil {
		resp.PublicPR = job.Payload["publicPr"]
		if diff, ok := job.Payload["publicDiff"].(string); ok {
			resp.PublicDiff = diff
		}
	}

	return resp, nil
}

// InMemoryJobQueueService provides an in-memory queue for testing and standalone dev.
type InMemoryJobQueueService struct {
	mu   sync.RWMutex
	jobs map[string]*JobQueueItem
}

// NewInMemoryJobQueueService creates an initialized in-memory job queue service.
func NewInMemoryJobQueueService() *InMemoryJobQueueService {
	return &InMemoryJobQueueService{
		jobs: make(map[string]*JobQueueItem),
	}
}

func (s *InMemoryJobQueueService) Enqueue(ctx context.Context, item *JobQueueItem) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[item.ID] = item
	return item.ID, nil
}

func (s *InMemoryJobQueueService) GetStatus(ctx context.Context, jobID string) (*JobQueueItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[jobID]
	if !ok {
		return nil, nil
	}
	return job, nil
}

func (s *InMemoryJobQueueService) UpdateStatus(ctx context.Context, jobID string, status string, result *domain.CliReviewResponse, errorMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[jobID]
	if !ok {
		return fmt.Errorf("job %s not found", jobID)
	}
	job.Status = status
	if result != nil {
		job.Result = result
	}
	if errorMsg != "" {
		job.LastError = errorMsg
	}
	now := time.Now().UTC()
	if status == "PROCESSING" && job.StartedAt == nil {
		job.StartedAt = &now
	}
	if status == "COMPLETED" || status == "FAILED" {
		job.CompletedAt = &now
	}
	return nil
}
