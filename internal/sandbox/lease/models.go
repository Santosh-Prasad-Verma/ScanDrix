package lease

import (
	"time"

	"github.com/google/uuid"
)

// LeaseState represents the current state of a sandbox lease document.
type LeaseState string

const (
	StateCreating    LeaseState = "CREATING"
	StateReady       LeaseState = "READY"
	StatePaused      LeaseState = "PAUSED"
	StateInvalidated LeaseState = "INVALIDATED"
)

// CleanupStatus tracks the lifecycle of filesystem / microVM cleanup for local or orphaned sandboxes.
type CleanupStatus string

const (
	CleanupPending    CleanupStatus = "pending"
	CleanupInProgress CleanupStatus = "in_progress"
	CleanupCompleted  CleanupStatus = "completed"
	CleanupFailed     CleanupStatus = "failed"
)

// SandboxLease represents the active ephemeral sandbox lease entity in Go.
type SandboxLease struct {
	PrKey            string        `json:"pr_key"`
	SandboxID        string        `json:"sandbox_id,omitempty"`
	OrganizationID   *uuid.UUID    `json:"organization_id,omitempty"`
	RepositoryID     string        `json:"repository_id,omitempty"`
	PRNumber         string        `json:"pr_number,omitempty"`
	Consumer         string        `json:"consumer,omitempty"`
	State            LeaseState    `json:"state"`
	LeaseCount       int           `json:"lease_count"`
	CreatedAt        time.Time     `json:"created_at"`
	ExpiresAt        time.Time     `json:"expires_at"`
	KillAt           *time.Time    `json:"kill_at,omitempty"`
	CleanupStatus    CleanupStatus `json:"cleanup_status,omitempty"`
	CleanupAttempts  int           `json:"cleanup_attempts"`
	CleanupRetryAt   *time.Time    `json:"cleanup_retry_at,omitempty"`
	CleanupError     string        `json:"cleanup_error,omitempty"`
	CleanupStartedAt *time.Time    `json:"cleanup_started_at,omitempty"`
}
