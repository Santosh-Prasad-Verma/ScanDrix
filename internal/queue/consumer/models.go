package consumer

import (
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// ReviewTaskPayload represents the structured task distributed via queue for review execution.
type ReviewTaskPayload struct {
	ID            uuid.UUID          `json:"id,omitempty"`
	TaskID        uuid.UUID          `json:"task_id"`
	EventID       uuid.UUID          `json:"event_id"`
	WorkspaceID   uuid.UUID          `json:"workspace_id"`
	Provider      models.SCMProvider `json:"provider"`
	RepoNamespace string             `json:"repo_namespace"`
	// RepositoryID is the tracked_repositories id. The worker resolves it from
	// RepoNamespace when absent, because pull_request_reviews.repository_id is
	// a foreign key and a review cannot persist without it.
	RepositoryID      uuid.UUID `json:"repository_id,omitempty"`
	PullRequestNumber int       `json:"pull_request_number"`
	HeadSHA           string    `json:"head_sha"`
	BaseSHA           string    `json:"base_sha"`
	Sender            string    `json:"sender"`
	AttemptCount      int       `json:"attempt_count"`
	EnqueuedAt        time.Time `json:"enqueued_at"`

	// RequiredFeature names a licensed capability this task depends on, if any.
	// The worker passes it to the entitlement gate before doing any work, so a
	// workspace without the entitlement is refused up front rather than after
	// paying for diff fetch, LLM calls and SCM writes.
	//
	// Empty means the task only needs the baseline review, which Community is
	// entitled to. Today no review stage is EE-only, so producers leave this
	// empty; it is data-driven rather than hardcoded so that adding a paid
	// stage cannot silently ship ungated.
	RequiredFeature string `json:"required_feature,omitempty"`
}

// ConsumerConfig specifies queue connection and concurrency limits.
type ConsumerConfig struct {
	QueueName       string        `json:"queue_name"`
	DeadLetterQueue string        `json:"dead_letter_queue"`
	MaxRetries      int           `json:"max_retries"` // Default 5
	Concurrency     int           `json:"concurrency"` // Number of parallel workers
	PollInterval    time.Duration `json:"poll_interval"`
	ClaimTTL        time.Duration `json:"claim_ttl"`
}

// TaskStatus classifies task execution outcome.
type TaskStatus string

const (
	TaskStatusSuccess    TaskStatus = "SUCCESS"
	TaskStatusRetry      TaskStatus = "RETRY"
	TaskStatusDeadLetter TaskStatus = "DEAD_LETTER"
	TaskStatusDuplicate  TaskStatus = "DUPLICATE"
	// TaskStatusSkipped marks a task the system deliberately refused to run,
	// such as one an entitlement gate rejected. It is acked, not retried: the
	// refusal is deterministic, so re-queueing it would spend five attempts to
	// reach the same DLQ entry an operator would only have to delete.
	TaskStatusSkipped TaskStatus = "SKIPPED"
)

// TaskExecutionResult logs worker execution telemetry.
type TaskExecutionResult struct {
	TaskID       uuid.UUID     `json:"task_id"`
	Status       TaskStatus    `json:"status"`
	AttemptCount int           `json:"attempt_count"`
	Duration     time.Duration `json:"duration"`
	ErrorMsg     string        `json:"error_msg,omitempty"`
	Timestamp    time.Time     `json:"timestamp"`
}

// WorkerJob encapsulates a task payload and its optional post-execution completion handler.
type WorkerJob struct {
	Task       ReviewTaskPayload
	OnComplete func(res TaskExecutionResult)
}
