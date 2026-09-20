package stages

import (
	"context"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/domain"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveAddedLineAnchor(t *testing.T) {
	validDiffLines := [][2]int{
		{10, 12}, // added lines 10, 11, 12
		{20, 25}, // added lines 20..25
	}

	t.Run("Anchor Already On Added Line", func(t *testing.T) {
		start := 10
		anchor := ResolveAddedLineAnchor(validDiffLines, &start, 12)
		require.NotNil(t, anchor)
		assert.False(t, anchor.Snapped)
		assert.Equal(t, 12, anchor.Line)
		require.NotNil(t, anchor.StartLine)
		assert.Equal(t, 10, *anchor.StartLine)
	})

	t.Run("Anchor Snaps To Nearest Added Line Within Span", func(t *testing.T) {
		// Span is 15..22, anchor is 15 (unchanged). Added lines inside span: 20, 21, 22. Nearest to 15 is 20.
		start := 15
		anchor := ResolveAddedLineAnchor(validDiffLines, &start, 22)
		require.NotNil(t, anchor)
		assert.True(t, anchor.Snapped)
		assert.Equal(t, 20, anchor.Line)
		assert.Nil(t, anchor.StartLine)
	})

	t.Run("Anchor Fails When No Added Line In Span", func(t *testing.T) {
		// Span is 13..18 (all unchanged lines)
		start := 13
		anchor := ResolveAddedLineAnchor(validDiffLines, &start, 18)
		assert.Nil(t, anchor)
	})
}

func TestCalculateCommentLineBoundaries(t *testing.T) {
	t.Run("Single Line", func(t *testing.T) {
		start := CalculateCommentStartLine(10, 10)
		assert.Nil(t, start)
		end := CalculateCommentEndLine(10, 10)
		assert.Equal(t, 10, end)
	})

	t.Run("Valid Multi Line Within 15 Lines", func(t *testing.T) {
		start := CalculateCommentStartLine(10, 20)
		require.NotNil(t, start)
		assert.Equal(t, 10, *start)
		end := CalculateCommentEndLine(10, 20)
		assert.Equal(t, 20, end)
	})

	t.Run("Exceeds 15 Lines Limit", func(t *testing.T) {
		start := CalculateCommentStartLine(10, 40)
		assert.Nil(t, start)
		end := CalculateCommentEndLine(10, 40)
		assert.Equal(t, 10, end)
	})
}

type mockPRLevelCommentManager struct {
	results []domain.LineCommentResult
	err     error
}

func (m *mockPRLevelCommentManager) CreatePrLevelReviewComments(
	ctx context.Context,
	workspaceID string,
	prNumber int,
	repoName string,
	suggestions []domain.CodeSuggestion,
	languageResultPrompt string,
	suggestionCopyPrompt bool,
) ([]domain.LineCommentResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.results, nil
}

type mockPRLevelStorageService struct {
	savedCount int
}

func (m *mockPRLevelStorageService) AddPrLevelSuggestions(
	ctx context.Context,
	prNumber int,
	repoName string,
	suggestions []domain.CodeSuggestion,
	workspaceID string,
) error {
	m.savedCount += len(suggestions)
	return nil
}

func TestDeepCreatePrLevelCommentsStage(t *testing.T) {
	logger := slog.Default()
	cm := &mockPRLevelCommentManager{
		results: []domain.LineCommentResult{
			{CommentID: 1001, DeliveryStatus: domain.DeliveryStatusSent},
		},
	}
	ss := &mockPRLevelStorageService{}
	stage := NewDeepCreatePrLevelCommentsStage(logger, cm, ss)

	t.Run("Empty Suggestions Returns Early", func(t *testing.T) {
		pCtx := &pipeline.PipelineContext{
			WorkspaceID:  uuid.New(),
			RepositoryID: uuid.New(),
			PullNumber:   1,
		}
		err := stage.Execute(context.Background(), pCtx)
		require.NoError(t, err)
		assert.Empty(t, pCtx.PRLevelCommentResults)
		assert.Equal(t, 0, ss.savedCount)
	})

	t.Run("Posts And Persists PR Level Suggestions", func(t *testing.T) {
		pCtx := &pipeline.PipelineContext{
			WorkspaceID:   uuid.New(),
			RepositoryID:  uuid.New(),
			RepoNamespace: "org/repo",
			PullNumber:    12,
			PRLevelSuggestions: []domain.CodeSuggestion{
				{ID: uuid.New(), SuggestionContent: "Refactor architecture across packages."},
			},
			BusinessLogicResults: []domain.CodeSuggestion{
				{ID: uuid.New(), SuggestionContent: "Missing ticket validation."},
			},
		}

		err := stage.Execute(context.Background(), pCtx)
		require.NoError(t, err)
		require.Len(t, pCtx.PRLevelCommentResults, 1)
		assert.Equal(t, 2, ss.savedCount)
	})
}

type mockSyntaxValidator struct {
	valid bool
	err   error
}

func (m *mockSyntaxValidator) ValidateSyntax(ctx context.Context, filePath, code string) (bool, error) {
	return m.valid, m.err
}

type mockLLMValidator struct {
	valid bool
	err   error
}

func (m *mockLLMValidator) ValidateCommittable(ctx context.Context, filePath, diff, suggestion string) (bool, error) {
	return m.valid, m.err
}

func TestDeepValidateSuggestionsStage(t *testing.T) {
	logger := slog.Default()
	syntaxVal := &mockSyntaxValidator{valid: true}
	llmVal := &mockLLMValidator{valid: true}
	stage := NewDeepValidateSuggestionsStage(logger, syntaxVal, llmVal)

	t.Run("Disabled Feature Flag Skips", func(t *testing.T) {
		pCtx := &pipeline.PipelineContext{
			Provider: models.SCMProviderGitHub,
			ResolvedConfig: domain.CodeReviewConfig{
				EnableCommittableSuggestions: false,
			},
			ValidSuggestions: []domain.CodeSuggestion{
				{ID: uuid.New(), RelevantFile: "main.go", ImprovedCode: "fmt.Println()"},
			},
		}
		err := stage.Execute(context.Background(), pCtx)
		require.NoError(t, err)
		assert.False(t, pCtx.ValidSuggestions[0].IsCommittable)
	})

	t.Run("Non-GitHub Platform Skips Committable Validation", func(t *testing.T) {
		pCtx := &pipeline.PipelineContext{
			Provider: models.SCMProviderGitLab,
			ResolvedConfig: domain.CodeReviewConfig{
				EnableCommittableSuggestions: true,
			},
			ValidSuggestions: []domain.CodeSuggestion{
				{ID: uuid.New(), RelevantFile: "main.go", ImprovedCode: "fmt.Println()"},
			},
		}
		err := stage.Execute(context.Background(), pCtx)
		require.NoError(t, err)
		assert.False(t, pCtx.ValidSuggestions[0].IsCommittable)
	})

	t.Run("Unsupported Extension Stays Non-Committable", func(t *testing.T) {
		pCtx := &pipeline.PipelineContext{
			Provider: models.SCMProviderGitHub,
			ResolvedConfig: domain.CodeReviewConfig{
				EnableCommittableSuggestions: true,
			},
			ChangedFiles: []pipeline.FileChangeInfo{
				{Filename: "config.yaml", Status: "modified"},
			},
			ValidSuggestions: []domain.CodeSuggestion{
				{ID: uuid.New(), RelevantFile: "config.yaml", ImprovedCode: "timeout: 5s"},
			},
		}
		err := stage.Execute(context.Background(), pCtx)
		require.NoError(t, err)
		assert.False(t, pCtx.ValidSuggestions[0].IsCommittable)
		assert.Nil(t, pCtx.ValidSuggestions[0].ValidatedData)
	})

	t.Run("Valid Code Becomes Committable", func(t *testing.T) {
		pCtx := &pipeline.PipelineContext{
			Provider: models.SCMProviderGitHub,
			ResolvedConfig: domain.CodeReviewConfig{
				EnableCommittableSuggestions: true,
			},
			ChangedFiles: []pipeline.FileChangeInfo{
				{Filename: "handler.go", Status: "modified", Patch: "@@ -10,2 +10,2 @@"},
			},
			ValidSuggestions: []domain.CodeSuggestion{
				{
					ID:                 uuid.New(),
					RelevantFile:       "handler.go",
					RelevantLinesStart: 10,
					RelevantLinesEnd:   12,
					ImprovedCode:       "if err != nil {\n\treturn err\n}",
				},
			},
		}
		err := stage.Execute(context.Background(), pCtx)
		require.NoError(t, err)
		assert.True(t, pCtx.ValidSuggestions[0].IsCommittable)
		require.NotNil(t, pCtx.ValidSuggestions[0].ValidatedData)
		assert.Equal(t, 10, pCtx.ValidSuggestions[0].ValidatedData.LineStart)
		assert.Equal(t, 12, pCtx.ValidSuggestions[0].ValidatedData.LineEnd)
	})
}

type mockFileCommentManager struct {
	postedComments []domain.LineCommentRequest
	err            error
}

func (m *mockFileCommentManager) CreateLineComments(
	ctx context.Context,
	orgID string,
	repo models.TrackedRepository,
	prNumber int,
	comments []domain.LineCommentRequest,
) ([]domain.LineCommentResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	m.postedComments = append(m.postedComments, comments...)
	res := make([]domain.LineCommentResult, len(comments))
	for i, c := range comments {
		res[i] = domain.LineCommentResult{
			CommentID:      int64(i + 1),
			DeliveryStatus: domain.DeliveryStatusSent,
			SuggestionID:   c.Suggestion.ID.String(),
		}
	}
	return res, nil
}

type mockImplementedResolver struct {
	called bool
}

func (m *mockImplementedResolver) ResolveImplementedSuggestionsOnPlatform(
	ctx context.Context,
	workspaceID string,
	repo models.TrackedRepository,
	prNumber int,
	provider models.SCMProvider,
) error {
	m.called = true
	return nil
}

type mockReviewPersistence struct {
	savedCount int
}

func (m *mockReviewPersistence) SavePullRequestReview(
	ctx context.Context,
	workspaceID string,
	prNumber int,
	repo models.TrackedRepository,
	changedFiles []pipeline.FileChangeInfo,
	prioritizedSuggestions []domain.CodeSuggestion,
	discardedSuggestions []domain.CodeSuggestion,
	provider models.SCMProvider,
	commits []pipeline.CommitInfo,
	heavy bool,
) error {
	m.savedCount += len(prioritizedSuggestions)
	return nil
}

func TestDeepCreateFileCommentsStage(t *testing.T) {
	logger := slog.Default()
	cm := &mockFileCommentManager{}
	ir := &mockImplementedResolver{}
	pers := &mockReviewPersistence{}
	stage := NewDeepCreateFileCommentsStage(logger, cm, ir, pers)

	t.Run("Empty Suggestions Skips Posting", func(t *testing.T) {
		pCtx := &pipeline.PipelineContext{
			WorkspaceID:  uuid.New(),
			RepositoryID: uuid.New(),
			PullNumber:   7,
		}
		err := stage.Execute(context.Background(), pCtx)
		require.NoError(t, err)
		assert.True(t, ir.called)
		assert.Empty(t, pCtx.LineCommentResults)
	})

	t.Run("Sorts By File ASC and Severity Rank DESC", func(t *testing.T) {
		cm.postedComments = nil
		pCtx := &pipeline.PipelineContext{
			WorkspaceID:   uuid.New(),
			RepositoryID:  uuid.New(),
			RepoNamespace: "backend/core",
			Provider:      models.SCMProviderGitHub,
			PullNumber:    20,
			ChangedFiles: []pipeline.FileChangeInfo{
				{Filename: "b.go", Status: "modified"},
				{Filename: "a.go", Status: "modified"},
			},
			ValidSuggestions: []domain.CodeSuggestion{
				{
					ID:                 uuid.New(),
					RelevantFile:       "b.go",
					Severity:           domain.SeverityLow,
					RelevantLinesStart: 10,
					RelevantLinesEnd:   10,
				},
				{
					ID:                 uuid.New(),
					RelevantFile:       "a.go",
					Severity:           domain.SeverityMedium,
					RelevantLinesStart: 5,
					RelevantLinesEnd:   5,
				},
				{
					ID:                 uuid.New(),
					RelevantFile:       "a.go",
					Severity:           domain.SeverityCritical,
					RelevantLinesStart: 1,
					RelevantLinesEnd:   1,
				},
			},
		}

		err := stage.Execute(context.Background(), pCtx)
		require.NoError(t, err)
		require.Len(t, cm.postedComments, 3)

		// First two comments should be for a.go, with Critical before Medium
		assert.Equal(t, "a.go", cm.postedComments[0].FilePath)
		assert.Equal(t, domain.SeverityCritical, cm.postedComments[0].Suggestion.Severity)
		assert.Equal(t, "a.go", cm.postedComments[1].FilePath)
		assert.Equal(t, domain.SeverityMedium, cm.postedComments[1].Suggestion.Severity)
		// Third comment for b.go
		assert.Equal(t, "b.go", cm.postedComments[2].FilePath)
	})

	t.Run("Clusters Child Items Are Discarded", func(t *testing.T) {
		cm.postedComments = nil
		pCtx := &pipeline.PipelineContext{
			WorkspaceID:   uuid.New(),
			RepositoryID:  uuid.New(),
			RepoNamespace: "backend/core",
			Provider:      models.SCMProviderGitHub,
			PullNumber:    21,
			ChangedFiles: []pipeline.FileChangeInfo{
				{Filename: "main.go", Status: "modified"},
			},
			ValidSuggestions: []domain.CodeSuggestion{
				{
					ID:                 uuid.New(),
					RelevantFile:       "main.go",
					Severity:           domain.SeverityHigh,
					RelevantLinesStart: 10,
					RelevantLinesEnd:   10,
					Clustering: &domain.ClusteringInfo{
						Type: domain.ClusteringTypeChild,
					},
				},
				{
					ID:                 uuid.New(),
					RelevantFile:       "main.go",
					Severity:           domain.SeverityHigh,
					RelevantLinesStart: 15,
					RelevantLinesEnd:   15,
					Clustering: &domain.ClusteringInfo{
						Type: domain.ClusteringTypeParent,
					},
				},
			},
		}

		err := stage.Execute(context.Background(), pCtx)
		require.NoError(t, err)
		require.Len(t, cm.postedComments, 1)
		assert.Equal(t, 15, cm.postedComments[0].LineNumber)
		require.Len(t, pCtx.DiscardedSuggestions, 1)
		assert.Equal(t, domain.PriorityStatusDiscardedByClustering, pCtx.DiscardedSuggestions[0].PriorityStatus)
	})

	t.Run("Suggestions on Removed Files Are Discarded", func(t *testing.T) {
		cm.postedComments = nil
		pCtx := &pipeline.PipelineContext{
			WorkspaceID:   uuid.New(),
			RepositoryID:  uuid.New(),
			RepoNamespace: "backend/core",
			Provider:      models.SCMProviderGitHub,
			PullNumber:    22,
			ChangedFiles: []pipeline.FileChangeInfo{
				{Filename: "deleted.go", Status: "removed"},
			},
			ValidSuggestions: []domain.CodeSuggestion{
				{
					ID:                 uuid.New(),
					RelevantFile:       "deleted.go",
					Severity:           domain.SeverityHigh,
					RelevantLinesStart: 5,
					RelevantLinesEnd:   5,
				},
			},
		}

		err := stage.Execute(context.Background(), pCtx)
		require.NoError(t, err)
		assert.Empty(t, cm.postedComments)
		require.Len(t, pCtx.DiscardedSuggestions, 1)
		assert.Equal(t, domain.PriorityStatusDiscardedBySafeguard, pCtx.DiscardedSuggestions[0].PriorityStatus)
	})

	t.Run("GitLab Non-Added Line Anchor Discards Comment", func(t *testing.T) {
		cm.postedComments = nil
		pCtx := &pipeline.PipelineContext{
			WorkspaceID:   uuid.New(),
			RepositoryID:  uuid.New(),
			RepoNamespace: "gitlab/project",
			Provider:      models.SCMProviderGitLab,
			PullNumber:    33,
			ChangedFiles: []pipeline.FileChangeInfo{
				{
					Filename:       "service.go",
					Status:         "modified",
					ValidDiffLines: [][2]int{{1, 5}}, // only lines 1..5 are added lines
				},
			},
			ValidSuggestions: []domain.CodeSuggestion{
				{
					ID:                 uuid.New(),
					RelevantFile:       "service.go",
					Severity:           domain.SeverityMedium,
					RelevantLinesStart: 50, // unchanged context line!
					RelevantLinesEnd:   50,
				},
			},
		}

		err := stage.Execute(context.Background(), pCtx)
		require.NoError(t, err)
		assert.Empty(t, cm.postedComments)
		require.Len(t, pCtx.DiscardedSuggestions, 1)
		assert.Equal(t, domain.PriorityStatusDiscardedByCodeDiff, pCtx.DiscardedSuggestions[0].PriorityStatus)
	})
}
