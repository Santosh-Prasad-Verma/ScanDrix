package try

import (
	"time"

	"github.com/google/uuid"
)

// PrInfo encapsulates public pull request metadata.
type PrInfo struct {
	Owner           string `json:"owner"`
	Repo            string `json:"repo"`
	PRNumber        int    `json:"prNumber"`
	Title           string `json:"title"`
	State           string `json:"state,omitempty"`
	HeadSHA         string `json:"headSha"`
	BaseSHA         string `json:"baseSha"`
	Additions       int    `json:"additions"`
	Deletions       int    `json:"deletions"`
	ChangedFiles    int    `json:"changedFiles"`
	HTMLURL         string `json:"htmlUrl"`
	AuthorUsername  string `json:"authorUsername,omitempty"`
	AuthorAvatarURL string `json:"authorAvatarUrl,omitempty"`
	Body            string `json:"body,omitempty"`
}

// ReviewIssue represents an actionable bug or security defect finding.
type ReviewIssue struct {
	File           string `json:"file"`
	Line           int    `json:"line"`
	EndLine        int    `json:"endLine,omitempty"`
	Severity       string `json:"severity"`
	Category       string `json:"category,omitempty"`
	Message        string `json:"message"`
	Suggestion     string `json:"suggestion,omitempty"`
	Recommendation string `json:"recommendation,omitempty"`
	RuleID         string `json:"ruleId,omitempty"`
}

// ReviewResult encapsulates the output of a completed review analysis.
type ReviewResult struct {
	Summary       string        `json:"summary"`
	Issues        []ReviewIssue `json:"issues"`
	FilesAnalyzed int           `json:"filesAnalyzed"`
	Duration      int64         `json:"duration"` // in milliseconds
}

// FeaturedReviewSummary represents card preview data for the marketing grid.
type FeaturedReviewSummary struct {
	Slug            string   `json:"slug"`
	Tags            []string `json:"tags"`
	Highlight       string   `json:"highlight,omitempty"`
	PRURL           string   `json:"prUrl"`
	PR              PrInfo   `json:"pr"`
	IssuesCount     int      `json:"issuesCount"`
	SortOrder       int      `json:"sortOrder,omitempty"`
	IsDemonstration bool     `json:"isDemonstration"` // true for curated showcase examples
}

// FeaturedReviewDetail represents the complete cached review snapshot.
// NOTE: Seeded entries with IsDemonstration=true are illustrative showcase examples,
// not real upstream pull request data.
type FeaturedReviewDetail struct {
	Slug            string       `json:"slug"`
	Tags            []string     `json:"tags"`
	Highlight       string       `json:"highlight,omitempty"`
	PRURL           string       `json:"prUrl"`
	PR              PrInfo       `json:"pr"`
	Diff            string       `json:"diff"`
	Result          ReviewResult `json:"result"`
	PublishedAt     time.Time    `json:"publishedAt"`
	IsDemonstration bool         `json:"isDemonstration"` // true for curated showcase examples
}

// JobStatus describes the state of a live review background job.
type JobStatus string

const (
	JobStatusPending    JobStatus = "PENDING"
	JobStatusProcessing JobStatus = "PROCESSING"
	JobStatusCompleted  JobStatus = "COMPLETED"
	JobStatusFailed     JobStatus = "FAILED"
)

// ReviewJob represents a live async review execution task.
type ReviewJob struct {
	JobID       uuid.UUID     `json:"jobId"`
	Status      JobStatus     `json:"status"`
	Fingerprint string        `json:"fingerprint"`
	PR          PrInfo        `json:"publicPr,omitempty"`
	Diff        string        `json:"publicDiff,omitempty"`
	Result      *ReviewResult `json:"result,omitempty"`
	Error       string        `json:"error,omitempty"`
	CreatedAt   time.Time     `json:"createdAt"`
	StartedAt   *time.Time    `json:"startedAt,omitempty"`
	CompletedAt *time.Time    `json:"completedAt,omitempty"`
}

// EnqueueRequest payload for POST /cli/public/review-pr
type EnqueueRequest struct {
	PRURL       string `json:"prUrl"`
	Fingerprint string `json:"fingerprint"`
}

// EnqueueResponse HTTP 202 response body
type EnqueueResponse struct {
	JobID     uuid.UUID `json:"jobId"`
	Status    JobStatus `json:"status"`
	StatusURL string    `json:"statusUrl"`
	PR        PrInfo    `json:"pr"`
	Diff      string    `json:"diff"`
}
