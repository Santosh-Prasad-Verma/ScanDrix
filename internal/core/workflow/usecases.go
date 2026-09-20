package workflow

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/core/domain"
	"github.com/scandrix/backend/internal/core/idgen"
)

// EnqueueCodeReviewInput specifies the required parameters to trigger a code review.
type EnqueueCodeReviewInput struct {
	TenantID      string         `json:"tenantId"`
	WorkspaceID   string         `json:"workspaceId"`
	RepositoryID  string         `json:"repositoryId"`
	PRNumber      int            `json:"prNumber"`
	CommitSHA     string         `json:"commitSha"`
	BaseSHA       string         `json:"baseSha,omitempty"`
	SourceBranch  string         `json:"sourceBranch,omitempty"`
	TargetBranch  string         `json:"targetBranch,omitempty"`
	Title         string         `json:"title,omitempty"`
	Author        string         `json:"author,omitempty"`
	Priority      int            `json:"priority,omitempty"`
	CorrelationID string         `json:"correlationId,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

// EnqueueCodeReviewJobUseCase mirrors ScanDrix EnqueueCodeReviewJobUseCase: validates, deduplicates, and enqueues.
type EnqueueCodeReviewJobUseCase struct {
	queueService *WorkflowJobQueueService
	idGen        idgen.IdGenerator
}

// NewEnqueueCodeReviewJobUseCase instantiates an enqueue use case.
func NewEnqueueCodeReviewJobUseCase(queueService *WorkflowJobQueueService) *EnqueueCodeReviewJobUseCase {
	return &EnqueueCodeReviewJobUseCase{
		queueService: queueService,
		idGen:        idgen.IdGenerator{},
	}
}

// Execute enqueues a code review job transactionally with deduplication.
func (uc *EnqueueCodeReviewJobUseCase) Execute(ctx context.Context, input EnqueueCodeReviewInput) (uuid.UUID, error) {
	if input.WorkspaceID == "" || input.RepositoryID == "" || input.PRNumber <= 0 || input.CommitSHA == "" {
		return uuid.Nil, fmt.Errorf("missing required review fields: workspaceId, repositoryId, prNumber, commitSha are mandatory")
	}

	correlationID := input.CorrelationID
	if correlationID == "" {
		correlationID = uc.idGen.CorrelationID()
	}

	priority := input.Priority
	if priority <= 0 || priority > 9 {
		priority = 5
	}

	jobID := uuid.New()
	now := time.Now().UTC()

	payload := map[string]any{
		"tenantId":      input.TenantID,
		"workspaceId":   input.WorkspaceID,
		"repositoryId":  input.RepositoryID,
		"prNumber":      input.PRNumber,
		"commitSha":     input.CommitSHA,
		"baseSha":       input.BaseSHA,
		"sourceBranch":  input.SourceBranch,
		"targetBranch":  input.TargetBranch,
		"title":         input.Title,
		"author":        input.Author,
		"metadata":      input.Metadata,
		"enqueuedAt":    now.Format(time.RFC3339),
	}

	job := &WorkflowJobModel{
		UUID:          jobID,
		WorkflowType:  domain.WorkflowTypeCodeReview,
		Status:        domain.JobStatusPending,
		Priority:      priority,
		CorrelationID: correlationID,
		Payload:       payload,
		MaxRetries:    5,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if orgUUID, err := uuid.Parse(input.TenantID); err == nil {
		job.OrganizationID = &orgUUID
	}

	if _, err := uc.queueService.Enqueue(ctx, job); err != nil {
		return uuid.Nil, fmt.Errorf("failed enqueuing code review job: %w", err)
	}

	return jobID, nil
}

// ProcessWorkflowJobUseCase mirrors ScanDrix ProcessWorkflowJobUseCase.
type ProcessWorkflowJobUseCase struct {
	router  *JobProcessorRouter
	jobRepo JobRepository
}

// NewProcessWorkflowJobUseCase instantiates a process job use case.
func NewProcessWorkflowJobUseCase(router *JobProcessorRouter, jobRepo JobRepository) *ProcessWorkflowJobUseCase {
	return &ProcessWorkflowJobUseCase{
		router:  router,
		jobRepo: jobRepo,
	}
}

// Execute executes an individual workflow job through the strategy router.
func (uc *ProcessWorkflowJobUseCase) Execute(ctx context.Context, jobID uuid.UUID) error {
	job, err := uc.jobRepo.FindOne(ctx, jobID)
	if err != nil {
		return fmt.Errorf("failed finding job %s: %w", jobID, err)
	}
	if job == nil {
		return fmt.Errorf("job %s does not exist", jobID)
	}

	return uc.router.ProcessJob(ctx, job)
}

// GetJobStatusUseCase mirrors ScanDrix GetJobStatusUseCase.
type GetJobStatusUseCase struct {
	statusService *JobStatusService
}

// NewGetJobStatusUseCase instantiates a get job status use case.
func NewGetJobStatusUseCase(statusService *JobStatusService) *GetJobStatusUseCase {
	return &GetJobStatusUseCase{
		statusService: statusService,
	}
}

// Execute retrieves the full status and execution timeline for a job.
func (uc *GetJobStatusUseCase) Execute(ctx context.Context, jobID uuid.UUID) (*JobDetailResponse, error) {
	return uc.statusService.GetJobDetail(ctx, jobID)
}
