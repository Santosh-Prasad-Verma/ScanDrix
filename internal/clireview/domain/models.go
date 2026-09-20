package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/try"
)

// 1. CLI Review Domain Types

// CliReviewIssueFix specifies a proposed automated code replacement.
type CliReviewIssueFix struct {
	Range struct {
		Start int `json:"start"`
		End   int `json:"end"`
	} `json:"range"`
	Replacement string `json:"replacement"`
}

// CliReviewIssue represents a single defect, finding, or suggestion reported by the review engine.
type CliReviewIssue struct {
	File           string             `json:"file"`
	Line           int                `json:"line"`
	EndLine        *int               `json:"endLine,omitempty"`
	Severity       string             `json:"severity"`
	Category       string             `json:"category,omitempty"`
	Message        string             `json:"message"`
	Suggestion     string             `json:"suggestion,omitempty"`
	Recommendation string             `json:"recommendation,omitempty"`
	RuleID         string             `json:"ruleId,omitempty"`
	Fixable        bool               `json:"fixable"`
	Fix            *CliReviewIssueFix `json:"fix,omitempty"`
}

// CliReviewResponse is the structured response returned to the CLI or caller.
type CliReviewResponse struct {
	Summary       string           `json:"summary"`
	Issues        []CliReviewIssue `json:"issues"`
	FilesAnalyzed int              `json:"filesAnalyzed"`
	Duration      int64            `json:"duration"` // in milliseconds
}

// TrialCliReviewResponse wraps CliReviewResponse with trial rate limit metadata.
type TrialCliReviewResponse struct {
	CliReviewResponse
	RateLimit *RateLimitMetadata `json:"rateLimit,omitempty"`
}

// RateLimitMetadata provides remaining request quota.
type RateLimitMetadata struct {
	Remaining int        `json:"remaining"`
	Limit     int        `json:"limit"`
	ResetAt   *time.Time `json:"resetAt,omitempty"`
}

// CliFileInput represents an explicitly passed file with diff and content.
type CliFileInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Status  string `json:"status"` // 'added' | 'modified' | 'deleted' | 'renamed'
	Diff    string `json:"diff"`
}

// CliReviewRuleToggles configures which categories of rules to run.
type CliReviewRuleToggles struct {
	Security      bool `json:"security"`
	Performance   bool `json:"performance"`
	Style         bool `json:"style"`
	BestPractices bool `json:"bestPractices"`
}

// CliReviewConfig encapsulates flags and execution options for CLI review.
type CliReviewConfig struct {
	Severity  string                `json:"severity,omitempty"`
	Rules     *CliReviewRuleToggles `json:"rules,omitempty"`
	RulesOnly bool                  `json:"rulesOnly,omitempty"`
	Fast      bool                  `json:"fast,omitempty"`
	Focus     string                `json:"focus,omitempty"`
	Heavy     bool                  `json:"heavy,omitempty"`
	Files     []CliFileInput        `json:"files,omitempty"`
}

// CliReviewInput is the primary input payload for local CLI code review.
type CliReviewInput struct {
	Diff   string           `json:"diff"`
	Config *CliReviewConfig `json:"config,omitempty"`
}

// 2. Auth & Key Validation Types

// ValidateCliKeyInput parameters for checking team key or user JWT.
type ValidateCliKeyInput struct {
	TeamKey     string `json:"teamKey,omitempty"`
	AuthHeader  string `json:"authHeader,omitempty"`
	QueryTeamID string `json:"queryTeamId,omitempty"`
	DeviceID    string `json:"deviceId,omitempty"`
	DeviceToken string `json:"deviceToken,omitempty"`
	UserAgent   string `json:"userAgent,omitempty"`
}

// ValidateCliKeyEntity provides id/name details for team and org.
type ValidateCliKeyEntity struct {
	ID   *string `json:"id"`
	Name string  `json:"name"`
}

// ValidateCliKeyUser provides basic user identity info.
type ValidateCliKeyUser struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

// ValidateCliKeyResult contains verified tenant and user identity.
type ValidateCliKeyResult struct {
	Valid            bool                  `json:"valid"`
	TeamID           *string               `json:"teamId,omitempty"`
	OrganizationID   *string               `json:"organizationId,omitempty"`
	TeamName         string                `json:"teamName,omitempty"`
	OrganizationName string                `json:"organizationName,omitempty"`
	Team             *ValidateCliKeyEntity `json:"team,omitempty"`
	Organization     *ValidateCliKeyEntity `json:"organization,omitempty"`
	User             *ValidateCliKeyUser   `json:"user,omitempty"`
	Email            string                `json:"email,omitempty"`
	UserEmail        string                `json:"userEmail,omitempty"`
	Error            string                `json:"error,omitempty"`
	Code             string                `json:"code,omitempty"`
	Details          any                   `json:"details,omitempty"`
	DeviceToken      string                `json:"deviceToken,omitempty"`
	Data             map[string]any        `json:"data,omitempty"`
}

// 3. Session Capture, Decisions & Events

// CliSessionDecisionType classifies what category of decision was extracted.
type CliSessionDecisionType string

const (
	DecisionArchitecturalDetail CliSessionDecisionType = "architectural_decision"
	DecisionConvention          CliSessionDecisionType = "convention"
	DecisionTradeoff            CliSessionDecisionType = "tradeoff"
	DecisionImplementation      CliSessionDecisionType = "implementation_detail"
	DecisionTooling             CliSessionDecisionType = "tooling"
	DecisionOther               CliSessionDecisionType = "other"
)

// CliSessionDecisionOrigin classifies where the decision came from.
type CliSessionDecisionOrigin string

const (
	OriginHuman         CliSessionDecisionOrigin = "human"
	OriginAgent         CliSessionDecisionOrigin = "agent"
	OriginCollaborative CliSessionDecisionOrigin = "collaborative"
)

// CliSessionClassifiedDecision represents a structured decision extracted from coding history.
type CliSessionClassifiedDecision struct {
	Type                 CliSessionDecisionType   `json:"type"`
	Origin               CliSessionDecisionOrigin `json:"origin,omitempty"`
	Decision             string                   `json:"decision"`
	Rationale            string                   `json:"rationale,omitempty"`
	Confidence           float64                  `json:"confidence,omitempty"`
	Evidence             []string                 `json:"evidence,omitempty"`
	AutoPromoteCandidate bool                     `json:"autoPromoteCandidate"`
	Scope                []string                 `json:"scope,omitempty"`
	Pinned               bool                     `json:"pinned,omitempty"`
}

// CliSessionToolUse records an individual tool action in an agent session.
type CliSessionToolUse struct {
	Tool     string `json:"tool"`
	FilePath string `json:"filePath,omitempty"`
	Summary  string `json:"summary,omitempty"`
}

// CliSessionSignals holds conversational and context artifacts from a session.
type CliSessionSignals struct {
	Prompt           string              `json:"prompt,omitempty"`
	AssistantMessage string              `json:"assistantMessage,omitempty"`
	ModifiedFiles    []string            `json:"modifiedFiles,omitempty"`
	ToolUses         []CliSessionToolUse `json:"toolUses,omitempty"`
}

// CliSessionCapture represents an ingested coding session snapshot.
type CliSessionCapture struct {
	CaptureID            string                         `json:"captureId"`
	OrganizationID       string                         `json:"organizationId,omitempty"`
	TeamID               string                         `json:"teamId,omitempty"`
	Event                string                         `json:"event"` // e.g. "stop", "start", "edit"
	Summary              string                         `json:"summary,omitempty"`
	Signals              *CliSessionSignals             `json:"signals,omitempty"`
	Status               string                         `json:"status"` // "pending", "processing", "completed", "skipped", "failed"
	ClassifiedDecisions  []CliSessionClassifiedDecision `json:"classifiedDecisions,omitempty"`
	ClassificationSource string                         `json:"classificationSource,omitempty"` // "llm", "heuristic", "empty"
	ErrorMessage         string                         `json:"errorMessage,omitempty"`
	CreatedAt            time.Time                      `json:"createdAt"`
	UpdatedAt            time.Time                      `json:"updatedAt"`
}

// SessionEvent represents an individual telemetry or activity event from IDE/CLI.
type SessionEvent struct {
	ID                   string                         `json:"id"`
	UUID                 string                         `json:"uuid"`
	OrganizationID       string                         `json:"organizationId,omitempty"`
	TeamID               string                         `json:"teamId,omitempty"`
	SessionID            string                         `json:"sessionId"`
	EventType            string                         `json:"eventType"` // "session_start", "turn_start", "turn_end", "subagent_start", "session_end"
	Branch               string                         `json:"branch,omitempty"`
	EventTimestamp       time.Time                      `json:"eventTimestamp"`
	Payload              map[string]any                 `json:"payload"`
	ClassificationStatus string                         `json:"classificationStatus,omitempty"`
	Decisions            []CliSessionClassifiedDecision `json:"decisions,omitempty"`
	ClassificationSource string                         `json:"classificationSource,omitempty"`
	ClassificationError  string                         `json:"classificationError,omitempty"`
	ClassifiedAt         *time.Time                     `json:"classifiedAt,omitempty"`
	CreatedAt            time.Time                      `json:"createdAt"`
	UpdatedAt            time.Time                      `json:"updatedAt"`
}

// 4. Trace Context Types

// TraceDecisionBranch represents an active decision node in a development session.
type TraceDecisionBranch struct {
	ID        string    `json:"id"`
	Branch    string    `json:"branch"`
	Decision  string    `json:"decision"`
	Type      string    `json:"type"`
	Rationale string    `json:"rationale,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// TraceContextPack bundles captured decisions and context to enrich PR reviews.
type TraceContextPack struct {
	SessionID string                         `json:"sessionId"`
	Decisions []CliSessionClassifiedDecision `json:"decisions"`
	Branches  []TraceDecisionBranch          `json:"branches"`
	Summary   string                         `json:"summary"`
	CreatedAt time.Time                      `json:"createdAt"`
}

// 5. Job & Public Review DTOs

// GitContext carries remote repo metadata for checkout in sandboxes.
type GitContext struct {
	Remote           string `json:"remote,omitempty"`
	Branch           string `json:"branch,omitempty"`
	CommitSHA        string `json:"commitSha,omitempty"`
	MergeBaseSHA     string `json:"mergeBaseSha,omitempty"`
	GitHubPAT        string `json:"githubPat,omitempty"`
	InferredPlatform string `json:"inferredPlatform,omitempty"`
	CLIVersion       string `json:"cliVersion,omitempty"`
}

// CliAuthContext holds authenticated caller metadata.
type CliAuthContext struct {
	Mode        string `json:"mode"` // "team-key" | "personal"
	TeamKeyID   string `json:"teamKeyId,omitempty"`
	TeamKeyName string `json:"teamKeyName,omitempty"`
	UserID      string `json:"userId,omitempty"`
	UserEmail   string `json:"userEmail,omitempty"`
}

// EnqueueCliReviewInput parameters for queuing async CLI review.
type EnqueueCliReviewInput struct {
	CorrelationID  string          `json:"correlationId,omitempty"`
	OrganizationID string          `json:"organizationId"`
	TeamID         string          `json:"teamId"`
	Input          CliReviewInput  `json:"input"`
	IsTrialMode    bool            `json:"isTrialMode"`
	UserEmail      string          `json:"userEmail,omitempty"`
	GitContext     *GitContext     `json:"gitContext,omitempty"`
	CliAuth        *CliAuthContext `json:"cliAuth,omitempty"`
	PublicPR       *try.PrInfo     `json:"publicPr,omitempty"`
	PublicDiff     string          `json:"publicDiff,omitempty"`
}

// EnqueueCliReviewResult response from enqueuing an async review.
type EnqueueCliReviewResult struct {
	JobID         uuid.UUID `json:"jobId"`
	CorrelationID string    `json:"correlationId"`
}

// 6. Dashboard Query Types

// CliReviewsQuery specifies search filters for reviewing CLI review history.
type CliReviewsQuery struct {
	OrganizationID string     `json:"organizationId"`
	TeamID         string     `json:"teamId,omitempty"`
	StartDate      *time.Time `json:"startDate,omitempty"`
	EndDate        *time.Time `json:"endDate,omitempty"`
	Limit          int        `json:"limit,omitempty"`
	Offset         int        `json:"offset,omitempty"`
	Search         string     `json:"search,omitempty"`
}

// CliReviewSummary is the dashboard representation of a historical CLI review.
type CliReviewSummary struct {
	ID            string    `json:"id"`
	CorrelationID string    `json:"correlationId"`
	Summary       string    `json:"summary"`
	IssuesCount   int       `json:"issuesCount"`
	FilesAnalyzed int       `json:"filesAnalyzed"`
	Duration      int64     `json:"duration"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"createdAt"`
	UserEmail     string    `json:"userEmail,omitempty"`
	Branch        string    `json:"branch,omitempty"`
}

// 7. Trace Decision & Public PR Models

// TraceContextDecision is a decision as it appears in the review context pack.
type TraceContextDecision struct {
	CliSessionClassifiedDecision
	Scope      []string `json:"scope,omitempty"`
	Pinned     bool     `json:"pinned,omitempty"`
	Branch     string   `json:"branch,omitempty"`
	SessionID  string   `json:"sessionId,omitempty"`
	RecordedAt string   `json:"recordedAt,omitempty"`
}

// TraceContextPackTokenBudget caps decision history to avoid crowding out diffs (2000 tokens).
const TraceContextPackTokenBudget = 2000

// EstimateTokens provides a rough token estimate (1 token ~ 4 characters).
func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}
	return (len(text) + 3) / 4
}

// TraceDecisionBranchRecord represents a versioned branch record of captured decisions.
type TraceDecisionBranchRecord struct {
	Version   int                    `json:"version"`
	Branch    string                 `json:"branch"`
	Decisions []TraceContextDecision `json:"decisions"`
}

// PublicPrAuthor represents the PR author on GitHub.
type PublicPrAuthor struct {
	Login     string `json:"login"`
	AvatarURL string `json:"avatarUrl,omitempty"`
	HTMLURL   string `json:"htmlUrl,omitempty"`
}

// PublicPrLabel represents a GitHub PR label.
type PublicPrLabel struct {
	Name        string `json:"name"`
	Color       string `json:"color,omitempty"`
	Description string `json:"description,omitempty"`
}

// PublicPrAssignee represents an assigned GitHub user.
type PublicPrAssignee struct {
	Login     string `json:"login"`
	AvatarURL string `json:"avatarUrl,omitempty"`
	HTMLURL   string `json:"htmlUrl,omitempty"`
}

// PublicPrReviewer represents a PR reviewer and their state.
type PublicPrReviewer struct {
	Login     string `json:"login"`
	AvatarURL string `json:"avatarUrl,omitempty"`
	State     string `json:"state"` // 'approved' | 'changes_requested' | 'commented' | 'pending'
}

// PublicPrCheckSummary aggregates PR CI check run conclusions.
type PublicPrCheckSummary struct {
	Total      int    `json:"total"`
	Passed     int    `json:"passed"`
	Failed     int    `json:"failed"`
	Pending    int    `json:"pending"`
	Conclusion string `json:"conclusion"` // 'success' | 'failure' | 'partial' | 'pending' | 'unknown'
}

// PublicPrCommit represents a single commit in a pull request.
type PublicPrCommit struct {
	Sha             string `json:"sha"`
	Message         string `json:"message"`
	AuthorLogin     string `json:"authorLogin,omitempty"`
	AuthorAvatarURL string `json:"authorAvatarUrl,omitempty"`
	AuthoredAt      string `json:"authoredAt,omitempty"`
	HTMLURL         string `json:"htmlUrl"`
}

// PublicPrComment represents an issue or review comment on a pull request.
type PublicPrComment struct {
	ID              int64  `json:"id"`
	AuthorLogin     string `json:"authorLogin,omitempty"`
	AuthorAvatarURL string `json:"authorAvatarUrl,omitempty"`
	Body            string `json:"body"`
	CreatedAt       string `json:"createdAt"`
	HTMLURL         string `json:"htmlUrl"`
	Kind            string `json:"kind"` // 'issue' | 'review'
	Path            string `json:"path,omitempty"`
	Line            int    `json:"line,omitempty"`
}

// PublicPrMetadata encapsulates complete pull request metadata and diff.
type PublicPrMetadata struct {
	Owner           string                `json:"owner"`
	Repo            string                `json:"repo"`
	PRNumber        int                   `json:"prNumber"`
	Title           string                `json:"title"`
	State           string                `json:"state"` // 'open' | 'closed'
	Merged          bool                  `json:"merged"`
	IsDraft         bool                  `json:"isDraft"`
	HeadSha         string                `json:"headSha"`
	HeadRef         string                `json:"headRef"`
	BaseSha         string                `json:"baseSha"`
	BaseRef         string                `json:"baseRef"`
	Additions       int                   `json:"additions"`
	Deletions       int                   `json:"deletions"`
	ChangedFiles    int                   `json:"changedFiles"`
	CommitsCount    int                   `json:"commitsCount"`
	DiscussionCount int                   `json:"discussionCount"`
	HTMLURL         string                `json:"htmlUrl"`
	CloneURL        string                `json:"cloneUrl"`
	Diff            string                `json:"diff"`
	Author          *PublicPrAuthor       `json:"author,omitempty"`
	Reviewers       []PublicPrReviewer    `json:"reviewers"`
	Checks          *PublicPrCheckSummary `json:"checks,omitempty"`
	Commits         []PublicPrCommit      `json:"commits"`
	Comments        []PublicPrComment     `json:"comments"`
	Labels          []PublicPrLabel       `json:"labels"`
	Assignees       []PublicPrAssignee    `json:"assignees"`
	Body            string                `json:"body,omitempty"`
}
// ParsedPrURL breaks down a GitHub pull request URL.
type ParsedPrURL struct {
	Owner    string `json:"owner"`
	Repo     string `json:"repo"`
	PRNumber int    `json:"prNumber"`
}

// PublicPrFetchError represents typed errors from fetching public PRs.
type PublicPrFetchError struct {
	Message    string `json:"message"`
	Code       string `json:"code"`
	StatusCode int    `json:"statusCode"`
}

func (e *PublicPrFetchError) Error() string {
	return e.Message
}

// PublicPrReviewResult holds the outcome of a public PR review request.
type PublicPrReviewResult struct {
	OK         bool                   `json:"ok"`
	Response   *PublicPrReviewPayload `json:"response,omitempty"`
	Code       string                 `json:"code,omitempty"`
	Message    string                 `json:"message,omitempty"`
	StatusCode int                    `json:"statusCode,omitempty"`
	RateLimit  *RateLimitMetadata     `json:"rateLimit,omitempty"`
}

// PublicPrReviewPayload wraps the queued review details.
type PublicPrReviewPayload struct {
	JobID     string             `json:"jobId"`
	Status    string             `json:"status"`
	StatusURL string             `json:"statusUrl"`
	PR        any                `json:"pr"`
	Diff      string             `json:"diff"`
	RateLimit *RateLimitMetadata `json:"rateLimit,omitempty"`
}

// FeaturedPublicReview represents a curated review showcase record.
type FeaturedPublicReview struct {
	Slug        string         `json:"slug"`
	Published   bool           `json:"published"`
	SortOrder   *int           `json:"sortOrder,omitempty"`
	Tags        []string       `json:"tags"`
	Highlight   string         `json:"highlight,omitempty"`
	PrURL       string         `json:"prUrl"`
	PR          map[string]any `json:"pr"`
	Diff        string         `json:"diff"`
	Result      map[string]any `json:"result"`
	SourceJobID string         `json:"sourceJobId,omitempty"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
}

// FeaturedPublicReviewListItem provides lightweight representation for home grids.
type FeaturedPublicReviewListItem struct {
	Slug        string         `json:"slug"`
	Tags        []string       `json:"tags"`
	Highlight   string         `json:"highlight,omitempty"`
	PrURL       string         `json:"prUrl"`
	PR          map[string]any `json:"pr"`
	SortOrder   *int           `json:"sortOrder,omitempty"`
	IssuesCount int            `json:"issuesCount"`
}

// CliReviewsListResponse provides paginated results.
type CliReviewsListResponse struct {
	Items []CliReviewSummary `json:"items"`
	Total int                `json:"total"`
}

// PublicPrGrouping represents an intent-based group of changed files.
type PublicPrGrouping struct {
	Title       string   `json:"title"`
	Explanation string   `json:"explanation"`
	Files       []string `json:"files"`
}

// ExecutionAuthContext holds credentials for review execution.
type ExecutionAuthContext struct {
	Mode        string `json:"mode"` // "team-key" | "personal"
	TeamKeyID   string `json:"teamKeyId,omitempty"`
	TeamKeyName string `json:"teamKeyName,omitempty"`
	UserID      string `json:"userId,omitempty"`
	UserEmail   string `json:"userEmail,omitempty"`
}

// ExecuteCliReviewInput encapsulates input parameters for the CLI review execution.
type ExecuteCliReviewInput struct {
	OrganizationAndTeamData OrganizationAndTeamData `json:"organizationAndTeamData"`
	Input                   CliReviewInput          `json:"input"`
	IsTrialMode             bool                    `json:"isTrialMode"`
	UserEmail               string                  `json:"userEmail,omitempty"`
	GitContext              *GitContext             `json:"gitContext,omitempty"`
	CliAuth                 *ExecutionAuthContext   `json:"cliAuth,omitempty"`
}

// OrganizationAndTeamData holds the tenancy identifiers for the caller.
type OrganizationAndTeamData struct {
	OrganizationID string `json:"organizationId"`
	TeamID         string `json:"teamId"`
}

// CodeReviewConfig embodies configuration parameters for reviewing code changes.
type CodeReviewConfig struct {
	CodeReviewVersion          string         `json:"codeReviewVersion"`
	AutomatedReviewActive      bool           `json:"automatedReviewActive"`
	PullRequestApprovalActive  bool           `json:"pullRequestApprovalActive"`
	LanguageResultPrompt       string         `json:"languageResultPrompt"`
	ReviewMode                 string         `json:"reviewMode,omitempty"`
	ReviewOptions              ReviewOptions  `json:"reviewOptions"`
	Rules                      []DrixyRule    `json:"rules,omitempty"`
	MemoryRules                []DrixyRule    `json:"memoryRules,omitempty"`
	AdditionalCustomParameters map[string]any `json:"additionalCustomParameters,omitempty"`
}

// ReviewOptions sets categories evaluated during review passes.
type ReviewOptions struct {
	Bug         bool `json:"bug"`
	Performance bool `json:"performance"`
	Security    bool `json:"security"`
	CrossFile   bool `json:"cross_file"`
}

// DrixyRule represents an organizational or repository-specific coding rule.
type DrixyRule struct {
	ID           string   `json:"id"`
	Rule         string   `json:"rule"`
	Title        string   `json:"title,omitempty"`
	Category     string   `json:"category,omitempty"`
	Severity     string   `json:"severity,omitempty"`
	Scope        []string `json:"scope,omitempty"`
	RepositoryID string   `json:"repositoryId,omitempty"`
	Type         string   `json:"type,omitempty"` // "standard" | "memory"
	Active       bool     `json:"active"`
	Locked       bool     `json:"locked,omitempty"`
}

// RepositoryRef represents an external code repository registered in ScanDrix.
type RepositoryRef struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	HTTPURL  string `json:"http_url,omitempty"`
	CloneURL string `json:"clone_url,omitempty"`
}

// AutomationExecutionRecord tracks execution telemetry in database.
type AutomationExecutionRecord struct {
	UUID           string         `json:"uuid"`
	Status         string         `json:"status"` // "in_progress", "success", "error"
	Origin         string         `json:"origin"` // "cli"
	OrganizationID string         `json:"organizationId"`
	TeamID         string         `json:"teamId"`
	RepositoryID   string         `json:"repositoryId,omitempty"`
	DataExecution  map[string]any `json:"dataExecution"`
	ErrorMessage   string         `json:"errorMessage,omitempty"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
}

// FileChange represents an individual file changed in a review batch.
type FileChange struct {
	Filename          string `json:"filename"`
	Patch             string `json:"patch"`
	PatchWithLinesStr string `json:"patchWithLinesStr"`
	Status            string `json:"status"` // 'added' | 'modified' | 'deleted' | 'renamed'
	Additions         int    `json:"additions"`
	Deletions         int    `json:"deletions"`
	Changes           int    `json:"changes"`
	SHA               string `json:"sha"`
	Content           string `json:"content,omitempty"`
}

// CodeSuggestion represents an automated code improvement or finding from review pipeline.
type CodeSuggestion struct {
	RuleID         string `json:"ruleId,omitempty"`
	File           string `json:"file"`
	Line           int    `json:"line"`
	EndLine        *int   `json:"endLine,omitempty"`
	Severity       string `json:"severity"`
	Category       string `json:"category,omitempty"`
	Title          string `json:"title,omitempty"`
	Message        string `json:"message"`
	Suggestion     string `json:"suggestion,omitempty"`
	Recommendation string `json:"recommendation,omitempty"`
	Fixable        bool   `json:"fixable"`
	Replacement    string `json:"replacement,omitempty"`
	StartIndex     *int   `json:"startIndex,omitempty"`
	EndIndex       *int   `json:"endIndex,omitempty"`
	Confidence     string `json:"confidence,omitempty"`
	Type           string `json:"type,omitempty"`
}


