package consumer

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/enterprise/license"
	"github.com/scandrix/backend/internal/queue/relay"
	"github.com/scandrix/backend/pkg/models"
)

// ═══════════════════════════════════════════════════════════════
// 1. CONSUMER SCHEMA & EXECUTOR CALLBACK (Worker state & retry thresholds)
// ═══════════════════════════════════════════════════════════════

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

// ═══════════════════════════════════════════════════════════════
// 2. CONSUMER FACTORY & WORKER INITIALIZATION (Worker ID & inbox binding)
// ═══════════════════════════════════════════════════════════════

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

// ═══════════════════════════════════════════════════════════════
// 3. IDEMPOTENT TASK PROCESSING ENGINE (Inbox claim, execution & exponential backoff)
// ═══════════════════════════════════════════════════════════════

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

		// 3. A deliberate refusal is permanent: ack it and record why, instead
		// of spending the retry budget on a decision that cannot change.
		var skipErr *skipError
		if errors.As(err, &skipErr) || errors.Is(err, license.ErrExecutionRefused) {
			if c.inbox != nil {
				_ = c.inbox.MarkCompleted(ctx, taskIDStr, c.workerID)
			}
			msg := err.Error()
			if skipErr != nil {
				msg = skipErr.Error()
			}
			return TaskExecutionResult{
				TaskID:       task.TaskID,
				Status:       TaskStatusSkipped,
				AttemptCount: task.AttemptCount,
				Duration:     time.Since(start),
				ErrorMsg:     msg,
				Timestamp:    time.Now().UTC(),
			}
		}

		// 4. Check if retries exhausted
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

// ═══════════════════════════════════════════════════════════════
// skipError marks an error as a deliberate refusal rather than a failure, so
// the consumer acks instead of retrying. The license package produces these via
// ExecutionDecision.RefusalError.
type skipError struct{ err error }

func (e *skipError) Error() string { return e.err.Error() }
func (e *skipError) Unwrap() error { return e.err }

// AsSkip reports whether err is a deliberate refusal, returning the underlying
// error when it is.
func AsSkip(err error) (error, bool) {
	var se *skipError
	if errors.As(err, &se) {
		return se.err, true
	}
	return nil, false
}

// 4. DEAD-LETTER AUDIT & REDRIVE REPOSITORY (DLQ payload inspection)
// ═══════════════════════════════════════════════════════════════

// GetDeadLetters returns messages routed to DLQ for redrive or audit inspection.
func (c *ReviewConsumer) GetDeadLetters() []ReviewTaskPayload {
	c.mu.Lock()
	defer c.mu.Unlock()
	res := make([]ReviewTaskPayload, len(c.deadLetters))
	copy(res, c.deadLetters)
	return res
}
