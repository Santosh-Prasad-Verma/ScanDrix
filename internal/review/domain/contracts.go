package domain

import (
	"context"

	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/pkg/models"
)

// LineCommentRequest parameters for posting inline comments to SCM platforms.
type LineCommentRequest struct {
	FilePath            string         `json:"filePath"`
	LineNumber          int            `json:"lineNumber"`
	StartLineNumber     int            `json:"startLineNumber,omitempty"`
	Side                string         `json:"side"` // "RIGHT", "LEFT"
	Body                string         `json:"body"`
	Suggestion          *CodeSuggestion `json:"suggestion,omitempty"`
	SuggestionCopyPrompt bool           `json:"suggestionCopyPrompt"`
}

// LineCommentResult output from posting inline comments to SCM.
type LineCommentResult struct {
	CommentID      int64          `json:"commentId"`
	ThreadID       string         `json:"threadId,omitempty"`
	DeliveryStatus DeliveryStatus `json:"deliveryStatus"`
	SuggestionID   string         `json:"suggestionId,omitempty"`
}

// ICommentManagerService formats and posts comments to GitHub, GitLab, and Bitbucket.
type ICommentManagerService interface {
	CreateInitialComment(ctx context.Context, orgID string, repo models.TrackedRepository, prNumber int, template string) (int64, error)
	CreateLineComments(ctx context.Context, orgID string, repo models.TrackedRepository, prNumber int, comments []LineCommentRequest) ([]LineCommentResult, error)
	UpdateOverallSummaryComment(ctx context.Context, orgID string, repo models.TrackedRepository, prNumber int, commentID int64, summaryBody string) error
	MinimizeOutdatedComments(ctx context.Context, orgID string, repo models.TrackedRepository, prNumber int, activeCommentIDs []int64) error
	PostPRReviewSubmission(ctx context.Context, orgID string, repo models.TrackedRepository, prNumber int, commitSHA, body, event string, comments []LineCommentRequest) error
}

// IMessageTemplateProcessor substitutes contextual placeholders in review messages.
type IMessageTemplateProcessor interface {
	Process(template string, vars TemplateVariables) string
}

// TemplateVariables represents dynamic tokens available in custom message templates.
type TemplateVariables struct {
	Author        string
	PRNumber      int
	RepoName      string
	Summary       string
	FindingsCount int
	FindingsList  string
	RulesChecked  int
	DurationSecs  float64
	ErrorMessage  string
}

// ISafeguardService analyzes suggestions to eliminate hallucinations and invalid diffs.
type ISafeguardService interface {
	FilterSuggestions(ctx context.Context, suggestions []CodeSuggestion, patches []*diff.FilePatch) ([]CodeSuggestion, []CodeSuggestion, error)
}

// SyntaxCheckResult holds status of compiler/linter check inside an isolated sandbox.
type SyntaxCheckResult struct {
	IsValid      bool     `json:"isValid"`
	Compiler     string   `json:"compiler"`
	ErrorMessage string   `json:"errorMessage,omitempty"`
	OutputLines  []string `json:"outputLines,omitempty"`
}

// ISandboxSyntaxValidator executes dry-run compilation inside an ephemeral sandbox.
type ISandboxSyntaxValidator interface {
	ValidateSyntax(ctx context.Context, language, filePath, modifiedContent string) (SyntaxCheckResult, error)
}

// LLMValidationResult captures the verdict of a second-tier LLM critic.
type LLMValidationResult struct {
	Approved        bool    `json:"approved"`
	ConfidenceScore float64 `json:"confidenceScore"`
	RejectionReason string  `json:"rejectionReason,omitempty"`
}

// ISuggestionLLMValidator performs a verification pass to filter false-positive advice.
type ISuggestionLLMValidator interface {
	ValidateSuggestion(ctx context.Context, s CodeSuggestion, fileContext string) (LLMValidationResult, error)
}

// IByokConcurrencyGate regulates concurrent requests to avoid provider rate limit throttling.
type IByokConcurrencyGate interface {
	AcquireSlot(ctx context.Context, orgID, provider, model string) (func(), error)
}

// IPrReviewDeferralService manages debounce windows when multiple rapid pushes occur.
type IPrReviewDeferralService interface {
	ShouldDefer(ctx context.Context, repoID string, prNumber int, headSHA string) (bool, error)
	MarkStarted(ctx context.Context, repoID string, prNumber int, headSHA string) error
	MarkCompleted(ctx context.Context, repoID string, prNumber int, headSHA string) error
}
