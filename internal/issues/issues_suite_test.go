package issues_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/issues/application/usecases"
	"github.com/scandrix/backend/internal/issues/domain"
	"github.com/scandrix/backend/internal/issues/infrastructure/repositories"
	"github.com/scandrix/backend/internal/issues/infrastructure/services"
	issuesync "github.com/scandrix/backend/internal/issues/infrastructure/services/sync"
)

type mockAnalysisEngine struct {
	mu           sync.Mutex
	mergeCalls   int
	resolveCalls int
}

func (m *mockAnalysisEngine) MergeSuggestionsIntoIssues(
	ctx context.Context,
	orgTeam domain.OrganizationAndTeamData,
	pr domain.PullRequestInfo,
	promptData any,
) (*domain.MergeResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mergeCalls++
	return &domain.MergeResult{
		Matches: []domain.SuggestionMatch{
			{SuggestionID: "sugg-1", ExistingIssueID: "iss-1"},
			{SuggestionID: "sugg-2"}, // Unmatched
		},
	}, nil
}

func (m *mockAnalysisEngine) ResolveExistingIssues(
	ctx context.Context,
	orgTeam domain.OrganizationAndTeamData,
	pr domain.PullRequestInfo,
	promptData any,
) (*domain.ResolveResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resolveCalls++
	return &domain.ResolveResult{
		IssueVerificationResults: []domain.IssueVerificationResult{
			{IssueID: "iss-1", IsIssuePresentInCode: false}, // Resolved
		},
	}, nil
}

type mockPullRequestService struct {
	mu          sync.Mutex
	syncedFlags map[string]bool
	suggestions map[int][]domain.SuggestionItem
	files       map[int][]domain.PRFileInfo
}

func newMockPullRequestService() *mockPullRequestService {
	return &mockPullRequestService{
		syncedFlags: make(map[string]bool),
		suggestions: make(map[int][]domain.SuggestionItem),
		files:       make(map[int][]domain.PRFileInfo),
	}
}

func (m *mockPullRequestService) FindByNumberAndRepositoryName(
	ctx context.Context,
	number int,
	repoName string,
	orgTeam domain.OrganizationAndTeamData,
) (*domain.PullRequestInfo, []domain.PRFileInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	files := m.files[number]
	return &domain.PullRequestInfo{
		Number: number,
		User:   domain.UserRef{GitID: "123", Username: "developer"},
	}, files, nil
}

func (m *mockPullRequestService) UpdateSyncedWithIssuesFlag(
	ctx context.Context,
	prNumber int,
	repoID, orgID string,
	synced bool,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%s:%s:%d", orgID, repoID, prNumber)
	m.syncedFlags[key] = synced
	return nil
}

func (m *mockPullRequestService) FindSuggestionsByPR(
	ctx context.Context,
	orgID string,
	prNumber int,
	deliveryStatus string,
) ([]domain.SuggestionItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.suggestions[prNumber], nil
}

type mockParametersService struct {
	cfg *domain.IssueCreationConfig
}

func (m *mockParametersService) GetIssueCreationConfig(
	ctx context.Context,
	orgTeam domain.OrganizationAndTeamData,
) (*domain.IssueCreationConfig, error) {
	return m.cfg, nil
}

type mockCacheService struct {
	mu    sync.RWMutex
	store map[string][]*domain.Issue
}

func newMockCacheService() *mockCacheService {
	return &mockCacheService{store: make(map[string][]*domain.Issue)}
}

func (c *mockCacheService) Get(ctx context.Context, key string) ([]*domain.Issue, bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.store[key]
	return v, ok, nil
}

func (c *mockCacheService) Set(ctx context.Context, key string, issues []*domain.Issue, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store[key] = issues
	return nil
}

func (c *mockCacheService) Delete(ctx context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.store, key)
	return nil
}

type mockFeedbackReader struct {
	feedbacks []domain.FeedbackItem
}

func (f *mockFeedbackReader) GetByOrganizationID(ctx context.Context, orgID string) ([]domain.FeedbackItem, error) {
	return f.feedbacks, nil
}

type mockAuthService struct {
	allowedRepos []string
	denied       bool
}

func (a *mockAuthService) Ensure(ctx context.Context, user domain.UserRef, action, resource string, repoIDs []string) error {
	if a.denied {
		return fmt.Errorf("access denied")
	}
	return nil
}

func (a *mockAuthService) GetRepositoryScope(ctx context.Context, user domain.UserRef, action, resource string) ([]string, error) {
	if a.denied {
		return nil, fmt.Errorf("access denied")
	}
	return a.allowedRepos, nil
}

func TestIssuesRepositoryCRUDAndFilters(t *testing.T) {
	ctx := context.Background()
	repo := repositories.NewInMemoryIssuesRepository()

	now := time.Now().UTC()
	issue1 := &domain.Issue{
		UUID:        "iss-1",
		Title:       "SQL Injection in user search",
		Description: "Unsanitized query parameters in search endpoint",
		FilePath:    "internal/api/search.go",
		Language:    "go",
		Label:       "security",
		Severity:    domain.SeverityCritical,
		Status:      domain.StatusOpen,
		Repository: domain.RepositoryToIssues{
			ID:       "repo-1",
			Name:     "backend",
			FullName: "scandrix/backend",
			Platform: domain.PlatformGitHub,
		},
		OrganizationID: "org-1",
		CreatedAt:      now.Add(-2 * time.Hour),
		UpdatedAt:      now.Add(-2 * time.Hour),
	}

	created, err := repo.Create(ctx, issue1)
	if err != nil || created == nil {
		t.Fatalf("failed creating issue: %v", err)
	}

	// 1. FindByID
	found, err := repo.FindByID(ctx, "iss-1")
	if err != nil || found == nil || found.Title != "SQL Injection in user search" {
		t.Fatalf("expected to find issue by ID: %v", err)
	}

	// 2. FindByFileAndStatus
	byFile, err := repo.FindByFileAndStatus(ctx, "org-1", "repo-1", "internal/api/search.go", domain.StatusOpen)
	if err != nil || len(byFile) != 1 {
		t.Fatalf("expected 1 issue by file and status, got %d, err: %v", len(byFile), err)
	}

	// 3. FindByFilters with regex and exact matches
	filter := map[string]any{
		"organizationId": "org-1",
		"severity":       "critical",
		"title":          "injection",
		"repository.id":  []string{"repo-1", "repo-2"},
	}
	filtered, err := repo.FindByFilters(ctx, filter)
	if err != nil || len(filtered) != 1 {
		t.Fatalf("expected 1 issue matching filter, got %d, err: %v", len(filtered), err)
	}

	count, err := repo.Count(ctx, filter)
	if err != nil || count != 1 {
		t.Errorf("expected count 1, got %d", count)
	}

	// 4. Update operations
	upLabel, err := repo.UpdateLabel(ctx, "iss-1", "vulnerability")
	if err != nil || upLabel.Label != "vulnerability" {
		t.Errorf("expected updated label, got %v", upLabel)
	}

	upSev, err := repo.UpdateSeverity(ctx, "iss-1", domain.SeverityHigh)
	if err != nil || upSev.Severity != domain.SeverityHigh {
		t.Errorf("expected updated severity, got %v", upSev)
	}

	upStatus, err := repo.UpdateStatus(ctx, "iss-1", domain.StatusInProgress)
	if err != nil || upStatus.Status != domain.StatusInProgress {
		t.Errorf("expected updated status, got %v", upStatus)
	}

	upSugg, err := repo.AddSuggestionIDs(ctx, "iss-1", []string{"sugg-99", "sugg-100"})
	if err != nil || len(upSugg.ContributingSuggestions) != 2 {
		t.Errorf("expected 2 added suggestions, got %d", len(upSugg.ContributingSuggestions))
	}

	// 5. Bulk status update
	bulkUp, err := repo.UpdateStatusByIds(ctx, []string{"iss-1"}, domain.StatusResolved)
	if err != nil || len(bulkUp) != 1 || bulkUp[0].Status != domain.StatusResolved {
		t.Errorf("expected bulk status update to resolved, got %+v", bulkUp)
	}
}

func TestDrixyIssuesManagementServicePipeline(t *testing.T) {
	ctx := context.Background()

	repo := repositories.NewInMemoryIssuesRepository()
	issuesSvc := services.NewIssuesService(repo)
	prSvc := newMockPullRequestService()
	analysis := &mockAnalysisEngine{}
	cache := newMockCacheService()
	jira := issuesync.NewJiraIssueTracker("https://jira.scandrix.dev", "SCAN")

	paramSvc := &mockParametersService{
		cfg: &domain.IssueCreationConfig{
			AutomaticCreationEnabled: true,
			SourceFilters: domain.SourceFilters{
				IncludeDrixyRules:       true,
				IncludeCodeReviewEngine: true,
			},
			SeverityFilters: domain.SeverityFilters{
				MinimumSeverity: domain.SeverityHigh,
			},
			OrganizationID: "org-1",
		},
	}

	mgmt := services.NewDrixyIssuesManagementService(
		nil,
		issuesSvc,
		prSvc,
		paramSvc,
		analysis,
		cache,
		jira,
	)

	// Seed an existing open issue for merge testing
	_, _ = issuesSvc.Create(ctx, &domain.Issue{
		UUID:           "iss-1",
		Title:          "Memory leak in buffer",
		FilePath:       "internal/buffer.go",
		Status:         domain.StatusOpen,
		Severity:       domain.SeverityCritical,
		OrganizationID: "org-1",
		Repository: domain.RepositoryToIssues{
			ID:       "repo-1",
			Name:     "backend",
			Platform: domain.PlatformGitHub,
		},
	})

	params := domain.ContextToGenerateIssues{
		OrganizationAndTeamData: domain.OrganizationAndTeamData{
			OrganizationID: "org-1",
			TeamID:         "team-1",
		},
		Repository: domain.RepositoryToIssues{
			ID:       "repo-1",
			Name:     "backend",
			FullName: "scandrix/backend",
			Platform: domain.PlatformGitHub,
		},
		PullRequest: domain.PullRequestInfo{
			Number: 42,
			User:   domain.UserRef{GitID: "u-1", Username: "coder"},
		},
		PRFiles: []domain.PRFileInfo{
			{
				Path:   "internal/buffer.go",
				Status: "modified",
				Suggestions: []domain.SuggestionItem{
					{
						ID:                   "sugg-1",
						OneSentenceSummary:   "Buffer memory leak fix",
						RelevantFile:         "internal/buffer.go",
						Severity:             domain.SeverityCritical,
						ImplementationStatus: domain.ImplementationNotImplemented,
					},
					{
						ID:                   "sugg-2",
						OneSentenceSummary:   "New goroutine leak detected",
						RelevantFile:         "internal/buffer.go",
						Severity:             domain.SeverityHigh,
						ImplementationStatus: domain.ImplementationNotImplemented,
					},
					{
						ID:                   "sugg-3",
						OneSentenceSummary:   "Low severity naming convention",
						RelevantFile:         "internal/buffer.go",
						Severity:             domain.SeverityLow, // Should be filtered out
						ImplementationStatus: domain.ImplementationNotImplemented,
					},
				},
			},
			{
				Path:   "internal/legacy_removed.go",
				Status: "removed", // Open issues on this file should be dismissed
			},
		},
	}

	// Seed open issue on removed file
	_, _ = issuesSvc.Create(ctx, &domain.Issue{
		UUID:           "iss-legacy",
		Title:          "Legacy function bug",
		FilePath:       "internal/legacy_removed.go",
		Status:         domain.StatusOpen,
		Severity:       domain.SeverityHigh,
		OrganizationID: "org-1",
		Repository: domain.RepositoryToIssues{
			ID:   "repo-1",
			Name: "backend",
		},
	})

	err := mgmt.ProcessClosedPR(ctx, params)
	if err != nil {
		t.Fatalf("ProcessClosedPR failed: %v", err)
	}

	// Verify iss-1 got sugg-1 appended
	iss1, _ := issuesSvc.FindByID(ctx, "iss-1")
	if iss1 == nil || len(iss1.ContributingSuggestions) != 1 || iss1.ContributingSuggestions[0].ID != "sugg-1" {
		t.Errorf("expected iss-1 to have sugg-1 merged, got %+v", iss1)
	}

	// Verify iss-legacy was dismissed
	issLegacy, _ := issuesSvc.FindByID(ctx, "iss-legacy")
	if issLegacy == nil || issLegacy.Status != domain.StatusDismissed {
		t.Errorf("expected iss-legacy to be dismissed, got status %s", issLegacy.Status)
	}

	// Verify PR synced flag was set
	if !prSvc.syncedFlags["org-1:repo-1:42"] {
		t.Error("expected PR synced flag to be set true")
	}

	// Verify age calculation
	age := mgmt.AgeCalculation(time.Now().UTC().Add(-40 * time.Hour))
	if age != "2 days ago" {
		t.Errorf("expected 2 days ago, got %s", age)
	}
	age1 := mgmt.AgeCalculation(time.Now().UTC().Add(-2 * time.Hour))
	if age1 != "1 day ago" {
		t.Errorf("expected 1 day ago, got %s", age1)
	}
}

func TestIssuesUseCases(t *testing.T) {
	ctx := context.Background()

	repo := repositories.NewInMemoryIssuesRepository()
	issuesSvc := services.NewIssuesService(repo)
	prSvc := newMockPullRequestService()
	cache := newMockCacheService()
	auth := &mockAuthService{allowedRepos: []string{"repo-1"}}
	feedback := &mockFeedbackReader{
		feedbacks: []domain.FeedbackItem{
			{
				SuggestionID: "sugg-10",
				Reactions: domain.ReactionStats{
					ThumbsUp:   5,
					ThumbsDown: 1,
				},
			},
		},
	}

	mgmt := services.NewDrixyIssuesManagementService(
		nil,
		issuesSvc,
		prSvc,
		nil,
		nil,
		cache,
		nil,
	)

	// Seed test issue
	testIssue, _ := issuesSvc.Create(ctx, &domain.Issue{
		UUID:        "iss-10",
		Title:       "Potential deadlock in mutex",
		Description: "Locks acquired in reverse order",
		FilePath:    "concurrency/worker.go",
		Language:    "go",
		Label:       "bug",
		Severity:    domain.SeverityCritical,
		Status:      domain.StatusOpen,
		Repository: domain.RepositoryToIssues{
			ID:       "repo-1",
			Name:     "worker",
			FullName: "scandrix/worker",
			Platform: domain.PlatformGitHub,
		},
		OrganizationID: "org-1",
		ContributingSuggestions: []domain.ContributingSuggestion{
			{
				ID:       "sugg-10",
				PRNumber: 77,
				PRAuthor: domain.UserRef{GitID: "dev-1", Username: "alice"},
			},
		},
	})

	user := domain.UserRef{GitID: "user-1", Username: "reviewer"}

	// 1. GetIssueByIDUseCase
	getByIDUC := usecases.NewGetIssueByIDUseCase(issuesSvc, mgmt, feedback, auth)
	details, err := getByIDUC.Execute(ctx, "iss-10", user)
	if err != nil || details == nil {
		t.Fatalf("failed getting issue details: %v", err)
	}
	if details.Reactions.ThumbsUp != 5 || details.Reactions.ThumbsDown != 1 {
		t.Errorf("expected 5 thumbsUp, 1 thumbsDown, got %+v", details.Reactions)
	}
	if details.FileLink.URL != "https://github.com/scandrix/worker/blob/main/concurrency/worker.go" {
		t.Errorf("unexpected file URL: %s", details.FileLink.URL)
	}
	if len(details.PRLinks) != 1 || details.PRLinks[0].URL != "https://github.com/scandrix/worker/pull/77" {
		t.Errorf("unexpected PR links: %+v", details.PRLinks)
	}

	// 2. GetIssuesUseCase (with cache and scoping)
	getIssuesUC := usecases.NewGetIssuesUseCase(issuesSvc, cache, auth)
	issuesList, err := getIssuesUC.Execute(ctx, domain.GetIssuesFilter{OrganizationID: "org-1"}, user)
	if err != nil || len(issuesList) != 1 {
		t.Fatalf("expected 1 issue, got %d, err: %v", len(issuesList), err)
	}
	if len(issuesList[0].PRNumbers) != 1 || issuesList[0].PRNumbers[0] != "77" {
		t.Errorf("expected PR number 77 mapped, got %+v", issuesList[0].PRNumbers)
	}
	if issuesList[0].ContributingSuggestions != nil {
		t.Errorf("expected raw suggestions stripped from summary list")
	}

	// Verify cache was populated
	cached, found, _ := cache.Get(ctx, "issues_org-1")
	if !found || len(cached) != 1 {
		t.Errorf("expected issues to be cached")
	}

	// 3. GetTotalIssuesUseCase
	getTotalUC := usecases.NewGetTotalIssuesUseCase(issuesSvc, mgmt, auth)
	total, err := getTotalUC.Execute(ctx, domain.GetIssuesFilter{OrganizationID: "org-1", Severity: "critical"}, user)
	if err != nil || total != 1 {
		t.Fatalf("expected total 1, got %d, err: %v", total, err)
	}

	// 4. UpdateIssuePropertyUseCase
	updatePropUC := usecases.NewUpdateIssuePropertyUseCase(issuesSvc, mgmt, auth, nil)
	updated, err := updatePropUC.Execute(ctx, testIssue.UUID, domain.FieldStatus, string(domain.StatusResolved), user)
	if err != nil || updated.Status != domain.StatusResolved {
		t.Fatalf("failed updating issue status: %v", err)
	}

	// Verify cache was invalidated
	_, foundAfterUpdate, _ := cache.Get(ctx, "issues_org-1")
	if foundAfterUpdate {
		t.Error("expected cache to be invalidated after property update")
	}
}

func TestExternalIssueTrackingSync(t *testing.T) {
	ctx := context.Background()

	testIssue := &domain.Issue{
		UUID:     "iss-sync-1",
		Title:    "Race condition in goroutine pool",
		Status:   domain.StatusOpen,
		Severity: domain.SeverityHigh,
		Repository: domain.RepositoryToIssues{
			ID:       "repo-1",
			Name:     "core-pool",
			FullName: "scandrix/core-pool",
			Platform: domain.PlatformGitHub,
		},
	}

	// 1. Jira Tracker
	jira := issuesync.NewJiraIssueTracker("https://jira.scandrix.dev", "SCAN")
	jiraRef, err := jira.CreateIssue(ctx, testIssue)
	if err != nil || jiraRef == nil {
		t.Fatalf("failed creating Jira issue: %v", err)
	}
	if jiraRef.ExternalKey != "SCAN-101" || jiraRef.ExternalURL != "https://jira.scandrix.dev/browse/SCAN-101" {
		t.Errorf("unexpected Jira ref: %+v", jiraRef)
	}
	_ = jira.UpdateIssueStatus(ctx, jiraRef.ExternalID, domain.StatusResolved)
	jGet, _ := jira.GetIssue(ctx, jiraRef.ExternalID)
	if jGet.Status != string(domain.StatusResolved) {
		t.Errorf("expected Jira status resolved, got %s", jGet.Status)
	}

	// 2. Linear Tracker
	linear := issuesync.NewLinearIssueTracker("https://linear.app", "SDX")
	linearRef, err := linear.CreateIssue(ctx, testIssue)
	if err != nil || linearRef == nil {
		t.Fatalf("failed creating Linear issue: %v", err)
	}
	if linearRef.ExternalKey != "SDX-1" || linearRef.ExternalURL != "https://linear.app/issue/SDX-1" {
		t.Errorf("unexpected Linear ref: %+v", linearRef)
	}

	// 3. GitHub Tracker
	gh := issuesync.NewGitHubIssueTracker()
	ghRef, err := gh.CreateIssue(ctx, testIssue)
	if err != nil || ghRef == nil {
		t.Fatalf("failed creating GitHub issue: %v", err)
	}
	if ghRef.ExternalKey != "#1" || ghRef.ExternalURL != "https://github.com/scandrix/core-pool/issues/1" {
		t.Errorf("unexpected GitHub ref: %+v", ghRef)
	}

	// 4. IssueSyncManager
	mgr := issuesync.NewIssueSyncManager()
	mgr.RegisterTracker(jira)
	mgr.RegisterTracker(linear)
	mgr.RegisterTracker(gh)

	syncedRef, err := mgr.SyncIssue(ctx, domain.TrackerLinear, testIssue)
	if err != nil || syncedRef.TrackerType != domain.TrackerLinear {
		t.Fatalf("failed syncing issue via manager: %v", err)
	}
	err = mgr.UpdateStatus(ctx, domain.TrackerLinear, syncedRef.ExternalID, domain.StatusResolved)
	if err != nil {
		t.Errorf("failed updating status via manager: %v", err)
	}
}
