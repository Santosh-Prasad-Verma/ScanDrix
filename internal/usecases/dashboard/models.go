package dashboard

import (
	"time"

	"github.com/google/uuid"
)

// DailyDigest aggregates today's security and code review posture for a workspace.
type DailyDigest struct {
	Date           string `json:"date"` // YYYY-MM-DD (UTC)
	ReviewedToday  int    `json:"reviewed_today"`
	NeedsAttention int    `json:"needs_attention"` // PRs with >= 1 Critical or High finding
	ErroredToday   int    `json:"errored_today"`
	AwaitingReview int    `json:"awaiting_review"`
}

// AwaitingPullRequest describes an open PR pending reviewer assurance.
type AwaitingPullRequest struct {
	PRID           uuid.UUID `json:"pr_id"`
	PRNumber       int       `json:"pr_number"`
	Title          string    `json:"title"`
	URL            string    `json:"url"`
	RepositoryName string    `json:"repository_name"`
	RepositoryID   uuid.UUID `json:"repository_id"`
	Author         string    `json:"author"`
	OpenedAt       time.Time `json:"opened_at"`
	HasBlocker     bool      `json:"has_blocker"`
}

// PullRequestFacets provides multi-dimensional count breakdown for UI filtering.
type PullRequestFacets struct {
	TotalReviewed   int            `json:"total_reviewed"`
	NeedsAttention  int            `json:"needs_attention"`
	CleanApproved   int            `json:"clean_approved"`
	Errored         int            `json:"errored"`
	SeverityCounts  map[string]int `json:"severity_counts"` // critical, high, medium, low
	TopAuthors      map[string]int `json:"top_authors"`
	RepositoryStats map[string]int `json:"repository_stats"`
}

// PullRequestRecord represents an internal review execution entry for aggregation.
type PullRequestRecord struct {
	PRID         uuid.UUID
	WorkspaceID  uuid.UUID
	RepositoryID uuid.UUID
	RepoName     string
	PRNumber     int
	Title        string
	URL          string
	Author       string
	Status       string // reviewed, errored, skipped, awaiting
	SeverityMax  string // critical, high, medium, low, clean
	FindingCount int
	OpenedAt     time.Time
	ReviewedAt   *time.Time
}
