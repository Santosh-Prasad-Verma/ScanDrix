package workflow

import (
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/core/domain"
)

// WaitingForEventSpec mirrors ScanDrix waitingForEvent metadata structure.
type WaitingForEventSpec struct {
	EventType string    `json:"eventType"`
	EventKey  string    `json:"eventKey"`
	TimeoutMs int64     `json:"timeout"`
	PausedAt  time.Time `json:"pausedAt"`
}

// WorkflowJobModel mirrors ScanDrix WorkflowJobModel (`workflow_jobs`).
type WorkflowJobModel struct {
	UUID                uuid.UUID                   `json:"uuid" db:"uuid"`
	CreatedAt           time.Time                   `json:"createdAt" db:"created_at"`
	UpdatedAt           time.Time                   `json:"updatedAt" db:"updated_at"`
	CorrelationID       string                      `json:"correlationId" db:"correlation_id"`
	WorkflowType        domain.WorkflowType         `json:"workflowType" db:"workflow_type"`
	HandlerType         domain.HandlerType          `json:"handlerType" db:"handler_type"`
	Payload             map[string]any              `json:"payload" db:"payload"`
	Status              domain.JobStatus            `json:"status" db:"status"`
	Priority            int                         `json:"priority" db:"priority"`
	RetryCount          int                         `json:"retryCount" db:"retry_count"`
	MaxRetries          int                         `json:"maxRetries" db:"max_retries"`
	OrganizationID      *uuid.UUID                  `json:"organizationId,omitempty" db:"organization_id"`
	TeamID              *uuid.UUID                  `json:"teamId,omitempty" db:"team_id"`
	ErrorClassification *domain.ErrorClassification `json:"errorClassification,omitempty" db:"error_classification"`
	LastError           *string                     `json:"lastError,omitempty" db:"last_error"`
	ScheduledAt         *time.Time                  `json:"scheduledAt,omitempty" db:"scheduled_at"`
	StartedAt           *time.Time                  `json:"startedAt,omitempty" db:"started_at"`
	CompletedAt         *time.Time                  `json:"completedAt,omitempty" db:"completed_at"`
	CurrentStage        *string                     `json:"currentStage,omitempty" db:"current_stage"`
	Metadata            map[string]any              `json:"metadata,omitempty" db:"metadata"`
	WaitingForEvent     *WaitingForEventSpec        `json:"waitingForEvent,omitempty" db:"waiting_for_event"`
}

// OutboxMessageModel mirrors ScanDrix OutboxMessageModel (`outbox_messages`).
type OutboxMessageModel struct {
	UUID          uuid.UUID           `json:"uuid" db:"uuid"`
	CreatedAt     time.Time           `json:"createdAt" db:"created_at"`
	UpdatedAt     time.Time           `json:"updatedAt" db:"updated_at"`
	JobID         *uuid.UUID          `json:"jobId,omitempty" db:"job_id"`
	Exchange      string              `json:"exchange" db:"exchange"`
	RoutingKey    string              `json:"routingKey" db:"routing_key"`
	Payload       map[string]any      `json:"payload" db:"payload"`
	Status        domain.OutboxStatus `json:"status" db:"status"`
	Attempts      int                 `json:"attempts" db:"attempts"`
	NextAttemptAt time.Time           `json:"nextAttemptAt" db:"next_attempt_at"`
	LockedAt      *time.Time          `json:"lockedAt,omitempty" db:"locked_at"`
	LockedBy      *string             `json:"lockedBy,omitempty" db:"locked_by"`
	LastError     *string             `json:"lastError,omitempty" db:"last_error"`
	ProcessedAt   *time.Time          `json:"processedAt,omitempty" db:"processed_at"`
}

// InboxMessageModel mirrors ScanDrix InboxMessageModel (`inbox_messages`).
type InboxMessageModel struct {
	UUID          uuid.UUID          `json:"uuid" db:"uuid"`
	CreatedAt     time.Time          `json:"createdAt" db:"created_at"`
	UpdatedAt     time.Time          `json:"updatedAt" db:"updated_at"`
	MessageID     string             `json:"messageId" db:"message_id"`
	ConsumerID    string             `json:"consumerId" db:"consumer_id"`
	JobID         *uuid.UUID         `json:"jobId,omitempty" db:"job_id"`
	Status        domain.InboxStatus `json:"status" db:"status"`
	Attempts      int                `json:"attempts" db:"attempts"`
	NextAttemptAt time.Time          `json:"nextAttemptAt" db:"next_attempt_at"`
	LockedAt      *time.Time         `json:"lockedAt,omitempty" db:"locked_at"`
	LockedBy      *string            `json:"lockedBy,omitempty" db:"locked_by"`
	LastError     *string            `json:"lastError,omitempty" db:"last_error"`
	ProcessedAt   *time.Time         `json:"processedAt,omitempty" db:"processed_at"`
}
