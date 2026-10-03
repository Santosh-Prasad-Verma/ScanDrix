package factory

import (
	"context"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/platform/domain/types"
	"github.com/scandrix/backend/pkg/models"
)

type dummyPlatformService struct {
	mockCodeManagementService
	getPRCalled             bool
	createCommentCalled     bool
	approveCalled           bool
	resolveAuthorCalled     bool
	getRecentCommentsCalled bool
}

func (d *dummyPlatformService) ResolveMrAuthorFromWebhookPayload(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	payload any,
) (*types.PullRequestUser, error) {
	d.resolveAuthorCalled = true
	return &types.PullRequestUser{
		ID:       "101",
		Username: "scandrix-dev",
		Name:     "ScanDrix Engineer",
	}, nil
}

func (d *dummyPlatformService) GetRecentRepositoryComments(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo types.RepositoryDescriptor,
	limit int,
) ([]*types.PullRequestReviewComment, error) {
	d.getRecentCommentsCalled = true
	return []*types.PullRequestReviewComment{
		{
			ID:        "rc-1",
			Body:      "Please check error handling",
			Path:      "pkg/auth/auth.go",
			CreatedAt: "2026-09-17T01:00:00Z",
		},
	}, nil
}

func (d *dummyPlatformService) GetPullRequest(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
) (*types.PullRequest, error) {
	d.getPRCalled = true
	return &types.PullRequest{
		Number: prNumber,
		Title:  "Resolved via Dynamic Router",
		State:  "open",
	}, nil
}

func (d *dummyPlatformService) CreateReviewComment(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
	comment types.PullRequestReviewComment,
) (*types.PullRequestReviewComment, error) {
	d.createCommentCalled = true
	return &types.PullRequestReviewComment{
		ID:   "cmt-123",
		Body: comment.Body,
	}, nil
}

func (d *dummyPlatformService) ApprovePullRequest(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
	message string,
) error {
	d.approveCalled = true
	return nil
}

func (d *dummyPlatformService) GetCloneParams(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo types.RepositoryDescriptor,
) (*types.GitCloneParams, error) {
	return &types.GitCloneParams{
		URL:      "https://git.scandrix.dev/org/repo.git",
		Provider: d.provider,
	}, nil
}

func TestCodeManagementService_Routing(t *testing.T) {
	fact := NewPlatformIntegrationFactory()

	ghDummy := &dummyPlatformService{mockCodeManagementService: mockCodeManagementService{provider: models.ProviderGitHub}}
	glDummy := &dummyPlatformService{mockCodeManagementService: mockCodeManagementService{provider: models.ProviderGitLab}}
	bbDummy := &dummyPlatformService{mockCodeManagementService: mockCodeManagementService{provider: models.ProviderBitbucket}}
	azDummy := &dummyPlatformService{mockCodeManagementService: mockCodeManagementService{provider: models.ProviderAzure}}
	fgDummy := &dummyPlatformService{mockCodeManagementService: mockCodeManagementService{provider: models.ProviderForgejo}}

	fact.RegisterCodeManagementService(models.ProviderGitHub, ghDummy)
	fact.RegisterCodeManagementService(models.ProviderGitLab, glDummy)
	fact.RegisterCodeManagementService(models.ProviderBitbucket, bbDummy)
	fact.RegisterCodeManagementService(models.ProviderAzure, azDummy)
	fact.RegisterCodeManagementService(models.ProviderForgejo, fgDummy)

	router := NewCodeManagementService(fact, nil)
	ctx := context.Background()

	// 1. Route to GitHub via orgData.Provider
	ghData := types.OrganizationAndTeamData{Provider: "github"}
	repo := &types.RepositoryDescriptor{Owner: "org", Name: "repo"}

	pr, err := router.GetPullRequest(ctx, ghData, repo, 42)
	if err != nil || pr == nil || !ghDummy.getPRCalled {
		t.Fatalf("expected GitHub service invocation, got pr: %+v, err: %v", pr, err)
	}

	// 2. Route to GitLab via credentials["provider"]
	glData := types.OrganizationAndTeamData{
		IntegrationCredentials: map[string]any{"provider": "gitlab"},
	}
	_, err = router.CreateReviewComment(ctx, glData, repo, 10, types.PullRequestReviewComment{Body: "LGTM"})
	if err != nil || !glDummy.createCommentCalled {
		t.Fatalf("expected GitLab service comment invocation, err: %v", err)
	}

	// 3. Route to Azure via credentials["platform"]
	azData := types.OrganizationAndTeamData{
		IntegrationCredentials: map[string]any{"platform": "azure"},
	}
	err = router.ApprovePullRequest(ctx, azData, repo, 5, "Approved")
	if err != nil || !azDummy.approveCalled {
		t.Fatalf("expected Azure service approve invocation, err: %v", err)
	}

	// 4. Route to Forgejo via orgData.Provider
	fgData := types.OrganizationAndTeamData{Provider: "forgejo"}
	cloneParams, err := router.GetCloneParams(ctx, fgData, *repo)
	if err != nil || cloneParams == nil || cloneParams.Provider != models.ProviderForgejo {
		t.Fatalf("expected Forgejo clone params, got %+v, err: %v", cloneParams, err)
	}

	// 5. Route ResolveMrAuthorFromWebhookPayload via GitHub
	author, err := router.ResolveMrAuthorFromWebhookPayload(ctx, ghData, map[string]any{"user": "scandrix-dev"})
	if err != nil || author == nil || !ghDummy.resolveAuthorCalled || author.Username != "scandrix-dev" {
		t.Fatalf("expected ResolveMrAuthorFromWebhookPayload routing, got author: %+v, err: %v", author, err)
	}

	// 6. Route GetRecentRepositoryComments via GitLab
	recentComments, err := router.GetRecentRepositoryComments(ctx, glData, *repo, 20)
	if err != nil || len(recentComments) == 0 || !glDummy.getRecentCommentsCalled {
		t.Fatalf("expected GetRecentRepositoryComments routing, got comments: %+v, err: %v", recentComments, err)
	}

	// 7. Test FormatReviewCommentBody standard branding
	body := router.FormatReviewCommentBody("Fix bounds check", "go", true, true)
	if !strings.Contains(body, "<!-- drixy-codereview -->") {
		t.Fatalf("expected drixy review marker, got: %s", body)
	}
	if !strings.Contains(body, "https://scandrix.dev") {
		t.Fatalf("expected scandrix.dev url, got: %s", body)
	}
}

func TestCodeManagementService_UnresolvableProvider(t *testing.T) {
	fact := NewPlatformIntegrationFactory()
	router := NewCodeManagementService(fact, nil)
	ctx := context.Background()

	emptyData := types.OrganizationAndTeamData{}
	repo := &types.RepositoryDescriptor{Owner: "org", Name: "repo"}

	// Verify that unresolvable provider returns graceful error
	_, err := router.GetPullRequest(ctx, emptyData, repo, 1)
	if err == nil {
		t.Error("expected error for unresolvable provider")
	}

	status, err := router.VerifyConnection(ctx, emptyData)
	if err == nil || status.HasConnection {
		t.Errorf("expected failed connection status, got: %+v", status)
	}
}
