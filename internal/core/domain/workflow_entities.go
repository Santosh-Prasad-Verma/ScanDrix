package domain

import (
	"time"

	"github.com/google/uuid"
)

// OutboxRecord models the transactional outbox pattern for guaranteed message delivery.
type OutboxRecord struct {
	TenantScopedEntity
	EventType     string     `json:"event_type" db:"event_type"`
	Payload       string     `json:"payload" db:"payload"`
	Status        string     `json:"status" db:"status"`
	RetryCount    int        `json:"retry_count" db:"retry_count"`
	MaxRetries    int        `json:"max_retries" db:"max_retries"`
	LastAttemptAt *time.Time `json:"last_attempt_at,omitempty" db:"last_attempt_at"`
	NextRetryAt   *time.Time `json:"next_retry_at,omitempty" db:"next_retry_at"`
	ErrorMessage  *string    `json:"error_message,omitempty" db:"error_message"`
	PublishedAt   *time.Time `json:"published_at,omitempty" db:"published_at"`
}

// OutboxMessageModel alias.
type OutboxMessageModel = OutboxRecord

// InboxRecord models the transactional inbox pattern for deduplication and idempotent processing.
type InboxRecord struct {
	TenantScopedEntity
	MessageID       string     `json:"message_id" db:"message_id"`
	HandlerType     string     `json:"handler_type" db:"handler_type"`
	PayloadHash     string     `json:"payload_hash" db:"payload_hash"`
	Status          string     `json:"status" db:"status"`
	ClaimedAt       *time.Time `json:"claimed_at,omitempty" db:"claimed_at"`
	ClaimedByWorker *string    `json:"claimed_by_worker,omitempty" db:"claimed_by_worker"`
	CompletedAt     *time.Time `json:"completed_at,omitempty" db:"completed_at"`
	ErrorMessage    *string    `json:"error_message,omitempty" db:"error_message"`
}

// InboxMessageModel alias.
type InboxMessageModel = InboxRecord

// WorkflowJob represents an asynchronous background job with state machine lifecycle.
type WorkflowJob struct {
	TenantScopedEntity
	CorrelationID       string     `json:"correlation_id" db:"correlation_id"`
	WorkflowType        string     `json:"workflow_type" db:"workflow_type"`
	HandlerType         string     `json:"handler_type" db:"handler_type"`
	Payload             JSONBMap   `json:"payload" db:"payload"`
	Status              string     `json:"status" db:"status"`
	Priority            int        `json:"priority" db:"priority"`
	RetryCount          int        `json:"retry_count" db:"retry_count"`
	MaxRetries          int        `json:"max_retries" db:"max_retries"`
	OrganizationID      *uuid.UUID `json:"organization_id,omitempty" db:"organization_id"`
	TeamID              *uuid.UUID `json:"team_id,omitempty" db:"team_id"`
	ErrorClassification *string    `json:"error_classification,omitempty" db:"error_classification"`
	ErrorMessage        *string    `json:"error_message,omitempty" db:"error_message"`
	LockedAt            *time.Time `json:"locked_at,omitempty" db:"locked_at"`
	LockedByWorker      *string    `json:"locked_by_worker,omitempty" db:"locked_by_worker"`
	CompletedAt         *time.Time `json:"completed_at,omitempty" db:"completed_at"`
}

// WorkflowJobModel alias.
type WorkflowJobModel = WorkflowJob
