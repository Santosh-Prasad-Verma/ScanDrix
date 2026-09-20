package workflow

import (
	"context"
	"fmt"
	"time"

	"github.com/scandrix/backend/internal/core/domain"
)

// ASTCompletedMessage mirrors ASTCompletedMessage from ScanDrix ast-event-handler.service.ts.
type ASTCompletedMessage struct {
	TaskID string         `json:"taskId"`
	Result map[string]any `json:"result"`
}

// ASTEventHandler mirrors ScanDrix ASTEventHandler: consumes and correlates asynchronous AST graph completion events.
type ASTEventHandler struct {
	waitingJobRepo WaitingJobRepository
	inboxRepo      InboxRepository
	publisher      MessagePublisher
	consumerID     string
	instanceID     string
}

// NewASTEventHandler instantiates an AST event handler.
func NewASTEventHandler(
	waitingJobRepo WaitingJobRepository,
	inboxRepo InboxRepository,
	publisher MessagePublisher,
	instanceID string,
) *ASTEventHandler {
	return &ASTEventHandler{
		waitingJobRepo: waitingJobRepo,
		inboxRepo:      inboxRepo,
		publisher:      publisher,
		consumerID:     "workflow-events-ast",
		instanceID:     instanceID,
	}
}

// HandleASTCompleted processes an incoming ast.task.completed event.
func (h *ASTEventHandler) HandleASTCompleted(ctx context.Context, msg ASTCompletedMessage, messageID string) error {
	if messageID == "" {
		messageID = fmt.Sprintf("ast.task.completed:%s", msg.TaskID)
	}

	// 1. Inbox claim for deduplication
	claimed, err := h.inboxRepo.Claim(ctx, messageID, h.consumerID, nil)
	if err != nil {
		return fmt.Errorf("inbox claim failed for ast event %s: %w", messageID, err)
	}
	if !claimed {
		return nil // Already processed or claimed by another worker
	}

	// 2. Correlate with waiting workflow jobs
	jobs, err := h.waitingJobRepo.FindWaitingJobsByEvent(ctx, "ast.task.completed", msg.TaskID)
	if err != nil {
		_ = h.inboxRepo.Release(ctx, messageID, h.consumerID, err)
		return fmt.Errorf("failed finding waiting jobs for AST task %s: %w", msg.TaskID, err)
	}

	now := time.Now().UTC()

	for _, job := range jobs {
		// Update pipeline state / payload with AST result
		if job.Payload == nil {
			job.Payload = make(map[string]any)
		}
		job.Payload["astResult"] = msg.Result

		updates := map[string]any{
			"status":            domain.JobStatusProcessing,
			"payload":           job.Payload,
			"waiting_for_event": nil,
			"updated_at":        now,
		}

		if err := h.waitingJobRepo.Update(ctx, job.UUID, updates); err != nil {
			_ = h.inboxRepo.Release(ctx, messageID, h.consumerID, err)
			return fmt.Errorf("failed resuming waiting workflow job %s: %w", job.UUID, err)
		}

		// Re-enqueue job back into active queue for continuation
		if h.publisher != nil {
			_ = h.publisher.Publish(
				ctx,
				"workflow.exchange",
				fmt.Sprintf("workflow.jobs.*.%s", job.WorkflowType),
				map[string]any{
					"jobId":         job.UUID.String(),
					"correlationId": job.CorrelationID,
					"workflowType":  string(job.WorkflowType),
					"resumedFrom":   "ast.task.completed",
				},
				domain.BrokerPublishOptions{Persistent: true},
			)
		}
	}

	return h.inboxRepo.Complete(ctx, messageID, h.consumerID)
}
