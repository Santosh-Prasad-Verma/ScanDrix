package feedback

import (
	"time"

	"github.com/google/uuid"
)

// ReactionType represents developer feedback emojis on inline review comments.
type ReactionType string

const (
	ReactionThumbsUp   ReactionType = "+1"
	ReactionThumbsDown ReactionType = "-1"
	ReactionHeart      ReactionType = "heart"
	ReactionConfused   ReactionType = "confused"
	ReactionRocket     ReactionType = "rocket"
)

// ReviewFeedback captures developer sentiment on an automated code review finding.
type ReviewFeedback struct {
	FindingID    uuid.UUID              `json:"finding_id"`
	WorkspaceID  uuid.UUID              `json:"workspace_id"`
	RepositoryID uuid.UUID              `json:"repository_id"`
	PRNumber     int                    `json:"pr_number"`
	Reactions    map[ReactionType]int   `json:"reactions"`
	CommentText  string                 `json:"comment_text,omitempty"`
	IsHelpful    bool                   `json:"is_helpful"`
	ReportedFP   bool                   `json:"reported_fp"` // Marked as False Positive
	UpdatedAt    time.Time              `json:"updated_at"`
}

// ScopedMessageRule attaches custom notifications or compliance notices based on modified file paths.
type ScopedMessageRule struct {
	ID             uuid.UUID `json:"id"`
	WorkspaceID    uuid.UUID `json:"workspace_id"`
	RepositoryPath string    `json:"repository_path,omitempty"` // empty for global
	PathPattern    string    `json:"path_pattern"`              // e.g. "migrations/**", "infra/**"
	HeaderNotice   string    `json:"header_notice,omitempty"`
	FooterNotice   string    `json:"footer_notice,omitempty"`
	BlockMerge     bool      `json:"block_merge"`
}
