package workflow

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/core/domain"
)

// WorkflowJobStatus represents the lifecycle state of a workflow job.
type WorkflowJobStatus string

const (
	JobStatusPending        WorkflowJobStatus = "PENDING"
	JobStatusProcessing     WorkflowJobStatus = "PROCESSING"
	JobStatusCompleted      WorkflowJobStatus = "COMPLETED"
	JobStatusFailed         WorkflowJobStatus = "FAILED"
	JobStatusWaitingForEvent WorkflowJobStatus = "WAITING_FOR_EVENT"
	JobStatusCancelled      WorkflowJobStatus = "CANCELLED"
	JobStatusRetrying       WorkflowJobStatus = "RETRYING"
	JobStatusPaused         WorkflowJobStatus = "PAUSED"
)

// JobExecutionHistoryEntry records a historical transition in job execution.
type JobExecutionHistoryEntry struct {
	Timestamp   time.Time               `json:"timestamp"`
	Status      WorkflowJobStatus       `json:"status"`
	Stage       string                  `json:"stage,omitempty"`
	Message     string                  `json:"message,omitempty"`
	WorkerID    string                  `json:"workerId,omitempty"`
	DurationMs  int64                   `json:"durationMs,omitempty"`
	ErrorDetail *string                 `json:"errorDetail,omitempty"`
	ErrorClass  *domain.ErrorClassification `json:"errorClass,omitempty"`
}

// WorkflowMetrics represents aggregate platform workflow telemetry.
type WorkflowMetrics struct {
	QueueSize             int                `json:"queueSize"`
	ProcessingCount       int                `json:"processingCount"`
	CompletedToday        int                `json:"completedToday"`
	FailedToday           int                `json:"failedToday"`
	AverageProcessingTime float64            `json:"averageProcessingTimeMs"`
	SuccessRate           float64            `json:"successRatePercent"`
	ByStatus              map[string]int     `json:"byStatus"`
	StaleProcessingJobs   int                `json:"staleProcessingJobs"`
	InboxLag              int                `json:"inboxLag"`
	OutboxLag             int                `json:"outboxLag"`
}

// JobDetailResponse bundles the active job state and its execution history.
type JobDetailResponse struct {
	Job              *WorkflowJobModel          `json:"job"`
	ExecutionHistory []JobExecutionHistoryEntry `json:"executionHistory"`
	ProgressPercent  int                        `json:"progressPercent"`
}

// ITaskProtectionService guards worker nodes against premature termination.
type ITaskProtectionService interface {
	AcquireProtection(ctx context.Context, taskID string, duration time.Duration) error
	ReleaseProtection(ctx context.Context, taskID string) error
	RenewProtection(ctx context.Context, taskID string, duration time.Duration) error
}

// NoopTaskProtectionService is the default container task protection implementation.
type NoopTaskProtectionService struct{}

func (s *NoopTaskProtectionService) AcquireProtection(ctx context.Context, taskID string, duration time.Duration) error {
	return nil
}

func (s *NoopTaskProtectionService) ReleaseProtection(ctx context.Context, taskID string) error {
	return nil
}

func (s *NoopTaskProtectionService) RenewProtection(ctx context.Context, taskID string, duration time.Duration) error {
	return nil
}

// RateLimitError encapsulates an upstream API rate limit response with its reset deadline.
type RateLimitError struct {
	ResetAt   time.Time
	Remaining int
	Message   string
}

func (e *RateLimitError) Error() string {
	return e.Message
}

// InboxRepository defines idempotency message operations.
type InboxRepository interface {
	Claim(ctx context.Context, messageID, consumerID string, jobID *uuid.UUID) (bool, error)
	Complete(ctx context.Context, messageID, consumerID string) error
	Release(ctx context.Context, messageID, consumerID string, lastError error) error
}

// JobProcessorRouter is an alias for JobProcessorRouterService.
type JobProcessorRouter = JobProcessorRouterService

// JobQueueServiceContract defines operations for queueing and managing workflow jobs.
type JobQueueServiceContract interface {
	Enqueue(ctx context.Context, job *WorkflowJobModel) (uuid.UUID, error)
	Schedule(ctx context.Context, job *WorkflowJobModel, scheduledAt time.Time) (uuid.UUID, error)
	Cancel(ctx context.Context, jobID uuid.UUID) error
	GetStatus(ctx context.Context, jobID uuid.UUID) (*WorkflowJobModel, error)
}
