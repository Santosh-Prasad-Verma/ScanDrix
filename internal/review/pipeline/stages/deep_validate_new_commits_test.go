package stages

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/pipeline"
)

type mockCommitFetcher struct {
	commits         []pipeline.CommitInfo
	latestExecution *pipeline.PreviousExecutionInfo
	hasEligibleFail bool
	fetchErr        error
}

func (m *mockCommitFetcher) GetCommitsForPR(ctx context.Context, repoNamespace string, prNumber int) ([]pipeline.CommitInfo, error) {
	if m.fetchErr != nil {
		return nil, m.fetchErr
	}
	return m.commits, nil
}

func (m *mockCommitFetcher) HasStageWithStatus(ctx context.Context, executionID string, stageNames []string, statuses []pipeline.AutomationStatus) (bool, error) {
	return m.hasEligibleFail, nil
}

func (m *mockCommitFetcher) FindLatestExecution(ctx context.Context, workspaceID, repoID string, prNumber int) (*pipeline.PreviousExecutionInfo, error) {
	return m.latestExecution, nil
}

func TestDeepValidateNewCommits_FirstRun(t *testing.T) {
	ctx := context.Background()
	fetcher := &mockCommitFetcher{
		commits: []pipeline.CommitInfo{
			{SHA: "c1", Message: "Initial commit"},
			{SHA: "c2", Message: "Add feature"},
		},
	}
	stage := NewDeepValidateNewCommitsStage(fetcher)

	pCtx := &pipeline.PipelineContext{
		WorkspaceID:   uuid.New(),
		RepositoryID:  uuid.New(),
		PullNumber:    1,
		RepoNamespace: "scandrix/backend",
		HeadSHA:       "c2",
	}

	if err := stage.Execute(ctx, pCtx); err != nil || pCtx.SkipReview {
		t.Fatalf("expected first run to proceed without skipping")
	}
	if len(pCtx.PrCommits) != 2 {
		t.Errorf("expected 2 commits analyzed, got %d", len(pCtx.PrCommits))
	}
}

func TestDeepValidateNewCommits_IncrementalRun(t *testing.T) {
	ctx := context.Background()
	fetcher := &mockCommitFetcher{
		commits: []pipeline.CommitInfo{
			{SHA: "c1", Message: "Commit 1"},
			{SHA: "c2", Message: "Commit 2"},
			{SHA: "c3", Message: "Commit 3"},
		},
		latestExecution: &pipeline.PreviousExecutionInfo{
			ExecutionID:        "exec-prev",
			LastAnalyzedCommit: "c1",
		},
	}
	stage := NewDeepValidateNewCommitsStage(fetcher)

	pCtx := &pipeline.PipelineContext{
		WorkspaceID:   uuid.New(),
		RepositoryID:  uuid.New(),
		PullNumber:    2,
		RepoNamespace: "scandrix/backend",
		HeadSHA:       "c3",
	}

	if err := stage.Execute(ctx, pCtx); err != nil || pCtx.SkipReview {
		t.Fatalf("expected incremental run to proceed")
	}
	if len(pCtx.PrCommits) != 2 || pCtx.PrCommits[0].SHA != "c2" || pCtx.PrCommits[1].SHA != "c3" {
		t.Errorf("expected commits c2 and c3, got %v", pCtx.PrCommits)
	}
}

func TestDeepValidateNewCommits_NoNewCommits(t *testing.T) {
	ctx := context.Background()
	fetcher := &mockCommitFetcher{
		commits: []pipeline.CommitInfo{
			{SHA: "c1", Message: "Commit 1"},
			{SHA: "c2", Message: "Commit 2"},
		},
		latestExecution: &pipeline.PreviousExecutionInfo{
			ExecutionID:        "exec-prev",
			LastAnalyzedCommit: "c2",
		},
	}
	stage := NewDeepValidateNewCommitsStage(fetcher)

	pCtx := &pipeline.PipelineContext{
		WorkspaceID:   uuid.New(),
		RepositoryID:  uuid.New(),
		PullNumber:    3,
		RepoNamespace: "scandrix/backend",
		HeadSHA:       "c2",
	}

	if err := stage.Execute(ctx, pCtx); err != nil || !pCtx.SkipReview || pCtx.StatusInfo.ReasonCode != "NO_NEW_COMMITS_SINCE_LAST" {
		t.Fatalf("expected no new commits to skip review")
	}
}

func TestDeepValidateNewCommits_RebaseHistoryRewrite_OrphanedBaseCommit(t *testing.T) {
	ctx := context.Background()
	// Prior run analyzed commit 'c_old', but developer rebased or force pushed, so 'c_old' no longer exists in commit history!
	fetcher := &mockCommitFetcher{
		commits: []pipeline.CommitInfo{
			{SHA: "rebased_c1", Message: "Rebased commit 1"},
			{SHA: "rebased_c2", Message: "Rebased commit 2"},
		},
		latestExecution: &pipeline.PreviousExecutionInfo{
			ExecutionID:        "exec-prev",
			LastAnalyzedCommit: "c_old_orphaned",
		},
	}
	stage := NewDeepValidateNewCommitsStage(fetcher)

	pCtx := &pipeline.PipelineContext{
		WorkspaceID:   uuid.New(),
		RepositoryID:  uuid.New(),
		PullNumber:    4,
		RepoNamespace: "scandrix/backend",
		HeadSHA:       "rebased_c2",
	}

	if err := stage.Execute(ctx, pCtx); err != nil || pCtx.SkipReview {
		t.Fatalf("expected rebase history rewrite to fall back to full review")
	}
	if pCtx.OrphanedBaseCommit == nil || pCtx.OrphanedBaseCommit.PreviousSHA != "c_old_orphaned" {
		t.Fatalf("expected orphaned base commit to be tracked")
	}
	if len(pCtx.PrCommits) != 2 {
		t.Errorf("expected all 2 rebased commits analyzed, got %d", len(pCtx.PrCommits))
	}
	if pCtx.PipelineMetadata["forceFullRerun"] != true {
		t.Errorf("expected forceFullRerun to be true")
	}
}

func TestDeepValidateNewCommits_OnlyMergeCommits_GraphTraversal(t *testing.T) {
	ctx := context.Background()
	// All new commits are merge commits or ancestors brought in by them
	fetcher := &mockCommitFetcher{
		commits: []pipeline.CommitInfo{
			{SHA: "base_c", Message: "Base commit"},
			{SHA: "merge_c1", Message: "Merge branch 'main'", Parents: []string{"base_c", "upstream_1"}},
			{SHA: "upstream_1", Message: "Upstream change", Parents: []string{"base_c"}},
		},
		latestExecution: &pipeline.PreviousExecutionInfo{
			ExecutionID:        "exec-prev",
			LastAnalyzedCommit: "base_c",
		},
	}
	stage := NewDeepValidateNewCommitsStage(fetcher)

	pCtx := &pipeline.PipelineContext{
		WorkspaceID:   uuid.New(),
		RepositoryID:  uuid.New(),
		PullNumber:    5,
		RepoNamespace: "scandrix/backend",
		HeadSHA:       "merge_c1",
	}

	if err := stage.Execute(ctx, pCtx); err != nil || !pCtx.SkipReview || pCtx.StatusInfo.ReasonCode != "ONLY_MERGE_COMMITS_SINCE_LAST" {
		t.Fatalf("expected only merge commits to skip review")
	}
}

func TestDeepValidateNewCommits_CommandForceBypass(t *testing.T) {
	ctx := context.Background()
	fetcher := &mockCommitFetcher{
		commits: []pipeline.CommitInfo{
			{SHA: "c1", Message: "Commit 1"},
		},
		latestExecution: &pipeline.PreviousExecutionInfo{
			ExecutionID:        "exec-prev",
			LastAnalyzedCommit: "c1",
		},
	}
	stage := NewDeepValidateNewCommitsStage(fetcher)

	pCtx := &pipeline.PipelineContext{
		WorkspaceID:   uuid.New(),
		RepositoryID:  uuid.New(),
		PullNumber:    6,
		RepoNamespace: "scandrix/backend",
		HeadSHA:       "c1",
		Origin:        "command-force", // @scandrix review --force
	}

	if err := stage.Execute(ctx, pCtx); err != nil || pCtx.SkipReview {
		t.Fatalf("expected command-force to bypass no-new-commits check")
	}
}

func TestDeepValidateNewCommits_RateLimit429(t *testing.T) {
	ctx := context.Background()
	fetcher := &mockCommitFetcher{
		fetchErr: errors.New("github api: 429 rate limit exceeded"),
	}
	stage := NewDeepValidateNewCommitsStage(fetcher)

	pCtx := &pipeline.PipelineContext{
		WorkspaceID:   uuid.New(),
		RepositoryID:  uuid.New(),
		PullNumber:    7,
		RepoNamespace: "scandrix/backend",
		HeadSHA:       "c1",
	}

	if err := stage.Execute(ctx, pCtx); err != nil || !pCtx.SkipReview || pCtx.StatusInfo.ReasonCode != "PROVIDER_RATE_LIMITED" {
		t.Fatalf("expected 429 error to gracefully skip review with PROVIDER_RATE_LIMITED")
	}
}
