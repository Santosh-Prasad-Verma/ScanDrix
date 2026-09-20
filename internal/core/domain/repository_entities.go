package domain

import (
	"time"

	"github.com/google/uuid"
)

// TrackedRepository models a connected Git repository being audited by ScanDrix.
type TrackedRepository struct {
	TenantScopedEntity
	OrganizationID  uuid.UUID  `json:"organization_id" db:"organization_id"`
	SCMProvider     string     `json:"scm_provider" db:"scm_provider"`
	ExternalRepoID  string     `json:"external_repo_id" db:"external_repo_id"`
	FullName        string     `json:"full_name" db:"full_name"`
	DefaultBranch   string     `json:"default_branch" db:"default_branch"`
	IsPrivate       bool       `json:"is_private" db:"is_private"`
	IsActive        bool       `json:"is_active" db:"is_active"`
	WebhooksEnabled bool       `json:"webhooks_enabled" db:"webhooks_enabled"`
	LastScannedAt   *time.Time `json:"last_scanned_at,omitempty" db:"last_scanned_at"`
	ConfigFilePath  string     `json:"config_file_path" db:"config_file_path"`
	CustomPrompt    *string    `json:"custom_prompt,omitempty" db:"custom_prompt"`
	Settings        JSONBMap   `json:"settings" db:"settings"`
}

// RepositoryModel alias.
type RepositoryModel = TrackedRepository

// PullRequest tracks an audited pull/merge request lifecycle.
type PullRequest struct {
	TenantScopedEntity
	RepositoryID      uuid.UUID  `json:"repository_id" db:"repository_id"`
	PullNumber        int        `json:"pull_number" db:"pull_number"`
	Title             string     `json:"title" db:"title"`
	AuthorUsername    string     `json:"author_username" db:"author_username"`
	AuthorAvatarURL   *string    `json:"author_avatar_url,omitempty" db:"author_avatar_url"`
	SourceBranch      string     `json:"source_branch" db:"source_branch"`
	TargetBranch      string     `json:"target_branch" db:"target_branch"`
	HeadSHA           string     `json:"head_sha" db:"head_sha"`
	BaseSHA           string     `json:"base_sha" db:"base_sha"`
	State             string     `json:"state" db:"state"`
	IsDraft           bool       `json:"is_draft" db:"is_draft"`
	ChangedFilesCount int        `json:"changed_files_count" db:"changed_files_count"`
	Additions         int        `json:"additions" db:"additions"`
	Deletions         int        `json:"deletions" db:"deletions"`
	ReviewRequestedAt *time.Time `json:"review_requested_at,omitempty" db:"review_requested_at"`
	MergedAt          *time.Time `json:"merged_at,omitempty" db:"merged_at"`
	ClosedAt          *time.Time `json:"closed_at,omitempty" db:"closed_at"`
}

// PullRequestsModel alias.
type PullRequestsModel = PullRequest

// PullRequestReview represents an individual automated review execution run.
type PullRequestReview struct {
	TenantScopedEntity
	RepositoryID     uuid.UUID  `json:"repository_id" db:"repository_id"`
	PullNumber       int        `json:"pull_number" db:"pull_number"`
	Title            string     `json:"title" db:"title"`
	HeadSHA          string     `json:"head_sha" db:"head_sha"`
	BaseSHA          string     `json:"base_sha" db:"base_sha"`
	AuthorUsername   string     `json:"author_username" db:"author_username"`
	State            string     `json:"state" db:"state"`
	FindingsCount    int        `json:"findings_count" db:"findings_count"`
	DurationMs       int64      `json:"duration_ms" db:"duration_ms"`
	PromptTokens     int        `json:"prompt_tokens" db:"prompt_tokens"`
	CompletionTokens int        `json:"completion_tokens" db:"completion_tokens"`
	ModelID          string     `json:"model_id" db:"model_id"`
	EstimatedCostUSD float64    `json:"estimated_cost_usd" db:"estimated_cost_usd"`
	CheckRunID       *string    `json:"check_run_id,omitempty" db:"check_run_id"`
	SummaryMarkdown  *string    `json:"summary_markdown,omitempty" db:"summary_markdown"`
	FailureReason    *string    `json:"failure_reason,omitempty" db:"failure_reason"`
}

// CodeFinding represents an individual security, performance, or architecture vulnerability identified in code.
type CodeFinding struct {
	TenantScopedEntity
	ReviewID        uuid.UUID `json:"review_id" db:"review_id"`
	RepositoryID    uuid.UUID `json:"repository_id" db:"repository_id"`
	RuleID          string    `json:"rule_id" db:"rule_id"`
	Category        string    `json:"category" db:"category"`
	Severity        string    `json:"severity" db:"severity"`
	FilePath        string    `json:"file_path" db:"file_path"`
	LineStart       int       `json:"line_start" db:"line_start"`
	LineEnd         int       `json:"line_end" db:"line_end"`
	Message         string    `json:"message" db:"message"`
	CodeSnippet     *string   `json:"code_snippet,omitempty" db:"code_snippet"`
	SuggestedFix    *string   `json:"suggested_fix,omitempty" db:"suggested_fix"`
	DiffHunk        *string   `json:"diff_hunk,omitempty" db:"diff_hunk"`
	CWE             *string   `json:"cwe,omitempty" db:"cwe"`
	OWASP           *string   `json:"owasp,omitempty" db:"owasp"`
	ConfidenceScore float32   `json:"confidence_score" db:"confidence_score"`
	IsResolved      bool      `json:"is_resolved" db:"is_resolved"`
	CommentID       *string   `json:"comment_id,omitempty" db:"comment_id"`
}

// FindingFeedback records engineer reactions (thumbs-up/thumbs-down) to train the fine-tuning feedback loop.
type FindingFeedback struct {
	TenantScopedEntity
	FindingID  uuid.UUID `json:"finding_id" db:"finding_id"`
	UserID     uuid.UUID `json:"user_id" db:"user_id"`
	Reaction   string    `json:"reaction" db:"reaction"`
	Comment    *string   `json:"comment,omitempty" db:"comment"`
	IsActioned bool      `json:"is_actioned" db:"is_actioned"`
}

// SuggestionEmbedding stores pgvector semantic embeddings for past suggestions to detect duplicates and fine-tune rules.
type SuggestionEmbedding struct {
	TenantScopedEntity
	RepositoryID uuid.UUID   `json:"repository_id" db:"repository_id"`
	FindingID    *uuid.UUID  `json:"finding_id,omitempty" db:"finding_id"`
	ContentHash  string      `json:"content_hash" db:"content_hash"`
	TextContent  string      `json:"text_content" db:"text_content"`
	Embedding    FloatVector `json:"embedding" db:"embedding"`
	Metadata     JSONBMap    `json:"metadata" db:"metadata"`
}

// SuggestionEmbeddedModel alias.
type SuggestionEmbeddedModel = SuggestionEmbedding
