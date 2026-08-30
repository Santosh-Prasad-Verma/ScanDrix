package relay

import (
	"time"

	"github.com/google/uuid"
)

// MessageState captures the lifecycle stage of an outbox message.
type MessageState string

const (
	StatePending    MessageState = "PENDING"
	StateClaimed    MessageState = "CLAIMED"
	StatePublished  MessageState = "PUBLISHED"
	StateFailed     MessageState = "FAILED"
	StateDeadLetter MessageState = "DEAD_LETTER"
)

// OutboxMessage represents an event scheduled for reliable asynchronous publication.
type OutboxMessage struct {
	ID             uuid.UUID    `json:"id"`
	WorkspaceID    uuid.UUID    `json:"workspace_id"`
	Topic          string       `json:"topic"`
	Payload        []byte       `json:"payload"`
	State          MessageState `json:"state"`
	RetryCount     int          `json:"retry_count"`
	MaxRetries     int          `json:"max_retries"`
	ClaimedBy      string       `json:"claimed_by,omitempty"`
	ClaimExpiresAt *time.Time   `json:"claim_expires_at,omitempty"`
	LastError      string       `json:"last_error,omitempty"`
	CreatedAt      time.Time    `json:"created_at"`
	PublishedAt    *time.Time   `json:"published_at,omitempty"`
}

// InboxStatus captures deduplication state for incoming consumer messages.
type InboxStatus string

const (
	InboxProcessing InboxStatus = "PROCESSING"
	InboxCompleted  InboxStatus = "COMPLETED"
	InboxFailed     InboxStatus = "FAILED"
	InboxRetry      InboxStatus = "RETRY"
)

// InboxRecord enforces idempotency across asynchronous consumers.
type InboxRecord struct {
	MessageID    string      `json:"message_id"`
	ConsumerID   string      `json:"consumer_id"`
	Status       InboxStatus `json:"status"`
	AttemptCount int         `json:"attempt_count"`
	LastError    string      `json:"last_error,omitempty"`
	ProcessedAt  time.Time   `json:"processed_at"`
}

// OutboxLagStats reports queue health and outbox latency metrics.
type OutboxLagStats struct {
	PendingCount    int           `json:"pending_count"`
	ClaimedCount    int           `json:"claimed_count"`
	DeadLetterCount int           `json:"dead_letter_count"`
	OldestPending   time.Duration `json:"oldest_pending"`
}

// RelayConfig configures the polling and backoff parameters.
type RelayConfig struct {
	WorkerID      string
	BatchSize     int
	PollInterval  time.Duration
	ClaimDuration time.Duration
	MaxRetries    int
	BackoffBase   time.Duration
	BackoffMax    time.Duration
}

// DefaultRelayConfig returns battle-tested production defaults.
func DefaultRelayConfig(workerID string) RelayConfig {
	return RelayConfig{
		WorkerID:      workerID,
		BatchSize:     50,
		PollInterval:  100 * time.Millisecond,
		ClaimDuration: 30 * time.Second,
		MaxRetries:    5,
		BackoffBase:   100 * time.Millisecond,
		BackoffMax:    10 * time.Second,
	}
}
