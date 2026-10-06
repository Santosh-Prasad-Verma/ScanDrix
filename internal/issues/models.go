package issues

import (
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// IssueStatus tracks the life-cycle of a detected code vulnerability across PRs.
type IssueStatus string

const (
	StatusOpen       IssueStatus = "OPEN"
	StatusInProgress IssueStatus = "IN_PROGRESS"
	StatusResolved   IssueStatus = "RESOLVED"
	StatusDismissed  IssueStatus = "DISMISSED"
)

// ValidIssueStatus reports whether the status is a known life-cycle value.
// Callers must check this before persisting: tracked_issues.status has no CHECK
// constraint, so an unvalidated write stores a value nothing else can interpret.
func ValidIssueStatus(status IssueStatus) bool {
	switch status {
	case StatusOpen, StatusInProgress, StatusResolved, StatusDismissed:
		return true
	default:
		return false
	}
}

// TrackedIssue models an enterprise tracked code finding persistent across commits and branches.
type TrackedIssue struct {
	ID               uuid.UUID              `json:"id" db:"id"`
	WorkspaceID      uuid.UUID              `json:"workspace_id" db:"workspace_id"`
	RepositoryID     uuid.UUID              `json:"repository_id" db:"repository_id"`
	Title            string                 `json:"title" db:"title"`
	Description      string                 `json:"description" db:"description"`
	FilePath         string                 `json:"file_path" db:"file_path"`
	StartLine        int                    `json:"start_line" db:"start_line"`
	EndLine          int                    `json:"end_line" db:"end_line"`
	Severity         models.FindingSeverity `json:"severity" db:"severity"`
	Category         string                 `json:"category" db:"category"`
	Status           IssueStatus            `json:"status" db:"status"`
	OriginReviewID   uuid.UUID              `json:"origin_review_id" db:"origin_review_id"`
	Remediation      string                 `json:"remediation" db:"remediation"`
	Fingerprint      string                 `json:"fingerprint" db:"fingerprint"`
	ExternalIssueURL string                 `json:"external_issue_url,omitempty" db:"external_issue_url"`
	CreatedAt        time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at" db:"updated_at"`
	ResolvedAt       *time.Time             `json:"resolved_at,omitempty" db:"resolved_at"`
}

// IssueCreationPolicy defines rules for automatically converting code review findings into issues.
type IssueCreationPolicy struct {
	AutoCreateFromSeverity models.FindingSeverity `json:"auto_create_from_severity"`
	SyncToGitProvider      bool                   `json:"sync_to_git_provider"`
}

// DefaultIssueCreationPolicy creates persistent issues for CRITICAL and HIGH severity findings.
func DefaultIssueCreationPolicy() IssueCreationPolicy {
	return IssueCreationPolicy{
		AutoCreateFromSeverity: models.SeverityHigh,
		SyncToGitProvider:      false,
	}
}
