package usecases

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/platform/application/services"
	"github.com/scandrix/backend/internal/platform/domain/contracts"
	"github.com/scandrix/backend/internal/platform/domain/types"
	"github.com/scandrix/backend/internal/platform/dtos"
	"github.com/scandrix/backend/pkg/models"
)

// Base mock implementing ICodeManagementService for usecase tests
type mockCodeManagementFull struct {
	contracts.ICodeManagementService

	getPRsFunc        func(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, state, author, branch string) ([]*types.PullRequest, error)
	getPRFunc         func(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) (*types.PullRequest, error)
	createCommentFunc func(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, body string) (*types.PullRequestReviewComment, error)
	getReposFunc      func(ctx context.Context, orgData types.OrganizationAndTeamData, archived *bool, visibility, language string) ([]*types.Repositories, error)
	getMembersFunc    func(ctx context.Context, orgData types.OrganizationAndTeamData) ([]types.PullRequestAuthor, error)
	getAuthorsFunc    func(ctx context.Context, orgData types.OrganizationAndTeamData, determineBots bool) ([]types.PullRequestAuthor, error)
	getUserByIDFunc   func(ctx context.Context, orgData types.OrganizationAndTeamData, userID string) (*types.PullRequestUser, error)
	getUserByUnameFunc func(ctx context.Context, orgData types.OrganizationAndTeamData, username string) (*types.PullRequestUser, error)
	getUserByEmailFunc func(ctx context.Context, orgData types.OrganizationAndTeamData, email, userName string) (*types.PullRequestUser, error)
	getCurrentUserFunc func(ctx context.Context, orgData types.OrganizationAndTeamData) (*types.PullRequestUser, error)
	isWebhookActFunc  func(ctx context.Context, orgData types.OrganizationAndTeamData, repositoryID string) (bool, error)
	deleteWebhookFunc func(ctx context.Context, orgData types.OrganizationAndTeamData) error
	getTreeDirFunc    func(ctx context.Context, orgData types.OrganizationAndTeamData, repositoryID, directoryPath string) ([]*types.TreeItem, error)
	addReactionToCommentFunc       func(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, prNumber int, commentID int64, reaction string) error
	removeReactionsFromCommentFunc func(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, prNumber int, commentID int64, reactions []string) error
	createResponseFunc             func(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, parentID, body string) (*types.PullRequestReviewComment, error)
	getPRCommentsFunc              func(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]*types.PullRequestReviewComment, error)
	createIssueCommentFunc         func(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, body string) (*types.PullRequestReviewComment, error)
	updateIssueCommentFunc         func(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, commentID string, body string) error
	getCloneParamsFunc             func(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor) (*types.GitCloneParams, error)
}

func (m *mockCodeManagementFull) Provider() models.SCMProvider { return models.ProviderGitHub }

func (m *mockCodeManagementFull) GetPullRequests(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, state, author, branch string) ([]*types.PullRequest, error) {
	if m.getPRsFunc != nil {
		return m.getPRsFunc(ctx, orgData, repo, state, author, branch)
	}
	return nil, nil
}

func (m *mockCodeManagementFull) GetPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) (*types.PullRequest, error) {
	if m.getPRFunc != nil {
		return m.getPRFunc(ctx, orgData, repo, prNumber)
	}
	return &types.PullRequest{Number: prNumber, Title: "Test PR", State: "open"}, nil
}

func (m *mockCodeManagementFull) CreateSingleIssueComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, body string) (*types.PullRequestReviewComment, error) {
	if m.createCommentFunc != nil {
		return m.createCommentFunc(ctx, orgData, repo, prNumber, body)
	}
	return &types.PullRequestReviewComment{ID: "cmt-123", Body: body}, nil
}

func (m *mockCodeManagementFull) GetRepositories(ctx context.Context, orgData types.OrganizationAndTeamData, archived *bool, visibility, language string) ([]*types.Repositories, error) {
	if m.getReposFunc != nil {
		return m.getReposFunc(ctx, orgData, archived, visibility, language)
	}
	return nil, nil
}

func (m *mockCodeManagementFull) GetListMembers(ctx context.Context, orgData types.OrganizationAndTeamData) ([]types.PullRequestAuthor, error) {
	if m.getMembersFunc != nil {
		return m.getMembersFunc(ctx, orgData)
	}
	return nil, nil
}

func (m *mockCodeManagementFull) GetPullRequestAuthors(ctx context.Context, orgData types.OrganizationAndTeamData, determineBots bool) ([]types.PullRequestAuthor, error) {
	if m.getAuthorsFunc != nil {
		return m.getAuthorsFunc(ctx, orgData, determineBots)
	}
	return nil, nil
}

func (m *mockCodeManagementFull) GetUserByID(ctx context.Context, orgData types.OrganizationAndTeamData, userID string) (*types.PullRequestUser, error) {
	if m.getUserByIDFunc != nil {
		return m.getUserByIDFunc(ctx, orgData, userID)
	}
	return nil, nil
}

func (m *mockCodeManagementFull) GetUserByUsername(ctx context.Context, orgData types.OrganizationAndTeamData, username string) (*types.PullRequestUser, error) {
	if m.getUserByUnameFunc != nil {
		return m.getUserByUnameFunc(ctx, orgData, username)
	}
	return nil, nil
}

func (m *mockCodeManagementFull) GetUserByEmailOrName(ctx context.Context, orgData types.OrganizationAndTeamData, email, userName string) (*types.PullRequestUser, error) {
	if m.getUserByEmailFunc != nil {
		return m.getUserByEmailFunc(ctx, orgData, email, userName)
	}
	return nil, nil
}

func (m *mockCodeManagementFull) GetCurrentUser(ctx context.Context, orgData types.OrganizationAndTeamData) (*types.PullRequestUser, error) {
	if m.getCurrentUserFunc != nil {
		return m.getCurrentUserFunc(ctx, orgData)
	}
	return &types.PullRequestUser{ID: "usr-current", Username: "lead-dev"}, nil
}

func (m *mockCodeManagementFull) IsWebhookActive(ctx context.Context, orgData types.OrganizationAndTeamData, repositoryID string) (bool, error) {
	if m.isWebhookActFunc != nil {
		return m.isWebhookActFunc(ctx, orgData, repositoryID)
	}
	return true, nil
}

func (m *mockCodeManagementFull) DeleteWebhook(ctx context.Context, orgData types.OrganizationAndTeamData) error {
	if m.deleteWebhookFunc != nil {
		return m.deleteWebhookFunc(ctx, orgData)
	}
	return nil
}

func (m *mockCodeManagementFull) GetRepositoryTreeByDirectory(ctx context.Context, orgData types.OrganizationAndTeamData, repositoryID, directoryPath string) ([]*types.TreeItem, error) {
	if m.getTreeDirFunc != nil {
		return m.getTreeDirFunc(ctx, orgData, repositoryID, directoryPath)
	}
	return nil, nil
}

func (m *mockCodeManagementFull) AddReactionToComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, prNumber int, commentID int64, reaction string) error {
	if m.addReactionToCommentFunc != nil {
		return m.addReactionToCommentFunc(ctx, orgData, repo, prNumber, commentID, reaction)
	}
	return nil
}

func (m *mockCodeManagementFull) RemoveReactionsFromComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, prNumber int, commentID int64, reactions []string) error {
	if m.removeReactionsFromCommentFunc != nil {
		return m.removeReactionsFromCommentFunc(ctx, orgData, repo, prNumber, commentID, reactions)
	}
	return nil
}

func (m *mockCodeManagementFull) CreateResponseToComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, parentCommentID, body string) (*types.PullRequestReviewComment, error) {
	if m.createResponseFunc != nil {
		return m.createResponseFunc(ctx, orgData, repo, prNumber, parentCommentID, body)
	}
	return &types.PullRequestReviewComment{ID: "resp-mock", Body: body}, nil
}

func (m *mockCodeManagementFull) GetPullRequestReviewComments(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]*types.PullRequestReviewComment, error) {
	if m.getPRCommentsFunc != nil {
		return m.getPRCommentsFunc(ctx, orgData, repo, prNumber)
	}
	return []*types.PullRequestReviewComment{}, nil
}

func (m *mockCodeManagementFull) CreateIssueComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, body string) (*types.PullRequestReviewComment, error) {
	if m.createIssueCommentFunc != nil {
		return m.createIssueCommentFunc(ctx, orgData, repo, prNumber, body)
	}
	return &types.PullRequestReviewComment{ID: "issue-comment-mock", Body: body}, nil
}

func (m *mockCodeManagementFull) UpdateIssueComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, commentID string, body string) error {
	if m.updateIssueCommentFunc != nil {
		return m.updateIssueCommentFunc(ctx, orgData, repo, commentID, body)
	}
	return nil
}

func (m *mockCodeManagementFull) GetCloneParams(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor) (*types.GitCloneParams, error) {
	if m.getCloneParamsFunc != nil {
		return m.getCloneParamsFunc(ctx, orgData, repo)
	}
	return &types.GitCloneParams{
		URL:    "https://github.com/org/repo.git",
		Branch: "main",
	}, nil
}

func (m *mockCodeManagementFull) ResolveMrAuthorFromWebhookPayload(ctx context.Context, orgData types.OrganizationAndTeamData, payload any) (*types.PullRequestUser, error) {
	return nil, nil
}

func (m *mockCodeManagementFull) GetRecentRepositoryComments(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, limit int) ([]*types.PullRequestReviewComment, error) {
	return []*types.PullRequestReviewComment{}, nil
}


// ═══════════════════════════════════════════════════════════════
// Test 1: GetPRsUseCase
// ═══════════════════════════════════════════════════════════════

func TestGetPRsUseCase(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	mockCM := &mockCodeManagementFull{
		getPRsFunc: func(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, state, author, branch string) ([]*types.PullRequest, error) {
			var list []*types.PullRequest
			for i := 1; i <= 25; i++ {
				list = append(list, &types.PullRequest{
					ID:         fmt.Sprintf("pr-%d", i),
					Number:     i,
					Title:      fmt.Sprintf("Feature %d", i),
					PRURL:      fmt.Sprintf("https://github.com/org/repo/pull/%d", i),
					CreatedAt:  now.Format(time.RFC3339),
					Repository: "repo-alpha",
					RepositoryData: types.RepositoryDescriptor{
						ID:   "repo-1",
						Name: "repo-alpha",
					},
				})
			}
			return list, nil
		},
	}

	uc := NewGetPRsUseCase(mockCM, nil)

	t.Run("caps at 20 PRs per repository", func(t *testing.T) {
		res, err := uc.Execute(ctx, GetPRsParams{
			OrganizationID: "org-1",
			TeamID:         "team-1",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 20 {
			t.Errorf("expected capped at 20 PRs, got %d", len(res))
		}
	})

	t.Run("filters by PR title", func(t *testing.T) {
		titleFilter := "Feature 10"
		res, err := uc.Execute(ctx, GetPRsParams{
			OrganizationID: "org-1",
			TeamID:         "team-1",
			Filters: GetPRsFilters{
				Title: &titleFilter,
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 1 || res[0].PullNumber != 10 {
			t.Errorf("expected exactly PR #10, got %v", res)
		}
	})
}

// ═══════════════════════════════════════════════════════════════
// Test 2: CreatePRCodeReviewUseCase (@drixy start-review)
// ═══════════════════════════════════════════════════════════════

type mockAutomationRecorder struct {
	lastStatus string
	lastPR     int
}

func (r *mockAutomationRecorder) RecordAutomationExecution(ctx context.Context, teamID string, status string, prNumber int, repoID string, err error) error {
	r.lastStatus = status
	r.lastPR = prNumber
	return nil
}

func TestCreatePRCodeReviewUseCase(t *testing.T) {
	ctx := context.Background()
	recorder := &mockAutomationRecorder{}

	var postedComment string
	mockCM := &mockCodeManagementFull{
		createCommentFunc: func(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, body string) (*types.PullRequestReviewComment, error) {
			postedComment = body
			return &types.PullRequestReviewComment{ID: "c-101", Body: body}, nil
		},
	}

	uc := NewCreatePRCodeReviewUseCase(mockCM, recorder)

	res, err := uc.Execute(ctx, CreatePRCodeReviewParams{
		OrganizationID: "org-1",
		TeamID:         "team-1",
		RepositoryID:   "repo-1",
		RepositoryName: "backend",
		PullNumber:     42,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !res.Success || res.CommentID != "c-101" {
		t.Errorf("expected success with comment ID c-101, got %+v", res)
	}

	if postedComment != "@drixy start-review" {
		t.Errorf("expected command body '@drixy start-review', got %q", postedComment)
	}

	if recorder.lastStatus != "SUCCESS" || recorder.lastPR != 42 {
		t.Errorf("expected recorder status SUCCESS for PR 42, got %s / %d", recorder.lastStatus, recorder.lastPR)
	}
}

// ═══════════════════════════════════════════════════════════════
// Test 3: Repositories Use Cases
// ═══════════════════════════════════════════════════════════════

type mockRepoConfigStore struct {
	savedRepos []*types.Repositories
	teamStatus string
}

func (s *mockRepoConfigStore) SaveRepositories(ctx context.Context, orgID, teamID string, repos []*types.Repositories) error {
	s.savedRepos = repos
	return nil
}

func (s *mockRepoConfigStore) GetRepositories(ctx context.Context, orgID, teamID string) ([]*types.Repositories, error) {
	return s.savedRepos, nil
}

func (s *mockRepoConfigStore) UpdateTeamStatus(ctx context.Context, teamID string, status string) error {
	s.teamStatus = status
	return nil
}

func TestRepositoriesUseCases(t *testing.T) {
	ctx := context.Background()

	mockCM := &mockCodeManagementFull{
		getReposFunc: func(ctx context.Context, orgData types.OrganizationAndTeamData, archived *bool, visibility, language string) ([]*types.Repositories, error) {
			return []*types.Repositories{
				{ID: "r1", Name: "repo-1", Selected: true},
				{ID: "r2", Name: "repo-2", Selected: false},
				{ID: "r3", Name: "repo-3", Selected: true},
			}, nil
		},
		getTreeDirFunc: func(ctx context.Context, orgData types.OrganizationAndTeamData, repositoryID, directoryPath string) ([]*types.TreeItem, error) {
			return []*types.TreeItem{
				{Path: "src", Type: "dir", SHA: "sha-src"},
				{Path: "main.go", Type: "file", SHA: "sha-main"},
			}, nil
		},
	}

	t.Run("GetRepositories with isSelected filter and pagination", func(t *testing.T) {
		uc := NewGetRepositoriesUseCase(mockCM, nil)
		selectedOnly := true
		page := 1
		perPage := 10

		resp, err := uc.Execute(ctx, GetRepositoriesParams{
			OrganizationID: "org-1",
			TeamID:         "team-1",
			IsSelected:     &selectedOnly,
			Page:           &page,
			PerPage:        &perPage,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resp.Pagination.Total != 2 || len(resp.Data) != 2 {
			t.Errorf("expected 2 selected repos, got total=%d len=%d", resp.Pagination.Total, len(resp.Data))
		}
	})

	t.Run("CreateRepositories saves repos and marks team ACTIVE", func(t *testing.T) {
		store := &mockRepoConfigStore{}
		uc := NewCreateRepositoriesUseCase(store, nil, nil)

		repos := []*types.Repositories{
			{ID: "r1", Name: "repo-1"},
		}

		err := uc.Execute(ctx, CreateRepositoriesParams{
			OrganizationID: "org-1",
			TeamID:         "team-1",
			Repositories:   repos,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(store.savedRepos) != 1 || store.teamStatus != "ACTIVE" {
			t.Errorf("expected savedRepos=1 and teamStatus=ACTIVE, got %d / %s", len(store.savedRepos), store.teamStatus)
		}
	})

	t.Run("GetRepositoryTreeByDirectory formats items and parent", func(t *testing.T) {
		uc := NewGetRepositoryTreeByDirectoryUseCase(mockCM, nil)

		dto := dtos.NewGetRepositoryTreeByDirectoryDTO("team-1", "repo-1", "pkg/models")
		resp, err := uc.Execute(ctx, "org-1", dto)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resp.CurrentPath != "pkg/models" {
			t.Errorf("expected current path pkg/models, got %s", resp.CurrentPath)
		}
		if resp.ParentPath == nil || *resp.ParentPath != "pkg" {
			t.Errorf("expected parent path pkg, got %v", resp.ParentPath)
		}
		if len(resp.Directories) != 2 {
			t.Errorf("expected 2 items, got %d", len(resp.Directories))
		}
	})
}

// ═══════════════════════════════════════════════════════════════
// Test 4: Member & User Use Cases
// ═══════════════════════════════════════════════════════════════

func TestMemberAndUserUseCases(t *testing.T) {
	ctx := context.Background()

	mockCM := &mockCodeManagementFull{
		getCurrentUserFunc: func(ctx context.Context, orgData types.OrganizationAndTeamData) (*types.PullRequestUser, error) {
			return &types.PullRequestUser{
				ID:        "usr-current",
				Name:      "Principal Engineer",
				Username:  "principal",
				Email:     "lead@scandrix.dev",
				AvatarURL: "https://scandrix.dev/avatar.png",
			}, nil
		},
		getUserByIDFunc: func(ctx context.Context, orgData types.OrganizationAndTeamData, userID string) (*types.PullRequestUser, error) {
			return &types.PullRequestUser{ID: userID, Name: "Found By ID", Username: "user_" + userID}, nil
		},
		getUserByUnameFunc: func(ctx context.Context, orgData types.OrganizationAndTeamData, username string) (*types.PullRequestUser, error) {
			return &types.PullRequestUser{ID: "id-" + username, Name: "Found By Username", Username: username}, nil
		},
		isWebhookActFunc: func(ctx context.Context, orgData types.OrganizationAndTeamData, repositoryID string) (bool, error) {
			return repositoryID == "repo-active", nil
		},
	}

	t.Run("GetCurrentCodeManagementUser normalizes correctly", func(t *testing.T) {
		uc := NewGetCurrentCodeManagementUserUseCase(mockCM)
		user, err := uc.Execute(ctx, types.OrganizationAndTeamData{OrganizationID: "org-1", TeamID: "team-1"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if user.Email != "lead@scandrix.dev" || user.Username != "principal" {
			t.Errorf("unexpected user values: %+v", user)
		}
	})

	t.Run("SearchCodeManagementUsers resolves by ID and query", func(t *testing.T) {
		uc := NewSearchCodeManagementUsersUseCase(mockCM, nil)
		users, err := uc.Execute(ctx, SearchUsersParams{
			OrganizationID: "org-1",
			TeamID:         "team-1",
			UserID:         "101",
			Query:          "dev_lead",
			Limit:          5,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(users) != 2 {
			t.Fatalf("expected 2 users (1 by ID, 1 by query), got %d", len(users))
		}
		if users[0].Source != "id" || users[1].Source != "username" {
			t.Errorf("unexpected sources: %s, %s", users[0].Source, users[1].Source)
		}
	})

	t.Run("GetWebhookStatus returns active status", func(t *testing.T) {
		uc := NewGetWebhookStatusUseCase(mockCM)
		active, err := uc.Execute(ctx, types.OrganizationAndTeamData{OrganizationID: "org-1", TeamID: "team-1"}, "repo-active")
		if err != nil || !active {
			t.Errorf("expected active=true for repo-active, got %v, err=%v", active, err)
		}

		inactive, err := uc.Execute(ctx, types.OrganizationAndTeamData{OrganizationID: "org-1", TeamID: "team-1"}, "repo-other")
		if err != nil || inactive {
			t.Errorf("expected active=false for repo-other, got %v, err=%v", inactive, err)
		}
	})
}

// ═══════════════════════════════════════════════════════════════
// Test 5: FinishOnboardingUseCase
// ═══════════════════════════════════════════════════════════════

type mockPlatformConfigStore struct {
	finishOnboardSet bool
}

func (s *mockPlatformConfigStore) SetFinishOnboard(ctx context.Context, orgID, teamID string) error {
	s.finishOnboardSet = true
	return nil
}

func (s *mockPlatformConfigStore) GetTeamAndOrgName(ctx context.Context, teamID string) (string, string, error) {
	return "Engineering", "ScanDrix Corp", nil
}

type mockTrialProvisioner struct {
	provisioned bool
}

func (p *mockTrialProvisioner) ProvisionTrial(ctx context.Context, orgID, teamID string) error {
	p.provisioned = true
	return nil
}

func TestFinishOnboardingUseCase(t *testing.T) {
	ctx := context.Background()
	configStore := &mockPlatformConfigStore{}
	trialProv := &mockTrialProvisioner{}

	var reviewCommentBody string
	mockCM := &mockCodeManagementFull{
		createCommentFunc: func(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, body string) (*types.PullRequestReviewComment, error) {
			reviewCommentBody = body
			return &types.PullRequestReviewComment{ID: "c-first-pr", Body: body}, nil
		},
		getMembersFunc: func(ctx context.Context, orgData types.OrganizationAndTeamData) ([]types.PullRequestAuthor, error) {
			return []types.PullRequestAuthor{
				{ID: "u1", Name: "Engineer One"},
				{ID: "u2", Name: "Engineer Two"},
			}, nil
		},
	}

	reviewUC := NewCreatePRCodeReviewUseCase(mockCM, nil)
	uc := NewFinishOnboardingUseCase(configStore, trialProv, nil, reviewUC, mockCM, nil)

	repoID := "repo-first"
	repoName := "core-app"
	prNum := 7

	dto := dtos.FinishOnboardingDTO{
		TeamID:         "team-123",
		ReviewPR:       true,
		RepositoryID:   &repoID,
		RepositoryName: &repoName,
		PullNumber:     &prNum,
	}

	err := uc.Execute(ctx, "org-456", "user-1", "user@scandrix.dev", dto)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !configStore.finishOnboardSet {
		t.Errorf("expected finishOnboardSet to be true")
	}

	if !trialProv.provisioned {
		t.Errorf("expected trial provisioned to be true")
	}

	if reviewCommentBody != "@drixy start-review" {
		t.Errorf("expected review comment body '@drixy start-review', got %q", reviewCommentBody)
	}
}

// ═══════════════════════════════════════════════════════════════
// Test 6: TriggerBusinessValidationUseCase
// ═══════════════════════════════════════════════════════════════

type mockBusinessValidationAgent struct {
	receivedCommand string
	receivedMode    string
}

func (a *mockBusinessValidationAgent) ValidateBusinessRules(ctx context.Context, orgData types.OrganizationAndTeamData, execCtx BusinessValidationExecutionContext, command string) (string, error) {
	a.receivedCommand = command
	a.receivedMode = execCtx.Mode
	return "PASS: All acceptance criteria verified.", nil
}

func TestTriggerBusinessValidationUseCase(t *testing.T) {
	ctx := context.Background()
	agent := &mockBusinessValidationAgent{}

	mockCM := &mockCodeManagementFull{
		getPRFunc: func(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) (*types.PullRequest, error) {
			return &types.PullRequest{
				Number: prNumber,
				Title:  "Implement Payment Gateway",
				Body:   "Given a valid card, when checkout submitted, then charge succeeded",
				PRURL:  "https://github.com/org/repo/pull/12",
				RepositoryData: types.RepositoryDescriptor{
					ID:   "repo-1",
					Name: "payments-service",
				},
			}, nil
		},
	}

	uc := NewTriggerBusinessValidationUseCase(mockCM, agent)
	orgData := types.OrganizationAndTeamData{OrganizationID: "org-1", TeamID: "team-1"}

	t.Run("valid PR input produces @drixy -v business-logic command", func(t *testing.T) {
		prNum := 12
		res, err := uc.Execute(ctx, orgData, TriggerBusinessValidationInput{
			PRNumber:     &prNum,
			RepositoryID: "repo-1",
			TaskID:       "PROJ-456",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !res.Accepted || res.Mode != "pull_request" {
			t.Errorf("expected accepted pull_request mode, got %+v", res)
		}

		if res.Command != "@drixy -v business-logic PROJ-456" {
			t.Errorf("expected command '@drixy -v business-logic PROJ-456', got %q", res.Command)
		}

		if agent.receivedCommand != "@drixy -v business-logic PROJ-456" || agent.receivedMode != "pull_request" {
			t.Errorf("agent received mismatch: cmd=%s mode=%s", agent.receivedCommand, agent.receivedMode)
		}
	})

	t.Run("local diff mode", func(t *testing.T) {
		res, err := uc.Execute(ctx, orgData, TriggerBusinessValidationInput{
			Diff:    "+ func Charge() error { return nil }",
			TaskURL: "https://linear.app/issue/123",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if res.Mode != "local_diff" {
			t.Errorf("expected local_diff mode, got %s", res.Mode)
		}
		if res.Command != "@drixy -v business-logic https://linear.app/issue/123" {
			t.Errorf("expected command with task url, got %s", res.Command)
		}
	})

	t.Run("rejects ambiguous input (both PR and diff)", func(t *testing.T) {
		prNum := 5
		_, err := uc.Execute(ctx, orgData, TriggerBusinessValidationInput{
			PRNumber: &prNum,
			Diff:     "+ diff",
		})
		if err == nil {
			t.Errorf("expected error when both PR and diff provided")
		}
	})
}

// ═══════════════════════════════════════════════════════════════
// Test 7: Integration Management & Seat Prune Use Cases
// ═══════════════════════════════════════════════════════════════

type mockLicenseService struct {
	activeSeats []string
	revoked     []string
}

func (l *mockLicenseService) GetLicensedUsers(ctx context.Context, orgID, teamID string) ([]string, error) {
	return l.activeSeats, nil
}

func (l *mockLicenseService) RevokeSeat(ctx context.Context, orgID, teamID, gitID string) error {
	l.revoked = append(l.revoked, gitID)
	return nil
}

func TestIntegrationManagementUseCases(t *testing.T) {
	ctx := context.Background()
	orgData := types.OrganizationAndTeamData{OrganizationID: "org-1", TeamID: "team-1"}

	t.Run("PruneRemovedLicenseSeats skips when members unavailable", func(t *testing.T) {
		mockCM := &mockCodeManagementFull{
			getMembersFunc: func(ctx context.Context, orgData types.OrganizationAndTeamData) ([]types.PullRequestAuthor, error) {
				return nil, errors.New("provider outage")
			},
			getAuthorsFunc: func(ctx context.Context, orgData types.OrganizationAndTeamData, determineBots bool) ([]types.PullRequestAuthor, error) {
				return nil, errors.New("provider outage")
			},
		}

		memberSvc := services.NewOrganizationMemberListService(mockCM, nil)
		licenseSvc := &mockLicenseService{activeSeats: []string{"user-1", "user-gone"}}

		uc := NewPruneRemovedLicenseSeatsUseCase(memberSvc, licenseSvc)

		res, err := uc.Execute(ctx, PruneRemovedLicenseSeatsParams{
			OrgData: orgData,
			DryRun:  false,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if res.Status != "members_unavailable" {
			t.Errorf("expected status members_unavailable, got %s", res.Status)
		}
		if len(licenseSvc.revoked) > 0 {
			t.Errorf("seats were revoked during outage! revoked: %v", licenseSvc.revoked)
		}
	})

	t.Run("PruneRemovedLicenseSeats revokes stale seats when members confirmed", func(t *testing.T) {
		mockCM := &mockCodeManagementFull{
			getMembersFunc: func(ctx context.Context, orgData types.OrganizationAndTeamData) ([]types.PullRequestAuthor, error) {
				return []types.PullRequestAuthor{
					{ID: "user-1", Name: "Alice Active"},
				}, nil
			},
		}

		memberSvc := services.NewOrganizationMemberListService(mockCM, nil)
		licenseSvc := &mockLicenseService{activeSeats: []string{"user-1", "user-gone"}}

		uc := NewPruneRemovedLicenseSeatsUseCase(memberSvc, licenseSvc)

		res, err := uc.Execute(ctx, PruneRemovedLicenseSeatsParams{
			OrgData: orgData,
			DryRun:  false,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if res.Status != "ok" {
			t.Errorf("expected status ok, got %s", res.Status)
		}
		if len(res.Candidates) != 1 || res.Candidates[0] != "user-gone" {
			t.Errorf("expected user-gone as candidate, got %v", res.Candidates)
		}
		if len(licenseSvc.revoked) != 1 || licenseSvc.revoked[0] != "user-gone" {
			t.Errorf("expected user-gone to be revoked, got %v", licenseSvc.revoked)
		}
	})
}
