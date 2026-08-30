package consumer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/queue/relay"
	"github.com/scandrix/backend/pkg/models"
)

// ReviewExecutor defines the callback invoked when a review job executes.
type ReviewExecutor func(ctx context.Context, task ReviewTaskPayload) ([]models.CodeFinding, error)

// ReviewConsumer processes tasks with inbox idempotency, retries, and dead-lettering.
type ReviewConsumer struct {
	mu          sync.Mutex
	cfg         ConsumerConfig
	inbox       *relay.InboxDeduplicator
	executor    ReviewExecutor
	workerID    string
	deadLetters []ReviewTaskPayload
}

// NewReviewConsumer initializes the consumer.
func NewReviewConsumer(cfg ConsumerConfig, inbox *relay.InboxDeduplicator, executor ReviewExecutor) *ReviewConsumer {
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 5
	}
	return &ReviewConsumer{
		cfg:         cfg,
		inbox:       inbox,
		executor:    executor,
		workerID:    fmt.Sprintf("worker-%s", uuid.New().String()[:8]),
		deadLetters: make([]ReviewTaskPayload, 0),
	}
}

// ProcessTask handles an incoming queue task enforcing exactly-once semantics.
func (c *ReviewConsumer) ProcessTask(ctx context.Context, task ReviewTaskPayload) TaskExecutionResult {
	start := time.Now()
	taskIDStr := task.TaskID.String()

	// 1. Claim task in inbox to enforce idempotency (Master Rule: Idempotency inbox pattern)
	if c.inbox != nil {
		claimed, err := c.inbox.ClaimMessage(ctx, taskIDStr, c.workerID)
		if err != nil || !claimed {
			return TaskExecutionResult{
				TaskID:       task.TaskID,
				Status:       TaskStatusDuplicate,
				AttemptCount: task.AttemptCount,
				Duration:     time.Since(start),
				ErrorMsg:     "task already claimed or processed by another consumer",
				Timestamp:    time.Now().UTC(),
			}
		}
		inboxAttempts := c.inbox.GetAttemptCount(taskIDStr, c.workerID)
		if inboxAttempts > task.AttemptCount {
			task.AttemptCount = inboxAttempts
		}
	}

	// 2. Execute Code Review Pipeline
	_, err := c.executor(ctx, task)
	if err != nil {
		task.AttemptCount++
		if c.inbox != nil {
			inboxAttempts := c.inbox.GetAttemptCount(taskIDStr, c.workerID)
			if inboxAttempts > task.AttemptCount {
				task.AttemptCount = inboxAttempts
			}
		}

		// 3. Check if retries exhausted
		if task.AttemptCount >= c.cfg.MaxRetries {
			c.mu.Lock()
			c.deadLetters = append(c.deadLetters, task)
			c.mu.Unlock()

			if c.inbox != nil {
				_ = c.inbox.MarkCompleted(ctx, taskIDStr, c.workerID)
			}

			return TaskExecutionResult{
				TaskID:       task.TaskID,
				Status:       TaskStatusDeadLetter,
				AttemptCount: task.AttemptCount,
				Duration:     time.Since(start),
				ErrorMsg:     fmt.Sprintf("max retries (%d) exceeded: %v", c.cfg.MaxRetries, err),
				Timestamp:    time.Now().UTC(),
			}
		}

		// Release lease for next retry attempt
		if c.inbox != nil {
			_ = c.inbox.ReleaseMessage(ctx, taskIDStr, c.workerID, task.AttemptCount)
		}

		return TaskExecutionResult{
			TaskID:       task.TaskID,
			Status:       TaskStatusRetry,
			AttemptCount: task.AttemptCount,
			Duration:     time.Since(start),
			ErrorMsg:     err.Error(),
			Timestamp:    time.Now().UTC(),
		}
	}

	// 4. Mark successfully completed in inbox
	if c.inbox != nil {
		_ = c.inbox.MarkCompleted(ctx, taskIDStr, c.workerID)
	}

	return TaskExecutionResult{
		TaskID:       task.TaskID,
		Status:       TaskStatusSuccess,
		AttemptCount: task.AttemptCount,
		Duration:     time.Since(start),
		Timestamp:    time.Now().UTC(),
	}
}

// GetDeadLetters returns messages routed to DLQ for redrive or audit inspection.
func (c *ReviewConsumer) GetDeadLetters() []ReviewTaskPayload {
	c.mu.Lock()
	defer c.mu.Unlock()
	res := make([]ReviewTaskPayload, len(c.deadLetters))
	copy(res, c.deadLetters)
	return res
}
