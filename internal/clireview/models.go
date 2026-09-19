package clireview

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
	ID             string         `json:"id"`
	OrganizationID string         `json:"organizationId,omitempty"`
	TeamID         string         `json:"teamId,omitempty"`
	SessionID      string         `json:"sessionId"`
	EventType      string         `json:"eventType"` // "prompt", "tool_use", "file_edit", "commit", "review"
	Payload        map[string]any `json:"payload"`
	CreatedAt      time.Time      `json:"createdAt"`
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
	CorrelationID   string             `json:"correlationId,omitempty"`
	OrganizationID  string             `json:"organizationId"`
	TeamID          string             `json:"teamId"`
	Input           CliReviewInput     `json:"input"`
	IsTrialMode     bool               `json:"isTrialMode"`
	UserEmail       string             `json:"userEmail,omitempty"`
	GitContext      *GitContext        `json:"gitContext,omitempty"`
	CliAuth         *CliAuthContext    `json:"cliAuth,omitempty"`
	PublicPR        *try.PrInfo        `json:"publicPr,omitempty"`
	PublicDiff      string             `json:"publicDiff,omitempty"`
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

// CliReviewsListResponse provides paginated results.
type CliReviewsListResponse struct {
	Items []CliReviewSummary `json:"items"`
	Total int                `json:"total"`
}
