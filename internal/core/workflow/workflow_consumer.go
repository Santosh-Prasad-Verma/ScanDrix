package workflow

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/core/domain"
)

// WorkflowJobMessage represents an incoming job dispatch message.
type WorkflowJobMessage struct {
	JobID         string `json:"jobId"`
	CorrelationID string `json:"correlationId,omitempty"`
	WorkflowType  string `json:"workflowType,omitempty"`
	HandlerType   string `json:"handlerType,omitempty"`
}

// WorkflowJobConsumer mirrors ScanDrix WorkflowJobConsumer: consumes and dispatches queued workflow jobs.
type WorkflowJobConsumer struct {
	router         *JobProcessorRouter
	jobRepo        JobRepository
	inboxRepo      InboxRepository
	publisher      MessagePublisher
	protection     ITaskProtectionService
	instanceID     string
	activeJobs     int32
	consumerPrefix string
}

// NewWorkflowJobConsumer instantiates a workflow job consumer.
func NewWorkflowJobConsumer(
	router *JobProcessorRouter,
	jobRepo JobRepository,
	inboxRepo InboxRepository,
	publisher MessagePublisher,
	protection ITaskProtectionService,
	instanceID string,
) *WorkflowJobConsumer {
	if protection == nil {
		protection = &NoopTaskProtectionService{}
	}
	return &WorkflowJobConsumer{
		router:         router,
		jobRepo:        jobRepo,
		inboxRepo:      inboxRepo,
		publisher:      publisher,
		protection:     protection,
		instanceID:     instanceID,
		consumerPrefix: "workflow-job-consumer",
	}
}

// HandleWorkflowJob processes an incoming job message with inbox idempotency, task protection, and error routing.
func (c *WorkflowJobConsumer) HandleWorkflowJob(
	ctx context.Context,
	consumerID string,
	queueName string,
	msg WorkflowJobMessage,
) error {
	jobUUID, err := uuid.Parse(msg.JobID)
	if err != nil {
		return fmt.Errorf("invalid job UUID %s: %w", msg.JobID, err)
	}

	messageID := fmt.Sprintf("workflow.jobs:%s:%s", queueName, msg.JobID)

	// 1. Inbox idempotency claim
	claimed, err := c.inboxRepo.Claim(ctx, messageID, consumerID, &jobUUID)
	if err != nil {
		return fmt.Errorf("inbox claim failed for message %s: %w", messageID, err)
	}
	if !claimed {
		return nil // Duplicate message or already being processed
	}

	atomic.AddInt32(&c.activeJobs, 1)
	defer atomic.AddInt32(&c.activeJobs, -1)

	// 2. Fetch Job State
	job, err := c.jobRepo.FindOne(ctx, jobUUID)
	if err != nil {
		_ = c.inboxRepo.Release(ctx, messageID, consumerID, err)
		return fmt.Errorf("failed fetching workflow job %s: %w", jobUUID, err)
	}
	if job == nil {
		_ = c.inboxRepo.Complete(ctx, messageID, consumerID)
		return nil // Non-existent job
	}

	// 3. Acquire container task protection
	_ = c.protection.AcquireProtection(ctx, msg.JobID, 60*time.Minute)
	defer func() {
		_ = c.protection.ReleaseProtection(ctx, msg.JobID)
	}()

	// 4. Process Job with timeout
	procErr := c.router.ProcessJob(ctx, job)
	if procErr != nil {
		return c.handleJobFailure(ctx, job, messageID, consumerID, queueName, procErr)
	}

	// 5. Success: complete inbox record
	return c.inboxRepo.Complete(ctx, messageID, consumerID)
}

func (c *WorkflowJobConsumer) handleJobFailure(
	ctx context.Context,
	job *WorkflowJobModel,
	messageID string,
	consumerID string,
	queueName string,
	err error,
) error {
	classification := c.router.GetClassifier().Classify(err)

	if classification == domain.ErrorClassificationPermanent || job.RetryCount >= job.MaxRetries {
		// Permanent failure or exhausted retries: mark FAILED and complete inbox
		now := time.Now().UTC()
		errMsg := err.Error()
		_ = c.jobRepo.Update(ctx, job.UUID, map[string]any{
			"status":               domain.JobStatusFailed,
			"error_classification": string(classification),
			"last_error":           errMsg,
			"completed_at":         now,
		})

		// Route to DLQ
		_ = c.publisher.Publish(
			ctx,
			"workflow.events.dlx",
			"workflow.job.failed",
			map[string]any{
				"jobId":         job.UUID.String(),
				"correlationId": job.CorrelationID,
				"workflowType":  string(job.WorkflowType),
				"queueName":     queueName,
				"error":         errMsg,
				"errorClass":    string(classification),
			},
			domain.BrokerPublishOptions{Persistent: true},
		)

		return c.inboxRepo.Complete(ctx, messageID, consumerID)
	}

	// Transient or RateLimit: Release inbox lock for scheduled retry
	_ = c.inboxRepo.Release(ctx, messageID, consumerID, err)
	return err
}

// HandleWebhookProcessingJob handles incoming webhook ingestion jobs.
func (c *WorkflowJobConsumer) HandleWebhookProcessingJob(ctx context.Context, msg WorkflowJobMessage) error {
	return c.HandleWorkflowJob(ctx, c.consumerPrefix+".webhook", "workflow.jobs.webhook.queue", msg)
}

// HandleCodeReviewJob handles full pull request code review orchestration jobs.
func (c *WorkflowJobConsumer) HandleCodeReviewJob(ctx context.Context, msg WorkflowJobMessage) error {
	return c.HandleWorkflowJob(ctx, c.consumerPrefix+".code_review", "workflow.jobs.code_review.queue", msg)
}

// HandleCLICodeReviewJob handles CLI-initiated review jobs.
func (c *WorkflowJobConsumer) HandleCLICodeReviewJob(ctx context.Context, msg WorkflowJobMessage) error {
	return c.HandleWorkflowJob(ctx, c.consumerPrefix+".cli_code_review", "workflow.jobs.cli_code_review.queue", msg)
}

// HandleASTGraphBuildJob handles asynchronous AST graph construction jobs.
func (c *WorkflowJobConsumer) HandleASTGraphBuildJob(ctx context.Context, msg WorkflowJobMessage) error {
	return c.HandleWorkflowJob(ctx, c.consumerPrefix+".ast_graph_build", "workflow.jobs.ast_graph_build.queue", msg)
}

// HandleASTGraphIncrementalJob handles incremental PR-diff AST update jobs.
func (c *WorkflowJobConsumer) HandleASTGraphIncrementalJob(ctx context.Context, msg WorkflowJobMessage) error {
	return c.HandleWorkflowJob(ctx, c.consumerPrefix+".ast_graph_incremental", "workflow.jobs.ast_graph_incremental.queue", msg)
}

// HandleCheckSuggestionImplementationJob handles suggestion verification jobs.
func (c *WorkflowJobConsumer) HandleCheckSuggestionImplementationJob(ctx context.Context, msg WorkflowJobMessage) error {
	return c.HandleWorkflowJob(ctx, c.consumerPrefix+".check_implementation", "workflow.jobs.check_suggestion_implementation.queue", msg)
}

// HandleCronDrixyLearningJob handles scheduled Drixy rule learning jobs.
func (c *WorkflowJobConsumer) HandleCronDrixyLearningJob(ctx context.Context, msg WorkflowJobMessage) error {
	return c.HandleWorkflowJob(ctx, c.consumerPrefix+".cron_drixy_learning", "workflow.jobs.cron_drixy_learning.queue", msg)
}

// HandleCronCheckPRApprovalJob handles scheduled PR approval checks.
func (c *WorkflowJobConsumer) HandleCronCheckPRApprovalJob(ctx context.Context, msg WorkflowJobMessage) error {
	return c.HandleWorkflowJob(ctx, c.consumerPrefix+".cron_check_pr_approval", "workflow.jobs.cron_check_pr_approval.queue", msg)
}

// HandleCronCodeReviewFeedbackJob handles scheduled review feedback aggregation jobs.
func (c *WorkflowJobConsumer) HandleCronCodeReviewFeedbackJob(ctx context.Context, msg WorkflowJobMessage) error {
	return c.HandleWorkflowJob(ctx, c.consumerPrefix+".cron_code_review_feedback", "workflow.jobs.cron_code_review_feedback.queue", msg)
}

// GetActiveJobsCount returns the current number of in-flight processing jobs.
func (c *WorkflowJobConsumer) GetActiveJobsCount() int32 {
	return atomic.LoadInt32(&c.activeJobs)
}
