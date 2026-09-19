package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// PullRequestReviewFilter criteria for querying review runs.
type PullRequestReviewFilter struct {
	PaginationQuery
	WorkspaceID    uuid.UUID  `json:"workspace_id"`
	RepositoryID   *uuid.UUID `json:"repository_id,omitempty"`
	State          *string    `json:"state,omitempty"`
	AuthorUsername *string    `json:"author_username,omitempty"`
	DateFrom       *time.Time `json:"date_from,omitempty"`
	DateTo         *time.Time `json:"date_to,omitempty"`
	MinFindings    *int       `json:"min_findings,omitempty"`
}

// CodeFindingFilter criteria for querying security and architecture findings.
type CodeFindingFilter struct {
	PaginationQuery
	WorkspaceID     uuid.UUID   `json:"workspace_id"`
	ReviewID        *uuid.UUID  `json:"review_id,omitempty"`
	RepositoryID    *uuid.UUID  `json:"repository_id,omitempty"`
	Category        *string     `json:"category,omitempty"`
	Severity        *string     `json:"severity,omitempty"`
	RuleID          *string     `json:"rule_id,omitempty"`
	IsResolved      *bool       `json:"is_resolved,omitempty"`
	OWASP           *string     `json:"owasp,omitempty"`
	CWE             *string     `json:"cwe,omitempty"`
	ConfidenceAbove *float32    `json:"confidence_above,omitempty"`
}

// FindingFeedbackCreateDTO records developer sentiment on an automated suggestion.
type FindingFeedbackCreateDTO struct {
	FindingID  uuid.UUID `json:"finding_id"`
	UserID     uuid.UUID `json:"user_id"`
	Reaction   string    `json:"reaction"` // "THUMBS_UP", "THUMBS_DOWN", "ACCEPT", "DISMISS"
	Comment    *string   `json:"comment,omitempty"`
	IsActioned bool      `json:"is_actioned"`
}

// PullRequestReviewRepository contract.
type PullRequestReviewRepository interface {
	FindByID(ctx context.Context, wsID, reviewID uuid.UUID) (*PullRequestReview, error)
	FindLatestByPR(ctx context.Context, wsID, repoID uuid.UUID, pullNumber int) (*PullRequestReview, error)
	Create(ctx context.Context, review *PullRequestReview) error
	Update(ctx context.Context, review *PullRequestReview) error
	Query(ctx context.Context, filter PullRequestReviewFilter) (*PaginatedResult[*PullRequestReview], error)
}

// CodeFindingRepository contract.
type CodeFindingRepository interface {
	FindByID(ctx context.Context, wsID, findingID uuid.UUID) (*CodeFinding, error)
	CreateBatch(ctx context.Context, findings []*CodeFinding) error
	Query(ctx context.Context, filter CodeFindingFilter) (*PaginatedResult[*CodeFinding], error)
	MarkResolved(ctx context.Context, wsID, findingID uuid.UUID) error
}

// FindingFeedbackRepository contract.
type FindingFeedbackRepository interface {
	Create(ctx context.Context, fb *FindingFeedback) error
	ListByFinding(ctx context.Context, wsID, findingID uuid.UUID) ([]*FindingFeedback, error)
}
