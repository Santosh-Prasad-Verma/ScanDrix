package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// WorkflowJobFilter criteria for queue monitoring.
type WorkflowJobFilter struct {
	PaginationQuery
	WorkspaceID    uuid.UUID  `json:"workspace_id"`
	WorkflowType   *string    `json:"workflow_type,omitempty"`
	Status         *string    `json:"status,omitempty"`
	CorrelationID  *string    `json:"correlation_id,omitempty"`
	OrganizationID *uuid.UUID `json:"organization_id,omitempty"`
	TeamID         *uuid.UUID `json:"team_id,omitempty"`
}

// OutboxRepository contract for transactional messaging.
type OutboxRepository interface {
	Insert(ctx context.Context, record *OutboxRecord) error
	FetchPendingBatch(ctx context.Context, limit int) ([]*OutboxRecord, error)
	MarkPublished(ctx context.Context, id uuid.UUID) error
	MarkFailed(ctx context.Context, id uuid.UUID, nextRetry time.Time, errMessage string) error
	PurgePublishedBefore(ctx context.Context, cutoff time.Time) (int64, error)
}

// InboxRepository contract for deduplicated message processing.
type InboxRepository interface {
	Claim(ctx context.Context, messageID, handlerType, payloadHash, workerID string, timeout time.Duration) (bool, error)
	MarkCompleted(ctx context.Context, messageID, handlerType string) error
	MarkFailed(ctx context.Context, messageID, handlerType string, errMessage string) error
	ReclaimStuck(ctx context.Context, timeout time.Duration) (int64, error)
}

// WorkflowJobRepository contract for asynchronous job scheduling.
type WorkflowJobRepository interface {
	Create(ctx context.Context, job *WorkflowJob) error
	FindByID(ctx context.Context, wsID, jobID uuid.UUID) (*WorkflowJob, error)
	ClaimNext(ctx context.Context, workerID string, supportedTypes []string) (*WorkflowJob, error)
	UpdateStatus(ctx context.Context, jobID uuid.UUID, status string, errMessage *string) error
	Query(ctx context.Context, filter WorkflowJobFilter) (*PaginatedResult[*WorkflowJob], error)
}

// MessagePayload mirrors ScanDrix MessagePayload<T> contract.
type MessagePayload[T any] struct {
	EventVersion int       `json:"event_version"`
	OccurredOn   time.Time `json:"occurred_on"`
	Payload      T         `json:"payload"`
	EventName    string    `json:"event_name"`
	MessageID    string    `json:"messageId"`
}

// BrokerConfig mirrors ScanDrix BrokerConfig.
type BrokerConfig struct {
	Exchange   string `json:"exchange"`
	RoutingKey string `json:"routingKey"`
}

// BrokerPublishOptions mirrors ScanDrix BrokerPublishOptions.
type BrokerPublishOptions struct {
	CorrelationID string         `json:"correlationId,omitempty"`
	Headers       map[string]any `json:"headers,omitempty"`
	Persistent    bool           `json:"persistent,omitempty"`
	TimeoutMs     int            `json:"timeout,omitempty"`
	Mandatory     bool           `json:"mandatory,omitempty"`
	Expiration    string         `json:"expiration,omitempty"`
	UserID        string         `json:"userId,omitempty"`
	ReplyTo       string         `json:"replyTo,omitempty"`
	MessageID     string         `json:"messageId,omitempty"`
	Timestamp     int64          `json:"timestamp,omitempty"`
	Type          string         `json:"type,omitempty"`
	AppID         string         `json:"appId,omitempty"`
}
