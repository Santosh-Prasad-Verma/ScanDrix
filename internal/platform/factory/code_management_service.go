// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package factory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/scandrix/backend/internal/platform/domain/contracts"
	"github.com/scandrix/backend/internal/platform/domain/types"
	"github.com/scandrix/backend/pkg/models"
)

// CodeManagementService coordinates multi-platform Git operations by dynamically routing
// calls to the appropriate VCS adapter (GitHub, GitLab, Bitbucket, Azure DevOps, Forgejo).
// Translates libs/platform/infrastructure/adapters/services/codeManagement.service.ts
var _ contracts.ICodeManagementService = (*CodeManagementService)(nil)

const (
	defaultDrixyMarker = "<!-- drixy-codereview -->"
	defaultFooter      = "\n\n---\n*Automated review by [ScanDrix](https://scandrix.dev)*"
)

// CodeManagementService routes VCS operations via the PlatformIntegrationFactory.
type CodeManagementService struct {
	factory *PlatformIntegrationFactory
	logger  *slog.Logger
	mu      sync.RWMutex
}

// NewCodeManagementService creates a new dynamic router service.
func NewCodeManagementService(factory *PlatformIntegrationFactory, logger *slog.Logger) *CodeManagementService {
	if logger == nil {
		logger = slog.Default()
	}
	return &CodeManagementService{
		factory: factory,
		logger:  logger,
	}
}

// Provider returns a virtual multi-provider indicator.
func (s *CodeManagementService) Provider() models.SCMProvider {
	return "multi-platform"
}

// ResolveProvider inspects tenant organization parameters to determine the active VCS platform.
func (s *CodeManagementService) ResolveProvider(orgData types.OrganizationAndTeamData) (models.SCMProvider, error) {
	if orgData.Provider != "" {
		p := models.SCMProvider(strings.ToLower(strings.TrimSpace(orgData.Provider)))
		if s.factory.HasCodeManagementService(p) {
			return p, nil
		}
	}

	if orgData.IntegrationCredentials != nil {
		if raw, ok := orgData.IntegrationCredentials["provider"].(string); ok && raw != "" {
			clean := strings.ToLower(strings.TrimSpace(raw))
			if s.factory.HasCodeManagementService(models.SCMProvider(clean)) {
				return models.SCMProvider(clean), nil
			}
		}
		if raw, ok := orgData.IntegrationCredentials["platform"].(string); ok && raw != "" {
			clean := strings.ToLower(strings.TrimSpace(raw))
			if s.factory.HasCodeManagementService(models.SCMProvider(clean)) {
				return models.SCMProvider(clean), nil
			}
		}
	}

	return "", errors.New("unable to resolve SCM provider from organization context")
}

// GetUnderlyingService retrieves the adapter instance for the given organization context.
func (s *CodeManagementService) GetUnderlyingService(orgData types.OrganizationAndTeamData) (contracts.ICodeManagementService, error) {
	provider, err := s.ResolveProvider(orgData)
	if err != nil {
		return nil, err
	}
	return s.factory.GetCodeManagementService(provider)
}

// -------------------------------------------------------------------------------------
// Issues API
// -------------------------------------------------------------------------------------

func (s *CodeManagementService) ListIssues(ctx context.Context, params types.ListIssuesParams) ([]types.CodeManagementIssue, error) {
	svc, err := s.GetUnderlyingService(params.OrganizationAndTeamData)
	if err != nil {
		s.logger.Warn("Failed to resolve provider for ListIssues", slog.Any("err", err))
		return []types.CodeManagementIssue{}, nil
	}
	return svc.ListIssues(ctx, params)
}

func (s *CodeManagementService) GetIssue(ctx context.Context, params types.GetIssueParams) (*types.CodeManagementIssue, error) {
	svc, err := s.GetUnderlyingService(params.OrganizationAndTeamData)
	if err != nil {
		s.logger.Warn("Failed to resolve provider for GetIssue", slog.Any("err", err))
		return nil, nil
	}
	return svc.GetIssue(ctx, params)
}

func (s *CodeManagementService) SupportsIssues(ctx context.Context, orgData types.OrganizationAndTeamData) (bool, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return false, nil
	}
	return svc.SupportsIssues(ctx, orgData)
}

// -------------------------------------------------------------------------------------
// Repository Inspection & Manipulation
// -------------------------------------------------------------------------------------

func (s *CodeManagementService) FindRepositoryByName(ctx context.Context, orgData types.OrganizationAndTeamData, name string) (*types.Repository, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return nil, err
	}
	return svc.FindRepositoryByName(ctx, orgData, name)
}

func (s *CodeManagementService) CreatePullRequestWithFiles(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo types.RepositoryDescriptor,
	sourceBranch, targetBranch, title, description, commitMessage string,
	author *types.GitActor,
	files []types.PullRequestFileChange,
) (*types.PullRequest, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return nil, err
	}
	return svc.CreatePullRequestWithFiles(ctx, orgData, repo, sourceBranch, targetBranch, title, description, commitMessage, author, files)
}

func (s *CodeManagementService) UploadFiles(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo types.RepositoryDescriptor,
	branchName, baseBranch, message string,
	author *types.GitActor,
	files []types.PullRequestFileChange,
) (bool, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return false, err
	}
	return svc.UploadFiles(ctx, orgData, repo, branchName, baseBranch, message, author, files)
}

// -------------------------------------------------------------------------------------
// Pull Requests
// -------------------------------------------------------------------------------------

func (s *CodeManagementService) GetPullRequests(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	state, author, branch string,
) ([]*types.PullRequest, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return []*types.PullRequest{}, err
	}
	return svc.GetPullRequests(ctx, orgData, repo, state, author, branch)
}

func (s *CodeManagementService) GetPullRequest(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
) (*types.PullRequest, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return nil, err
	}
	return svc.GetPullRequest(ctx, orgData, repo, prNumber)
}

func (s *CodeManagementService) GetPullRequestByNumber(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
) (*types.PullRequest, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return nil, err
	}
	return svc.GetPullRequestByNumber(ctx, orgData, repo, prNumber)
}

func (s *CodeManagementService) GetPullRequestsByRepository(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo types.RepositoryDescriptor,
) ([]*types.PullRequest, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return []*types.PullRequest{}, err
	}
	return svc.GetPullRequestsByRepository(ctx, orgData, repo)
}

func (s *CodeManagementService) GetRepositories(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	archived *bool,
	visibility, language string,
) ([]*types.Repositories, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return []*types.Repositories{}, err
	}
	return svc.GetRepositories(ctx, orgData, archived, visibility, language)
}

func (s *CodeManagementService) GetListMembers(ctx context.Context, orgData types.OrganizationAndTeamData) ([]types.PullRequestAuthor, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return []types.PullRequestAuthor{}, err
	}
	return svc.GetListMembers(ctx, orgData)
}

func (s *CodeManagementService) VerifyConnection(ctx context.Context, orgData types.OrganizationAndTeamData) (*types.CodeManagementConnectionStatus, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return &types.CodeManagementConnectionStatus{
			HasConnection:   false,
			IsConnected:     false,
			IsSetupComplete: false,
			Message:         fmt.Sprintf("Provider resolution failed: %v", err),
			PlatformName:    "unknown",
		}, err
	}
	return svc.VerifyConnection(ctx, orgData)
}

func (s *CodeManagementService) GetPullRequestsWithFiles(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
) ([]*types.PullRequestWithFiles, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return []*types.PullRequestWithFiles{}, err
	}
	return svc.GetPullRequestsWithFiles(ctx, orgData, repo)
}

func (s *CodeManagementService) GetPullRequestsForRTTM(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
) ([]*types.PullRequestCodeReviewTime, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return []*types.PullRequestCodeReviewTime{}, err
	}
	return svc.GetPullRequestsForRTTM(ctx, orgData, repo)
}

func (s *CodeManagementService) GetCommits(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	branch, author string,
) ([]*types.Commit, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return []*types.Commit{}, err
	}
	return svc.GetCommits(ctx, orgData, repo, branch, author)
}

func (s *CodeManagementService) GetOrganizations(ctx context.Context, orgData types.OrganizationAndTeamData) ([]*types.Organization, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return []*types.Organization{}, err
	}
	return svc.GetOrganizations(ctx, orgData)
}

// -------------------------------------------------------------------------------------
// Diff & File Content
// -------------------------------------------------------------------------------------

func (s *CodeManagementService) GetFilesByPullRequestId(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
) ([]*types.PullRequestFile, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return []*types.PullRequestFile{}, err
	}
	return svc.GetFilesByPullRequestId(ctx, orgData, repo, prNumber)
}

func (s *CodeManagementService) GetChangedFilesSinceLastCommit(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
	baseSHA, headSHA string,
) ([]string, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return []string{}, err
	}
	return svc.GetChangedFilesSinceLastCommit(ctx, orgData, repo, prNumber, baseSHA, headSHA)
}

func (s *CodeManagementService) CreateReviewComment(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
	comment types.PullRequestReviewComment,
) (*types.PullRequestReviewComment, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return nil, err
	}
	return svc.CreateReviewComment(ctx, orgData, repo, prNumber, comment)
}

func (s *CodeManagementService) CreateCommentInPullRequest(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
	body string,
) (*types.PullRequestReviewComment, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return nil, err
	}
	return svc.CreateCommentInPullRequest(ctx, orgData, repo, prNumber, body)
}

func (s *CodeManagementService) GetRepositoryContentFile(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	path, ref string,
) (*types.RepositoryFile, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return nil, err
	}
	return svc.GetRepositoryContentFile(ctx, orgData, repo, path, ref)
}

func (s *CodeManagementService) GetRepositoryContentBatch(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo types.RepositoryDescriptor,
	files []string,
	ref string,
) (map[string]*types.RepositoryFile, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return map[string]*types.RepositoryFile{}, err
	}
	return svc.GetRepositoryContentBatch(ctx, orgData, repo, files, ref)
}

// -------------------------------------------------------------------------------------
// Commits & Comments
// -------------------------------------------------------------------------------------

func (s *CodeManagementService) GetCommitsForPullRequestForCodeReview(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
) ([]*types.Commit, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return []*types.Commit{}, err
	}
	return svc.GetCommitsForPullRequestForCodeReview(ctx, orgData, repo, prNumber)
}

func (s *CodeManagementService) CreateIssueComment(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
	body string,
) (*types.PullRequestReviewComment, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return nil, err
	}
	return svc.CreateIssueComment(ctx, orgData, repo, prNumber, body)
}

func (s *CodeManagementService) CreateSingleIssueComment(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
	body string,
) (*types.PullRequestReviewComment, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return nil, err
	}
	return svc.CreateSingleIssueComment(ctx, orgData, repo, prNumber, body)
}

func (s *CodeManagementService) UpdateIssueComment(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	commentID string,
	body string,
) error {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return err
	}
	return svc.UpdateIssueComment(ctx, orgData, repo, commentID, body)
}

func (s *CodeManagementService) MinimizeComment(ctx context.Context, orgData types.OrganizationAndTeamData, commentID string, reason string) error {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return err
	}
	return svc.MinimizeComment(ctx, orgData, commentID, reason)
}

// -------------------------------------------------------------------------------------
// Branches & Discussion Threads
// -------------------------------------------------------------------------------------

func (s *CodeManagementService) GetDefaultBranch(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
) (string, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return "main", err
	}
	return svc.GetDefaultBranch(ctx, orgData, repo)
}

func (s *CodeManagementService) GetPullRequestReviewComment(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	commentID string,
) (*types.PullRequestReviewComment, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return nil, err
	}
	return svc.GetPullRequestReviewComment(ctx, orgData, repo, commentID)
}

func (s *CodeManagementService) CreateResponseToComment(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
	parentCommentID, body string,
) (*types.PullRequestReviewComment, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return nil, err
	}
	return svc.CreateResponseToComment(ctx, orgData, repo, prNumber, parentCommentID, body)
}

func (s *CodeManagementService) UpdateDescriptionInPullRequest(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
	description string,
) error {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return err
	}
	return svc.UpdateDescriptionInPullRequest(ctx, orgData, repo, prNumber, description)
}

func (s *CodeManagementService) GetAuthenticationOAuthToken(ctx context.Context, orgData types.OrganizationAndTeamData) (string, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return "", err
	}
	return svc.GetAuthenticationOAuthToken(ctx, orgData)
}

func (s *CodeManagementService) CountReactions(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
) ([]types.ReactionsInComments, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return []types.ReactionsInComments{}, err
	}
	return svc.CountReactions(ctx, orgData, repo, prNumber)
}

func (s *CodeManagementService) GetLanguageRepository(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
) (map[string]int, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return map[string]int{}, err
	}
	return svc.GetLanguageRepository(ctx, orgData, repo)
}

func (s *CodeManagementService) GetRepositoryAllFiles(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo types.RepositoryDescriptor,
	branch string,
	filePatterns, excludePatterns []string,
	maxFiles int,
) ([]*types.RepositoryFile, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return []*types.RepositoryFile{}, err
	}
	return svc.GetRepositoryAllFiles(ctx, orgData, repo, branch, filePatterns, excludePatterns, maxFiles)
}

func (s *CodeManagementService) GetCloneParams(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo types.RepositoryDescriptor,
) (*types.GitCloneParams, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return nil, err
	}
	return svc.GetCloneParams(ctx, orgData, repo)
}

func (s *CodeManagementService) MergePullRequest(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
	method string,
) error {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return err
	}
	return svc.MergePullRequest(ctx, orgData, repo, prNumber, method)
}

func (s *CodeManagementService) ApprovePullRequest(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
	message string,
) error {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return err
	}
	return svc.ApprovePullRequest(ctx, orgData, repo, prNumber, message)
}

func (s *CodeManagementService) RequestChangesPullRequest(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
	message string,
) error {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return err
	}
	return svc.RequestChangesPullRequest(ctx, orgData, repo, prNumber, message)
}

// -------------------------------------------------------------------------------------
// Comments & Users
// -------------------------------------------------------------------------------------

func (s *CodeManagementService) GetAllCommentsInPullRequest(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
) ([]*types.PullRequestReviewComment, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return []*types.PullRequestReviewComment{}, err
	}
	return svc.GetAllCommentsInPullRequest(ctx, orgData, repo, prNumber)
}

func (s *CodeManagementService) GetUserByUsername(ctx context.Context, orgData types.OrganizationAndTeamData, username string) (*types.PullRequestUser, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return nil, err
	}
	return svc.GetUserByUsername(ctx, orgData, username)
}

func (s *CodeManagementService) GetUsersByUsername(ctx context.Context, orgData types.OrganizationAndTeamData, usernames []string) (map[string]*types.PullRequestUser, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return map[string]*types.PullRequestUser{}, err
	}
	return svc.GetUsersByUsername(ctx, orgData, usernames)
}

func (s *CodeManagementService) GetUserByEmailOrName(ctx context.Context, orgData types.OrganizationAndTeamData, email, userName string) (*types.PullRequestUser, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return nil, err
	}
	return svc.GetUserByEmailOrName(ctx, orgData, email, userName)
}

func (s *CodeManagementService) GetUserByID(ctx context.Context, orgData types.OrganizationAndTeamData, userID string) (*types.PullRequestUser, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return nil, err
	}
	return svc.GetUserByID(ctx, orgData, userID)
}

func (s *CodeManagementService) GetCurrentUser(ctx context.Context, orgData types.OrganizationAndTeamData) (*types.PullRequestUser, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return nil, err
	}
	return svc.GetCurrentUser(ctx, orgData)
}

func (s *CodeManagementService) MarkReviewCommentAsResolved(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	threadID string,
) error {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return err
	}
	return svc.MarkReviewCommentAsResolved(ctx, orgData, repo, threadID)
}

func (s *CodeManagementService) GetPullRequestReviewComments(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
) ([]*types.PullRequestReviewComment, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return []*types.PullRequestReviewComment{}, err
	}
	return svc.GetPullRequestReviewComments(ctx, orgData, repo, prNumber)
}

func (s *CodeManagementService) GetPullRequestReviewThreads(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
) ([]*types.PullRequestReviewComment, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return []*types.PullRequestReviewComment{}, err
	}
	return svc.GetPullRequestReviewThreads(ctx, orgData, repo, prNumber)
}

func (s *CodeManagementService) GetListOfValidReviews(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
) ([]string, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return []string{}, err
	}
	return svc.GetListOfValidReviews(ctx, orgData, repo, prNumber)
}

func (s *CodeManagementService) GetPullRequestsWithChangesRequested(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
) ([]types.PullRequestsWithChangesRequested, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return []types.PullRequestsWithChangesRequested{}, err
	}
	return svc.GetPullRequestsWithChangesRequested(ctx, orgData, repo)
}

func (s *CodeManagementService) GetPullRequestAuthors(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	determineBots bool,
) ([]types.PullRequestAuthor, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return []types.PullRequestAuthor{}, err
	}
	return svc.GetPullRequestAuthors(ctx, orgData, determineBots)
}

func (s *CodeManagementService) CheckIfPullRequestShouldBeApproved(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	prNumber int,
	repo types.RepositoryDescriptor,
) (bool, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return false, err
	}
	return svc.CheckIfPullRequestShouldBeApproved(ctx, orgData, prNumber, repo)
}

// -------------------------------------------------------------------------------------
// Webhook lifecycle
// -------------------------------------------------------------------------------------

func (s *CodeManagementService) DeleteWebhook(ctx context.Context, orgData types.OrganizationAndTeamData) error {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return err
	}
	return svc.DeleteWebhook(ctx, orgData)
}

func (s *CodeManagementService) IsWebhookActive(ctx context.Context, orgData types.OrganizationAndTeamData, repositoryID string) (bool, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return false, err
	}
	return svc.IsWebhookActive(ctx, orgData, repositoryID)
}

// -------------------------------------------------------------------------------------
// Formatting & Tree
// -------------------------------------------------------------------------------------

func (s *CodeManagementService) FormatReviewCommentBody(suggestion any, repoLanguage string, includeHeader, includeFooter bool) string {
	var sb strings.Builder

	if includeHeader {
		sb.WriteString(defaultDrixyMarker)
		sb.WriteString("\n### 🛡️ ScanDrix AI Review Suggestion\n\n")
	}

	if text, ok := suggestion.(string); ok {
		sb.WriteString(text)
	} else if data, err := json.MarshalIndent(suggestion, "", "  "); err == nil {
		sb.WriteString("```json\n")
		sb.WriteString(string(data))
		sb.WriteString("\n```")
	}

	if includeFooter {
		sb.WriteString(defaultFooter)
	}

	return sb.String()
}

func (s *CodeManagementService) GetRepositoryTree(ctx context.Context, orgData types.OrganizationAndTeamData, repositoryID string) ([]*types.TreeItem, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return []*types.TreeItem{}, err
	}
	return svc.GetRepositoryTree(ctx, orgData, repositoryID)
}

func (s *CodeManagementService) GetRepositoryTreeByDirectory(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repositoryID, directoryPath string,
) ([]*types.TreeItem, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return []*types.TreeItem{}, err
	}
	return svc.GetRepositoryTreeByDirectory(ctx, orgData, repositoryID, directoryPath)
}

func (s *CodeManagementService) UpdateResponseToComment(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
	parentID, commentID, body string,
) (*types.PullRequestReviewComment, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return nil, err
	}
	return svc.UpdateResponseToComment(ctx, orgData, repo, prNumber, parentID, commentID, body)
}

func (s *CodeManagementService) IsDraftPullRequest(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
) (bool, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return false, err
	}
	return svc.IsDraftPullRequest(ctx, orgData, repo, prNumber)
}

func (s *CodeManagementService) GetReviewStatusByPullRequest(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
) (types.PullRequestReviewState, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return types.PullRequestReviewStatePending, err
	}
	return svc.GetReviewStatusByPullRequest(ctx, orgData, repo, prNumber)
}

// -------------------------------------------------------------------------------------
// Reactions
// -------------------------------------------------------------------------------------

func (s *CodeManagementService) AddReactionToPR(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo types.RepositoryDescriptor,
	prNumber int,
	reaction string,
) error {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return err
	}
	return svc.AddReactionToPR(ctx, orgData, repo, prNumber, reaction)
}

func (s *CodeManagementService) AddReactionToComment(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo types.RepositoryDescriptor,
	prNumber int,
	commentID int64,
	reaction string,
) error {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return err
	}
	return svc.AddReactionToComment(ctx, orgData, repo, prNumber, commentID, reaction)
}

func (s *CodeManagementService) RemoveReactionsFromPR(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo types.RepositoryDescriptor,
	prNumber int,
	reactions []string,
) error {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return err
	}
	return svc.RemoveReactionsFromPR(ctx, orgData, repo, prNumber, reactions)
}

func (s *CodeManagementService) RemoveReactionsFromComment(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo types.RepositoryDescriptor,
	prNumber int,
	commentID int64,
	reactions []string,
) error {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return err
	}
	return svc.RemoveReactionsFromComment(ctx, orgData, repo, prNumber, commentID, reactions)
}

func (s *CodeManagementService) ResolveMrAuthorFromWebhookPayload(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	payload any,
) (*types.PullRequestUser, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return nil, err
	}
	return svc.ResolveMrAuthorFromWebhookPayload(ctx, orgData, payload)
}

func (s *CodeManagementService) GetRecentRepositoryComments(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo types.RepositoryDescriptor,
	limit int,
) ([]*types.PullRequestReviewComment, error) {
	svc, err := s.GetUnderlyingService(orgData)
	if err != nil {
		return []*types.PullRequestReviewComment{}, err
	}
	return svc.GetRecentRepositoryComments(ctx, orgData, repo, limit)
}

