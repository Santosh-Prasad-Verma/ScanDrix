package contracts

import (
	"context"

	"github.com/scandrix/backend/internal/platform/domain/types"
	"github.com/scandrix/backend/pkg/models"
)

// ICodeManagementService specifies the comprehensive interface for all Git hosting providers.
// Matches libs/platform/domain/platformIntegrations/interfaces/code-management.interface.ts
type ICodeManagementService interface {
	// Provider identification
	Provider() models.SCMProvider

	// Issues
	ListIssues(ctx context.Context, params types.ListIssuesParams) ([]types.CodeManagementIssue, error)
	GetIssue(ctx context.Context, params types.GetIssueParams) (*types.CodeManagementIssue, error)
	SupportsIssues(ctx context.Context, orgData types.OrganizationAndTeamData) (bool, error)

	// Repository inspection & manipulation
	FindRepositoryByName(ctx context.Context, orgData types.OrganizationAndTeamData, name string) (*types.Repository, error)
	CreatePullRequestWithFiles(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, sourceBranch, targetBranch, title, description, commitMessage string, author *types.GitActor, files []types.PullRequestFileChange) (*types.PullRequest, error)
	UploadFiles(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, branchName, baseBranch, message string, author *types.GitActor, files []types.PullRequestFileChange) (bool, error)

	// Pull requests
	GetPullRequests(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, state, author, branch string) ([]*types.PullRequest, error)
	GetPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) (*types.PullRequest, error)
	GetPullRequestByNumber(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) (*types.PullRequest, error)
	GetRepositories(ctx context.Context, orgData types.OrganizationAndTeamData, archived *bool, visibility, language string) ([]*types.Repositories, error)
	GetListMembers(ctx context.Context, orgData types.OrganizationAndTeamData) ([]types.PullRequestAuthor, error)
	VerifyConnection(ctx context.Context, orgData types.OrganizationAndTeamData) (*types.CodeManagementConnectionStatus, error)
	GetPullRequestsWithFiles(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor) ([]*types.PullRequestWithFiles, error)
	GetPullRequestsForRTTM(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor) ([]*types.PullRequestCodeReviewTime, error)
	GetCommits(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, branch, author string) ([]*types.Commit, error)
	GetOrganizations(ctx context.Context, orgData types.OrganizationAndTeamData) ([]*types.Organization, error)

	// Diff & File Content
	GetFilesByPullRequestId(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]*types.PullRequestFile, error)
	GetChangedFilesSinceLastCommit(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, baseSHA, headSHA string) ([]string, error)
	CreateReviewComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, comment types.PullRequestReviewComment) (*types.PullRequestReviewComment, error)
	CreateCommentInPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, body string) (*types.PullRequestReviewComment, error)
	GetRepositoryContentFile(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, path, ref string) (*types.RepositoryFile, error)
	GetRepositoryContentBatch(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, files []string, ref string) (map[string]*types.RepositoryFile, error)

	// Commits & Comments
	GetCommitsForPullRequestForCodeReview(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]*types.Commit, error)
	CreateIssueComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, body string) (*types.PullRequestReviewComment, error)
	CreateSingleIssueComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, body string) (*types.PullRequestReviewComment, error)
	UpdateIssueComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, commentID string, body string) error
	MinimizeComment(ctx context.Context, orgData types.OrganizationAndTeamData, commentID string, reason string) error

	// Branches & Discussion Threads
	GetDefaultBranch(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor) (string, error)
	GetPullRequestReviewComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, commentID string) (*types.PullRequestReviewComment, error)
	CreateResponseToComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, parentCommentID, body string) (*types.PullRequestReviewComment, error)
	UpdateDescriptionInPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, description string) error
	GetAuthenticationOAuthToken(ctx context.Context, orgData types.OrganizationAndTeamData) (string, error)
	CountReactions(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]types.ReactionsInComments, error)
	GetLanguageRepository(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor) (map[string]int, error)
	GetRepositoryAllFiles(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, branch string, filePatterns, excludePatterns []string, maxFiles int) ([]*types.RepositoryFile, error)
	GetCloneParams(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor) (*types.GitCloneParams, error)
	MergePullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, method string) error
	ApprovePullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, message string) error
	RequestChangesPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, message string) error

	// Comments & Users
	GetAllCommentsInPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]*types.PullRequestReviewComment, error)
	GetUserByUsername(ctx context.Context, orgData types.OrganizationAndTeamData, username string) (*types.PullRequestUser, error)
	GetUsersByUsername(ctx context.Context, orgData types.OrganizationAndTeamData, usernames []string) (map[string]*types.PullRequestUser, error)
	GetUserByEmailOrName(ctx context.Context, orgData types.OrganizationAndTeamData, email, userName string) (*types.PullRequestUser, error)
	GetUserByID(ctx context.Context, orgData types.OrganizationAndTeamData, userID string) (*types.PullRequestUser, error)
	GetCurrentUser(ctx context.Context, orgData types.OrganizationAndTeamData) (*types.PullRequestUser, error)

	// Review Resolution & Threads
	MarkReviewCommentAsResolved(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, threadID string) error
	GetPullRequestReviewComments(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]*types.PullRequestReviewComment, error)
	GetPullRequestsByRepository(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor) ([]*types.PullRequest, error)
	GetPullRequestReviewThreads(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]*types.PullRequestReviewComment, error)
	GetListOfValidReviews(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]string, error)
	GetPullRequestsWithChangesRequested(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor) ([]types.PullRequestsWithChangesRequested, error)
	GetPullRequestAuthors(ctx context.Context, orgData types.OrganizationAndTeamData, determineBots bool) ([]types.PullRequestAuthor, error)
	CheckIfPullRequestShouldBeApproved(ctx context.Context, orgData types.OrganizationAndTeamData, prNumber int, repo types.RepositoryDescriptor) (bool, error)

	// Webhook lifecycle
	DeleteWebhook(ctx context.Context, orgData types.OrganizationAndTeamData) error
	IsWebhookActive(ctx context.Context, orgData types.OrganizationAndTeamData, repositoryID string) (bool, error)

	// Review comment formatting & Tree
	FormatReviewCommentBody(suggestion any, repoLanguage string, includeHeader, includeFooter bool) string
	GetRepositoryTree(ctx context.Context, orgData types.OrganizationAndTeamData, repositoryID string) ([]*types.TreeItem, error)
	GetRepositoryTreeByDirectory(ctx context.Context, orgData types.OrganizationAndTeamData, repositoryID, directoryPath string) ([]*types.TreeItem, error)
	UpdateResponseToComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, parentID, commentID, body string) (*types.PullRequestReviewComment, error)
	IsDraftPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) (bool, error)
	GetReviewStatusByPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) (types.PullRequestReviewState, error)

	// Reactions
	AddReactionToPR(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, prNumber int, reaction string) error
	AddReactionToComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, prNumber int, commentID int64, reaction string) error
	RemoveReactionsFromPR(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, prNumber int, reactions []string) error
	RemoveReactionsFromComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, prNumber int, commentID int64, reactions []string) error

	// Webhook MR author resolution & repository comments sampling
	ResolveMrAuthorFromWebhookPayload(ctx context.Context, orgData types.OrganizationAndTeamData, payload any) (*types.PullRequestUser, error)
	GetRecentRepositoryComments(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, limit int) ([]*types.PullRequestReviewComment, error)
}

