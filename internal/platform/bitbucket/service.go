// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package bitbucket

import (
	"context"
	"strings"

	"github.com/scandrix/backend/internal/platform/domain/contracts"
	"github.com/scandrix/backend/internal/platform/domain/types"
	"github.com/scandrix/backend/pkg/models"
)

// BitbucketService dynamically delegates calls to either BitbucketCloudService or BitbucketServerService
// depending on whether the integration points to Bitbucket Cloud (bitbucket.org) or Bitbucket Data Center / Server.
// Translates libs/platform/infrastructure/adapters/services/bitbucket.service.ts
var _ contracts.ICodeManagementService = (*BitbucketService)(nil)

type BitbucketService struct {
	cloudService  *BitbucketCloudService
	serverService *BitbucketServerService
}

// NewBitbucketService creates a unified Bitbucket router service.
func NewBitbucketService(cloud *BitbucketCloudService, server *BitbucketServerService) *BitbucketService {
	if cloud == nil {
		cloud = NewBitbucketCloudService(BitbucketCloudConfig{})
	}
	if server == nil {
		server = NewBitbucketServerService(BitbucketServerConfig{})
	}
	return &BitbucketService{
		cloudService:  cloud,
		serverService: server,
	}
}

func (s *BitbucketService) Provider() models.SCMProvider {
	return models.ProviderBitbucket
}

// resolveService determines whether the request targets Bitbucket Data Center/Server or Bitbucket Cloud.
func (s *BitbucketService) resolveService(orgData types.OrganizationAndTeamData) contracts.ICodeManagementService {
	if orgData.IntegrationCredentials != nil {
		if isDC, ok := orgData.IntegrationCredentials["isDataCenter"].(bool); ok && isDC {
			return s.serverService
		}
		if isServer, ok := orgData.IntegrationCredentials["isServer"].(bool); ok && isServer {
			return s.serverService
		}
		if host, ok := orgData.IntegrationCredentials["host"].(string); ok {
			host = strings.ToLower(strings.TrimSpace(host))
			if host != "" && !strings.Contains(host, "bitbucket.org") {
				return s.serverService
			}
		}
		if u, ok := orgData.IntegrationCredentials["url"].(string); ok {
			u = strings.ToLower(strings.TrimSpace(u))
			if u != "" && !strings.Contains(u, "bitbucket.org") {
				return s.serverService
			}
		}
	}
	return s.cloudService
}

// -------------------------------------------------------------------------------------
// ICodeManagementService Implementation (Delegates to Cloud or Server)
// -------------------------------------------------------------------------------------

func (s *BitbucketService) SupportsIssues(ctx context.Context, orgData types.OrganizationAndTeamData) (bool, error) {
	return s.resolveService(orgData).SupportsIssues(ctx, orgData)
}

func (s *BitbucketService) ListIssues(ctx context.Context, params types.ListIssuesParams) ([]types.CodeManagementIssue, error) {
	return s.resolveService(params.OrganizationAndTeamData).ListIssues(ctx, params)
}

func (s *BitbucketService) GetIssue(ctx context.Context, params types.GetIssueParams) (*types.CodeManagementIssue, error) {
	return s.resolveService(params.OrganizationAndTeamData).GetIssue(ctx, params)
}

func (s *BitbucketService) FindRepositoryByName(ctx context.Context, orgData types.OrganizationAndTeamData, name string) (*types.Repository, error) {
	return s.resolveService(orgData).FindRepositoryByName(ctx, orgData, name)
}

func (s *BitbucketService) CreatePullRequestWithFiles(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, sourceBranch, targetBranch, title, description, commitMessage string, author *types.GitActor, files []types.PullRequestFileChange) (*types.PullRequest, error) {
	return s.resolveService(orgData).CreatePullRequestWithFiles(ctx, orgData, repo, sourceBranch, targetBranch, title, description, commitMessage, author, files)
}

func (s *BitbucketService) UploadFiles(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, branchName, baseBranch, message string, author *types.GitActor, files []types.PullRequestFileChange) (bool, error) {
	return s.resolveService(orgData).UploadFiles(ctx, orgData, repo, branchName, baseBranch, message, author, files)
}

func (s *BitbucketService) GetPullRequests(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, state, author, branch string) ([]*types.PullRequest, error) {
	return s.resolveService(orgData).GetPullRequests(ctx, orgData, repo, state, author, branch)
}

func (s *BitbucketService) GetPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) (*types.PullRequest, error) {
	return s.resolveService(orgData).GetPullRequest(ctx, orgData, repo, prNumber)
}

func (s *BitbucketService) GetPullRequestByNumber(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) (*types.PullRequest, error) {
	return s.resolveService(orgData).GetPullRequestByNumber(ctx, orgData, repo, prNumber)
}

func (s *BitbucketService) GetRepositories(ctx context.Context, orgData types.OrganizationAndTeamData, archived *bool, visibility, language string) ([]*types.Repositories, error) {
	return s.resolveService(orgData).GetRepositories(ctx, orgData, archived, visibility, language)
}

func (s *BitbucketService) GetListMembers(ctx context.Context, orgData types.OrganizationAndTeamData) ([]types.PullRequestAuthor, error) {
	return s.resolveService(orgData).GetListMembers(ctx, orgData)
}

func (s *BitbucketService) VerifyConnection(ctx context.Context, orgData types.OrganizationAndTeamData) (*types.CodeManagementConnectionStatus, error) {
	return s.resolveService(orgData).VerifyConnection(ctx, orgData)
}

func (s *BitbucketService) GetPullRequestsWithFiles(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor) ([]*types.PullRequestWithFiles, error) {
	return s.resolveService(orgData).GetPullRequestsWithFiles(ctx, orgData, repo)
}

func (s *BitbucketService) GetPullRequestsForRTTM(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor) ([]*types.PullRequestCodeReviewTime, error) {
	return s.resolveService(orgData).GetPullRequestsForRTTM(ctx, orgData, repo)
}

func (s *BitbucketService) GetCommits(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, branch, author string) ([]*types.Commit, error) {
	return s.resolveService(orgData).GetCommits(ctx, orgData, repo, branch, author)
}

func (s *BitbucketService) GetOrganizations(ctx context.Context, orgData types.OrganizationAndTeamData) ([]*types.Organization, error) {
	return s.resolveService(orgData).GetOrganizations(ctx, orgData)
}

func (s *BitbucketService) GetFilesByPullRequestId(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]*types.PullRequestFile, error) {
	return s.resolveService(orgData).GetFilesByPullRequestId(ctx, orgData, repo, prNumber)
}

func (s *BitbucketService) GetChangedFilesSinceLastCommit(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, baseSHA, headSHA string) ([]string, error) {
	return s.resolveService(orgData).GetChangedFilesSinceLastCommit(ctx, orgData, repo, prNumber, baseSHA, headSHA)
}

func (s *BitbucketService) CreateReviewComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, comment types.PullRequestReviewComment) (*types.PullRequestReviewComment, error) {
	return s.resolveService(orgData).CreateReviewComment(ctx, orgData, repo, prNumber, comment)
}

func (s *BitbucketService) CreateCommentInPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, body string) (*types.PullRequestReviewComment, error) {
	return s.resolveService(orgData).CreateCommentInPullRequest(ctx, orgData, repo, prNumber, body)
}

func (s *BitbucketService) GetRepositoryContentFile(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, path, ref string) (*types.RepositoryFile, error) {
	return s.resolveService(orgData).GetRepositoryContentFile(ctx, orgData, repo, path, ref)
}

func (s *BitbucketService) GetRepositoryContentBatch(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, files []string, ref string) (map[string]*types.RepositoryFile, error) {
	return s.resolveService(orgData).GetRepositoryContentBatch(ctx, orgData, repo, files, ref)
}

func (s *BitbucketService) GetCommitsForPullRequestForCodeReview(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]*types.Commit, error) {
	return s.resolveService(orgData).GetCommitsForPullRequestForCodeReview(ctx, orgData, repo, prNumber)
}

func (s *BitbucketService) CreateIssueComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, body string) (*types.PullRequestReviewComment, error) {
	return s.resolveService(orgData).CreateIssueComment(ctx, orgData, repo, prNumber, body)
}

func (s *BitbucketService) CreateSingleIssueComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, body string) (*types.PullRequestReviewComment, error) {
	return s.resolveService(orgData).CreateSingleIssueComment(ctx, orgData, repo, prNumber, body)
}

func (s *BitbucketService) UpdateIssueComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, commentID string, body string) error {
	return s.resolveService(orgData).UpdateIssueComment(ctx, orgData, repo, commentID, body)
}

func (s *BitbucketService) MinimizeComment(ctx context.Context, orgData types.OrganizationAndTeamData, commentID string, reason string) error {
	return s.resolveService(orgData).MinimizeComment(ctx, orgData, commentID, reason)
}

func (s *BitbucketService) GetDefaultBranch(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor) (string, error) {
	return s.resolveService(orgData).GetDefaultBranch(ctx, orgData, repo)
}

func (s *BitbucketService) GetPullRequestReviewComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, commentID string) (*types.PullRequestReviewComment, error) {
	return s.resolveService(orgData).GetPullRequestReviewComment(ctx, orgData, repo, commentID)
}

func (s *BitbucketService) CreateResponseToComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, parentCommentID, body string) (*types.PullRequestReviewComment, error) {
	return s.resolveService(orgData).CreateResponseToComment(ctx, orgData, repo, prNumber, parentCommentID, body)
}

func (s *BitbucketService) UpdateDescriptionInPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, description string) error {
	return s.resolveService(orgData).UpdateDescriptionInPullRequest(ctx, orgData, repo, prNumber, description)
}

func (s *BitbucketService) GetAuthenticationOAuthToken(ctx context.Context, orgData types.OrganizationAndTeamData) (string, error) {
	return s.resolveService(orgData).GetAuthenticationOAuthToken(ctx, orgData)
}

func (s *BitbucketService) CountReactions(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]types.ReactionsInComments, error) {
	return s.resolveService(orgData).CountReactions(ctx, orgData, repo, prNumber)
}

func (s *BitbucketService) GetLanguageRepository(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor) (map[string]int, error) {
	return s.resolveService(orgData).GetLanguageRepository(ctx, orgData, repo)
}

func (s *BitbucketService) GetRepositoryAllFiles(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, branch string, filePatterns, excludePatterns []string, maxFiles int) ([]*types.RepositoryFile, error) {
	return s.resolveService(orgData).GetRepositoryAllFiles(ctx, orgData, repo, branch, filePatterns, excludePatterns, maxFiles)
}

func (s *BitbucketService) GetCloneParams(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor) (*types.GitCloneParams, error) {
	return s.resolveService(orgData).GetCloneParams(ctx, orgData, repo)
}

func (s *BitbucketService) MergePullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, method string) error {
	return s.resolveService(orgData).MergePullRequest(ctx, orgData, repo, prNumber, method)
}

func (s *BitbucketService) ApprovePullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, message string) error {
	return s.resolveService(orgData).ApprovePullRequest(ctx, orgData, repo, prNumber, message)
}

func (s *BitbucketService) RequestChangesPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, message string) error {
	return s.resolveService(orgData).RequestChangesPullRequest(ctx, orgData, repo, prNumber, message)
}

func (s *BitbucketService) GetAllCommentsInPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]*types.PullRequestReviewComment, error) {
	return s.resolveService(orgData).GetAllCommentsInPullRequest(ctx, orgData, repo, prNumber)
}

func (s *BitbucketService) GetUserByUsername(ctx context.Context, orgData types.OrganizationAndTeamData, username string) (*types.PullRequestUser, error) {
	return s.resolveService(orgData).GetUserByUsername(ctx, orgData, username)
}

func (s *BitbucketService) GetUsersByUsername(ctx context.Context, orgData types.OrganizationAndTeamData, usernames []string) (map[string]*types.PullRequestUser, error) {
	return s.resolveService(orgData).GetUsersByUsername(ctx, orgData, usernames)
}

func (s *BitbucketService) GetUserByEmailOrName(ctx context.Context, orgData types.OrganizationAndTeamData, email, userName string) (*types.PullRequestUser, error) {
	return s.resolveService(orgData).GetUserByEmailOrName(ctx, orgData, email, userName)
}

func (s *BitbucketService) GetUserByID(ctx context.Context, orgData types.OrganizationAndTeamData, userID string) (*types.PullRequestUser, error) {
	return s.resolveService(orgData).GetUserByID(ctx, orgData, userID)
}

func (s *BitbucketService) GetCurrentUser(ctx context.Context, orgData types.OrganizationAndTeamData) (*types.PullRequestUser, error) {
	return s.resolveService(orgData).GetCurrentUser(ctx, orgData)
}

func (s *BitbucketService) MarkReviewCommentAsResolved(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, threadID string) error {
	return s.resolveService(orgData).MarkReviewCommentAsResolved(ctx, orgData, repo, threadID)
}

func (s *BitbucketService) GetPullRequestReviewComments(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]*types.PullRequestReviewComment, error) {
	return s.resolveService(orgData).GetPullRequestReviewComments(ctx, orgData, repo, prNumber)
}

func (s *BitbucketService) GetPullRequestsByRepository(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor) ([]*types.PullRequest, error) {
	return s.resolveService(orgData).GetPullRequestsByRepository(ctx, orgData, repo)
}

func (s *BitbucketService) GetPullRequestReviewThreads(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]*types.PullRequestReviewComment, error) {
	return s.resolveService(orgData).GetPullRequestReviewThreads(ctx, orgData, repo, prNumber)
}

func (s *BitbucketService) GetListOfValidReviews(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]string, error) {
	return s.resolveService(orgData).GetListOfValidReviews(ctx, orgData, repo, prNumber)
}

func (s *BitbucketService) GetPullRequestsWithChangesRequested(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor) ([]types.PullRequestsWithChangesRequested, error) {
	return s.resolveService(orgData).GetPullRequestsWithChangesRequested(ctx, orgData, repo)
}

func (s *BitbucketService) GetPullRequestAuthors(ctx context.Context, orgData types.OrganizationAndTeamData, determineBots bool) ([]types.PullRequestAuthor, error) {
	return s.resolveService(orgData).GetPullRequestAuthors(ctx, orgData, determineBots)
}

func (s *BitbucketService) CheckIfPullRequestShouldBeApproved(ctx context.Context, orgData types.OrganizationAndTeamData, prNumber int, repo types.RepositoryDescriptor) (bool, error) {
	return s.resolveService(orgData).CheckIfPullRequestShouldBeApproved(ctx, orgData, prNumber, repo)
}

func (s *BitbucketService) DeleteWebhook(ctx context.Context, orgData types.OrganizationAndTeamData) error {
	return s.resolveService(orgData).DeleteWebhook(ctx, orgData)
}

func (s *BitbucketService) IsWebhookActive(ctx context.Context, orgData types.OrganizationAndTeamData, repositoryID string) (bool, error) {
	return s.resolveService(orgData).IsWebhookActive(ctx, orgData, repositoryID)
}

func (s *BitbucketService) FormatReviewCommentBody(suggestion any, repoLanguage string, includeHeader, includeFooter bool) string {
	return s.cloudService.FormatReviewCommentBody(suggestion, repoLanguage, includeHeader, includeFooter)
}

func (s *BitbucketService) GetRepositoryTree(ctx context.Context, orgData types.OrganizationAndTeamData, repositoryID string) ([]*types.TreeItem, error) {
	return s.resolveService(orgData).GetRepositoryTree(ctx, orgData, repositoryID)
}

func (s *BitbucketService) GetRepositoryTreeByDirectory(ctx context.Context, orgData types.OrganizationAndTeamData, repositoryID, directoryPath string) ([]*types.TreeItem, error) {
	return s.resolveService(orgData).GetRepositoryTreeByDirectory(ctx, orgData, repositoryID, directoryPath)
}

func (s *BitbucketService) UpdateResponseToComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, parentID, commentID, body string) (*types.PullRequestReviewComment, error) {
	return s.resolveService(orgData).UpdateResponseToComment(ctx, orgData, repo, prNumber, parentID, commentID, body)
}

func (s *BitbucketService) IsDraftPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) (bool, error) {
	return s.resolveService(orgData).IsDraftPullRequest(ctx, orgData, repo, prNumber)
}

func (s *BitbucketService) GetReviewStatusByPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) (types.PullRequestReviewState, error) {
	return s.resolveService(orgData).GetReviewStatusByPullRequest(ctx, orgData, repo, prNumber)
}

func (s *BitbucketService) AddReactionToPR(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, prNumber int, reaction string) error {
	return s.resolveService(orgData).AddReactionToPR(ctx, orgData, repo, prNumber, reaction)
}

func (s *BitbucketService) AddReactionToComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, prNumber int, commentID int64, reaction string) error {
	return s.resolveService(orgData).AddReactionToComment(ctx, orgData, repo, prNumber, commentID, reaction)
}

func (s *BitbucketService) RemoveReactionsFromPR(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, prNumber int, reactions []string) error {
	return s.resolveService(orgData).RemoveReactionsFromPR(ctx, orgData, repo, prNumber, reactions)
}

func (s *BitbucketService) RemoveReactionsFromComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, prNumber int, commentID int64, reactions []string) error {
	return s.resolveService(orgData).RemoveReactionsFromComment(ctx, orgData, repo, prNumber, commentID, reactions)
}

func (s *BitbucketService) ResolveMrAuthorFromWebhookPayload(ctx context.Context, orgData types.OrganizationAndTeamData, payload any) (*types.PullRequestUser, error) {
	return s.resolveService(orgData).ResolveMrAuthorFromWebhookPayload(ctx, orgData, payload)
}

func (s *BitbucketService) GetRecentRepositoryComments(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, limit int) ([]*types.PullRequestReviewComment, error) {
	return s.resolveService(orgData).GetRecentRepositoryComments(ctx, orgData, repo, limit)
}

