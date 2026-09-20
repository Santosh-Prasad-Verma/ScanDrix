package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// FeedbackReactions captures developer sentiment counts on inline review comments.
type FeedbackReactions struct {
	ThumbsUp   int `json:"thumbsUp"`
	ThumbsDown int `json:"thumbsDown"`
}

// CommentLocationRef identifies the target inline comment on GitHub/GitLab.
type CommentLocationRef struct {
	ID                  int64  `json:"id"`
	PullRequestReviewID string `json:"pullRequestReviewId,omitempty"`
}

// PRRepositoryRef identifies the repository associated with a pull request.
type PRRepositoryRef struct {
	ID       string `json:"id"`
	FullName string `json:"fullName"`
}

// PRContextRef holds minimal pull request metadata for feedback association.
type PRContextRef struct {
	ID         string          `json:"id"`
	Number     int             `json:"number"`
	Repository PRRepositoryRef `json:"repository"`
}

// CodeReviewFeedback tracks developer interaction and acceptance rates of suggestions.
type CodeReviewFeedback struct {
	ID                         uuid.UUID          `json:"id"`
	OrganizationID             string             `json:"organizationId"`
	SuggestionID               string             `json:"suggestionId"`
	RuleID                     string             `json:"ruleId,omitempty"`
	Category                   ReviewCategory     `json:"category,omitempty"`
	Reactions                  FeedbackReactions  `json:"reactions"`
	Comment                    CommentLocationRef `json:"comment"`
	PullRequest                PRContextRef       `json:"pullRequest"`
	SyncedEmbeddedSuggestions  bool               `json:"syncedEmbeddedSuggestions"`
	DeveloperFeedbackNote      string             `json:"developerFeedbackNote,omitempty"`
	CreatedAt                  time.Time          `json:"createdAt"`
	UpdatedAt                  time.Time          `json:"updatedAt"`
}

// FeedbackAggregateMetrics summarizes developer sentiment and accuracy for rules and models.
type FeedbackAggregateMetrics struct {
	TotalSuggestions ReviewedCount `json:"totalSuggestions"`
	PositiveFeedback int           `json:"positiveFeedback"`
	NegativeFeedback int           `json:"negativeFeedback"`
	HelpfulnessRatio float64       `json:"helpfulnessRatio"`
	DisputedCount    int           `json:"disputedCount"`
}

// ReviewedCount breakdown of review suggestions.
type ReviewedCount struct {
	Total      int `json:"total"`
	Accepted   int `json:"accepted"`
	Overridden int `json:"overridden"`
}

// FeedbackFilter criteria for querying stored feedback.
type FeedbackFilter struct {
	OrganizationID string
	RepositoryID   string
	SuggestionID   string
	CommentID      int64
	PRNumber       int
}

// ICodeReviewFeedbackRepository defines the persistence layer for developer feedback.
type ICodeReviewFeedbackRepository interface {
	Save(ctx context.Context, feedback *CodeReviewFeedback) error
	FindBySuggestionID(ctx context.Context, orgID, suggestionID string) (*CodeReviewFeedback, error)
	FindByCommentID(ctx context.Context, orgID string, commentID int64) (*CodeReviewFeedback, error)
	ListByPR(ctx context.Context, orgID, repoID string, prNumber int) ([]CodeReviewFeedback, error)
	GetMetricsByOrg(ctx context.Context, orgID string) (*FeedbackAggregateMetrics, error)
}

// ICodeReviewFeedbackService defines application logic for processing SCM reactions.
type ICodeReviewFeedbackService interface {
	RecordReaction(ctx context.Context, orgID string, commentID int64, prNumber int, repo PRRepositoryRef, reaction string) error
	GetFeedbackSummary(ctx context.Context, orgID, repoID string) (*FeedbackAggregateMetrics, error)
}
