// Package orchestrator coordinates specialized review agents and synthesizes multi-perspective findings.
package orchestrator

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// ReviewMode configures the execution depth and step budget for review agents.
type ReviewMode string

const (
	ReviewModeFast   ReviewMode = "fast"
	ReviewModeNormal ReviewMode = "normal"
	ReviewModeDeep   ReviewMode = "deep"
)

// ReviewWarningKind classifies runtime adaptations or degradations during review.
type ReviewWarningKind string

const (
	WarningContextTruncation ReviewWarningKind = "context_truncation"
	WarningTimeout           ReviewWarningKind = "timeout"
	WarningMaxStepsExceeded  ReviewWarningKind = "max_steps_exceeded"
	WarningBatchChunked      ReviewWarningKind = "batch_chunked"
	WarningModelFallback     ReviewWarningKind = "model_fallback"
	WarningRateLimitThrottled ReviewWarningKind = "rate_limit_throttled"
	WarningCrossRepoGated    ReviewWarningKind = "cross_repo_gated"
)

// ReviewWarning captures actionable telemetry about review fidelity.
type ReviewWarning struct {
	Kind                 ReviewWarningKind `json:"kind"`
	Message              string            `json:"message"`
	AgentName            string            `json:"agent_name,omitempty"`
	ModelName            string            `json:"model_name,omitempty"`
	ContextWindowTokens  int               `json:"context_window_tokens,omitempty"`
	Timestamp            time.Time         `json:"timestamp"`
}

// ReviewOptions specifies category enablement, mode, and thresholds.
type ReviewOptions struct {
	Bug           bool                   `json:"bug"`
	Security      bool                   `json:"security"`
	Performance   bool                   `json:"performance"`
	Architecture  bool                   `json:"architecture"`
	DrixyRules    bool                   `json:"drixy_rules"`
	BusinessLogic bool                   `json:"business_logic"`
	ReviewMode    ReviewMode             `json:"review_mode"`
	MaxTokens     int                    `json:"max_tokens"`
	MaxSteps      int                    `json:"max_steps"`
	FastBudget    int                    `json:"fast_budget"`
	FailOnSeverity models.FindingSeverity `json:"fail_on_severity,omitempty"`
}

// DefaultReviewOptions returns standard production review parameters.
func DefaultReviewOptions() ReviewOptions {
	return ReviewOptions{
		Bug:           true,
		Security:      true,
		Performance:   true,
		Architecture:  true,
		DrixyRules:    true,
		BusinessLogic: true,
		ReviewMode:    ReviewModeNormal,
		MaxTokens:     64000,
		MaxSteps:      20,
		FastBudget:    4,
	}
}

// ChangedFile represents a file altered in a pull request or commit.
type ChangedFile struct {
	Filename        string `json:"filename"`
	OldFilename     string `json:"old_filename,omitempty"`
	Status          string `json:"status"` // added, modified, removed, renamed
	Additions       int    `json:"additions"`
	Deletions       int    `json:"deletions"`
	Changes         int    `json:"changes"`
	Patch           string `json:"patch"`
	Content         string `json:"content,omitempty"`
	OldContent      string `json:"old_content,omitempty"`
	Language        string `json:"language,omitempty"`
	IsBinary        bool   `json:"is_binary,omitempty"`
	IsGenerated     bool   `json:"is_generated,omitempty"`
	IsVendored      bool   `json:"is_vendored,omitempty"`
	EstimatedTokens int    `json:"estimated_tokens,omitempty"`
}

// DrixyRuleExample provides concrete correct and incorrect code snippets for a rule.
type DrixyRuleExample struct {
	IsCorrect bool   `json:"is_correct"`
	Snippet   string `json:"snippet"`
}

// LoadedRuleReference represents a referenced convention file loaded from repository or Context OS.
type LoadedRuleReference struct {
	FilePath    string `json:"file_path,omitempty"`
	Content     string `json:"content,omitempty"`
	Description string `json:"description,omitempty"`
}

// DrixyRule models a custom organizational or repository review rule.
type DrixyRule struct {
	ID                 uuid.UUID              `json:"id"`
	OrgID              uuid.UUID              `json:"org_id"`
	RepoID             *uuid.UUID             `json:"repo_id,omitempty"`
	Name               string                 `json:"name"`
	Description        string                 `json:"description"`
	Prompt             string                 `json:"prompt"`
	Severity           models.FindingSeverity `json:"severity"`
	Scope              string                 `json:"scope"` // "file" | "pull_request" | "repository"
	PathGlobs          []string               `json:"path_globs,omitempty"`
	IsActive           bool                   `json:"is_active"`
	Examples           []DrixyRuleExample     `json:"examples,omitempty"`
	SourcePath         string                 `json:"source_path,omitempty"`
	SourceAnchor       string                 `json:"source_anchor,omitempty"`
	ContextReferenceID string                 `json:"context_reference_id,omitempty"`
	CreatedAt          time.Time              `json:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at"`
}

// ReviewAgentInput provides context supplied to each specialist agent.
type ReviewAgentInput struct {
	PRNumber        int             `json:"pr_number"`
	Title           string          `json:"title"`
	Description     string          `json:"description"`
	RepositoryName  string          `json:"repository_name"`
	BaseSHA         string          `json:"base_sha"`
	HeadSHA         string          `json:"head_sha"`
	BaseBranch      string          `json:"base_branch"`
	HeadBranch      string          `json:"head_branch"`
	AuthorUsername  string          `json:"author_username"`
	ChangedFiles    []ChangedFile   `json:"changed_files"`
	ReviewOptions   ReviewOptions   `json:"review_options"`
	DrixyRules      []DrixyRule     `json:"drixy_rules,omitempty"`
	WorkspaceID     uuid.UUID       `json:"workspace_id"`
	RepositoryID    uuid.UUID       `json:"repository_id"`
	ExternalContext string          `json:"external_context,omitempty"`
	ParentWarnings  []ReviewWarning `json:"parent_warnings,omitempty"`
}

// AgentFinding represents a code issue detected by a specialist reviewer.
type AgentFinding struct {
	ID                 uuid.UUID              `json:"id"`
	RuleID             *uuid.UUID             `json:"rule_id,omitempty"`
	AgentName          string                 `json:"agent_name"`
	FilePath           string                 `json:"file_path"`
	StartLine          int                    `json:"start_line"`
	EndLine            int                    `json:"end_line"`
	Severity           models.FindingSeverity `json:"severity"`
	Confidence         string                 `json:"confidence"` // "HIGH", "MEDIUM", "LOW"
	Category           string                 `json:"category"`
	Title              string                 `json:"title"`
	Description        string                 `json:"description"`
	Remediation        string                 `json:"remediation"`
	SuggestedDiff      string                 `json:"suggested_diff,omitempty"`
	ExistingCode       string                 `json:"existing_code,omitempty"`
	ImprovedCode       string                 `json:"improved_code,omitempty"`
	OneSentenceSummary string                 `json:"one_sentence_summary,omitempty"`
	Blocking           bool                   `json:"blocking"`
	Fingerprint        string                 `json:"fingerprint"`
	DisputeStatus      string                 `json:"dispute_status,omitempty"`  // "NONE", "PARTIAL", "RESOLVED"
	ArbiterVerdict     string                 `json:"arbiter_verdict,omitempty"` // "CONFIRMED", "DOWNGRADED", "ENHANCED", "DISPUTED"
	ContributingAgents []string               `json:"contributing_agents,omitempty"`
}

// ReviewAgentOutput aggregates the findings and telemetry of a single agent execution.
type ReviewAgentOutput struct {
	AgentName           string          `json:"agent_name"`
	Category            string          `json:"category"`
	Findings            []AgentFinding  `json:"findings"`
	DiscardedBySeverity []AgentFinding  `json:"discarded_by_severity,omitempty"`
	DiscardedByVerify   []AgentFinding  `json:"discarded_by_verify,omitempty"`
	Warnings            []ReviewWarning `json:"warnings,omitempty"`
	TotalTurns          int             `json:"total_turns"`
	TokensConsumed      int             `json:"tokens_consumed"`
	DurationMs          int64           `json:"duration_ms"`
	FinishReason        string          `json:"finish_reason"` // "completed", "timeout", "max-steps", "error"
	Error               string          `json:"error,omitempty"`
}

// OrchestratorAgentFailure captures an unrecoverable failure in an agent task.
type OrchestratorAgentFailure struct {
	AgentName  string `json:"agent_name"`
	Category   string `json:"category"`
	Error      string `json:"error"`
	DurationMs int64  `json:"duration_ms"`
}

// OrchestratorAgentIncomplete captures an agent cut short by its step or time budget.
type OrchestratorAgentIncomplete struct {
	AgentName        string `json:"agent_name"`
	Category         string `json:"category"`
	FinishReason     string `json:"finish_reason"` // "timeout" | "max-steps"
	SuggestionsFound int    `json:"suggestions_found"`
	DurationMs       int64  `json:"duration_ms"`
}

// ReviewVerdict indicates the overall pull request decision.
type ReviewVerdict string

const (
	VerdictApprove        ReviewVerdict = "APPROVE"
	VerdictCommentOnly    ReviewVerdict = "COMMENT_ONLY"
	VerdictRequestChanges ReviewVerdict = "REQUEST_CHANGES"
)

// DeliberationMetadata details how findings were analyzed, pruned, and synthesized.
type DeliberationMetadata struct {
	PersonasConsulted            []string `json:"personas_consulted"`
	FindingsPreSynthesis         int      `json:"findings_pre_synthesis"`
	FindingsPostArbiter          int      `json:"findings_post_arbiter"`
	FindingsPostDedup            int      `json:"findings_post_dedup"`
	FindingsRemovedByArbiter     int      `json:"findings_removed_by_arbiter"`
	FindingsDowngraded           int      `json:"findings_downgraded"`
	FindingsEnhanced             int      `json:"findings_enhanced"`
	FindingsRemovedByQualityGate int      `json:"findings_removed_by_quality_gate"`
}

// OrchestratorOutput models the final consolidated review delivered to SCM or CI.
type OrchestratorOutput struct {
	Verdict              ReviewVerdict                 `json:"verdict"`
	Summary              string                        `json:"summary"`
	Findings             []AgentFinding                `json:"findings"`
	AgentOutputs         []ReviewAgentOutput           `json:"agent_outputs"`
	Failures             []OrchestratorAgentFailure    `json:"failures,omitempty"`
	Incomplete           []OrchestratorAgentIncomplete `json:"incomplete,omitempty"`
	Warnings             []ReviewWarning               `json:"warnings,omitempty"`
	DeliberationMetadata DeliberationMetadata          `json:"deliberation_metadata"`
	TotalDurationMs      int64                         `json:"total_duration_ms"`
	TotalTokensConsumed  int                           `json:"total_tokens_consumed"`
}

// IReviewSpecialist defines the contract implemented by each specialist reviewer.
type IReviewSpecialist interface {
	Name() string
	Category() string
	Review(ctx context.Context, input ReviewAgentInput) (*ReviewAgentOutput, error)
}
