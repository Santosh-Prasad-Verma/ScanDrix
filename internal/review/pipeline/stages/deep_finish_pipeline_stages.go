package stages

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/domain"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/pkg/models"
)

// DeepAggregateResultsStage aggregates file-level and PR-level analysis outputs.
type DeepAggregateResultsStage struct {
	logger *slog.Logger
}

// NewDeepAggregateResultsStage instantiates Stage 14.
func NewDeepAggregateResultsStage(logger *slog.Logger) *DeepAggregateResultsStage {
	if logger == nil {
		logger = slog.Default()
	}
	return &DeepAggregateResultsStage{
		logger: logger.With("stage", "DeepAggregateResultsStage"),
	}
}

// Name returns stage identifier.
func (s *DeepAggregateResultsStage) Name() string {
	return "DeepAggregateResultsStage"
}

// Execute aggregates suggestions and ensures non-nil collections on pipeline context.
func (s *DeepAggregateResultsStage) Execute(ctx context.Context, pCtx *pipeline.PipelineContext) error {
	if pCtx.ValidSuggestions == nil {
		pCtx.ValidSuggestions = []domain.CodeSuggestion{}
	}
	if pCtx.DiscardedSuggestions == nil {
		pCtx.DiscardedSuggestions = []domain.CodeSuggestion{}
	}
	if pCtx.PRLevelSuggestions == nil {
		pCtx.PRLevelSuggestions = []domain.CodeSuggestion{}
	}

	// Merge static, agent and business logic suggestions
	s.logger.Info("Aggregated review results",
		"pr_number", pCtx.PullNumber,
		"valid_suggestions", len(pCtx.ValidSuggestions),
		"discarded_suggestions", len(pCtx.DiscardedSuggestions),
		"pr_level_suggestions", len(pCtx.PRLevelSuggestions),
		"business_logic_suggestions", len(pCtx.BusinessLogicResults),
	)
	return nil
}

// PostTraceCommentUseCase delivers the ScanDrix Trace architectural reasoning comment.
type PostTraceCommentUseCase interface {
	Execute(ctx context.Context, input TraceCommentInput) error
}

// TraceCommentInput contains parameters for posting or updating architectural trace records.
type TraceCommentInput struct {
	WorkspaceID string
	PRNumber    int
	RepoName    string
	Decisions   []pipeline.TraceDecisionInfo
	Provider    models.SCMProvider
	DryRun      bool
}

// SummaryManagerService generates PR descriptions and manages top-level summary comments.
type SummaryManagerService interface {
	GenerateSummaryPR(
		ctx context.Context,
		workspaceID string,
		prNumber int,
		repoName string,
		changedFiles []pipeline.FileChangeInfo,
		languagePrompt string,
		isCommitRun bool,
	) (string, error)
	UpdateSummarizationInPR(
		ctx context.Context,
		workspaceID string,
		prNumber int,
		repoName string,
		summary string,
	) error
	UpdateOverallComment(
		ctx context.Context,
		workspaceID string,
		prNumber int,
		repoName string,
		initialCommentID int64,
		provider models.SCMProvider,
		lineComments []domain.LineCommentResult,
		body string,
		reviewFailed bool,
		reviewErrorMessage string,
		reviewHasPartialErrors bool,
		reviewErrorCustomMessage string,
	) error
}

// DeepFinishCommentsStage updates the overall sticky comment, renders summaries, and posts trace records.
type DeepFinishCommentsStage struct {
	logger         *slog.Logger
	summaryManager SummaryManagerService
	traceUseCase   PostTraceCommentUseCase
}

// NewDeepFinishCommentsStage instantiates Stage 15.
func NewDeepFinishCommentsStage(
	logger *slog.Logger,
	sm SummaryManagerService,
	tc PostTraceCommentUseCase,
) *DeepFinishCommentsStage {
	if logger == nil {
		logger = slog.Default()
	}
	return &DeepFinishCommentsStage{
		logger:         logger.With("stage", "DeepFinishCommentsStage"),
		summaryManager: sm,
		traceUseCase:   tc,
	}
}

// Name returns stage identifier.
func (s *DeepFinishCommentsStage) Name() string {
	return "DeepFinishCommentsStage"
}

// Execute performs summary generation, sticky comment updates, and trace comment publishing.
func (s *DeepFinishCommentsStage) Execute(ctx context.Context, pCtx *pipeline.PipelineContext) error {
	isCommitRun := pCtx.LastExecution != nil
	commitBehaviour := pCtx.ResolvedConfig.Summary.BehaviourForNewCommits
	if commitBehaviour == "" {
		commitBehaviour = domain.CommitBehaviourReplace
	}

	shouldGenerateSummary := (!isCommitRun && pCtx.ResolvedConfig.Summary.GeneratePRSummary) ||
		(isCommitRun && pCtx.ResolvedConfig.Summary.GeneratePRSummary && commitBehaviour != domain.CommitBehaviourNone)

	if shouldGenerateSummary && s.summaryManager != nil {
		s.logger.Info("Generating pull request summary", "pr_number", pCtx.PullNumber)
		summaryText, err := s.summaryManager.GenerateSummaryPR(
			ctx,
			pCtx.WorkspaceID.String(),
			pCtx.PullNumber,
			pCtx.RepoNamespace,
			pCtx.ChangedFiles,
			pCtx.ResolvedConfig.LanguageResultPrompt,
			isCommitRun,
		)
		if err != nil {
			s.logger.Warn("Failed to generate summary for pull request", "error", err, "pr_number", pCtx.PullNumber)
			pCtx.AddError(s.Name(), "GenerateSummaryPR", err, "partial", map[string]interface{}{
				"message": "Failed to generate summary",
				"reason":  "summary_generation_failed",
			})
		} else {
			pCtx.PRSummaryBody = summaryText
			if updateErr := s.summaryManager.UpdateSummarizationInPR(
				ctx,
				pCtx.WorkspaceID.String(),
				pCtx.PullNumber,
				pCtx.RepoNamespace,
				summaryText,
			); updateErr != nil {
				s.logger.Warn("Failed to update summarization in PR description", "error", updateErr)
			}
		}
	}

	// Post ScanDrix Trace architectural decisions sticky comment
	if s.traceUseCase != nil && len(pCtx.TraceDecisions) > 0 {
		traceErr := s.traceUseCase.Execute(ctx, TraceCommentInput{
			WorkspaceID: pCtx.WorkspaceID.String(),
			PRNumber:    pCtx.PullNumber,
			RepoName:    pCtx.RepoNamespace,
			Decisions:   pCtx.TraceDecisions,
			Provider:    pCtx.Provider,
		})
		if traceErr != nil {
			s.logger.Warn("Failed to post ScanDrix Trace comment", "error", traceErr)
		} else {
			s.logger.Info("ScanDrix Trace comment successfully updated", "pr_number", pCtx.PullNumber)
		}
	}

	// Classify pipeline errors
	reviewFailed := false
	reviewHasPartialErrors := false
	for _, e := range pCtx.PipelineErrors {
		if e.Severity == "critical" {
			reviewFailed = true
			break
		}
		if e.Severity == "partial" {
			reviewHasPartialErrors = true
		}
	}

	var reviewErrorMessage string
	if pCtx.LastReviewError != nil {
		reviewErrorMessage = pCtx.LastReviewError.FriendlyMessage
	}

	var reviewErrorCustomMessage string
	if reviewFailed && pCtx.PullRequestMessages != nil && pCtx.PullRequestMessages.ErrorReviewMessage != nil {
		content := strings.TrimSpace(pCtx.PullRequestMessages.ErrorReviewMessage.Content)
		if content != "" {
			reviewErrorCustomMessage = content
		}
	}

	// Update overall sticky comment
	if s.summaryManager != nil && pCtx.InitialCommentID > 0 {
		body := pCtx.PRSummaryBody
		if err := s.summaryManager.UpdateOverallComment(
			ctx,
			pCtx.WorkspaceID.String(),
			pCtx.PullNumber,
			pCtx.RepoNamespace,
			pCtx.InitialCommentID,
			pCtx.Provider,
			pCtx.LineCommentResults,
			body,
			reviewFailed,
			reviewErrorMessage,
			reviewHasPartialErrors,
			reviewErrorCustomMessage,
		); err != nil {
			s.logger.Warn("Failed to update overall comment", "error", err, "pr_number", pCtx.PullNumber)
		}
	}

	return nil
}

// PullRequestReviewManagement handles approving or requesting changes on the remote git provider.
type PullRequestReviewManagement interface {
	GetReviewStatus(ctx context.Context, workspaceID string, prNumber int, repo models.TrackedRepository) (string, error)
	RequestChanges(ctx context.Context, workspaceID string, prNumber int, repo models.TrackedRepository, criticalCount int) error
	ApprovePullRequest(ctx context.Context, workspaceID string, prNumber int, repo models.TrackedRepository) error
}

// NotificationEmitter broadcasts approval notices to team channels and authors.
type NotificationEmitter interface {
	EmitAutoApproved(ctx context.Context, prNumber int, repoName, authorEmail, prURL string) error
}

// DeepRequestChangesOrApproveStage finalized review by approving or requesting changes.
type DeepRequestChangesOrApproveStage struct {
	logger        *slog.Logger
	reviewMgmt    PullRequestReviewManagement
	notifications NotificationEmitter
}

// NewDeepRequestChangesOrApproveStage instantiates Stage 16.
func NewDeepRequestChangesOrApproveStage(
	logger *slog.Logger,
	rm PullRequestReviewManagement,
	ne NotificationEmitter,
) *DeepRequestChangesOrApproveStage {
	if logger == nil {
		logger = slog.Default()
	}
	return &DeepRequestChangesOrApproveStage{
		logger:        logger.With("stage", "DeepRequestChangesOrApproveStage"),
		reviewMgmt:    rm,
		notifications: ne,
	}
}

// Name returns stage identifier.
func (s *DeepRequestChangesOrApproveStage) Name() string {
	return "DeepRequestChangesOrApproveStage"
}

// Execute checks for critical comments to request changes, or auto-approves clean pull requests.
func (s *DeepRequestChangesOrApproveStage) Execute(ctx context.Context, pCtx *pipeline.PipelineContext) error {
	if pCtx.WorkspaceID == uuid.Nil || pCtx.PullNumber == 0 || pCtx.RepositoryID == uuid.Nil {
		s.logger.Warn("Missing required fields for request changes / approve", "pr_number", pCtx.PullNumber)
		return nil
	}

	trackedRepo := models.TrackedRepository{
		ID:            pCtx.RepositoryID,
		NamespacePath: pCtx.RepoNamespace,
		Provider:      pCtx.Provider,
	}

	// 1. Request changes if critical findings are present
	if pCtx.ResolvedConfig.IsRequestChangesActive && s.reviewMgmt != nil {
		criticalCount := 0
		for _, sug := range pCtx.ValidSuggestions {
			if sug.Severity == domain.SeverityCritical {
				criticalCount++
			}
		}

		if criticalCount > 0 {
			s.logger.Info("Requesting changes on PR due to critical findings",
				"pr_number", pCtx.PullNumber,
				"critical_count", criticalCount,
			)
			if err := s.reviewMgmt.RequestChanges(ctx, pCtx.WorkspaceID.String(), pCtx.PullNumber, trackedRepo, criticalCount); err != nil {
				s.logger.Error("Error requesting changes on PR", "error", err, "pr_number", pCtx.PullNumber)
			}
		}
	}

	// 2. Auto-approve if approval active and 0 inline comments
	reviewHasFailures := len(pCtx.PipelineErrors) > 0
	if pCtx.ResolvedConfig.PullRequestApprovalActive && len(pCtx.LineCommentResults) == 0 && s.reviewMgmt != nil {
		if reviewHasFailures {
			s.logger.Info("Skipping auto-approve because review encountered errors/degradations",
				"pr_number", pCtx.PullNumber,
				"error_count", len(pCtx.PipelineErrors),
			)
			return nil
		}

		status, err := s.reviewMgmt.GetReviewStatus(ctx, pCtx.WorkspaceID.String(), pCtx.PullNumber, trackedRepo)
		if err != nil {
			s.logger.Warn("Failed fetching current PR review status", "error", err)
		} else if status == "APPROVED" {
			s.logger.Info("Pull request already approved, skipping approval action", "pr_number", pCtx.PullNumber)
			return nil
		}

		if status == "CHANGES_REQUESTED" {
			s.logger.Info("Clearing previously requested changes by approving PR", "pr_number", pCtx.PullNumber)
		} else {
			s.logger.Info("Approving PR as no issues were found and review is clean", "pr_number", pCtx.PullNumber)
		}

		if err := s.reviewMgmt.ApprovePullRequest(ctx, pCtx.WorkspaceID.String(), pCtx.PullNumber, trackedRepo); err != nil {
			s.logger.Error("Failed approving pull request", "error", err, "pr_number", pCtx.PullNumber)
		} else {
			pCtx.PassedReview = true
			if s.notifications != nil {
				_ = s.notifications.EmitAutoApproved(
					ctx,
					pCtx.PullNumber,
					pCtx.RepoNamespace,
					pCtx.AuthorEmail,
					fmt.Sprintf("https://scandrix.dev/%s/pull/%d", pCtx.RepoNamespace, pCtx.PullNumber),
				)
			}
		}
	}

	return nil
}
