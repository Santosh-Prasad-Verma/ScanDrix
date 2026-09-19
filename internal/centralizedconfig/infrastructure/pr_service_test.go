package infrastructure

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/centralizedconfig/domain"
)

type mockGitPlatformClient struct {
	branches      map[string]string
	commits       map[string][]domain.FileMutationOp
	contents      map[string][]byte
	prs           []RemotePullRequest
	getPRError    error
	customPRState string
}

func newMockGitPlatformClient() *mockGitPlatformClient {
	return &mockGitPlatformClient{
		branches: make(map[string]string),
		commits:  make(map[string][]domain.FileMutationOp),
		contents: make(map[string][]byte),
	}
}

func (m *mockGitPlatformClient) CreateBranch(ctx context.Context, orgID, teamID, repoID, branchName, baseBranch string) error {
	m.branches[branchName] = baseBranch
	return nil
}

func (m *mockGitPlatformClient) CreateCommit(ctx context.Context, orgID, teamID, repoID, branchName, commitMessage string, mutations []domain.FileMutationOp) (string, error) {
	m.commits[branchName] = append(m.commits[branchName], mutations...)
	return "commit-sha-123", nil
}

func (m *mockGitPlatformClient) OpenPullRequest(ctx context.Context, orgID, teamID, repoID, title, description, sourceBranch, targetBranch string) (string, error) {
	prURL := "https://github.com/scandrix/central-config/pull/42"
	m.prs = append(m.prs, RemotePullRequest{
		Number:       42,
		Title:        title,
		SourceBranch: sourceBranch,
		TargetBranch: targetBranch,
		URL:          prURL,
		State:        "open",
	})
	return prURL, nil
}

func (m *mockGitPlatformClient) GetDefaultBranch(ctx context.Context, orgID, teamID, repoID string) (string, error) {
	return "main", nil
}

func (m *mockGitPlatformClient) GetFileContent(ctx context.Context, orgID, teamID, repoID, filePath, ref string) ([]byte, error) {
	return m.contents[filePath], nil
}

func (m *mockGitPlatformClient) ListOpenPullRequests(ctx context.Context, orgID, teamID, repoID string) ([]RemotePullRequest, error) {
	return m.prs, nil
}

func (m *mockGitPlatformClient) GetPullRequest(ctx context.Context, orgID, teamID, repoID string, prNumber int) (*RemotePullRequest, error) {
	if m.getPRError != nil {
		return nil, m.getPRError
	}
	state := "OPENED"
	if m.customPRState != "" {
		state = m.customPRState
	}
	for _, pr := range m.prs {
		if pr.Number == prNumber {
			return &RemotePullRequest{
				Number:       pr.Number,
				Title:        pr.Title,
				SourceBranch: pr.SourceBranch,
				TargetBranch: pr.TargetBranch,
				URL:          pr.URL,
				State:        state,
			}, nil
		}
	}
	return &RemotePullRequest{
		Number: prNumber,
		State:  state,
		URL:    "https://github.com/scandrix/central-config/pull/42",
	}, nil
}

func TestPRServiceLifecycleAndTracking(t *testing.T) {
	ctx := context.Background()
	gitClient := newMockGitPlatformClient()
	svc := NewPRService(gitClient)

	orgID := "org-1"
	teamID := "team-1"

	// 1. Path helpers
	if svc.BuildCentralizedPath("repo-1", "rules/auth.yaml") != "repo-1/rules/auth.yaml" {
		t.Errorf("failed BuildCentralizedPath")
	}
	if svc.BuildCentralizedPath("", "rules/auth.yaml") != "rules/auth.yaml" {
		t.Errorf("failed empty folder BuildCentralizedPath")
	}
	if svc.BuildScanDrixConfigRelativePath("src/api") != "src/api/scandrix-config.yaml" {
		t.Errorf("failed BuildScanDrixConfigRelativePath")
	}

	// 2. PR Number Extraction
	prNum := svc.ExtractPullRequestNumber("https://github.com/scandrix/config/pull/1234")
	if prNum != 1234 {
		t.Errorf("expected 1234, got %d", prNum)
	}

	// 3. Metadata persistence & tracking
	svc.PersistActivePullRequestMetadata(orgID, teamID, ActivePRMetadata{
		PRNumber:     1234,
		PRURL:        "https://github.com/scandrix/config/pull/1234",
		BranchName:   "scandrix-centralized-branch",
		RepositoryID: "repo-central",
		Title:        "Update Rules",
	})

	meta := svc.GetActivePullRequestMetadata(orgID, teamID)
	if meta == nil || meta.PRNumber != 1234 {
		t.Fatalf("expected metadata with PR 1234")
	}

	// 4. Create Mutation PR (uses active branch)
	ops := []domain.FileMutationOp{
		{Path: "repo-1/scandrix-config.yaml", Operation: "upsert", Content: "language: go"},
	}
	prURL, err := svc.CreateMutationPR(ctx, orgID, teamID, ops, "Add config", "Automated sync")
	if err != nil {
		t.Fatalf("failed CreateMutationPR: %v", err)
	}
	if !strings.Contains(prURL, "42") {
		t.Errorf("expected URL with PR 42, got %s", prURL)
	}

	// 5. Scoped config reading
	gitClient.contents["repo-1/scandrix-config.yaml"] = []byte("rules:\n  strict: true")
	content, err := svc.GetScopedScanDrixConfigFileContent(ctx, orgID, teamID, "repo-central", "repo-1", nil)
	if err != nil {
		t.Fatalf("failed GetScopedScanDrixConfigFileContent: %v", err)
	}
	if !strings.Contains(content, "strict: true") {
		t.Errorf("expected remote content, got: %s", content)
	}

	// 6. PR Close handling
	svc.HandleTrackedPullRequestClose(ctx, orgID, teamID, 42, true)
	if svc.GetActivePullRequestMetadata(orgID, teamID) != nil {
		t.Errorf("expected active metadata to be cleared")
	}
}

func TestPRService_SanitizeFileName_And_BuildRuleFileName(t *testing.T) {
	svc := NewPRService(nil)

	// Test sanitization
	if res := svc.SanitizeFileName("No Console Log Allowed!!", "item", 30); res != "no-console-log-allowed" {
		t.Errorf("expected no-console-log-allowed, got: %s", res)
	}
	if res := svc.SanitizeFileName("   ", "default-fallback", 30); res != "default-fallback" {
		t.Errorf("expected default-fallback, got: %s", res)
	}
	if res := svc.SanitizeFileName("a-very-long-rule-title-that-exceeds-maximum-length-threshold", "item", 20); res != "a-very-long-rule-tit" {
		t.Errorf("expected length limited to 20 chars, got: %s", res)
	}

	// Test rule file name with UUID
	fileName := svc.BuildRuleFileName("Ensure SQL Bindings", "12345678-abcd-ef00-1122-334455667788")
	if fileName != "ensure-sql-bindings-12345678.yml" {
		t.Errorf("expected ensure-sql-bindings-12345678.yml, got: %s", fileName)
	}

	// Test rule file name without UUID
	fileNameNoUUID := svc.BuildRuleFileName("Ensure SQL Bindings", "")
	if fileNameNoUUID != "ensure-sql-bindings.yml" {
		t.Errorf("expected ensure-sql-bindings.yml, got: %s", fileNameNoUUID)
	}
}

func TestPRService_BuildDirectoryGroupPaths(t *testing.T) {
	svc := NewPRService(nil)

	// Global scope
	globalConfig := svc.BuildDirectoryGroupConfigPath("global", "group-1")
	if globalConfig != "group-1/scandrix-config.yaml" {
		t.Errorf("expected group-1/scandrix-config.yaml, got %s", globalConfig)
	}

	globalRule := svc.BuildDirectoryGroupRulesPath("global", "group-1", "review", "auth-rule.yml")
	if globalRule != "group-1/.drixy-rules/review/auth-rule.yml" {
		t.Errorf("expected group-1/.drixy-rules/review/auth-rule.yml, got %s", globalRule)
	}

	// Repo scope
	repoConfig := svc.BuildDirectoryGroupConfigPath("my-repo", "group-1")
	if repoConfig != "my-repo/group-1/scandrix-config.yaml" {
		t.Errorf("expected my-repo/group-1/scandrix-config.yaml, got %s", repoConfig)
	}

	repoRule := svc.BuildDirectoryGroupRulesPath("my-repo", "group-1", "memories", "memory-1.yml")
	if repoRule != "my-repo/group-1/.drixy-rules/memories/memory-1.yml" {
		t.Errorf("expected my-repo/group-1/.drixy-rules/memories/memory-1.yml, got %s", repoRule)
	}
}

func TestPRService_GetCentralizedConfigWithValidatedPullRequest(t *testing.T) {
	ctx := context.Background()
	orgID := "org-1"
	teamID := "team-1"
	centralRepo := domain.RepoRef{ID: "repo-central", Name: "central-config"}

	t.Run("returns config unchanged when there is no active pull request", func(t *testing.T) {
		gitClient := newMockGitPlatformClient()
		svc := NewPRService(gitClient)

		cfg := &domain.CentralizedConfigParameter{
			Enabled:           true,
			Repository:        &centralRepo,
			ActivePullRequest: nil,
		}
		svc.SetCentralizedConfigParameter(orgID, teamID, cfg)

		res, err := svc.GetCentralizedConfigWithValidatedPullRequest(ctx, orgID, teamID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.ActivePullRequest != nil {
			t.Errorf("expected ActivePullRequest to be nil")
		}
	})

	t.Run("returns config unchanged when active pull request is still open", func(t *testing.T) {
		gitClient := newMockGitPlatformClient()
		gitClient.customPRState = "OPENED"
		svc := NewPRService(gitClient)

		activePR := &domain.CentralizedConfigActivePullRequest{
			PRURL:        "https://github.com/scandrix/central-config/pull/123",
			PRNumber:     123,
			SourceBranch: "scandrix-centralized-branch",
			TargetBranch: "main",
			Repository:   centralRepo,
		}
		cfg := &domain.CentralizedConfigParameter{
			Enabled:           true,
			Repository:        &centralRepo,
			ActivePullRequest: activePR,
		}
		svc.SetCentralizedConfigParameter(orgID, teamID, cfg)

		res, err := svc.GetCentralizedConfigWithValidatedPullRequest(ctx, orgID, teamID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.ActivePullRequest == nil || res.ActivePullRequest.PRNumber != 123 {
			t.Errorf("expected ActivePullRequest 123 preserved")
		}
	})

	t.Run("clears active pull request when it has been merged", func(t *testing.T) {
		gitClient := newMockGitPlatformClient()
		gitClient.customPRState = "MERGED"
		svc := NewPRService(gitClient)

		activePR := &domain.CentralizedConfigActivePullRequest{
			PRURL:        "https://github.com/scandrix/central-config/pull/123",
			PRNumber:     123,
			SourceBranch: "scandrix-centralized-branch",
			TargetBranch: "main",
			Repository:   centralRepo,
		}
		cfg := &domain.CentralizedConfigParameter{
			Enabled:           true,
			Repository:        &centralRepo,
			ActivePullRequest: activePR,
		}
		svc.SetCentralizedConfigParameter(orgID, teamID, cfg)

		res, err := svc.GetCentralizedConfigWithValidatedPullRequest(ctx, orgID, teamID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.ActivePullRequest != nil {
			t.Errorf("expected merged ActivePullRequest to be cleared")
		}
	})

	t.Run("clears active pull request when it has been closed without merging", func(t *testing.T) {
		gitClient := newMockGitPlatformClient()
		gitClient.customPRState = "CLOSED"
		svc := NewPRService(gitClient)

		activePR := &domain.CentralizedConfigActivePullRequest{
			PRURL:        "https://github.com/scandrix/central-config/pull/123",
			PRNumber:     123,
			SourceBranch: "scandrix-centralized-branch",
			TargetBranch: "main",
			Repository:   centralRepo,
		}
		cfg := &domain.CentralizedConfigParameter{
			Enabled:           true,
			Repository:        &centralRepo,
			ActivePullRequest: activePR,
		}
		svc.SetCentralizedConfigParameter(orgID, teamID, cfg)

		res, err := svc.GetCentralizedConfigWithValidatedPullRequest(ctx, orgID, teamID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.ActivePullRequest != nil {
			t.Errorf("expected closed ActivePullRequest to be cleared")
		}
	})

	t.Run("clears active pull request when tracked repository does not match centralized repository", func(t *testing.T) {
		gitClient := newMockGitPlatformClient()
		svc := NewPRService(gitClient)

		activePR := &domain.CentralizedConfigActivePullRequest{
			PRURL:        "https://github.com/scandrix/central-config/pull/123",
			PRNumber:     123,
			SourceBranch: "scandrix-centralized-branch",
			TargetBranch: "main",
			Repository:   domain.RepoRef{ID: "other-repo", Name: "other-repo"},
		}
		cfg := &domain.CentralizedConfigParameter{
			Enabled:           true,
			Repository:        &centralRepo,
			ActivePullRequest: activePR,
		}
		svc.SetCentralizedConfigParameter(orgID, teamID, cfg)

		res, err := svc.GetCentralizedConfigWithValidatedPullRequest(ctx, orgID, teamID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.ActivePullRequest != nil {
			t.Errorf("expected mismatched repo ActivePullRequest to be cleared")
		}
	})

	t.Run("returns config unchanged when pull request state cannot be fetched due to API error", func(t *testing.T) {
		gitClient := newMockGitPlatformClient()
		gitClient.getPRError = errors.New("temporary 500 internal server error from GitHub")
		svc := NewPRService(gitClient)

		activePR := &domain.CentralizedConfigActivePullRequest{
			PRURL:        "https://github.com/scandrix/central-config/pull/123",
			PRNumber:     123,
			SourceBranch: "scandrix-centralized-branch",
			TargetBranch: "main",
			Repository:   centralRepo,
		}
		cfg := &domain.CentralizedConfigParameter{
			Enabled:           true,
			Repository:        &centralRepo,
			ActivePullRequest: activePR,
		}
		svc.SetCentralizedConfigParameter(orgID, teamID, cfg)

		res, err := svc.GetCentralizedConfigWithValidatedPullRequest(ctx, orgID, teamID)
		if err != nil {
			t.Fatalf("expected error tolerance, got: %v", err)
		}
		if res.ActivePullRequest == nil || res.ActivePullRequest.PRNumber != 123 {
			t.Errorf("expected active pull request retained on transient API error")
		}
	})
}

func TestPRService_TryDiscoverAndReuseTrackedPullRequest(t *testing.T) {
	ctx := context.Background()
	gitClient := newMockGitPlatformClient()
	svc := NewPRService(gitClient)

	orgID := "org-1"
	teamID := "team-1"
	repoID := "repo-central"

	gitClient.prs = []RemotePullRequest{
		{
			Number:       321,
			Title:        "Remove Drixy Rule from global",
			SourceBranch: "scandrix-centralized-standard-delete-1775678312159",
			TargetBranch: "main",
			URL:          "https://github.com/scandrix/central-config/pull/321",
			State:        "open",
		},
	}

	mutations := []domain.FileMutationOp{
		{Path: ".drixy-rules/review/sample.yml", Operation: "delete"},
	}

	res, reused, err := svc.TryDiscoverAndReuseTrackedPullRequest(ctx, orgID, teamID, repoID, mutations, "delete rule via centralized config", "main")
	if err != nil {
		t.Fatalf("failed TryDiscoverAndReuseTrackedPullRequest: %v", err)
	}
	if !reused {
		t.Fatalf("expected PR to be reused")
	}
	if res.PRURL != "https://github.com/scandrix/central-config/pull/321" {
		t.Errorf("expected PR URL 321, got: %s", res.PRURL)
	}

	meta := svc.GetActivePullRequestMetadata(orgID, teamID)
	if meta == nil || meta.PRNumber != 321 {
		t.Errorf("expected metadata recorded with PR 321")
	}
}

func TestPRService_ClearActivePullRequestMetadataIfMatchingRepoAndPR(t *testing.T) {
	svc := NewPRService(nil)
	orgID := "org-1"
	teamID := "team-1"

	svc.PersistActivePullRequestMetadata(orgID, teamID, ActivePRMetadata{
		PRNumber:     456,
		RepositoryID: "123",
	})

	// Matching repo and PR number
	cleared := svc.ClearActivePullRequestMetadataIfMatchingRepoAndPR(orgID, teamID, "123", 456)
	if !cleared {
		t.Errorf("expected PR metadata to be cleared")
	}

	if svc.GetActivePullRequestMetadata(orgID, teamID) != nil {
		t.Errorf("expected metadata to be deleted")
	}
}
