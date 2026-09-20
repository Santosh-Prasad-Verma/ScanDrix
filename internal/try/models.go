package try

import (
	"time"

	"github.com/google/uuid"
)

// PrAuthor represents the pull request creator.
type PrAuthor struct {
	Login     string `json:"login"`
	AvatarURL string `json:"avatarUrl"`
	HTMLURL   string `json:"htmlUrl"`
}

// PrGrouping groups changed files logically (e.g. Core, Tests, Docs).
type PrGrouping struct {
	Title       string   `json:"title"`
	Explanation string   `json:"explanation,omitempty"`
	Files       []string `json:"files"`
}

// PrInfo encapsulates public pull request metadata.
type PrInfo struct {
	Owner           string       `json:"owner"`
	Repo            string       `json:"repo"`
	PRNumber        int          `json:"prNumber"`
	Title           string       `json:"title"`
	State           string       `json:"state,omitempty"`
	Merged          bool         `json:"merged,omitempty"`
	IsDraft         bool         `json:"isDraft,omitempty"`
	HeadSHA         string       `json:"headSha"`
	HeadRef         string       `json:"headRef,omitempty"`
	BaseSHA         string       `json:"baseSha"`
	BaseRef         string       `json:"baseRef,omitempty"`
	Additions       int          `json:"additions"`
	Deletions       int          `json:"deletions"`
	ChangedFiles    int          `json:"changedFiles"`
	HTMLURL         string       `json:"htmlUrl"`
	AuthorUsername  string       `json:"authorUsername,omitempty"`
	AuthorAvatarURL string       `json:"authorAvatarUrl,omitempty"`
	Author          *PrAuthor    `json:"author,omitempty"`
	Body            string       `json:"body,omitempty"`
	AIAnalysis      string       `json:"aiAnalysis,omitempty"`
	Groupings       []PrGrouping `json:"groupings,omitempty"`
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

// JobStatusResponse represents the HTTP status response for an async review job.
type JobStatusResponse struct {
	JobID       string        `json:"jobId"`
	Status      string        `json:"status"`
	StartedAt   *string       `json:"startedAt,omitempty"`
	CompletedAt *string       `json:"completedAt,omitempty"`
	CreatedAt   string        `json:"createdAt"`
	Error       string        `json:"error,omitempty"`
	Result      *ReviewResult `json:"result,omitempty"`
	PublicPR    *PrInfo       `json:"publicPr,omitempty"`
	PublicDiff  string        `json:"publicDiff,omitempty"`
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

// DiffLineType represents line categorization in a unified diff hunk.
type DiffLineType string

const (
	DiffLineAdd     DiffLineType = "add"
	DiffLineDel     DiffLineType = "del"
	DiffLineContext DiffLineType = "context"
	DiffLineHunk    DiffLineType = "hunk"
)

// DiffLine models an individual line in a diff hunk.
type DiffLine struct {
	Type    DiffLineType `json:"type"`
	Text    string       `json:"text"`
	NewLine *int         `json:"newLine"`
	OldLine *int         `json:"oldLine"`
}

// DiffHunk models a unified diff hunk header and lines.
type DiffHunk struct {
	Header string     `json:"header"`
	Lines  []DiffLine `json:"lines"`
}

// FileStatus represents file modification status in a PR diff.
type FileStatus string

const (
	FileStatusAdded    FileStatus = "added"
	FileStatusDeleted  FileStatus = "deleted"
	FileStatusRenamed  FileStatus = "renamed"
	FileStatusModified FileStatus = "modified"
)

// DiffFile models a single file changed in a pull request.
type DiffFile struct {
	Path      string     `json:"path"`
	OldPath   *string    `json:"oldPath,omitempty"`
	Status    FileStatus `json:"status"`
	Additions int        `json:"additions"`
	Deletions int        `json:"deletions"`
	Hunks     []DiffHunk `json:"hunks"`
}

// DiffStyle configures diff viewer layout.
type DiffStyle string

const (
	DiffStyleSplit   DiffStyle = "split"
	DiffStyleUnified DiffStyle = "unified"
)

// FileTreeMode configures file list presentation.
type FileTreeMode string

const (
	FileTreeModeTree    FileTreeMode = "tree"
	FileTreeModeGrouped FileTreeMode = "grouped"
)

// Preferences stores user viewing preferences.
type Preferences struct {
	DiffStyle         DiffStyle    `json:"diffStyle"`
	HideHighlights    bool         `json:"hideHighlights"`
	CollapseByDefault bool         `json:"collapseByDefault"`
	FileTreeHidden    bool         `json:"fileTreeHidden"`
	FileTreeMode      FileTreeMode `json:"fileTreeMode"`
}

// ReviewSnapshot encapsulates PR metadata and unified diff for caching.
type ReviewSnapshot struct {
	PR   PrInfo `json:"pr"`
	Diff string `json:"diff"`
}

