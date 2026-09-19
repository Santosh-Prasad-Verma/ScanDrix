package workflow

import (
	"context"
	"fmt"
	"time"

	"github.com/scandrix/backend/internal/core/domain"
)

// WaitingJobRepository extends JobRepository to support event correlation queries.
type WaitingJobRepository interface {
	JobRepository
	FindWaitingJobsByEvent(ctx context.Context, eventType, eventKey string) ([]*WorkflowJobModel, error)
}

// HeavyStageEventHandler mirrors ScanDrix HeavyStageEventHandler.
// Resumes paused workflows when heavy asynchronous stage completion events arrive.
type HeavyStageEventHandler struct {
	jobRepo      WaitingJobRepository
	stateManager *PipelineStateManager
	eventBuffer  *EventBufferService
	inboxRepo    InboxRepository
	publisher    MessagePublisher
	consumerID   string
}

// NewHeavyStageEventHandler instantiates a heavy stage event handler.
func NewHeavyStageEventHandler(
	jobRepo WaitingJobRepository,
	stateManager *PipelineStateManager,
	eventBuffer *EventBufferService,
	inboxRepo InboxRepository,
	publisher MessagePublisher,
) *HeavyStageEventHandler {
	return &HeavyStageEventHandler{
		jobRepo:      jobRepo,
		stateManager: stateManager,
		eventBuffer:  eventBuffer,
		inboxRepo:    inboxRepo,
		publisher:    publisher,
		consumerID:   "workflow-events-stage-completed",
	}
}

// OnStageCompleted handles stage completion event from RabbitMQ (workflow.events / stage.completed.*).
func (h *HeavyStageEventHandler) OnStageCompleted(ctx context.Context, event StageCompletedEvent) error {
	messageID := fmt.Sprintf("stage.completed:%s:%s:%s", event.EventType, event.EventKey, event.TaskID)

	// 1. Idempotency claim in inbox
	claimed, err := h.inboxRepo.Claim(ctx, messageID, h.consumerID, nil)
	if err != nil {
		return fmt.Errorf("inbox claim failed for heavy stage event %s: %w", messageID, err)
	}
	if !claimed {
		return nil // duplicate event
	}

	// 2. Buffer event to guarantee no loss under race conditions
	h.eventBuffer.Store(event.EventType, event.EventKey, event, DefaultEventBufferTTL)

	// 3. Find paused workflow jobs waiting for this event
	waitingJobs, err := h.jobRepo.FindWaitingJobsByEvent(ctx, event.EventType, event.EventKey)
	if err != nil {
		_ = h.inboxRepo.Release(ctx, messageID, h.consumerID, err)
		return fmt.Errorf("failed querying waiting jobs for event %s:%s: %w", event.EventType, event.EventKey, err)
	}

	for _, job := range waitingJobs {
		// 4. Save intermediate stage results into pipeline state snapshot
		if event.Result != nil {
			_ = h.stateManager.SaveState(ctx, job.UUID, event.Result, event.EventType)
		}

		// 5. Transition job status back to PROCESSING and clear waitingForEvent
		now := time.Now().UTC()
		err := h.jobRepo.Update(ctx, job.UUID, map[string]any{
			"status":            domain.JobStatusProcessing,
			"waiting_for_event": nil,
			"updated_at":        now,
		})
		if err != nil {
			continue
		}

		// 6. Re-enqueue job to resume pipeline execution
		resumePayload := map[string]any{
			"jobId":         job.UUID.String(),
			"correlationId": job.CorrelationID,
			"workflowType":  string(job.WorkflowType),
			"resumedAt":     now.Format(time.RFC3339),
			"resumedEvent":  event.EventType,
		}

		routingKey := fmt.Sprintf("workflow.jobs.resume.%s", job.WorkflowType)
		_ = h.publisher.Publish(
			ctx,
			"workflow.exchange",
			routingKey,
			resumePayload,
			domain.BrokerPublishOptions{
				Persistent: true,
			},
		)
	}

	return h.inboxRepo.Complete(ctx, messageID, h.consumerID)
}

// CheckAndResumeWaitingJob checks if an event was already received and buffered before the job paused.
func (h *HeavyStageEventHandler) CheckAndResumeWaitingJob(ctx context.Context, job *WorkflowJobModel) (bool, error) {
	if job == nil || job.WaitingForEvent == nil {
		return false, nil
	}

	spec := job.WaitingForEvent
	event, found := h.eventBuffer.Check(spec.EventType, spec.EventKey)
	if !found {
		return false, nil
	}

	// Found buffered event: resume immediately
	now := time.Now().UTC()
	if event.Result != nil {
		_ = h.stateManager.SaveState(ctx, job.UUID, event.Result, event.EventType)
	}

	err := h.jobRepo.Update(ctx, job.UUID, map[string]any{
		"status":            domain.JobStatusProcessing,
		"waiting_for_event": nil,
		"updated_at":        now,
	})
	if err != nil {
		return false, err
	}

	resumePayload := map[string]any{
		"jobId":         job.UUID.String(),
		"correlationId": job.CorrelationID,
		"workflowType":  string(job.WorkflowType),
		"resumedAt":     now.Format(time.RFC3339),
		"buffered":      true,
	}

	routingKey := fmt.Sprintf("workflow.jobs.resume.%s", job.WorkflowType)
	_ = h.publisher.Publish(
		ctx,
		"workflow.exchange",
		routingKey,
		resumePayload,
		domain.BrokerPublishOptions{
			Persistent: true,
		},
	)

	return true, nil
}
