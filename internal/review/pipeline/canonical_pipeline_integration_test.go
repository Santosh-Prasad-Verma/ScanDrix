package pipeline_test

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/review/domain"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/internal/review/pipeline/stages"
	"github.com/scandrix/backend/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockPermValidator struct{}

func (m *mockPermValidator) ValidateExecutionPermissions(ctx context.Context, orgID, userGitID string) (stages.PermissionValidationResult, error) {
	return stages.PermissionValidationResult{Allowed: true}, nil
}

func (m *mockPermValidator) AutoAssignLicense(ctx context.Context, orgID, userGitID string, prNumber int) (bool, string, error) {
	return true, "ASSIGNED", nil
}

func (m *mockPermValidator) TryHealMissingTrial(ctx context.Context, orgID string) (bool, error) {
	return false, nil
}

func (m *mockPermValidator) UserHoldsLicenseSeat(ctx context.Context, orgID, userGitID string) (bool, error) {
	return true, nil
}

type mockExclusionChecker struct{}

func (m *mockExclusionChecker) IsCentralizedConfigRepo(ctx context.Context, orgID, repoID string) (bool, error) {
	return false, nil
}

func (m *mockExclusionChecker) IsGlobalRulesSourceRepo(ctx context.Context, orgID, repoID string) (bool, error) {
	return false, nil
}

type mockRateLimiter struct{}

func (m *mockRateLimiter) ShouldEmit(ctx context.Context, rateLimitKey string, ttlSeconds int) (bool, error) {
	return true, nil
}

func (m *mockRateLimiter) EmitSkippedNoLicense(ctx context.Context, orgID, prURL, repoName, authorUsername string) error {
	return nil
}

type mockFeedbackReaction struct{}

func (m *mockFeedbackReaction) AddReaction(ctx context.Context, repo models.TrackedRepository, prNumber int, commentID string, reaction string) error {
	return nil
}

func (m *mockFeedbackReaction) CreateNoticeComment(ctx context.Context, repo models.TrackedRepository, prNumber int, commentBody string) error {
	return nil
}

type mockCommitFetcher struct{}

func (m *mockCommitFetcher) GetCommitsForPR(ctx context.Context, repoNamespace string, prNumber int) ([]pipeline.CommitInfo, error) {
	return []pipeline.CommitInfo{
		{SHA: "commit-1", Message: "init", Author: "alice"},
		{SHA: "commit-2", Message: "feat", Author: "alice"},
	}, nil
}

func (m *mockCommitFetcher) HasStageWithStatus(ctx context.Context, executionID string, stageNames []string, statuses []pipeline.AutomationStatus) (bool, error) {
	return false, nil
}

func (m *mockCommitFetcher) FindLatestExecution(ctx context.Context, workspaceID, repoID string, prNumber int) (*pipeline.PreviousExecutionInfo, error) {
	return nil, nil
}

type mockSlotResolver struct{}

func (m *mockSlotResolver) ResolveTaskSlot(ctx context.Context, orgID, taskName, modelOverride string) (*domain.ResolvedModelSlotInfo, error) {
	return &domain.ResolvedModelSlotInfo{
		ModelID:   "drixy-fast",
		ModelName: "Drixy Fast Reviewer",
		Provider:  "scandrix",
	}, nil
}

type mockConfigResolver struct {
	cfg domain.CodeReviewConfig
}

func (m *mockConfigResolver) GetCodeReviewParameter(ctx context.Context, orgID, teamID, repoID string) (domain.CodeReviewConfig, error) {
	return m.cfg, nil
}

func (m *mockConfigResolver) FindByRepoOrDirectory(ctx context.Context, orgID, repoID, dirID string) (*domain.PullRequestMessages, error) {
	return nil, nil
}

type mockCadenceStore struct{}

func (m *mockCadenceStore) FindRecentSuccessfulRuns(ctx context.Context, repoID string, prNumber int, since time.Time) ([]time.Time, error) {
	return []time.Time{}, nil
}

func (m *mockCadenceStore) GetLastReviewCadenceState(ctx context.Context, repoID string, prNumber int) (domain.ReviewCadenceState, error) {
	return domain.CadenceStateAutomatic, nil
}

type mockTraceLoader struct{}

func (m *mockTraceLoader) IsTraceContextEnabled(ctx context.Context, orgID, repoID string) (bool, error) {
	return true, nil
}

func (m *mockTraceLoader) LoadDecisionsForFiles(ctx context.Context, orgID, repoID string, filePaths []string) ([]pipeline.TraceDecisionInfo, error) {
	return []pipeline.TraceDecisionInfo{
		{
			DecisionKey: "ADR-005",
			Title:       "Use Stateless JWTs",
			Rationale:   "Scale auth service horizontally",
			Files:       filePaths,
		},
	}, nil
}

type mockIssueTrackerResolver struct{}

func (m *mockIssueTrackerResolver) ResolveIssueContext(ctx context.Context, title, description string) (*pipeline.ExternalIssueContext, error) {
	return &pipeline.ExternalIssueContext{
		IssueKey:    "AUTH-101",
		Title:       "Support JWT verification",
		Description: "Verify tokens in pkg/auth",
	}, nil
}

type mockCommentManagerService struct {
	createdID int64
}

func (m *mockCommentManagerService) CreateInitialComment(ctx context.Context, orgID string, repo models.TrackedRepository, prNumber int, template string) (int64, error) {
	m.createdID = 5555
	return 5555, nil
}

func (m *mockCommentManagerService) CreateLineComments(ctx context.Context, orgID string, repo models.TrackedRepository, prNumber int, comments []domain.LineCommentRequest) ([]domain.LineCommentResult, error) {
	return nil, nil
}

func (m *mockCommentManagerService) UpdateOverallSummaryComment(ctx context.Context, orgID string, repo models.TrackedRepository, prNumber int, commentID int64, summaryBody string) error {
	return nil
}

func (m *mockCommentManagerService) MinimizeOutdatedComments(ctx context.Context, orgID string, repo models.TrackedRepository, prNumber int, activeCommentIDs []int64) error {
	return nil
}

func (m *mockCommentManagerService) PostPRReviewSubmission(ctx context.Context, orgID string, repo models.TrackedRepository, prNumber int, commitSHA, body, event string, comments []domain.LineCommentRequest) error {
	return nil
}

type mockTemplateProcessor struct{}

func (m *mockTemplateProcessor) Process(template string, vars domain.TemplateVariables) string {
	return "Review in progress by ScanDrix"
}

type mockBusinessRulesAgent struct{}

func (m *mockBusinessRulesAgent) Execute(ctx context.Context, input stages.BusinessRulesValidationInput) (string, error) {
	return "No gaps detected. Verified against AUTH-101 acceptance criteria.", nil
}

type mockMCPManagerService struct{}

func (m *mockMCPManagerService) GetConnections(ctx context.Context, orgID, teamID string) ([]stages.MCPConnection, error) {
	return []stages.MCPConnection{
		{
			Category:    "task-management",
			AppName:     "Jira",
			IsConnected: true,
		},
	}, nil
}

type mockPRLevelCommentsMgr struct{}

func (m *mockPRLevelCommentsMgr) CreatePrLevelReviewComments(ctx context.Context, workspaceID string, prNumber int, repoName string, suggestions []domain.CodeSuggestion, languageResultPrompt string, suggestionCopyPrompt bool) ([]domain.LineCommentResult, error) {
	return []domain.LineCommentResult{}, nil
}

type mockPRLevelStorageService struct{}

func (m *mockPRLevelStorageService) AddPrLevelSuggestions(ctx context.Context, prNumber int, repoName string, suggestions []domain.CodeSuggestion, workspaceID string) error {
	return nil
}

type mockFileCommentsMgr struct {
	posted []domain.LineCommentRequest
}

func (m *mockFileCommentsMgr) CreateLineComments(ctx context.Context, orgID string, repo models.TrackedRepository, prNumber int, comments []domain.LineCommentRequest) ([]domain.LineCommentResult, error) {
	m.posted = append(m.posted, comments...)
	results := make([]domain.LineCommentResult, len(comments))
	for i, c := range comments {
		results[i] = domain.LineCommentResult{
			CommentID:      int64(i + 1),
			DeliveryStatus: domain.DeliveryStatusSent,
			SuggestionID:   c.Suggestion.ID.String(),
		}
	}
	return results, nil
}

type mockReviewSummaryMgr struct {
	summaryText string
}

func (m *mockReviewSummaryMgr) GenerateSummaryPR(ctx context.Context, workspaceID string, prNumber int, repoName string, changedFiles []pipeline.FileChangeInfo, languagePrompt string, isCommitRun bool) (string, error) {
	m.summaryText = "## Walkthrough\n- Updated JWT verification"
	return m.summaryText, nil
}

func (m *mockReviewSummaryMgr) UpdateSummarizationInPR(ctx context.Context, workspaceID string, prNumber int, repoName string, summary string) error {
	return nil
}

func (m *mockReviewSummaryMgr) UpdateOverallComment(ctx context.Context, workspaceID string, prNumber int, repoName string, initialCommentID int64, provider models.SCMProvider, lineComments []domain.LineCommentResult, body string, reviewFailed bool, reviewErrorMessage string, reviewHasPartialErrors bool, reviewErrorCustomMessage string) error {
	return nil
}

type mockReviewTraceCommenter struct{}

func (m *mockReviewTraceCommenter) Execute(ctx context.Context, input stages.TraceCommentInput) error {
	return nil
}

type mockReviewStateMgr struct {
	approved bool
	changes  bool
}

func (m *mockReviewStateMgr) GetReviewStatus(ctx context.Context, workspaceID string, prNumber int, repo models.TrackedRepository) (string, error) {
	return "PENDING", nil
}

func (m *mockReviewStateMgr) RequestChanges(ctx context.Context, workspaceID string, prNumber int, repo models.TrackedRepository, criticalCount int) error {
	m.changes = true
	return nil
}

func (m *mockReviewStateMgr) ApprovePullRequest(ctx context.Context, workspaceID string, prNumber int, repo models.TrackedRepository) error {
	m.approved = true
	return nil
}

type mockAutoApproveNotifier struct {
	notified bool
}

func (m *mockAutoApproveNotifier) EmitAutoApproved(ctx context.Context, prNumber int, repoName, authorEmail, prURL string) error {
	m.notified = true
	return nil
}

func TestCanonicalCodeReviewPipelineStrategy_EndToEnd(t *testing.T) {
	logger := slog.Default()

	// Stage 1: Validate Prerequisites
	stage1 := stages.NewDeepValidatePrerequisitesStage(
		&mockPermValidator{},
		&mockExclusionChecker{},
		&mockRateLimiter{},
		&mockFeedbackReaction{},
	)

	// Stage 2: Validate New Commits
	stage2 := stages.NewDeepValidateNewCommitsStage(&mockCommitFetcher{})

	// Stage 3: Resolve Config
	baseCfg := domain.DefaultCodeReviewConfig()
	baseCfg.ReviewOptions.BusinessLogic = true
	stage3 := stages.NewDeepResolveConfigStage(&mockConfigResolver{cfg: baseCfg}, &mockSlotResolver{})

	// Stage 4: Validate Config
	cadenceEngine := stages.NewReviewCadenceEngine(&mockCadenceStore{})
	stage4 := stages.NewDeepValidateConfigStage(cadenceEngine, &mockFeedbackReaction{})

	// Stage 5: Fetch Changed Files
	stage5 := stages.NewDeepFetchChangedFilesStage(350)

	// Stage 6: External Context
	stage6 := stages.NewDeepLoadExternalContextStage(&mockTraceLoader{}, &mockIssueTrackerResolver{})

	// Stage 7: Initial Sticky Comment
	initialCommentMgr := &mockCommentManagerService{}
	stage7 := stages.NewDeepInitialCommentStage(initialCommentMgr, &mockTemplateProcessor{})

	// Stage 8: Business Logic Validation
	stage8 := stages.NewDeepBusinessLogicValidationStage(logger, &mockBusinessRulesAgent{}, &mockMCPManagerService{})

	// Stage 9: Create Sandbox
	stage9 := stages.NewCreateSandboxStage(nil)

	// Stage 10: File Filter / Analysis
	stage10 := stages.NewFileFilterStage()

	// Stage 11: Create PR Level Comments
	stage11 := stages.NewDeepCreatePrLevelCommentsStage(logger, &mockPRLevelCommentsMgr{}, &mockPRLevelStorageService{})

	// Stage 12: Validate Suggestions
	stage12 := stages.NewDeepValidateSuggestionsStage(logger, nil, nil)

	// Stage 13: Create File Comments
	fileCm := &mockFileCommentsMgr{}
	stage13 := stages.NewDeepCreateFileCommentsStage(logger, fileCm, nil, nil)

	// Stage 14: Aggregate Results
	stage14 := stages.NewDeepAggregateResultsStage(logger)

	// Stage 15: Finish Comments
	summaryMgr := &mockReviewSummaryMgr{}
	stage15 := stages.NewDeepFinishCommentsStage(logger, summaryMgr, &mockReviewTraceCommenter{})

	// Stage 16: Request Changes or Approve
	reviewStateMgr := &mockReviewStateMgr{}
	notifier := &mockAutoApproveNotifier{}
	stage16 := stages.NewDeepRequestChangesOrApproveStage(logger, reviewStateMgr, notifier)

	strategy := pipeline.NewCanonicalPipelineStrategy(
		stage1, stage2, stage3, stage4, stage5,
		stage6, stage7,
		stage8, stage9, stage10,
		stage11, stage12, stage13,
		stage14, stage15, stage16,
	)

	require.Equal(t, "CanonicalCodeReviewPipeline", strategy.GetPipelineName())
	stagesList := strategy.ConfigureStages()
	require.Len(t, stagesList, 16)

	t.Run("Clean Review Auto-Approves End-To-End", func(t *testing.T) {
		rawDiff := `diff --git a/pkg/auth/token.go b/pkg/auth/token.go
new file mode 100644
index 0000000..abcdef1
--- /dev/null
+++ b/pkg/auth/token.go
@@ -0,0 +1,5 @@
+package auth
+
+func VerifyToken(token string) bool {
+    return len(token) > 0
+}
`
		patches, err := diff.ParseUnifiedDiff(strings.NewReader(rawDiff))
		require.NoError(t, err)

		pCtx := &pipeline.PipelineContext{
			ReviewID:      uuid.New(),
			WorkspaceID:   uuid.New(),
			RepositoryID:  uuid.New(),
			RepoNamespace: "backend/auth",
			Provider:      models.SCMProviderGitHub,
			PullNumber:    101,
			Title:         "feat(auth): add JWT verification token logic",
			Description:   "Implements AUTH-101 ticket requirements.",
			Author:        "alice",
			AuthorEmail:   "alice@scandrix.dev",
			Branch:        "feature/jwt",
			BaseBranch:    "main",
			PRState:       "open",
			RawDiff:       rawDiff,
			ParsedPatches: patches,
			ResolvedConfig: domain.CodeReviewConfig{
				Enabled:                   true,
				AutomatedReviewActive:     true,
				PullRequestApprovalActive: true,
				IsRequestChangesActive:    true,
				Summary: domain.SummaryConfig{
					GeneratePRSummary:      true,
					BehaviourForNewCommits: domain.CommitBehaviourReplace,
				},
				ReviewOptions: domain.ReviewOptions{
					BusinessLogic: true,
					Security:      true,
				},
			},
			StartTime: time.Now().UTC(),
		}

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		err = strategy.Execute(ctx, pCtx)
		require.NoError(t, err)

		// Verify pipeline progression through stages
		assert.Equal(t, 5555, int(initialCommentMgr.createdID))
		require.NotNil(t, pCtx.ExternalContext)
		assert.Equal(t, "AUTH-101", pCtx.ExternalContext.IssueKey)
		require.Len(t, pCtx.TraceDecisions, 1)
		assert.Equal(t, "ADR-005", pCtx.TraceDecisions[0].DecisionKey)
		require.Len(t, pCtx.ChangedFiles, 1)
		assert.Equal(t, "pkg/auth/token.go", pCtx.ChangedFiles[0].Filename)
		assert.Equal(t, "success", pCtx.BusinessLogicOutcome.Kind)
		assert.Contains(t, pCtx.PRSummaryBody, "Walkthrough")
		assert.True(t, pCtx.PassedReview)
		assert.True(t, reviewStateMgr.approved)
		assert.True(t, notifier.notified)
		assert.False(t, reviewStateMgr.changes)
	})

	t.Run("Critical Findings Trigger Request Changes End-To-End", func(t *testing.T) {
		reviewStateMgr.approved = false
		reviewStateMgr.changes = false
		notifier.notified = false

		rawDiff := `diff --git a/pkg/auth/token.go b/pkg/auth/token.go
new file mode 100644
index 0000000..abcdef1
--- /dev/null
+++ b/pkg/auth/token.go
@@ -0,0 +1,5 @@
+package auth
+
+func VerifyToken(token string) bool {
+    return len(token) > 0
+}
`
		patches, err := diff.ParseUnifiedDiff(strings.NewReader(rawDiff))
		require.NoError(t, err)

		pCtx := &pipeline.PipelineContext{
			ReviewID:      uuid.New(),
			WorkspaceID:   uuid.New(),
			RepositoryID:  uuid.New(),
			RepoNamespace: "backend/auth",
			Provider:      models.SCMProviderGitHub,
			PullNumber:    102,
			Title:         "feat(auth): token update",
			Description:   "Addresses AUTH-101.",
			Author:        "alice",
			Branch:        "feature/jwt",
			BaseBranch:    "main",
			PRState:       "open",
			RawDiff:       rawDiff,
			ParsedPatches: patches,
			ResolvedConfig: domain.CodeReviewConfig{
				Enabled:                   true,
				AutomatedReviewActive:     true,
				PullRequestApprovalActive: true,
				IsRequestChangesActive:    true,
				ReviewOptions: domain.ReviewOptions{
					BusinessLogic: true,
				},
			},
			ValidSuggestions: []domain.CodeSuggestion{
				{
					ID:                 uuid.New(),
					RelevantFile:       "pkg/auth/token.go",
					Severity:           domain.SeverityCritical,
					RelevantLinesStart: 2,
					RelevantLinesEnd:   4,
					SuggestionContent:  "Hardcoded secret detected in token validation routine.",
				},
			},
			StartTime: time.Now().UTC(),
		}

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		err = strategy.Execute(ctx, pCtx)
		require.NoError(t, err)

		// Critical finding must request changes and block approval
		assert.True(t, reviewStateMgr.changes)
		assert.False(t, reviewStateMgr.approved)
		assert.False(t, pCtx.PassedReview)
		assert.False(t, notifier.notified)
	})
}
