// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package stages_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scandrix/backend/internal/review/contextpack"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/review/domain"
	"github.com/scandrix/backend/internal/review/knowledge"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/internal/review/pipeline/stages"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/pkg/models"
)

type mockIssueResolver struct {
	resolved *pipeline.ExternalIssueContext
	err      error
}

func (m *mockIssueResolver) ResolveIssueContext(ctx context.Context, title, description string) (*pipeline.ExternalIssueContext, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.resolved, nil
}

type mockCommentManager struct {
	initialCommentID int64
	capturedContent  string
}

func (m *mockCommentManager) CreateInitialComment(ctx context.Context, workspaceID string, repo models.TrackedRepository, pullNumber int, body string) (int64, error) {
	m.initialCommentID = 8888
	m.capturedContent = body
	return m.initialCommentID, nil
}

func (m *mockCommentManager) CreateLineComments(ctx context.Context, orgID string, repo models.TrackedRepository, prNumber int, comments []domain.LineCommentRequest) ([]domain.LineCommentResult, error) {
	return nil, nil
}

func (m *mockCommentManager) UpdateOverallSummaryComment(ctx context.Context, orgID string, repo models.TrackedRepository, prNumber int, commentID int64, summaryBody string) error {
	m.capturedContent = summaryBody
	return nil
}

func (m *mockCommentManager) MinimizeOutdatedComments(ctx context.Context, orgID string, repo models.TrackedRepository, prNumber int, activeCommentIDs []int64) error {
	return nil
}

func (m *mockCommentManager) PostPRReviewSubmission(ctx context.Context, orgID string, repo models.TrackedRepository, prNumber int, commitSHA, body, event string, comments []domain.LineCommentRequest) error {
	return nil
}

type mockTemplateProcessor struct{}

func (m *mockTemplateProcessor) Process(template string, vars domain.TemplateVariables) string {
	return "Processed: " + template + " for " + vars.Author
}

func TestDeepLoadExternalContextStage_TicketDetectionAndContextPack(t *testing.T) {
	ctx := context.Background()
	orgID := uuid.New()
	repoID := uuid.New()

	traceStore := contextpack.NewTraceDecisionStore()
	traceStore.AddDecision(orgID.String(), repoID.String(), contextpack.TraceDecision{
		DecisionKey: "ADR-042",
		Title:       "Token Service Architecture",
		Summary:     "All cryptographic operations must use constant-time comparisons",
		Rationale:   "Prevents timing attack vulnerabilities",
		Files:       []string{"services/auth/token.go"},
	})

	stage := stages.NewDeepLoadExternalContextStage(traceStore, nil)

	pCtx := &pipeline.PipelineContext{
		ReviewID:      uuid.New(),
		WorkspaceID:   orgID,
		RepositoryID:  repoID,
		RepoNamespace: "scandrix/auth-service",
		PullNumber:    101,
		Title:         "feat(auth): add secure token validation [AUTH-999]",
		Description:   "Implements secure token hashing. Relates to PROJ-555 and fixes commit abc1234.",
		ChangedFiles: []pipeline.FileChangeInfo{
			{
				Filename:  "services/auth/token.go",
				Status:    "modified",
				Additions: 25,
				Deletions: 2,
			},
		},
		ParsedPatches: []*diff.FilePatch{
			{
				OldPath:   "services/auth/token.go",
				NewPath:   "services/auth/token.go",
				Additions: 25,
				Deletions: 2,
				Hunks: []diff.Hunk{
					{
						Header:   "@@ -1,5 +1,15 @@",
						OldStart: 1,
						OldLines: 5,
						NewStart: 1,
						NewLines: 15,
						Lines: []diff.DiffLine{
							{Type: diff.LineAddition, Content: "package auth"},
							{Type: diff.LineAddition, Content: "func VerifyToken(token string) bool {"},
							{Type: diff.LineAddition, Content: "\treturn SubtleCompare(token, expected)"},
							{Type: diff.LineAddition, Content: "}"},
						},
					},
				},
			},
		},
		ActiveRules: []rules.RuleSpec{
			{
				ID:          uuid.New(),
				Name:        "Constant Time Comparison",
				Description: "Verify sensitive secrets using constant-time algorithms",
				Severity:    "critical",
			},
		},
	}

	err := stage.Execute(ctx, pCtx)
	require.NoError(t, err)

	// 1. Verify Reference Detection
	require.NotNil(t, pCtx.ExternalContext)
	assert.Equal(t, "AUTH-999", pCtx.ExternalContext.IssueKey)

	refs, ok := pCtx.PipelineMetadata["detected_references"].(*contextpack.DetectedReferences)
	require.True(t, ok)
	require.NotNil(t, refs)
	assert.True(t, len(refs.TicketKeys) >= 2) // AUTH-999 and PROJ-555

	// 2. Verify Trace Architectural Decisions Loaded
	require.Len(t, pCtx.TraceDecisions, 1)
	assert.Equal(t, "ADR-042", pCtx.TraceDecisions[0].DecisionKey)
	assert.Contains(t, pCtx.TraceDecisions[0].Rationale, "timing attack")

	// 3. Verify Blast Radius Report Generated
	blastReport, ok := pCtx.PipelineMetadata["blast_radius_report"].(*knowledge.BlastRadiusReport)
	require.True(t, ok)
	require.NotNil(t, blastReport)
	assert.Contains(t, blastReport.DirectlyModifiedFiles, "services/auth/token.go")

	// 4. Verify Context Pack Assembled
	pack, ok := pCtx.PipelineMetadata["context_pack"].(*contextpack.ContextPack)
	require.True(t, ok)
	require.NotNil(t, pack)
	assert.Greater(t, pack.TotalTokens, 0)

	packPrompt, ok := pCtx.PipelineMetadata["context_pack_prompt"].(string)
	require.True(t, ok)
	assert.Contains(t, packPrompt, "AUTH-999")
	assert.Contains(t, packPrompt, "ADR-042")
	assert.Contains(t, packPrompt, "Constant Time Comparison")
	assert.Contains(t, packPrompt, "VerifyToken")
}

func TestDeepLoadExternalContextStage_WithExplicitResolver(t *testing.T) {
	ctx := context.Background()
	orgID := uuid.New()
	repoID := uuid.New()

	resolver := &mockIssueResolver{
		resolved: &pipeline.ExternalIssueContext{
			IssueKey:    "JIRA-777",
			Title:       "Enterprise OAuth2 Integration",
			Description: "Must enforce PKCE and state token verification",
			Status:      "IN_PROGRESS",
		},
	}

	stage := stages.NewDeepLoadExternalContextStage(nil, resolver)

	pCtx := &pipeline.PipelineContext{
		ReviewID:      uuid.New(),
		WorkspaceID:   orgID,
		RepositoryID:  repoID,
		RepoNamespace: "scandrix/platform",
		PullNumber:    42,
		Title:         "feat: oauth integration",
		Description:   "connects oauth provider",
	}

	err := stage.Execute(ctx, pCtx)
	require.NoError(t, err)

	require.NotNil(t, pCtx.ExternalContext)
	assert.Equal(t, "JIRA-777", pCtx.ExternalContext.IssueKey)
	assert.Equal(t, "Enterprise OAuth2 Integration", pCtx.ExternalContext.Title)
	assert.Equal(t, "IN_PROGRESS", pCtx.ExternalContext.Status)

	packPrompt, ok := pCtx.PipelineMetadata["context_pack_prompt"].(string)
	require.True(t, ok)
	assert.Contains(t, packPrompt, "JIRA-777")
	assert.Contains(t, packPrompt, "PKCE")
}

func TestDeepInitialCommentStage_Execute(t *testing.T) {
	ctx := context.Background()
	orgID := uuid.New()
	repoID := uuid.New()

	mgr := &mockCommentManager{}
	tpl := &mockTemplateProcessor{}

	stage := stages.NewDeepInitialCommentStage(mgr, tpl)

	pCtx := &pipeline.PipelineContext{
		ReviewID:      uuid.New(),
		WorkspaceID:   orgID,
		RepositoryID:  repoID,
		RepoNamespace: "scandrix/core",
		PullNumber:    12,
		Author:        "octocat",
		ChangedFiles: []pipeline.FileChangeInfo{
			{Filename: "main.go"},
			{Filename: "config.go"},
		},
		PrCommits: []pipeline.CommitInfo{
			{SHA: "abc", Message: "initial"},
			{SHA: "def", Message: "update"},
		},
	}

	err := stage.Execute(ctx, pCtx)
	require.NoError(t, err)

	assert.Equal(t, int64(8888), pCtx.InitialCommentID)
	assert.Contains(t, mgr.capturedContent, "Processed:")
	assert.Contains(t, mgr.capturedContent, "ScanDrix-Reviewing")
	assert.Contains(t, mgr.capturedContent, "Analyzing 2 changed files across 2 commits")
}
