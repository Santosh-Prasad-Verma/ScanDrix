package stages

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/domain"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockSummaryManager struct {
	generatedSummary string
	updatedInPR      string
	overallComment   string
	genErr           error
}

func (m *mockSummaryManager) GenerateSummaryPR(
	ctx context.Context,
	workspaceID string,
	prNumber int,
	repoName string,
	changedFiles []pipeline.FileChangeInfo,
	languagePrompt string,
	isCommitRun bool,
) (string, error) {
	if m.genErr != nil {
		return "", m.genErr
	}
	m.generatedSummary = "### PR Summary\n- Implemented OAuth authentication"
	return m.generatedSummary, nil
}

func (m *mockSummaryManager) UpdateSummarizationInPR(
	ctx context.Context,
	workspaceID string,
	prNumber int,
	repoName string,
	summary string,
) error {
	m.updatedInPR = summary
	return nil
}

func (m *mockSummaryManager) UpdateOverallComment(
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
) error {
	m.overallComment = body
	return nil
}

type mockTraceUseCase struct {
	executed bool
}

func (m *mockTraceUseCase) Execute(ctx context.Context, input TraceCommentInput) error {
	m.executed = true
	return nil
}

func TestDeepFinishCommentsStage(t *testing.T) {
	logger := slog.Default()
	sm := &mockSummaryManager{}
	tu := &mockTraceUseCase{}
	stage := NewDeepFinishCommentsStage(logger, sm, tu)

	t.Run("Generates Summary And Posts Trace", func(t *testing.T) {
		pCtx := &pipeline.PipelineContext{
			WorkspaceID:      uuid.New(),
			RepositoryID:     uuid.New(),
			RepoNamespace:    "backend/api",
			PullNumber:       50,
			InitialCommentID: 999,
			ResolvedConfig: domain.CodeReviewConfig{
				Summary: domain.SummaryConfig{
					GeneratePRSummary:      true,
					BehaviourForNewCommits: domain.CommitBehaviourReplace,
				},
			},
			TraceDecisions: []pipeline.TraceDecisionInfo{
				{DecisionKey: "ADR-001", Title: "Use Postgres for persistence"},
			},
		}

		err := stage.Execute(context.Background(), pCtx)
		require.NoError(t, err)
		assert.Contains(t, sm.generatedSummary, "PR Summary")
		assert.Equal(t, sm.generatedSummary, pCtx.PRSummaryBody)
		assert.True(t, tu.executed)
		assert.Equal(t, sm.generatedSummary, sm.overallComment)
	})

	t.Run("Summary Generation Failure Records Partial Pipeline Error", func(t *testing.T) {
		smErr := &mockSummaryManager{genErr: errors.New("provider timeout")}
		stageWithErr := NewDeepFinishCommentsStage(logger, smErr, tu)

		pCtx := &pipeline.PipelineContext{
			WorkspaceID:   uuid.New(),
			RepositoryID:  uuid.New(),
			RepoNamespace: "backend/api",
			PullNumber:    51,
			ResolvedConfig: domain.CodeReviewConfig{
				Summary: domain.SummaryConfig{
					GeneratePRSummary: true,
				},
			},
		}

		err := stageWithErr.Execute(context.Background(), pCtx)
		require.NoError(t, err)
		require.Len(t, pCtx.PipelineErrors, 1)
		assert.Equal(t, "partial", pCtx.PipelineErrors[0].Severity)
		assert.Equal(t, "DeepFinishCommentsStage", pCtx.PipelineErrors[0].Stage)
	})
}

type mockReviewManagement struct {
	currentStatus  string
	changesReq     bool
	criticalCount  int
	approvedCalled bool
}

func (m *mockReviewManagement) GetReviewStatus(ctx context.Context, workspaceID string, prNumber int, repo models.TrackedRepository) (string, error) {
	return m.currentStatus, nil
}

func (m *mockReviewManagement) RequestChanges(ctx context.Context, workspaceID string, prNumber int, repo models.TrackedRepository, criticalCount int) error {
	m.changesReq = true
	m.criticalCount = criticalCount
	return nil
}

func (m *mockReviewManagement) ApprovePullRequest(ctx context.Context, workspaceID string, prNumber int, repo models.TrackedRepository) error {
	m.approvedCalled = true
	return nil
}

type mockNotificationEmitter struct {
	emitted bool
}

func (m *mockNotificationEmitter) EmitAutoApproved(ctx context.Context, prNumber int, repoName, authorEmail, prURL string) error {
	m.emitted = true
	return nil
}

func TestDeepRequestChangesOrApproveStage(t *testing.T) {
	logger := slog.Default()

	t.Run("Requests Changes When Critical Suggestion Present", func(t *testing.T) {
		rm := &mockReviewManagement{currentStatus: "PENDING"}
		ne := &mockNotificationEmitter{}
		stage := NewDeepRequestChangesOrApproveStage(logger, rm, ne)

		pCtx := &pipeline.PipelineContext{
			WorkspaceID:  uuid.New(),
			RepositoryID: uuid.New(),
			PullNumber:   100,
			ResolvedConfig: domain.CodeReviewConfig{
				IsRequestChangesActive:    true,
				PullRequestApprovalActive: true,
			},
			ValidSuggestions: []domain.CodeSuggestion{
				{Severity: domain.SeverityCritical},
			},
			LineCommentResults: []domain.LineCommentResult{
				{CommentID: 1},
			},
		}

		err := stage.Execute(context.Background(), pCtx)
		require.NoError(t, err)
		assert.True(t, rm.changesReq)
		assert.Equal(t, 1, rm.criticalCount)
		assert.False(t, rm.approvedCalled)
	})

	t.Run("Blocks Auto Approve When Review Had Pipeline Errors", func(t *testing.T) {
		rm := &mockReviewManagement{currentStatus: "PENDING"}
		ne := &mockNotificationEmitter{}
		stage := NewDeepRequestChangesOrApproveStage(logger, rm, ne)

		pCtx := &pipeline.PipelineContext{
			WorkspaceID:  uuid.New(),
			RepositoryID: uuid.New(),
			PullNumber:   101,
			ResolvedConfig: domain.CodeReviewConfig{
				PullRequestApprovalActive: true,
			},
			LineCommentResults: []domain.LineCommentResult{}, // 0 comments
			PipelineErrors: []pipeline.PipelineErrorInfo{
				{Stage: "AstAnalysis", Severity: "partial", ErrorMsg: "AST timed out"},
			},
		}

		err := stage.Execute(context.Background(), pCtx)
		require.NoError(t, err)
		assert.False(t, rm.approvedCalled)
		assert.False(t, ne.emitted)
	})

	t.Run("Approves Clean PR And Emits Notification", func(t *testing.T) {
		rm := &mockReviewManagement{currentStatus: "CHANGES_REQUESTED"}
		ne := &mockNotificationEmitter{}
		stage := NewDeepRequestChangesOrApproveStage(logger, rm, ne)

		pCtx := &pipeline.PipelineContext{
			WorkspaceID:   uuid.New(),
			RepositoryID:  uuid.New(),
			RepoNamespace: "backend/auth",
			PullNumber:    102,
			AuthorEmail:   "dev@scandrix.dev",
			ResolvedConfig: domain.CodeReviewConfig{
				PullRequestApprovalActive: true,
			},
			LineCommentResults: []domain.LineCommentResult{},
			PipelineErrors:     []pipeline.PipelineErrorInfo{},
		}

		err := stage.Execute(context.Background(), pCtx)
		require.NoError(t, err)
		assert.True(t, rm.approvedCalled)
		assert.True(t, pCtx.PassedReview)
		assert.True(t, ne.emitted)
	})
}
