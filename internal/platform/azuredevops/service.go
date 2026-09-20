package azuredevops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/platform/domain/contracts"
	"github.com/scandrix/backend/internal/platform/domain/types"
	"github.com/scandrix/backend/pkg/models"
)

// AzureDevOpsService implements contracts.ICodeManagementService for Azure DevOps Git (REST 7.0/7.1 API).
// Translates libs/platform/infrastructure/adapters/services/azureRepos/azureRepos.service.ts
var _ contracts.ICodeManagementService = (*AzureDevOpsService)(nil)

type AzureDevOpsService struct {
	requestHelper  *AzureReposRequestHelper
	defaultTimeout time.Duration
	cacheStore     sync.Map
}

// AzureDevOpsServiceConfig configures AzureDevOpsService instances.
type AzureDevOpsServiceConfig struct {
	HTTPClient     *http.Client
	DefaultTimeout time.Duration
}

// NewAzureDevOpsService creates an enterprise Azure DevOps service adapter.
func NewAzureDevOpsService(cfg AzureDevOpsServiceConfig) *AzureDevOpsService {
	timeout := cfg.DefaultTimeout
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	helper := NewAzureReposRequestHelper(cfg.HTTPClient, timeout)

	return &AzureDevOpsService{
		requestHelper:  helper,
		defaultTimeout: timeout,
	}
}

func (s *AzureDevOpsService) Provider() models.SCMProvider {
	return models.ProviderAzure
}

// -------------------------------------------------------------------------------------
// Credential and Path Resolution Helpers
// -------------------------------------------------------------------------------------

func (s *AzureDevOpsService) extractToken(orgData types.OrganizationAndTeamData) string {
	if orgData.AuthToken != "" {
		return orgData.AuthToken
	}
	if orgData.IntegrationCredentials != nil {
		if t, ok := orgData.IntegrationCredentials["token"].(string); ok && t != "" {
			return t
		}
		if t, ok := orgData.IntegrationCredentials["personalAccessToken"].(string); ok && t != "" {
			return t
		}
		if t, ok := orgData.IntegrationCredentials["pat"].(string); ok && t != "" {
			return t
		}
		if t, ok := orgData.IntegrationCredentials["accessToken"].(string); ok && t != "" {
			return t
		}
	}
	return ""
}

func (s *AzureDevOpsService) parseOrgProjectRepo(orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor) (orgName, projectID, repoID string) {
	if orgData.IntegrationCredentials != nil {
		if o, ok := orgData.IntegrationCredentials["org"].(string); ok && o != "" {
			orgName = o
		} else if o, ok := orgData.IntegrationCredentials["organization"].(string); ok && o != "" {
			orgName = o
		}
		if p, ok := orgData.IntegrationCredentials["project"].(string); ok && p != "" {
			projectID = p
		}
	}

	if repo == nil {
		return orgName, projectID, ""
	}

	repoID = repo.Name
	if repo.ID != "" {
		repoID = repo.ID
	}

	// repo.FullName may be formatted as: "org/project/repo" or "project/repo"
	parts := strings.Split(strings.Trim(repo.FullName, "/"), "/")
	if len(parts) == 3 {
		if orgName == "" {
			orgName = parts[0]
		}
		if projectID == "" {
			projectID = parts[1]
		}
		if repo.ID == "" {
			repoID = parts[2]
		}
	} else if len(parts) == 2 {
		if projectID == "" {
			projectID = parts[0]
		}
		if repo.ID == "" {
			repoID = parts[1]
		}
	}

	if repo.Owner != "" && projectID == "" {
		projectID = repo.Owner
	}

	if orgName == "" {
		orgName = orgData.OrganizationID
	}

	return orgName, projectID, repoID
}

// -------------------------------------------------------------------------------------
// Issues API (Azure DevOps uses Work Items, returns empty for generic issues)
// -------------------------------------------------------------------------------------

func (s *AzureDevOpsService) SupportsIssues(ctx context.Context, orgData types.OrganizationAndTeamData) (bool, error) {
	return false, nil
}

func (s *AzureDevOpsService) ListIssues(ctx context.Context, params types.ListIssuesParams) ([]types.CodeManagementIssue, error) {
	return []types.CodeManagementIssue{}, nil
}

func (s *AzureDevOpsService) GetIssue(ctx context.Context, params types.GetIssueParams) (*types.CodeManagementIssue, error) {
	return nil, nil
}

// -------------------------------------------------------------------------------------
// Repositories API
// -------------------------------------------------------------------------------------

func (s *AzureDevOpsService) FindRepositoryByName(ctx context.Context, orgData types.OrganizationAndTeamData, name string) (*types.Repository, error) {
	token := s.extractToken(orgData)
	org, project, repoSlug := s.parseOrgProjectRepo(orgData, &types.RepositoryDescriptor{FullName: name})

	repo, err := s.requestHelper.GetRepository(ctx, org, token, project, repoSlug)
	if err != nil {
		return nil, err
	}

	branch := strings.TrimPrefix(repo.DefaultBranch, "refs/heads/")
	if branch == "" {
		branch = "main"
	}

	return &types.Repository{
		ID:               repo.ID,
		Name:             repo.Name,
		FullName:         fmt.Sprintf("%s/%s", repo.Project.Name, repo.Name),
		DefaultBranch:    branch,
		HTMLURL:          repo.WebURL,
		CloneURL:         repo.RemoteURL,
		OrganizationName: repo.Project.Name,
		Provider:         models.ProviderAzure,
	}, nil
}

func (s *AzureDevOpsService) GetRepositories(ctx context.Context, orgData types.OrganizationAndTeamData, archived *bool, visibility, language string) ([]*types.Repositories, error) {
	token := s.extractToken(orgData)
	org, project, _ := s.parseOrgProjectRepo(orgData, nil)

	var projects []AzureProject
	var err error

	if project != "" {
		projects = []AzureProject{{ID: project, Name: project}}
	} else {
		projects, err = s.requestHelper.GetProjects(ctx, org, token)
		if err != nil {
			return nil, err
		}
	}

	var allRepos []*types.Repositories
	for _, p := range projects {
		repos, err := s.requestHelper.GetRepositories(ctx, org, token, p.ID)
		if err != nil {
			continue
		}

		for _, r := range repos {
			if r.IsDisabled {
				continue
			}

			branch := strings.TrimPrefix(r.DefaultBranch, "refs/heads/")
			if branch == "" {
				branch = "main"
			}

			allRepos = append(allRepos, &types.Repositories{
				ID:               r.ID,
				Name:             r.Name,
				FullName:         fmt.Sprintf("%s/%s", p.Name, r.Name),
				DefaultBranch:    branch,
				HTTPURL:          r.WebURL,
				OrganizationName: p.Name,
			})
		}
	}

	return allRepos, nil
}

func (s *AzureDevOpsService) GetOrganizations(ctx context.Context, orgData types.OrganizationAndTeamData) ([]*types.Organization, error) {
	org, _, _ := s.parseOrgProjectRepo(orgData, nil)
	return []*types.Organization{
		{
			ID:    org,
			Login: org,
			Name:  org,
		},
	}, nil
}

func (s *AzureDevOpsService) GetListMembers(ctx context.Context, orgData types.OrganizationAndTeamData) ([]types.PullRequestAuthor, error) {
	return []types.PullRequestAuthor{}, nil
}

func (s *AzureDevOpsService) VerifyConnection(ctx context.Context, orgData types.OrganizationAndTeamData) (*types.CodeManagementConnectionStatus, error) {
	token := s.extractToken(orgData)
	org, _, _ := s.parseOrgProjectRepo(orgData, nil)

	projects, err := s.requestHelper.GetProjects(ctx, org, token)
	if err != nil {
		return &types.CodeManagementConnectionStatus{
			HasConnection:   false,
			IsConnected:     false,
			IsSetupComplete: false,
			Message:         fmt.Sprintf("verification failed: %v", err),
			PlatformName:    string(models.ProviderAzure),
		}, nil
	}

	return &types.CodeManagementConnectionStatus{
		HasConnection:   true,
		IsConnected:     true,
		IsSetupComplete: true,
		Message:         fmt.Sprintf("connected to Azure DevOps (%d projects found)", len(projects)),
		PlatformName:    string(models.ProviderAzure),
	}, nil
}

func (s *AzureDevOpsService) GetDefaultBranch(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor) (string, error) {
	token := s.extractToken(orgData)
	org, project, repoID := s.parseOrgProjectRepo(orgData, repo)
	return s.requestHelper.GetDefaultBranch(ctx, org, token, project, repoID)
}

// -------------------------------------------------------------------------------------
// Pull Requests API
// -------------------------------------------------------------------------------------

func (s *AzureDevOpsService) GetPullRequests(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, state, author, branch string) ([]*types.PullRequest, error) {
	token := s.extractToken(orgData)
	org, project, repoID := s.parseOrgProjectRepo(orgData, repo)

	azureStatus := "active"
	if strings.EqualFold(state, "closed") {
		azureStatus = "completed"
	} else if strings.EqualFold(state, "all") {
		azureStatus = "all"
	}

	rawPRs, err := s.requestHelper.GetPullRequestsByRepo(ctx, org, token, project, repoID, azureStatus, author, branch)
	if err != nil {
		return nil, err
	}

	prs := make([]*types.PullRequest, 0, len(rawPRs))
	for _, it := range rawPRs {
		srcBranch := strings.TrimPrefix(it.SourceRefName, "refs/heads/")
		tgtBranch := strings.TrimPrefix(it.TargetRefName, "refs/heads/")

		headSHA := ""
		if it.LastMergeSourceCommit != nil {
			headSHA = it.LastMergeSourceCommit.ObjectID
		}
		baseSHA := ""
		if it.LastMergeTargetCommit != nil {
			baseSHA = it.LastMergeTargetCommit.ObjectID
		}

		prs = append(prs, &types.PullRequest{
			ID:           strconv.Itoa(it.PullRequestID),
			Number:       it.PullRequestID,
			PullNumber:   it.PullRequestID,
			Title:        it.Title,
			Description:  it.Description,
			Body:         it.Description,
			State:        it.Status,
			URL:          it.URL,
			PRURL:        it.URL,
			SourceBranch: srcBranch,
			TargetBranch: tgtBranch,
			HeadSHA:      headSHA,
			BaseSHA:      baseSHA,
			CreatedAt:    it.CreationDate.Format(time.RFC3339),
			Author:       it.CreatedBy.DisplayName,
			IsDraft:      it.IsDraft,
			User: types.PullRequestUser{
				ID:        it.CreatedBy.ID,
				Login:     it.CreatedBy.UniqueName,
				Username:  it.CreatedBy.UniqueName,
				Name:      it.CreatedBy.DisplayName,
				AvatarURL: it.CreatedBy.ImageURL,
			},
		})
	}

	return prs, nil
}

func (s *AzureDevOpsService) GetPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) (*types.PullRequest, error) {
	return s.GetPullRequestByNumber(ctx, orgData, repo, prNumber)
}

func (s *AzureDevOpsService) GetPullRequestByNumber(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) (*types.PullRequest, error) {
	token := s.extractToken(orgData)
	org, project, repoID := s.parseOrgProjectRepo(orgData, repo)

	it, err := s.requestHelper.GetPullRequestDetails(ctx, org, token, project, repoID, prNumber)
	if err != nil {
		return nil, err
	}

	srcBranch := strings.TrimPrefix(it.SourceRefName, "refs/heads/")
	tgtBranch := strings.TrimPrefix(it.TargetRefName, "refs/heads/")

	headSHA := ""
	if it.LastMergeSourceCommit != nil {
		headSHA = it.LastMergeSourceCommit.ObjectID
	}
	baseSHA := ""
	if it.LastMergeTargetCommit != nil {
		baseSHA = it.LastMergeTargetCommit.ObjectID
	}

	return &types.PullRequest{
		ID:           strconv.Itoa(it.PullRequestID),
		Number:       it.PullRequestID,
		PullNumber:   it.PullRequestID,
		Title:        it.Title,
		Description:  it.Description,
		Body:         it.Description,
		State:        it.Status,
		URL:          it.URL,
		PRURL:        it.URL,
		SourceBranch: srcBranch,
		TargetBranch: tgtBranch,
		HeadSHA:      headSHA,
		BaseSHA:      baseSHA,
		CreatedAt:    it.CreationDate.Format(time.RFC3339),
		Author:       it.CreatedBy.DisplayName,
		IsDraft:      it.IsDraft,
		User: types.PullRequestUser{
			ID:        it.CreatedBy.ID,
			Login:     it.CreatedBy.UniqueName,
			Username:  it.CreatedBy.UniqueName,
			Name:      it.CreatedBy.DisplayName,
			AvatarURL: it.CreatedBy.ImageURL,
		},
	}, nil
}

func (s *AzureDevOpsService) GetPullRequestsByRepository(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor) ([]*types.PullRequest, error) {
	return s.GetPullRequests(ctx, orgData, &repo, "active", "", "")
}

func (s *AzureDevOpsService) GetPullRequestsWithFiles(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor) ([]*types.PullRequestWithFiles, error) {
	prs, err := s.GetPullRequests(ctx, orgData, repo, "active", "", "")
	if err != nil {
		return nil, err
	}

	results := make([]*types.PullRequestWithFiles, 0, len(prs))
	for _, pr := range prs {
		files, err := s.GetFilesByPullRequestId(ctx, orgData, repo, pr.Number)
		if err != nil {
			files = []*types.PullRequestFile{}
		}
		results = append(results, &types.PullRequestWithFiles{
			PullRequest:      *pr,
			PullNumber:       pr.Number,
			State:            pr.State,
			Title:            pr.Title,
			PullRequestFiles: files,
			Files:            files,
		})
	}

	return results, nil
}

func (s *AzureDevOpsService) GetPullRequestsForRTTM(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor) ([]*types.PullRequestCodeReviewTime, error) {
	prs, err := s.GetPullRequests(ctx, orgData, repo, "closed", "", "")
	if err != nil {
		return nil, err
	}

	var results []*types.PullRequestCodeReviewTime
	for _, pr := range prs {
		created, _ := time.Parse(time.RFC3339, pr.CreatedAt)
		results = append(results, &types.PullRequestCodeReviewTime{
			PRNumber:       pr.Number,
			ReviewDuration: 0,
			CreatedAt:      created,
			Author:         pr.Author,
		})
	}

	return results, nil
}

func (s *AzureDevOpsService) GetPullRequestsWithChangesRequested(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor) ([]types.PullRequestsWithChangesRequested, error) {
	token := s.extractToken(orgData)
	org, project, repoID := s.parseOrgProjectRepo(orgData, repo)

	rawPRs, err := s.requestHelper.GetPullRequestsByRepo(ctx, org, token, project, repoID, "active", "", "")
	if err != nil {
		return nil, err
	}

	var results []types.PullRequestsWithChangesRequested
	for _, pr := range rawPRs {
		for _, r := range pr.Reviewers {
			if r.Vote < 0 {
				results = append(results, types.PullRequestsWithChangesRequested{
					Title:            pr.Title,
					Number:           pr.PullRequestID,
					ReviewDecision:   types.PullRequestReviewStateChangesRequested,
					ChangesRequested: true,
				})
				break
			}
		}
	}

	return results, nil
}

func (s *AzureDevOpsService) IsDraftPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) (bool, error) {
	pr, err := s.GetPullRequestByNumber(ctx, orgData, repo, prNumber)
	if err != nil {
		return false, err
	}
	return pr.IsDraft, nil
}

func (s *AzureDevOpsService) GetReviewStatusByPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) (types.PullRequestReviewState, error) {
	token := s.extractToken(orgData)
	org, project, repoID := s.parseOrgProjectRepo(orgData, repo)

	pr, err := s.requestHelper.GetPullRequestDetails(ctx, org, token, project, repoID, prNumber)
	if err != nil {
		return types.PullRequestReviewStatePending, err
	}

	hasApproved := false
	for _, r := range pr.Reviewers {
		if r.Vote == -10 {
			return types.PullRequestReviewStateChangesRequested, nil
		}
		if r.Vote >= 10 {
			hasApproved = true
		}
	}

	if hasApproved {
		return types.PullRequestReviewStateApproved, nil
	}

	return types.PullRequestReviewStatePending, nil
}

func (s *AzureDevOpsService) UpdateDescriptionInPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, description string) error {
	token := s.extractToken(orgData)
	org, project, repoID := s.parseOrgProjectRepo(orgData, repo)
	return s.requestHelper.UpdatePullRequestDescription(ctx, org, token, project, repoID, prNumber, description)
}

func (s *AzureDevOpsService) MergePullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, method string) error {
	token := s.extractToken(orgData)
	org, project, repoID := s.parseOrgProjectRepo(orgData, repo)

	pr, err := s.requestHelper.GetPullRequestDetails(ctx, org, token, project, repoID, prNumber)
	if err != nil {
		return err
	}

	lastCommit := ""
	if pr.LastMergeSourceCommit != nil {
		lastCommit = pr.LastMergeSourceCommit.ObjectID
	}

	_, err = s.requestHelper.CompletePullRequest(ctx, org, token, project, repoID, prNumber, lastCommit)
	return err
}

func (s *AzureDevOpsService) ApprovePullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, message string) error {
	token := s.extractToken(orgData)
	org, project, repoID := s.parseOrgProjectRepo(orgData, repo)

	// In Azure DevOps, approval vote is 10
	currentUser, _ := s.GetCurrentUser(ctx, orgData)
	reviewerID := ""
	if currentUser != nil {
		reviewerID = currentUser.ID
	}
	if reviewerID == "" {
		reviewerID = "self"
	}

	err := s.requestHelper.VotePullRequest(ctx, org, token, project, repoID, prNumber, reviewerID, 10)
	if err != nil {
		return err
	}

	if message != "" {
		_, _ = s.CreateCommentInPullRequest(ctx, orgData, repo, prNumber, message)
	}

	return nil
}

func (s *AzureDevOpsService) RequestChangesPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, message string) error {
	token := s.extractToken(orgData)
	org, project, repoID := s.parseOrgProjectRepo(orgData, repo)

	// In Azure DevOps, rejection/changes requested vote is -10
	currentUser, _ := s.GetCurrentUser(ctx, orgData)
	reviewerID := ""
	if currentUser != nil {
		reviewerID = currentUser.ID
	}
	if reviewerID == "" {
		reviewerID = "self"
	}

	err := s.requestHelper.VotePullRequest(ctx, org, token, project, repoID, prNumber, reviewerID, -10)
	if err != nil {
		return err
	}

	if message != "" {
		_, _ = s.CreateCommentInPullRequest(ctx, orgData, repo, prNumber, message)
	}

	return nil
}

func (s *AzureDevOpsService) CheckIfPullRequestShouldBeApproved(ctx context.Context, orgData types.OrganizationAndTeamData, prNumber int, repo types.RepositoryDescriptor) (bool, error) {
	status, err := s.GetReviewStatusByPullRequest(ctx, orgData, &repo, prNumber)
	if err != nil {
		return false, err
	}
	return status == types.PullRequestReviewStateApproved, nil
}

func (s *AzureDevOpsService) CreatePullRequestWithFiles(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, sourceBranch, targetBranch, title, description, commitMessage string, author *types.GitActor, files []types.PullRequestFileChange) (*types.PullRequest, error) {
	token := s.extractToken(orgData)
	org, project, repoID := s.parseOrgProjectRepo(orgData, &repo)

	created, err := s.requestHelper.CreatePullRequest(ctx, org, token, project, repoID, sourceBranch, targetBranch, title, description)
	if err != nil {
		return nil, err
	}

	return s.GetPullRequestByNumber(ctx, orgData, &repo, created.PullRequestID)
}

func (s *AzureDevOpsService) UploadFiles(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, branchName, baseBranch, message string, author *types.GitActor, files []types.PullRequestFileChange) (bool, error) {
	return true, nil
}

// -------------------------------------------------------------------------------------
// Commits & Diffs API
// -------------------------------------------------------------------------------------

func (s *AzureDevOpsService) GetCommits(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, branch, author string) ([]*types.Commit, error) {
	token := s.extractToken(orgData)
	org, project, repoID := s.parseOrgProjectRepo(orgData, repo)

	rawCommits, err := s.requestHelper.GetCommits(ctx, org, token, project, repoID, branch)
	if err != nil {
		return nil, err
	}

	commits := make([]*types.Commit, 0, len(rawCommits))
	for _, it := range rawCommits {
		if author != "" && !strings.EqualFold(it.Author.Name, author) && !strings.EqualFold(it.Author.Email, author) {
			continue
		}

		commits = append(commits, &types.Commit{
			SHA:     it.CommitID,
			Message: it.Comment,
			Author: types.GitActor{
				Name:  it.Author.Name,
				Email: it.Author.Email,
			},
			AuthorName:  it.Author.Name,
			AuthorEmail: it.Author.Email,
			Date:        it.Author.Date,
		})
	}

	return commits, nil
}

func (s *AzureDevOpsService) GetCommitsForPullRequestForCodeReview(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]*types.Commit, error) {
	pr, err := s.GetPullRequestByNumber(ctx, orgData, repo, prNumber)
	if err != nil {
		return nil, err
	}
	return s.GetCommits(ctx, orgData, repo, pr.SourceBranch, "")
}

func (s *AzureDevOpsService) GetFilesByPullRequestId(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]*types.PullRequestFile, error) {
	token := s.extractToken(orgData)
	org, project, repoID := s.parseOrgProjectRepo(orgData, repo)

	iterations, err := s.requestHelper.GetPullRequestIterations(ctx, org, token, project, repoID, prNumber)
	if err != nil || len(iterations) == 0 {
		return []*types.PullRequestFile{}, nil
	}

	lastIteration := iterations[len(iterations)-1]
	changes, err := s.requestHelper.GetPullRequestIterationChanges(ctx, org, token, project, repoID, prNumber, lastIteration.ID)
	if err != nil {
		return []*types.PullRequestFile{}, nil
	}

	// Retrieve PR details to obtain base and target commit IDs
	baseCommitID := ""
	targetCommitID := ""
	prDetails, err := s.requestHelper.GetPullRequestDetails(ctx, org, token, project, repoID, prNumber)
	if err == nil && prDetails != nil {
		if prDetails.LastMergeTargetCommit != nil {
			baseCommitID = prDetails.LastMergeTargetCommit.ObjectID
		}
		if prDetails.LastMergeSourceCommit != nil {
			targetCommitID = prDetails.LastMergeSourceCommit.ObjectID
		}
	}

	files := make([]*types.PullRequestFile, 0, len(changes))
	for _, it := range changes {
		cleanPath := strings.TrimPrefix(it.Item.Path, "/")
		status := "modified"
		if strings.EqualFold(it.ChangeType, "add") {
			status = "added"
		} else if strings.EqualFold(it.ChangeType, "delete") {
			status = "removed"
		}

		var originalContent, modifiedContent string
		if status != "added" && baseCommitID != "" {
			orig, err := s.requestHelper.GetFileContent(ctx, org, token, project, repoID, it.Item.Path, baseCommitID)
			if err == nil {
				originalContent = orig
			}
		}
		if status != "removed" && targetCommitID != "" {
			mod, err := s.requestHelper.GetFileContent(ctx, org, token, project, repoID, it.Item.Path, targetCommitID)
			if err == nil {
				modifiedContent = mod
			}
		}

		patch, adds, dels := GenerateTwoFilesPatch(cleanPath, cleanPath, originalContent, modifiedContent, baseCommitID, targetCommitID)
		changesCount := adds + dels
		if changesCount == 0 {
			changesCount = 1
		}

		files = append(files, &types.PullRequestFile{
			Filename:  cleanPath,
			Status:    status,
			Changes:   changesCount,
			Additions: adds,
			Deletions: dels,
			Patch:     patch,
		})
	}

	return files, nil
}

func (s *AzureDevOpsService) GetChangedFilesSinceLastCommit(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, baseSHA, headSHA string) ([]string, error) {
	files, err := s.GetFilesByPullRequestId(ctx, orgData, repo, prNumber)
	if err != nil {
		return nil, err
	}
	filenames := make([]string, 0, len(files))
	for _, f := range files {
		filenames = append(filenames, f.Filename)
	}
	return filenames, nil
}

// -------------------------------------------------------------------------------------
// Comments & Reviews API (Threads)
// -------------------------------------------------------------------------------------

func (s *AzureDevOpsService) GetAllCommentsInPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]*types.PullRequestReviewComment, error) {
	token := s.extractToken(orgData)
	org, project, repoID := s.parseOrgProjectRepo(orgData, repo)

	threads, err := s.requestHelper.GetPullRequestComments(ctx, org, token, project, repoID, prNumber)
	if err != nil {
		return nil, err
	}

	var results []*types.PullRequestReviewComment
	for _, t := range threads {
		if t.IsDeleted {
			continue
		}

		filePath := ""
		line := 0
		if t.ThreadContext != nil {
			filePath = strings.TrimPrefix(t.ThreadContext.FilePath, "/")
			if t.ThreadContext.RightFileEnd != nil {
				line = t.ThreadContext.RightFileEnd.Line
			}
		}

		threadIDStr := strconv.Itoa(t.ID)

		for _, c := range t.Comments {
			if c.IsDeleted {
				continue
			}

			results = append(results, &types.PullRequestReviewComment{
				ID:        strconv.Itoa(c.ID),
				ThreadID:  threadIDStr,
				Path:      filePath,
				Line:      line,
				StartLine: line,
				Body:      c.Content,
				Author: &types.PullRequestCommentAuthor{
					ID:       c.Author.ID,
					Username: c.Author.UniqueName,
					Name:     c.Author.DisplayName,
				},
				CreatedAt: c.PublishedDate.Format(time.RFC3339),
			})
		}
	}

	return results, nil
}

func (s *AzureDevOpsService) GetPullRequestReviewComments(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]*types.PullRequestReviewComment, error) {
	all, err := s.GetAllCommentsInPullRequest(ctx, orgData, repo, prNumber)
	if err != nil {
		return nil, err
	}
	var inline []*types.PullRequestReviewComment
	for _, c := range all {
		if c.Path != "" && c.Line > 0 {
			inline = append(inline, c)
		}
	}
	return inline, nil
}

func (s *AzureDevOpsService) GetPullRequestReviewThreads(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]*types.PullRequestReviewComment, error) {
	token := s.extractToken(orgData)
	org, project, repoID := s.parseOrgProjectRepo(orgData, repo)

	threads, err := s.requestHelper.GetPullRequestComments(ctx, org, token, project, repoID, prNumber)
	if err != nil {
		return nil, err
	}

	var results []*types.PullRequestReviewComment
	for _, t := range threads {
		if t.IsDeleted || len(t.Comments) == 0 {
			continue
		}
		c := t.Comments[0]
		filePath := ""
		line := 0
		if t.ThreadContext != nil {
			filePath = strings.TrimPrefix(t.ThreadContext.FilePath, "/")
			if t.ThreadContext.RightFileEnd != nil {
				line = t.ThreadContext.RightFileEnd.Line
			}
		}

		results = append(results, &types.PullRequestReviewComment{
			ID:        strconv.Itoa(c.ID),
			ThreadID:  strconv.Itoa(t.ID),
			Path:      filePath,
			Line:      line,
			StartLine: line,
			Body:      c.Content,
			Author: &types.PullRequestCommentAuthor{
				ID:       c.Author.ID,
				Username: c.Author.UniqueName,
				Name:     c.Author.DisplayName,
			},
			CreatedAt: c.PublishedDate.Format(time.RFC3339),
		})
	}

	return results, nil
}

func (s *AzureDevOpsService) GetPullRequestReviewComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, commentID string) (*types.PullRequestReviewComment, error) {
	token := s.extractToken(orgData)
	org, project, repoID := s.parseOrgProjectRepo(orgData, repo)

	// commentID is expected to be in format "threadID" or "threadID:commentID"
	parts := strings.SplitN(commentID, ":", 2)
	threadID, _ := strconv.Atoi(parts[0])
	targetCommentID := 0
	if len(parts) > 1 {
		targetCommentID, _ = strconv.Atoi(parts[1])
	}

	// Fetch all threads for the first open PR (commentID doesn't include prNumber)
	prs, err := s.requestHelper.GetPullRequestsByRepo(ctx, org, token, project, repoID, "active", "", "")
	if err != nil || len(prs) == 0 {
		return nil, err
	}

	for _, pr := range prs {
		threads, err := s.requestHelper.GetPullRequestComments(ctx, org, token, project, repoID, pr.PullRequestID)
		if err != nil {
			continue
		}

		for _, t := range threads {
			if t.ID != threadID {
				continue
			}
			for _, c := range t.Comments {
				if targetCommentID > 0 && c.ID != targetCommentID {
					continue
				}
				var filePath string
				if t.ThreadContext != nil {
					filePath = t.ThreadContext.FilePath
				}
				return &types.PullRequestReviewComment{
					ID:        strconv.Itoa(c.ID),
					ThreadID:  strconv.Itoa(t.ID),
					Path:      filePath,
					Body:      c.Content,
					CreatedAt: c.PublishedDate.Format(time.RFC3339),
					Author: &types.PullRequestCommentAuthor{
						ID:       c.Author.ID,
						Username: c.Author.UniqueName,
						Name:     c.Author.DisplayName,
					},
				}, nil
			}
		}
	}

	return nil, nil
}

func (s *AzureDevOpsService) CreateReviewComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, comment types.PullRequestReviewComment) (*types.PullRequestReviewComment, error) {
	token := s.extractToken(orgData)
	org, project, repoID := s.parseOrgProjectRepo(orgData, repo)

	thread, err := s.requestHelper.CreateReviewComment(ctx, org, token, project, repoID, prNumber, comment.Path, comment.StartLine, comment.Line, comment.Body)
	if err != nil {
		return nil, err
	}

	commentID := strconv.Itoa(thread.ID)
	if len(thread.Comments) > 0 {
		commentID = strconv.Itoa(thread.Comments[0].ID)
	}

	comment.ID = commentID
	comment.ThreadID = strconv.Itoa(thread.ID)
	return &comment, nil
}

func (s *AzureDevOpsService) CreateCommentInPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, body string) (*types.PullRequestReviewComment, error) {
	token := s.extractToken(orgData)
	org, project, repoID := s.parseOrgProjectRepo(orgData, repo)

	thread, err := s.requestHelper.CreateGeneralThread(ctx, org, token, project, repoID, prNumber, body)
	if err != nil {
		return nil, err
	}

	commentID := strconv.Itoa(thread.ID)
	if len(thread.Comments) > 0 {
		commentID = strconv.Itoa(thread.Comments[0].ID)
	}

	return &types.PullRequestReviewComment{
		ID:       commentID,
		ThreadID: strconv.Itoa(thread.ID),
		Body:     body,
	}, nil
}

func (s *AzureDevOpsService) CreateIssueComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, body string) (*types.PullRequestReviewComment, error) {
	return s.CreateCommentInPullRequest(ctx, orgData, repo, prNumber, body)
}

func (s *AzureDevOpsService) CreateSingleIssueComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, body string) (*types.PullRequestReviewComment, error) {
	return s.CreateCommentInPullRequest(ctx, orgData, repo, prNumber, body)
}

func (s *AzureDevOpsService) CreateResponseToComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, parentCommentID, body string) (*types.PullRequestReviewComment, error) {
	token := s.extractToken(orgData)
	org, project, repoID := s.parseOrgProjectRepo(orgData, repo)

	threadID, _ := strconv.Atoi(parentCommentID)
	c, err := s.requestHelper.AddCommentToThread(ctx, org, token, project, repoID, prNumber, threadID, 0, body)
	if err != nil {
		return nil, err
	}

	return &types.PullRequestReviewComment{
		ID:       strconv.Itoa(c.ID),
		ThreadID: parentCommentID,
		Body:     body,
	}, nil
}

func (s *AzureDevOpsService) UpdateResponseToComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, parentID, commentID, body string) (*types.PullRequestReviewComment, error) {
	err := s.UpdateIssueComment(ctx, orgData, repo, commentID, body)
	if err != nil {
		return nil, err
	}
	return &types.PullRequestReviewComment{
		ID:       commentID,
		ThreadID: parentID,
		Body:     body,
	}, nil
}

func (s *AzureDevOpsService) UpdateIssueComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, commentID string, body string) error {
	token := s.extractToken(orgData)
	org, project, repoID := s.parseOrgProjectRepo(orgData, repo)

	// commentID is in format "threadID:commentID" or "threadID"
	parts := strings.SplitN(commentID, ":", 2)
	threadID, _ := strconv.Atoi(parts[0])
	cID := 1 // Default to first comment in thread
	if len(parts) > 1 {
		cID, _ = strconv.Atoi(parts[1])
	}

	// We need the PR number - find PRs for the repo to update comment
	prs, err := s.requestHelper.GetPullRequestsByRepo(ctx, org, token, project, repoID, "active", "", "")
	if err != nil || len(prs) == 0 {
		return fmt.Errorf("no active pull requests found to update comment")
	}

	for _, pr := range prs {
		err := s.requestHelper.UpdateCommentInThread(ctx, org, token, project, repoID, pr.PullRequestID, threadID, cID, body)
		if err == nil {
			return nil
		}
	}

	return fmt.Errorf("failed to update comment %s", commentID)
}

func (s *AzureDevOpsService) MinimizeComment(ctx context.Context, orgData types.OrganizationAndTeamData, commentID string, reason string) error {
	return nil
}

func (s *AzureDevOpsService) MarkReviewCommentAsResolved(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, threadID string) error {
	token := s.extractToken(orgData)
	org, project, repoID := s.parseOrgProjectRepo(orgData, repo)
	tID, _ := strconv.Atoi(threadID)
	return s.requestHelper.UpdateThreadStatus(ctx, org, token, project, repoID, 0, tID, "fixed")
}

func (s *AzureDevOpsService) GetListOfValidReviews(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]string, error) {
	token := s.extractToken(orgData)
	org, project, repoID := s.parseOrgProjectRepo(orgData, repo)

	reviewers, err := s.requestHelper.GetPullRequestReviewers(ctx, org, token, project, repoID, prNumber)
	if err != nil {
		return nil, err
	}

	var validIDs []string
	for _, r := range reviewers {
		// A valid review is a non-zero vote: >0 = approved variants, <0 = reject/wait
		if r.Vote != 0 {
			validIDs = append(validIDs, r.ID)
		}
	}

	return validIDs, nil
}

// -------------------------------------------------------------------------------------
// Users & Authors API
// -------------------------------------------------------------------------------------

func (s *AzureDevOpsService) GetCurrentUser(ctx context.Context, orgData types.OrganizationAndTeamData) (*types.PullRequestUser, error) {
	token := s.extractToken(orgData)
	org, _, _ := s.parseOrgProjectRepo(orgData, nil)

	userID, err := s.requestHelper.GetAuthenticatedUserID(ctx, org, token)
	if err != nil || userID == "" {
		return &types.PullRequestUser{
			Username: "azure-service",
			Login:    "azure-service",
			Name:     "Azure DevOps Service",
			ID:       "service",
		}, nil
	}

	return &types.PullRequestUser{
		Username: userID,
		Login:    userID,
		Name:     "Azure DevOps User",
		ID:       userID,
	}, nil
}

func (s *AzureDevOpsService) GetUserByUsername(ctx context.Context, orgData types.OrganizationAndTeamData, username string) (*types.PullRequestUser, error) {
	return &types.PullRequestUser{
		Username: username,
		Login:    username,
		Name:     username,
		ID:       username,
	}, nil
}

func (s *AzureDevOpsService) GetUsersByUsername(ctx context.Context, orgData types.OrganizationAndTeamData, usernames []string) (map[string]*types.PullRequestUser, error) {
	res := make(map[string]*types.PullRequestUser)
	for _, u := range usernames {
		res[u] = &types.PullRequestUser{
			Username: u,
			Login:    u,
			Name:     u,
			ID:       u,
		}
	}
	return res, nil
}

func (s *AzureDevOpsService) GetUserByEmailOrName(ctx context.Context, orgData types.OrganizationAndTeamData, email, userName string) (*types.PullRequestUser, error) {
	return s.GetUserByUsername(ctx, orgData, userName)
}

func (s *AzureDevOpsService) GetUserByID(ctx context.Context, orgData types.OrganizationAndTeamData, userID string) (*types.PullRequestUser, error) {
	return s.GetUserByUsername(ctx, orgData, userID)
}

func (s *AzureDevOpsService) GetPullRequestAuthors(ctx context.Context, orgData types.OrganizationAndTeamData, determineBots bool) ([]types.PullRequestAuthor, error) {
	token := s.extractToken(orgData)
	org, project, _ := s.parseOrgProjectRepo(orgData, nil)

	var projects []AzureProject
	var err error
	if project != "" {
		projects = []AzureProject{{ID: project, Name: project}}
	} else {
		projects, err = s.requestHelper.GetProjects(ctx, org, token)
		if err != nil {
			return nil, err
		}
	}

	authorsSet := make(map[string]bool)
	var authors []types.PullRequestAuthor

	for _, p := range projects {
		repos, err := s.requestHelper.GetRepositories(ctx, org, token, p.ID)
		if err != nil {
			continue
		}

		for _, r := range repos {
			prs, err := s.requestHelper.GetPullRequestsByRepo(ctx, org, token, p.ID, r.ID, "all", "", "")
			if err != nil {
				continue
			}

			for _, pr := range prs {
				userID := pr.CreatedBy.ID
				if userID == "" || authorsSet[userID] {
					continue
				}
				authorsSet[userID] = true
				authorType := "user"
				if determineBots && strings.HasPrefix(pr.CreatedBy.UniqueName, "svc.") {
					authorType = "bot"
				}
				name := pr.CreatedBy.DisplayName
				if name == "" {
					name = pr.CreatedBy.UniqueName
				}
				authors = append(authors, types.PullRequestAuthor{
					ID:   userID,
					Name: name,
					Type: authorType,
				})
			}
		}
	}

	sort.Slice(authors, func(i, j int) bool {
		return authors[i].Name < authors[j].Name
	})

	return authors, nil
}

// -------------------------------------------------------------------------------------
// Content, Batch Files, Trees & Languages
// -------------------------------------------------------------------------------------

func (s *AzureDevOpsService) GetRepositoryContentFile(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, filePath, ref string) (*types.RepositoryFile, error) {
	token := s.extractToken(orgData)
	org, project, repoID := s.parseOrgProjectRepo(orgData, repo)

	content, err := s.requestHelper.GetFileContent(ctx, org, token, project, repoID, filePath, ref)
	if err != nil {
		return nil, err
	}

	return &types.RepositoryFile{
		Path:    filePath,
		Content: content,
		Size:    int64(len(content)),
		SHA:     ref,
	}, nil
}

func (s *AzureDevOpsService) GetRepositoryContentBatch(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, files []string, ref string) (map[string]*types.RepositoryFile, error) {
	token := s.extractToken(orgData)
	org, project, repoID := s.parseOrgProjectRepo(orgData, &repo)

	batch, err := s.requestHelper.GetBatchItems(ctx, org, token, project, repoID, files, ref)
	if err != nil {
		// Fallback to single file requests
		result := make(map[string]*types.RepositoryFile)
		for _, f := range files {
			if file, err := s.GetRepositoryContentFile(ctx, orgData, &repo, f, ref); err == nil && file != nil {
				result[f] = file
			}
		}
		return result, nil
	}

	result := make(map[string]*types.RepositoryFile)
	for pathStr, content := range batch {
		result[pathStr] = &types.RepositoryFile{
			Path:    pathStr,
			Content: content,
			Size:    int64(len(content)),
			SHA:     ref,
		}
	}

	return result, nil
}

func (s *AzureDevOpsService) GetLanguageRepository(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor) (map[string]int, error) {
	token := s.extractToken(orgData)
	org, project, _ := s.parseOrgProjectRepo(orgData, repo)

	langs, err := s.requestHelper.GetLanguageStats(ctx, org, token, project)
	if err != nil {
		return nil, err
	}

	result := make(map[string]int)
	for _, l := range langs {
		result[l.Name] = int(l.LanguagePercentage)
	}

	return result, nil
}

func (s *AzureDevOpsService) GetRepositoryTree(ctx context.Context, orgData types.OrganizationAndTeamData, repositoryID string) ([]*types.TreeItem, error) {
	token := s.extractToken(orgData)
	org, project, _ := s.parseOrgProjectRepo(orgData, nil)

	items, err := s.requestHelper.GetItems(ctx, org, token, project, repositoryID, "", "Full", "")
	if err != nil {
		return nil, err
	}

	var result []*types.TreeItem
	for _, item := range items {
		normalizedPath := strings.TrimPrefix(item.Path, "/")
		itemType := "file"
		if item.GitObjectType == "tree" || item.IsFolder {
			itemType = "directory"
		}
		result = append(result, &types.TreeItem{
			Path: normalizedPath,
			Type: itemType,
			SHA:  item.ObjectID,
			URL:  "",
		})
	}

	return result, nil
}

func (s *AzureDevOpsService) GetRepositoryTreeByDirectory(ctx context.Context, orgData types.OrganizationAndTeamData, repositoryID, directoryPath string) ([]*types.TreeItem, error) {
	token := s.extractToken(orgData)
	org, project, _ := s.parseOrgProjectRepo(orgData, nil)

	scopePath := directoryPath
	if scopePath != "" && !strings.HasPrefix(scopePath, "/") {
		scopePath = "/" + scopePath
	}

	items, err := s.requestHelper.GetItems(ctx, org, token, project, repositoryID, scopePath, "OneLevel", "")
	if err != nil {
		return nil, err
	}

	normalizedScopePath := strings.TrimPrefix(directoryPath, "/")

	var result []*types.TreeItem
	for _, item := range items {
		normalizedPath := strings.TrimPrefix(item.Path, "/")

		// Exclude the parent directory itself to prevent loops
		if normalizedScopePath != "" && normalizedPath == normalizedScopePath {
			continue
		}

		// Only include directories
		if item.GitObjectType != "tree" || !item.IsFolder {
			continue
		}

		result = append(result, &types.TreeItem{
			Path: normalizedPath,
			Type: "directory",
			SHA:  item.ObjectID,
			URL:  "",
		})
	}

	return result, nil
}

func (s *AzureDevOpsService) GetRepositoryAllFiles(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, branch string, filePatterns, excludePatterns []string, maxFiles int) ([]*types.RepositoryFile, error) {
	token := s.extractToken(orgData)
	org, project, repoID := s.parseOrgProjectRepo(orgData, &repo)

	if branch == "" {
		defBranch, err := s.GetDefaultBranch(ctx, orgData, &repo)
		if err != nil || defBranch == "" {
			return nil, fmt.Errorf("could not resolve default branch for repository %s", repo.Name)
		}
		branch = defBranch
	}

	items, err := s.requestHelper.GetItems(ctx, org, token, project, repoID, "", "Full", branch)
	if err != nil {
		return nil, err
	}

	if maxFiles <= 0 {
		maxFiles = 1000
	}

	var result []*types.RepositoryFile
	for _, item := range items {
		if item.IsFolder || item.GitObjectType == "tree" {
			continue
		}

		normalizedPath := strings.TrimPrefix(item.Path, "/")
		filename := filepath.Base(normalizedPath)

		// Apply file patterns filter
		if len(filePatterns) > 0 {
			matched := false
			for _, pattern := range filePatterns {
				if ok, _ := filepath.Match(strings.ToLower(pattern), strings.ToLower(filename)); ok {
					matched = true
					break
				}
				if ok, _ := filepath.Match(strings.ToLower(pattern), strings.ToLower(normalizedPath)); ok {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}

		// Apply exclude patterns filter
		if len(excludePatterns) > 0 {
			excluded := false
			for _, pattern := range excludePatterns {
				if ok, _ := filepath.Match(pattern, filename); ok {
					excluded = true
					break
				}
				if ok, _ := filepath.Match(pattern, normalizedPath); ok {
					excluded = true
					break
				}
			}
			if excluded {
				continue
			}
		}

		result = append(result, &types.RepositoryFile{
			Path: normalizedPath,
			SHA:  item.ObjectID,
			Size: -1, // Size not available from Azure Items API
		})

		if len(result) >= maxFiles {
			break
		}
	}

	return result, nil
}

func (s *AzureDevOpsService) GetCloneParams(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor) (*types.GitCloneParams, error) {
	token := s.extractToken(orgData)
	org, project, repoID := s.parseOrgProjectRepo(orgData, &repo)

	cloneURL := fmt.Sprintf("https://PAT:%s@dev.azure.com/%s/%s/_git/%s", token, org, project, repoID)

	return &types.GitCloneParams{
		URL:      cloneURL,
		Branch:   "main",
		Token:    token,
		Username: "PAT",
		Provider: models.ProviderAzure,
		Auth: &types.GitCloneAuth{
			Type:     types.AuthModePAT,
			Username: "PAT",
			Token:    token,
		},
	}, nil
}

func (s *AzureDevOpsService) GetAuthenticationOAuthToken(ctx context.Context, orgData types.OrganizationAndTeamData) (string, error) {
	token := s.extractToken(orgData)
	if token != "" {
		return token, nil
	}
	return "", errors.New("token not found")
}

// -------------------------------------------------------------------------------------
// Webhooks API
// -------------------------------------------------------------------------------------

func (s *AzureDevOpsService) IsWebhookActive(ctx context.Context, orgData types.OrganizationAndTeamData, repositoryID string) (bool, error) {
	token := s.extractToken(orgData)
	org, project, _ := s.parseOrgProjectRepo(orgData, nil)

	if project == "" {
		return false, fmt.Errorf("project ID required for webhook check")
	}

	subs, err := s.requestHelper.ListServiceHookSubscriptions(ctx, org, token, project)
	if err != nil {
		return false, err
	}

	for _, sub := range subs {
		if sub.PublisherInputs["repository"] == repositoryID && sub.Status == "enabled" {
			if url, ok := sub.ConsumerInputs["url"]; ok && strings.Contains(url, "scandrix") {
				return true, nil
			}
		}
	}

	return false, nil
}

func (s *AzureDevOpsService) DeleteWebhook(ctx context.Context, orgData types.OrganizationAndTeamData) error {
	token := s.extractToken(orgData)
	org, project, _ := s.parseOrgProjectRepo(orgData, nil)

	if project == "" {
		// Try to get all projects
		projects, err := s.requestHelper.GetProjects(ctx, org, token)
		if err != nil {
			return err
		}

		for _, p := range projects {
			if err := s.deleteWebhooksForProject(ctx, org, token, p.ID); err != nil {
				log.Printf("[scandrix] error deleting webhooks for project %s: %v", p.Name, err)
			}
		}
		return nil
	}

	return s.deleteWebhooksForProject(ctx, org, token, project)
}

// deleteWebhooksForProject removes all ScanDrix service hook subscriptions from a project.
func (s *AzureDevOpsService) deleteWebhooksForProject(ctx context.Context, org, token, projectID string) error {
	subs, err := s.requestHelper.ListServiceHookSubscriptions(ctx, org, token, projectID)
	if err != nil {
		return err
	}

	for _, sub := range subs {
		url, ok := sub.ConsumerInputs["url"]
		if !ok || !strings.Contains(url, "scandrix") {
			continue
		}

		if err := s.requestHelper.DeleteServiceHookSubscription(ctx, org, token, sub.ID); err != nil {
			log.Printf("[scandrix] failed to delete webhook subscription %s: %v", sub.ID, err)
		} else {
			log.Printf("[scandrix] deleted webhook subscription %s (event: %s)", sub.ID, sub.EventType)
		}
	}

	return nil
}

// -------------------------------------------------------------------------------------
// Reactions API
// -------------------------------------------------------------------------------------

func (s *AzureDevOpsService) CountReactions(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]types.ReactionsInComments, error) {
	token := s.extractToken(orgData)
	org, project, repoID := s.parseOrgProjectRepo(orgData, repo)

	threads, err := s.requestHelper.GetPullRequestComments(ctx, org, token, project, repoID, prNumber)
	if err != nil {
		return nil, err
	}

	// Azure DevOps doesn't have native reactions; count "likes" from comment votes
	// Track thumbs up/down code blocks in comment bodies as a proxy
	reactionCounts := make(map[string]int)
	for _, t := range threads {
		for _, c := range t.Comments {
			if strings.Contains(c.Content, "👍") {
				reactionCounts["thumbsUp"]++
			}
			if strings.Contains(c.Content, "👎") {
				reactionCounts["thumbsDown"]++
			}
		}
	}

	var result []types.ReactionsInComments
	for reaction, count := range reactionCounts {
		result = append(result, types.ReactionsInComments{
			Reaction: reaction,
			Count:    count,
		})
	}

	return result, nil
}

func (s *AzureDevOpsService) AddReactionToPR(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, prNumber int, reaction string) error {
	return nil
}

func (s *AzureDevOpsService) AddReactionToComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, prNumber int, commentID int64, reaction string) error {
	return nil
}

func (s *AzureDevOpsService) RemoveReactionsFromPR(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, prNumber int, reactions []string) error {
	return nil
}

func (s *AzureDevOpsService) RemoveReactionsFromComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, prNumber int, commentID int64, reactions []string) error {
	return nil
}

// -------------------------------------------------------------------------------------
// Comment Formatting
// -------------------------------------------------------------------------------------

func (s *AzureDevOpsService) FormatReviewCommentBody(suggestion any, repoLanguage string, includeHeader, includeFooter bool) string {
	var sb strings.Builder

	if includeHeader {
		sb.WriteString("<!-- drixy-codereview -->\n")
		sb.WriteString("### 🛡️ ScanDrix AI Review Suggestion\n\n")
	}

	if text, ok := suggestion.(string); ok {
		sb.WriteString(text)
	} else if data, err := json.MarshalIndent(suggestion, "", "  "); err == nil {
		sb.WriteString("```json\n")
		sb.WriteString(string(data))
		sb.WriteString("\n```")
	}

	if includeFooter {
		sb.WriteString("\n\n---\n*Automated review by [ScanDrix](https://scandrix.dev)*")
	}

	return sb.String()
}

func (s *AzureDevOpsService) ResolveMrAuthorFromWebhookPayload(ctx context.Context, orgData types.OrganizationAndTeamData, payload any) (*types.PullRequestUser, error) {
	if m, ok := payload.(map[string]any); ok {
		if res, ok := m["resource"].(map[string]any); ok {
			if createdBy, ok := res["createdBy"].(map[string]any); ok {
				idStr, _ := createdBy["id"].(string)
				displayName, _ := createdBy["displayName"].(string)
				uniqueName, _ := createdBy["uniqueName"].(string)
				var avatar string
				if links, ok := createdBy["_links"].(map[string]any); ok {
					if av, ok := links["avatar"].(map[string]any); ok {
						avatar, _ = av["href"].(string)
					}
				}
				return &types.PullRequestUser{
					ID:        idStr,
					Login:     uniqueName,
					Username:  uniqueName,
					Name:      displayName,
					AvatarURL: avatar,
				}, nil
			}
		}
	}
	return nil, nil
}

func (s *AzureDevOpsService) GetRecentRepositoryComments(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, limit int) ([]*types.PullRequestReviewComment, error) {
	if limit <= 0 {
		limit = 100
	}
	prs, err := s.GetPullRequests(ctx, orgData, &repo, "all", "", "")
	if err != nil {
		return nil, err
	}

	var results []*types.PullRequestReviewComment
	for _, pr := range prs {
		if len(results) >= limit {
			break
		}
		comments, err := s.GetPullRequestReviewComments(ctx, orgData, &repo, pr.Number)
		if err != nil {
			continue
		}
		for _, c := range comments {
			results = append(results, c)
			if len(results) >= limit {
				break
			}
		}
	}
	return results, nil
}

