package models

import (
	"time"

	"github.com/google/uuid"
)

// TenantStatus represents the operational lifecycle state of an organization workspace.
type TenantStatus string

const (
	TenantStatusActive    TenantStatus = "ACTIVE"
	TenantStatusSuspended TenantStatus = "SUSPENDED"
	TenantStatusPending   TenantStatus = "PENDING"
)

// Workspace represents an enterprise organization boundary isolating code, policies, and audit data.
type Workspace struct {
	ID        uuid.UUID    `json:"id" db:"id"`
	Slug      string       `json:"slug" db:"slug"`
	Name      string       `json:"name" db:"name"`
	Status    TenantStatus `json:"status" db:"status"`
	CreatedAt time.Time    `json:"created_at" db:"created_at"`
	UpdatedAt time.Time    `json:"updated_at" db:"updated_at"`
}

// UserRole defines access permissions within a workspace.
type UserRole string

const (
	RoleOwner  UserRole = "OWNER"
	RoleAdmin  UserRole = "ADMIN"
	RoleMember UserRole = "MEMBER"
	RoleViewer UserRole = "VIEWER"
)

// AccountProfile represents an authenticated identity provisioned via SSO, OAuth, or local tokens.
type AccountProfile struct {
	ID          uuid.UUID `json:"id" db:"id"`
	WorkspaceID uuid.UUID `json:"workspace_id" db:"workspace_id"`
	Email       string    `json:"email" db:"email"`
	DisplayName string    `json:"display_name" db:"display_name"`
	Role        UserRole  `json:"role" db:"role"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// SCMProvider enumerates supported source control management platforms.
type SCMProvider string

const (
	ProviderGitHub    SCMProvider = "github"
	ProviderGitLab    SCMProvider = "gitlab"
	ProviderBitbucket SCMProvider = "bitbucket"
	ProviderAzure     SCMProvider = "azure"
	ProviderForgejo   SCMProvider = "forgejo"
)

// TrackedRepository represents a Git repository monitored for automated pull request assurance.
type TrackedRepository struct {
	ID            uuid.UUID   `json:"id" db:"id"`
	WorkspaceID   uuid.UUID   `json:"workspace_id" db:"workspace_id"`
	Provider      SCMProvider `json:"provider" db:"provider"`
	ExternalID    string      `json:"external_id" db:"external_id"`
	NamespacePath string      `json:"namespace_path" db:"namespace_path"`
	DefaultBranch string      `json:"default_branch" db:"default_branch"`
	IsActive      bool        `json:"is_active" db:"is_active"`
	CreatedAt     time.Time   `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at" db:"updated_at"`
}

// ReviewState tracks the life-cycle of a pull request code review pipeline execution.
type ReviewState string

const (
	ReviewStateReceived   ReviewState = "RECEIVED"
	ReviewStateQueued     ReviewState = "QUEUED"
	ReviewStateProcessing ReviewState = "PROCESSING"
	ReviewStateCompleted  ReviewState = "COMPLETED"
	ReviewStateFailed     ReviewState = "FAILED"
)

// PullRequestReview holds metadata and execution state for a PR review run.
type PullRequestReview struct {
	ID             uuid.UUID   `json:"id" db:"id"`
	WorkspaceID    uuid.UUID   `json:"workspace_id" db:"workspace_id"`
	RepositoryID   uuid.UUID   `json:"repository_id" db:"repository_id"`
	PullNumber     int         `json:"pull_number" db:"pull_number"`
	Title          string      `json:"title" db:"title"`
	HeadSHA        string      `json:"head_sha" db:"head_sha"`
	BaseSHA        string      `json:"base_sha" db:"base_sha"`
	AuthorUsername string      `json:"author_username" db:"author_username"`
	State          ReviewState `json:"state" db:"state"`
	FindingsCount  int         `json:"findings_count" db:"findings_count"`
	CreatedAt      time.Time   `json:"created_at" db:"created_at"`
	CompletedAt    *time.Time  `json:"completed_at,omitempty" db:"completed_at"`
}

// FindingSeverity categorizes the impact of an identified code issue.
type FindingSeverity string

const (
	SeverityCritical FindingSeverity = "CRITICAL"
	SeverityHigh     FindingSeverity = "HIGH"
	SeverityMedium   FindingSeverity = "MEDIUM"
	SeverityLow      FindingSeverity = "LOW"
	SeverityInfo     FindingSeverity = "INFO"
)

// CodeFinding represents an individual actionable recommendation generated during review.
type CodeFinding struct {
	ID          uuid.UUID       `json:"id" db:"id"`
	ReviewID    uuid.UUID       `json:"review_id" db:"review_id"`
	WorkspaceID uuid.UUID       `json:"workspace_id" db:"workspace_id"`
	FilePath    string          `json:"file_path" db:"file_path"`
	StartLine   int             `json:"start_line" db:"start_line"`
	EndLine     int             `json:"end_line" db:"end_line"`
	Severity    FindingSeverity `json:"severity" db:"severity"`
	Category    string          `json:"category" db:"category"`
	Title       string          `json:"title" db:"title"`
	Description string          `json:"description" db:"description"`
	Remediation string          `json:"remediation" db:"remediation"`
	SuggestedDiff string        `json:"suggested_diff,omitempty" db:"suggested_diff"`
	Fingerprint string          `json:"fingerprint" db:"fingerprint"`
	CreatedAt   time.Time       `json:"created_at" db:"created_at"`
}

// OutboxStatus defines processing states for the transactional outbox pattern.
type OutboxStatus string

const (
	OutboxPending   OutboxStatus = "PENDING"
	OutboxPublished OutboxStatus = "PUBLISHED"
	OutboxFailed    OutboxStatus = "FAILED"
)

// OutboxRecord holds events published asynchronously via RabbitMQ.
type OutboxRecord struct {
	ID          uuid.UUID    `json:"id" db:"id"`
	WorkspaceID uuid.UUID    `json:"workspace_id" db:"workspace_id"`
	EventType   string       `json:"event_type" db:"event_type"`
	Payload     []byte       `json:"payload" db:"payload"`
	Status      OutboxStatus `json:"status" db:"status"`
	RetryCount  int          `json:"retry_count" db:"retry_count"`
	LastError   string       `json:"last_error,omitempty" db:"last_error"`
	CreatedAt   time.Time    `json:"created_at" db:"created_at"`
	PublishedAt *time.Time   `json:"published_at,omitempty" db:"published_at"`
}
