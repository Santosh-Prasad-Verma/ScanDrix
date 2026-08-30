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

// FineTuningVector stores semantic embedding vectors for rules and code suggestions.
type FineTuningVector struct {
	ID             uuid.UUID `json:"id" db:"id"`
	WorkspaceID    uuid.UUID `json:"workspace_id" db:"workspace_id"`
	RuleID         *uuid.UUID `json:"rule_id,omitempty" db:"rule_id"`
	EntityType     string    `json:"entity_type" db:"entity_type"`
	EmbeddingModel string    `json:"embedding_model" db:"embedding_model"`
	EmbeddingDim   int       `json:"embedding_dim" db:"embedding_dim"`
	ContentHash    string    `json:"content_hash" db:"content_hash"`
	RawContent     string    `json:"raw_content" db:"raw_content"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
}

// BillingSeatAllocation tracks seat consumption and feature allowances per organization.
type BillingSeatAllocation struct {
	ID             uuid.UUID  `json:"id" db:"id"`
	WorkspaceID    uuid.UUID  `json:"workspace_id" db:"workspace_id"`
	Tier           string     `json:"tier" db:"tier"`
	MaxSeats       int        `json:"max_seats" db:"max_seats"`
	AllocatedSeats int        `json:"allocated_seats" db:"allocated_seats"`
	BYOKEnabled    bool       `json:"byok_enabled" db:"byok_enabled"`
	DORAEnabled    bool       `json:"dora_enabled" db:"dora_enabled"`
	ActiveUntil    *time.Time `json:"active_until,omitempty" db:"active_until"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
}

// RepoRuleOverride configures per-repository rule adjustments and severity thresholds.
type RepoRuleOverride struct {
	ID                uuid.UUID       `json:"id" db:"id"`
	WorkspaceID       uuid.UUID       `json:"workspace_id" db:"workspace_id"`
	RepositoryID      uuid.UUID       `json:"repository_id" db:"repository_id"`
	RuleName          string          `json:"rule_name" db:"rule_name"`
	Disabled          bool            `json:"disabled" db:"disabled"`
	OverriddenSeverity FindingSeverity `json:"overridden_severity,omitempty" db:"overridden_severity"`
	CustomPathGlobs   []string        `json:"custom_path_globs,omitempty" db:"custom_path_globs"`
	CreatedAt         time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at" db:"updated_at"`
}

// Team represents a group of developers within a workspace.
type Team struct {
	ID          uuid.UUID `json:"id" db:"id"`
	WorkspaceID uuid.UUID `json:"workspace_id" db:"workspace_id"`
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description" db:"description"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// TeamMember represents a user membership inside a team.
type TeamMember struct {
	ID        uuid.UUID `json:"id" db:"id"`
	TeamID    uuid.UUID `json:"team_id" db:"team_id"`
	UserID    uuid.UUID `json:"user_id" db:"user_id"`
	Email     string    `json:"email" db:"email"`
	Role      string    `json:"role" db:"role"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// NotificationChannel represents an alert destination for security reviews.
type NotificationChannel struct {
	ID          uuid.UUID       `json:"id" db:"id"`
	WorkspaceID uuid.UUID       `json:"workspace_id" db:"workspace_id"`
	Type        string          `json:"type" db:"type"`
	Target      string          `json:"target" db:"target"`
	Severity    FindingSeverity `json:"severity" db:"severity"`
	Enabled     bool            `json:"enabled" db:"enabled"`
	CreatedAt   time.Time       `json:"created_at" db:"created_at"`
}

// IntegrationConnection represents an active connection to an SCM platform.
type IntegrationConnection struct {
	ID             uuid.UUID   `json:"id" db:"id"`
	WorkspaceID    uuid.UUID   `json:"workspace_id" db:"workspace_id"`
	Provider       SCMProvider `json:"provider" db:"provider"`
	AccountName    string      `json:"account_name" db:"account_name"`
	IsConnected    bool        `json:"is_connected" db:"is_connected"`
	AccessTokenEnc string      `json:"-" db:"access_token_enc"`
	RepoCount      int         `json:"repo_count" db:"repo_count"`
	LastSyncedAt   *time.Time  `json:"last_synced_at,omitempty" db:"last_synced_at"`
	CreatedAt      time.Time   `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at" db:"updated_at"`
}

// FindingFeedback represents developer sentiment on an identified finding.
type FindingFeedback struct {
	ID          uuid.UUID `json:"id" db:"id"`
	WorkspaceID uuid.UUID `json:"workspace_id" db:"workspace_id"`
	FindingID   uuid.UUID `json:"finding_id" db:"finding_id"`
	Sentiment   string    `json:"sentiment" db:"sentiment"`
	Comment     string    `json:"comment" db:"comment"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

// AuditLogRecord represents an immutable compliance audit record in PostgreSQL.
type AuditLogRecord struct {
	ID          uuid.UUID `json:"id" db:"id"`
	WorkspaceID uuid.UUID `json:"workspace_id" db:"workspace_id"`
	ActorID     string    `json:"actor_id" db:"actor_id"`
	ActorEmail  string    `json:"actor_email" db:"actor_email"`
	IPAddress   string    `json:"ip_address" db:"ip_address"`
	Action      string    `json:"action" db:"action"`
	TargetType  string    `json:"target_type" db:"target_type"`
	TargetID    string    `json:"target_id" db:"target_id"`
	Metadata    []byte    `json:"metadata" db:"metadata"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

// TokenUsageRecord represents token consumption for a review execution.
type TokenUsageRecord struct {
	ID               uuid.UUID  `json:"id" db:"id"`
	WorkspaceID      uuid.UUID  `json:"workspace_id" db:"workspace_id"`
	ReviewID         *uuid.UUID `json:"review_id,omitempty" db:"review_id"`
	PromptTokens     int64      `json:"prompt_tokens" db:"prompt_tokens"`
	CompletionTokens int64      `json:"completion_tokens" db:"completion_tokens"`
	CostUSD          float64    `json:"cost_usd" db:"cost_usd"`
	CreatedAt        time.Time  `json:"created_at" db:"created_at"`
}

// OrganizationLicense represents an enterprise seat license.
type OrganizationLicense struct {
	ID               uuid.UUID `json:"id" db:"id"`
	WorkspaceID      uuid.UUID `json:"workspace_id" db:"workspace_id"`
	LicenseKey       string    `json:"license_key" db:"license_key"`
	OrganizationName string    `json:"organization_name" db:"organization_name"`
	PlanTier         string    `json:"plan_tier" db:"plan_tier"`
	TotalSeats       int       `json:"total_seats" db:"total_seats"`
	AllocatedSeats   int       `json:"allocated_seats" db:"allocated_seats"`
	ExpiresAt        time.Time `json:"expires_at" db:"expires_at"`
	IsAirGapped      bool      `json:"is_air_gapped" db:"is_air_gapped"`
	FeaturesEnabled  []string  `json:"features_enabled"`
	ActivatedAt      time.Time `json:"activated_at" db:"activated_at"`
	UpdatedAt        time.Time `json:"updated_at" db:"updated_at"`
}

// BillingTransaction represents a payment or subscription transaction in PostgreSQL.
type BillingTransaction struct {
	ID          uuid.UUID `json:"id" db:"id"`
	WorkspaceID uuid.UUID `json:"workspace_id" db:"workspace_id"`
	Provider    string    `json:"provider" db:"provider"`
	OrderID     string    `json:"order_id" db:"order_id"`
	PaymentID   string    `json:"payment_id" db:"payment_id"`
	Signature   string    `json:"signature" db:"signature"`
	Amount      int64     `json:"amount" db:"amount"`
	Currency    string    `json:"currency" db:"currency"`
	PlanTier    string    `json:"plan_tier" db:"plan_tier"`
	Status      string    `json:"status" db:"status"`
	Receipt     string    `json:"receipt" db:"receipt"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// PlanConfiguration defines database-driven pricing, quotas, models and features.
type PlanConfiguration struct {
	Tier                 string    `json:"tier" db:"tier"`
	DisplayName          string    `json:"display_name" db:"display_name"`
	AmountINR            int64     `json:"amount_inr" db:"amount_inr"`
	AmountUSD            int64     `json:"amount_usd" db:"amount_usd"`
	MonthlyTokens        int64     `json:"monthly_tokens" db:"monthly_tokens"`
	BurstLimitPerMin     int64     `json:"burst_limit_per_min" db:"burst_limit_per_min"`
	MaxSeats             int       `json:"max_seats" db:"max_seats"`
	MaxRepositories      int       `json:"max_repositories" db:"max_repositories"`
	MaxConcurrentReviews int       `json:"max_concurrent_reviews" db:"max_concurrent_reviews"`
	BYOKAllowed          bool      `json:"byok_allowed" db:"byok_allowed"`
	AllocatedModels      []string  `json:"allocated_models"`
	FeaturesEnabled      []string  `json:"features_enabled"`
	CreatedAt            time.Time `json:"created_at" db:"created_at"`
	UpdatedAt            time.Time `json:"updated_at" db:"updated_at"`
}

// WorkspacePlanDetails holds real-time plan status and database-aggregated token usage.
type WorkspacePlanDetails struct {
	WorkspaceID        uuid.UUID            `json:"workspace_id"`
	PlanTier           string               `json:"plan_tier"`
	OrganizationName   string               `json:"organization_name"`
	TotalSeats         int                  `json:"total_seats"`
	AllocatedSeats     int                  `json:"allocated_seats"`
	ExpiresAt          time.Time            `json:"expires_at"`
	MonthlyTokenLimit  int64                `json:"monthly_token_limit"`
	MonthlyTokensUsed  int64                `json:"monthly_tokens_used"`
	BurstLimitPerMin   int64                `json:"burst_limit_per_min"`
	BurstTokensUsed    int64                `json:"burst_tokens_used"`
	AllocatedModels    []string             `json:"allocated_models"`
	FeaturesEnabled    []string             `json:"features_enabled"`
	BYOKAllowed        bool                 `json:"byok_allowed"`
	RecentTransactions []BillingTransaction `json:"recent_transactions,omitempty"`
}

// CockpitMetrics provides real aggregated executive security and engineering KPIs.
type CockpitMetrics struct {
	TotalReviews       int     `json:"total_reviews"`
	TotalFindings      int     `json:"total_findings"`
	CriticalFindings   int     `json:"critical_findings"`
	HighFindings       int     `json:"high_findings"`
	PassRatePercentage float64 `json:"pass_rate_percentage"`
	ActiveRepositories int     `json:"active_repositories"`
	TotalDevelopers    int     `json:"total_developers"`
}
