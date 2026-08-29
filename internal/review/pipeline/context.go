package pipeline

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/codeanalysis"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/rules"
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
	FindingID      uuid.UUID `json:"finding_id"`
	FilePath       string    `json:"file_path"`
	StartLine      int       `json:"start_line"`
	EndLine        int       `json:"end_line"`
	OriginalCode   string    `json:"original_code"`
	SuggestedCode  string    `json:"suggested_code"`
	SyntaxValid    bool      `json:"syntax_valid"`
	ConfidenceScore float64   `json:"confidence_score"`
}

// SCMInlineComment formatted for GitHub/GitLab pull request inline comments.
type SCMInlineComment struct {
	FilePath  string `json:"file_path"`
	Line      int    `json:"line"`
	StartLine int    `json:"start_line,omitempty"`
	Body      string `json:"body"`
}

// PipelineContext carries shared state across all pipeline stages.
type PipelineContext struct {
	// Review Identifiers
	ReviewID      uuid.UUID          `json:"review_id"`
	WorkspaceID   uuid.UUID          `json:"workspace_id"`
	RepositoryID  uuid.UUID          `json:"repository_id"`
	RepoNamespace string             `json:"repo_namespace"`
	Provider      models.SCMProvider `json:"provider"`
	PullNumber    int                `json:"pull_number"`
	Title         string             `json:"title"`
	HeadSHA       string             `json:"head_sha"`
	BaseSHA       string             `json:"base_sha"`
	Author        string             `json:"author"`
	PRState       string             `json:"pr_state"` // open, closed, merged
	IsLocked      bool               `json:"is_locked"`
	IsDraft       bool               `json:"is_draft"`
	Description   string             `json:"description"`

	// Input Content
	RawDiff string `json:"raw_diff"`

	// Configuration & Rules
	ReviewParams dtos.ReviewParametersDTO `json:"review_params"`
	ActiveRules  []rules.RuleSpec         `json:"active_rules"`

	// Enriched Pipeline Data
	ParsedPatches   []*diff.FilePatch          `json:"parsed_patches,omitempty"`
	FilteredPatches []*diff.FilePatch          `json:"filtered_patches,omitempty"`
	EnrichedHunks   []codeanalysis.EnrichedHunk `json:"enriched_hunks,omitempty"`
	ExternalContext *ExternalIssueContext      `json:"external_context,omitempty"`

	// Findings & Suggestions
	StaticFindings []models.CodeFinding   `json:"static_findings,omitempty"`
	AgentFindings  []models.CodeFinding   `json:"agent_findings,omitempty"`
	AllFindings    []models.CodeFinding   `json:"all_findings,omitempty"`
	Suggestions    []ValidatedSuggestion  `json:"suggestions,omitempty"`

	// Output Formatting
	InlineComments []SCMInlineComment `json:"inline_comments,omitempty"`
	PRSummaryBody  string             `json:"pr_summary_body,omitempty"`
	PassedReview   bool               `json:"passed_review"`

	// Stage Execution Telemetry
	StageMetrics []StageTelemetry `json:"stage_metrics"`
	StartTime    time.Time        `json:"start_time"`
	EndTime      time.Time        `json:"end_time"`
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
