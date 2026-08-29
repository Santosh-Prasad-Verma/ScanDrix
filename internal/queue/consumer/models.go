package consumer

import (
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// ReviewTaskPayload represents the structured task distributed via queue for review execution.
type ReviewTaskPayload struct {
	TaskID            uuid.UUID          `json:"task_id"`
	EventID           uuid.UUID          `json:"event_id"`
	WorkspaceID       uuid.UUID          `json:"workspace_id"`
	Provider          models.SCMProvider `json:"provider"`
	RepoNamespace     string             `json:"repo_namespace"`
	PullRequestNumber int                `json:"pull_request_number"`
	HeadSHA           string             `json:"head_sha"`
	BaseSHA           string             `json:"base_sha"`
	Sender            string             `json:"sender"`
	AttemptCount      int                `json:"attempt_count"`
	EnqueuedAt        time.Time          `json:"enqueued_at"`
}

// ConsumerConfig specifies queue connection and concurrency limits.
type ConsumerConfig struct {
	QueueName        string        `json:"queue_name"`
	DeadLetterQueue  string        `json:"dead_letter_queue"`
	MaxRetries       int           `json:"max_retries"`       // Default 5
	Concurrency      int           `json:"concurrency"`       // Number of parallel workers
	PollInterval     time.Duration `json:"poll_interval"`
	ClaimTTL         time.Duration `json:"claim_ttl"`
}

// TaskStatus classifies task execution outcome.
type TaskStatus string

const (
	TaskStatusSuccess    TaskStatus = "SUCCESS"
	TaskStatusRetry      TaskStatus = "RETRY"
	TaskStatusDeadLetter TaskStatus = "DEAD_LETTER"
	TaskStatusDuplicate  TaskStatus = "DUPLICATE"
)

// TaskExecutionResult logs worker execution telemetry.
type TaskExecutionResult struct {
	TaskID    uuid.UUID     `json:"task_id"`
	Status    TaskStatus    `json:"status"`
	Duration  time.Duration `json:"duration"`
	ErrorMsg  string        `json:"error_msg,omitempty"`
	Timestamp time.Time     `json:"timestamp"`
}
