package pipeline

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/codeanalysis"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/review/domain"
	"github.com/scandrix/backend/internal/review/orchestrator"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/internal/sandbox/contracts"
	"github.com/scandrix/backend/pkg/models"
)

// PipelineStage defines an atomic processing step in code review execution.
type PipelineStage interface {
	Name() string
	Execute(ctx context.Context, pCtx *PipelineContext) error
}

// StageTelemetry tracks execution timing and status of each pipeline stage.
type StageTelemetry struct {
	StageName  string        `json:"stage_name"`
	Duration   time.Duration `json:"duration"`
	Success    bool          `json:"success"`
	ErrorMsg   string        `json:"error_msg,omitempty"`
	ItemsCount int           `json:"items_count"`
}

// ExternalIssueContext holds project management metadata (e.g. Jira/Linear).
type ExternalIssueContext struct {
	IssueKey    string `json:"issue_key"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

// ValidatedSuggestion holds a syntax-verified code fix replacement.
type ValidatedSuggestion struct {
	FindingID       uuid.UUID `json:"finding_id"`
	FilePath        string    `json:"file_path"`
	StartLine       int       `json:"start_line"`
	EndLine         int       `json:"end_line"`
	OriginalCode    string    `json:"original_code"`
	SuggestedCode   string    `json:"suggested_code"`
	SyntaxValid     bool      `json:"syntax_valid"`
	IsCommittable   bool      `json:"is_committable"`
	ConfidenceScore float64   `json:"confidence_score"`
}

// SCMInlineComment formatted for GitHub/GitLab pull request inline comments.
type SCMInlineComment struct {
	FilePath  string `json:"file_path"`
	Line      int    `json:"line"`
	StartLine int    `json:"start_line,omitempty"`
	Body      string `json:"body"`
}

// AutomationStatus represents the overall or stage-level outcome.
type AutomationStatus string

const (
	StatusSuccess      AutomationStatus = "SUCCESS"
	StatusSkipped      AutomationStatus = "SKIPPED"
	StatusPartialError AutomationStatus = "PARTIAL_ERROR"
	StatusError        AutomationStatus = "ERROR"
)

// PipelineStatusInfo conveys high-level status messages and reason codes.
type PipelineStatusInfo struct {
	Status     AutomationStatus `json:"status"`
	Message    string           `json:"message"`
	ReasonCode string           `json:"reason_code,omitempty"`
}

// CommitInfo records individual commit metadata in the pull request history.
type CommitInfo struct {
	SHA     string   `json:"sha"`
	Message string   `json:"message"`
	Author  string   `json:"author"`
	Parents []string `json:"parents,omitempty"`
}

// PreviousExecutionInfo holds state from the last successful analysis on the PR.
type PreviousExecutionInfo struct {
	ExecutionID        string `json:"execution_id"`
	LastAnalyzedCommit string `json:"last_analyzed_commit"`
	CommentID          int64  `json:"comment_id,omitempty"`
	NoteID             int64  `json:"note_id,omitempty"`
	ThreadID           string `json:"thread_id,omitempty"`
	Status             string `json:"status,omitempty"`
}

// OrphanedBaseCommitInfo tracks history rewrite or rebase events where base SHA disappeared.
type OrphanedBaseCommitInfo struct {
	PreviousSHA    string `json:"previous_sha"`
	CurrentHeadSHA string `json:"current_head_sha"`
	TotalCommits   int    `json:"total_commits"`
}

// FileChangeInfo represents a tracked file modification within a review context.
type FileChangeInfo struct {
	Filename          string   `json:"filename"`
	OldPath           string   `json:"old_path,omitempty"`
	Status            string   `json:"status"` // "added", "modified", "removed", "renamed"
	Additions         int      `json:"additions"`
	Deletions         int      `json:"deletions"`
	Patch             string   `json:"patch,omitempty"`
	PatchWithLinesStr string   `json:"patch_with_lines_str,omitempty"`
	ValidDiffLines    [][2]int `json:"valid_diff_lines,omitempty"` // [start, end] on right-side
}

// PRStatsInfo aggregates diff size across all changed files.
type PRStatsInfo struct {
	TotalAdditions    int `json:"total_additions"`
	TotalDeletions    int `json:"total_deletions"`
	TotalFiles        int `json:"total_files"`
	TotalLinesChanged int `json:"total_lines_changed"`
}

// BusinessLogicOutcomeInfo summarizes ticket/requirement validation results.
type BusinessLogicOutcomeInfo struct {
	Kind    string `json:"kind"` // "clean", "gap_found", "limitation", "skipped"
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message"`
}

// TraceDecisionInfo carries architectural decision records linked to changed files.
type TraceDecisionInfo struct {
	ID          string   `json:"id"`
	DecisionKey string   `json:"decision_key"`
	Title       string   `json:"title"`
	Summary     string   `json:"summary"`
	Rationale   string   `json:"rationale"`
	Files       []string `json:"files"`
}

// PipelineErrorInfo tracks stage or substage errors with explicit severity.
type PipelineErrorInfo struct {
	Stage     string                 `json:"stage"`
	Substage  string                 `json:"substage,omitempty"`
	ErrorMsg  string                 `json:"error_msg"`
	Severity  string                 `json:"severity"` // "critical", "partial"
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// PipelineContext carries shared state across all pipeline stages.
type PipelineContext struct {
	// Review Identifiers
	ReviewID          uuid.UUID          `json:"review_id"`
	WorkspaceID       uuid.UUID          `json:"workspace_id"`
	RepositoryID      uuid.UUID          `json:"repository_id"`
	RepoNamespace     string             `json:"repo_namespace"`
	Provider          models.SCMProvider `json:"provider"`
	PullNumber        int                `json:"pull_number"`
	Title             string             `json:"title"`
	HeadSHA           string             `json:"head_sha"`
	BaseSHA           string             `json:"base_sha"`
	Author            string             `json:"author"`
	AuthorEmail       string             `json:"author_email,omitempty"`
	PRState           string             `json:"pr_state"` // open, closed, merged
	IsLocked          bool               `json:"is_locked"`
	IsDraft           bool               `json:"is_draft"`
	Description       string             `json:"description"`
	Origin            string             `json:"origin"` // "webhook", "command", "command-force", "cli"
	TriggerCommentID  string             `json:"trigger_comment_id,omitempty"`

	// Input Content
	RawDiff string `json:"raw_diff"`

	// Sandbox Lifecycle & Lease Coordination
	SandboxHandle   contracts.SandboxInstance `json:"-"`
	SandboxLeaseID  string                    `json:"sandbox_lease_id,omitempty"`
	SandboxPlatform string                    `json:"sandbox_platform,omitempty"`
	CloneURL        string                    `json:"clone_url,omitempty"`
	AuthToken       string                    `json:"-"`
	AuthUsername    string                    `json:"-"`
	Branch          string                    `json:"branch,omitempty"`
	BaseBranch      string                    `json:"base_branch,omitempty"`
	IsCLI           bool                      `json:"is_cli,omitempty"`

	// Status & Flow Control
	StatusInfo       PipelineStatusInfo     `json:"status_info"`
	PipelineMetadata map[string]interface{} `json:"pipeline_metadata,omitempty"`
	SkipReview       bool                   `json:"skip_review"`
	SkipReason       string                 `json:"skip_reason,omitempty"`

	// Commit & History Tracking
	PrCommits          []CommitInfo            `json:"pr_commits,omitempty"`
	PrAllCommits       []CommitInfo            `json:"pr_all_commits,omitempty"`
	LastExecution      *PreviousExecutionInfo  `json:"last_execution,omitempty"`
	LastAnalyzedSHA    string                  `json:"last_analyzed_sha,omitempty"`
	OrphanedBaseCommit *OrphanedBaseCommitInfo `json:"orphaned_base_commit,omitempty"`

	// Configuration & Rules
	ReviewParams        dtos.ReviewParametersDTO    `json:"review_params"`
	ResolvedConfig      domain.CodeReviewConfig     `json:"resolved_config"`
	PullRequestMessages *domain.PullRequestMessages `json:"pull_request_messages,omitempty"`
	ActiveRules         []rules.RuleSpec            `json:"active_rules"`
	DrixyRules          []orchestrator.DrixyRule    `json:"drixy_rules,omitempty"`
	ReviewDirective     string                      `json:"review_directive,omitempty"`
	InitialCommentID    int64                       `json:"initial_comment_id,omitempty"`

	// Enriched Files & Diff Context
	ParsedPatches   []*diff.FilePatch           `json:"parsed_patches,omitempty"`
	FilteredPatches []*diff.FilePatch           `json:"filtered_patches,omitempty"`
	ChangedFiles    []FileChangeInfo            `json:"changed_files,omitempty"`
	Files           []FileChangeInfo            `json:"files,omitempty"`
	IgnoredFiles    []string                    `json:"ignored_files,omitempty"`
	PRStats         PRStatsInfo                 `json:"pr_stats"`
	EnrichedHunks   []codeanalysis.EnrichedHunk `json:"enriched_hunks,omitempty"`
	ExternalContext *ExternalIssueContext       `json:"external_context,omitempty"`
	TraceDecisions  []TraceDecisionInfo         `json:"trace_decisions,omitempty"`

	// Business Logic Outcome
	BusinessLogicResults    []domain.CodeSuggestion   `json:"business_logic_results,omitempty"`
	BusinessLogicOutcome    *BusinessLogicOutcomeInfo `json:"business_logic_outcome,omitempty"`
	BusinessLogicPrBodyHash string                    `json:"business_logic_pr_body_hash,omitempty"`

	// Findings & Suggestions
	StaticFindings        []models.CodeFinding      `json:"static_findings,omitempty"`
	AgentFindings         []models.CodeFinding      `json:"agent_findings,omitempty"`
	AllFindings           []models.CodeFinding      `json:"all_findings,omitempty"`
	SuppressedFindings    []models.CodeFinding      `json:"suppressed_findings,omitempty"`
	Suggestions           []ValidatedSuggestion     `json:"suggestions,omitempty"`
	ValidSuggestions      []domain.CodeSuggestion   `json:"valid_suggestions,omitempty"`
	DiscardedSuggestions  []domain.CodeSuggestion   `json:"discarded_suggestions,omitempty"`
	PRLevelSuggestions    []domain.CodeSuggestion   `json:"pr_level_suggestions,omitempty"`

	// Output Comments & SCM Results
	InlineComments         []SCMInlineComment         `json:"inline_comments,omitempty"`
	LineCommentResults     []domain.LineCommentResult `json:"line_comment_results,omitempty"`
	PRLevelCommentResults  []domain.LineCommentResult `json:"pr_level_comment_results,omitempty"`
	PRSummaryBody          string                     `json:"pr_summary_body,omitempty"`
	PassedReview           bool                       `json:"passed_review"`

	// Error Diagnostics & Collection
	PipelineErrors  []PipelineErrorInfo `json:"pipeline_errors,omitempty"`
	LastReviewError *ReviewErrorInfo    `json:"last_review_error,omitempty"`

	// Stage Execution Telemetry
	StageMetrics []StageTelemetry `json:"stage_metrics"`
	StartTime    time.Time        `json:"start_time"`
	EndTime      time.Time        `json:"end_time"`
}

// ReviewErrorInfo stores classified diagnostic facts about an execution failure.
type ReviewErrorInfo struct {
	Category        string    `json:"category"`
	Provider        string    `json:"provider,omitempty"`
	Model           string    `json:"model,omitempty"`
	HTTPStatus      int       `json:"http_status,omitempty"`
	FriendlyMessage string    `json:"friendly_message"`
	ProviderMessage string    `json:"provider_message,omitempty"`
	AgentName       string    `json:"agent_name,omitempty"`
	OccurredAt      time.Time `json:"occurred_at"`
}

// AddMetric records completion of a stage.
func (p *PipelineContext) AddMetric(name string, duration time.Duration, success bool, err error, items int) {
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	p.StageMetrics = append(p.StageMetrics, StageTelemetry{
		StageName:  name,
		Duration:   duration,
		Success:    success,
		ErrorMsg:   errMsg,
		ItemsCount: items,
	})
}

// AddError records a structured pipeline error.
func (p *PipelineContext) AddError(stage, substage string, err error, severity string, metadata map[string]interface{}) {
	if err == nil {
		return
	}
	if severity == "" {
		severity = "critical"
	}
	p.PipelineErrors = append(p.PipelineErrors, PipelineErrorInfo{
		Stage:    stage,
		Substage: substage,
		ErrorMsg: err.Error(),
		Severity: severity,
		Metadata: metadata,
	})
}
