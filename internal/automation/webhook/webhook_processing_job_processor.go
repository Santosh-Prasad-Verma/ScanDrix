package webhook

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// PlatformType defines supported git host integrations.
type PlatformType string

const (
	PlatformGitHub     PlatformType = "github"
	PlatformGitLab     PlatformType = "gitlab"
	PlatformBitbucket  PlatformType = "bitbucket"
	PlatformAzureRepos PlatformType = "azure-repos"
	PlatformForgejo    PlatformType = "forgejo"
)

// WebhookEventParams holds context and payload for a webhook event.
type WebhookEventParams struct {
	Payload       map[string]any
	PlatformType  PlatformType
	Event         string
	CorrelationID string
}

// WebhookEventHandler handles platform-specific webhook ingestion and parsing.
type WebhookEventHandler interface {
	CanHandle(params WebhookEventParams) bool
	Handle(ctx context.Context, params WebhookEventParams) error
}

// JobStatus describes the outcome of background job execution.
type JobStatus string

const (
	JobStatusPending    JobStatus = "PENDING"
	JobStatusInProgress JobStatus = "IN_PROGRESS"
	JobStatusCompleted  JobStatus = "COMPLETED"
	JobStatusFailed     JobStatus = "FAILED"
)

// WorkflowJob models a background job queue item.
type WorkflowJob struct {
	ID            string         `json:"id"`
	WorkflowType  string         `json:"workflow_type"`
	CorrelationID string         `json:"correlation_id"`
	Payload       map[string]any `json:"payload"`
	Metadata      map[string]any `json:"metadata"`
	Status        JobStatus      `json:"status"`
}

// WorkflowJobRepository persists workflow jobs.
type WorkflowJobRepository interface {
	FindOne(ctx context.Context, id string) (*WorkflowJob, error)
	Update(ctx context.Context, id string, updates map[string]any) error
}

// WebhookProcessingJobProcessor executes WEBHOOK_PROCESSING jobs.
type WebhookProcessingJobProcessor struct {
	logger        *slog.Logger
	jobRepo       WorkflowJobRepository
	mu            sync.RWMutex
	handlers      map[PlatformType]WebhookEventHandler
}

// NewWebhookProcessingJobProcessor creates an initialized processor.
func NewWebhookProcessingJobProcessor(
	logger *slog.Logger,
	jobRepo WorkflowJobRepository,
) *WebhookProcessingJobProcessor {
	if logger == nil {
		logger = slog.Default()
	}
	return &WebhookProcessingJobProcessor{
		logger:   logger,
		jobRepo:  jobRepo,
		handlers: make(map[PlatformType]WebhookEventHandler),
	}
}

// RegisterHandler registers a platform-specific webhook event handler.
func (p *WebhookProcessingJobProcessor) RegisterHandler(platform PlatformType, handler WebhookEventHandler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.handlers[platform] = handler
}

// Process processes a webhook job by ID.
func (p *WebhookProcessingJobProcessor) Process(ctx context.Context, jobID string) error {
	job, err := p.jobRepo.FindOne(ctx, jobID)
	if err != nil || job == nil {
		return fmt.Errorf("workflow job %s not found: %w", jobID, err)
	}

	if job.WorkflowType != "WEBHOOK_PROCESSING" {
		return fmt.Errorf("job %s is not a WEBHOOK_PROCESSING workflow, got: %s", jobID, job.WorkflowType)
	}

	p.logger.Debug("processing WEBHOOK_PROCESSING job",
		"jobId", jobID,
		"correlationId", job.CorrelationID,
	)

	platformRaw, _ := job.Metadata["platformType"].(string)
	if platformRaw == "" {
		return fmt.Errorf("job %s missing platformType in metadata", jobID)
	}
	platform := PlatformType(platformRaw)

	event, _ := job.Metadata["event"].(string)
	if event == "" {
		return fmt.Errorf("job %s missing event in metadata", jobID)
	}

	p.mu.RLock()
	handler, exists := p.handlers[platform]
	p.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no handler found for platform %s", platform)
	}

	params := WebhookEventParams{
		Payload:       job.Payload,
		PlatformType:  platform,
		Event:         event,
		CorrelationID: job.CorrelationID,
	}

	if !handler.CanHandle(params) {
		p.logger.Warn("handler cannot handle event",
			"jobId", jobID,
			"platformType", platform,
			"event", event,
		)
		return p.jobRepo.Update(ctx, jobID, map[string]any{
			"status": JobStatusCompleted,
		})
	}

	err = handler.Handle(ctx, params)
	if err != nil {
		p.logger.Error("error handling webhook event", "jobId", jobID, "error", err)
		_ = p.jobRepo.Update(ctx, jobID, map[string]any{
			"status": JobStatusFailed,
		})
		return err
	}

	return p.jobRepo.Update(ctx, jobID, map[string]any{
		"status": JobStatusCompleted,
	})
}
