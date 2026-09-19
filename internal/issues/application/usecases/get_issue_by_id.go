package usecases

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/scandrix/backend/internal/issues/domain"
)

// GetIssueByIDUseCase retrieves an issue's full details including resolved deep links and sentiment reactions.
type GetIssueByIDUseCase struct {
	issuesService     domain.IssuesService
	managementService domain.DrixyIssuesManagementService
	feedbackReader    domain.CodeReviewFeedbackReader
	authService       domain.IssuesAuthorizationService
}

// NewGetIssueByIDUseCase creates an initialized use case.
func NewGetIssueByIDUseCase(
	issuesService domain.IssuesService,
	managementService domain.DrixyIssuesManagementService,
	feedbackReader domain.CodeReviewFeedbackReader,
	authService domain.IssuesAuthorizationService,
) *GetIssueByIDUseCase {
	return &GetIssueByIDUseCase{
		issuesService:     issuesService,
		managementService: managementService,
		feedbackReader:    feedbackReader,
		authService:       authService,
	}
}

// Execute looks up an issue by ID and verifies authorization.
func (uc *GetIssueByIDUseCase) Execute(ctx context.Context, id string, user domain.UserRef) (*domain.IssueDetails, error) {
	issue, err := uc.issuesService.FindByID(ctx, id)
	if err != nil || issue == nil || issue.Repository.ID == "" {
		return nil, err
	}

	if uc.authService != nil {
		if err := uc.authService.Ensure(ctx, user, "read", "issues", []string{issue.Repository.ID}); err != nil {
			return nil, fmt.Errorf("authorization denied: %w", err)
		}
	}

	reactions := domain.ReactionStats{ThumbsUp: 0, ThumbsDown: 0}
	if uc.feedbackReader != nil {
		feedbacks, err := uc.feedbackReader.GetByOrganizationID(ctx, issue.OrganizationID)
		if err == nil {
			reactions = uc.calculateReactions(issue, feedbacks)
		}
	}

	dataToBuildURLs := urlBuildData{
		Platform:           issue.Repository.Platform,
		RepositoryName:     issue.Repository.Name,
		RepositoryFullName: issue.Repository.FullName,
		HTTPURL:            issue.Repository.HTTPURL,
		RepositoryURL:      issue.Repository.URL,
	}

	prLinks := uc.buildPRLinks(issue, dataToBuildURLs)
	fileLink := domain.LinkInfo{
		Label: issue.FilePath,
		URL:   uc.buildFileURL(dataToBuildURLs, issue.FilePath, "main"),
	}
	repoLink := domain.LinkInfo{
		Label: issue.Repository.Name,
		URL:   uc.buildRepositoryURL(dataToBuildURLs),
	}

	var enrichedSuggestions []domain.ContributingSuggestion
	if uc.managementService != nil {
		enriched, err := uc.managementService.EnrichContributingSuggestions(ctx, issue.ContributingSuggestions, issue.OrganizationID)
		if err == nil {
			enrichedSuggestions = enriched
		} else {
			enrichedSuggestions = issue.ContributingSuggestions
		}
	} else {
		enrichedSuggestions = issue.ContributingSuggestions
	}

	age := ""
	if uc.managementService != nil {
		age = uc.managementService.AgeCalculation(issue.CreatedAt)
	}

	gitOrgName := ""
	if parts := strings.Split(issue.Repository.FullName, "/"); len(parts) > 0 {
		gitOrgName = parts[0]
	}

	return &domain.IssueDetails{
		ID:                      issue.UUID,
		Title:                   issue.Title,
		Description:             issue.Description,
		Age:                     age,
		Label:                   issue.Label,
		Severity:                issue.Severity,
		Status:                  issue.Status,
		ContributingSuggestions: enrichedSuggestions,
		FileLink:                fileLink,
		PRLinks:                 prLinks,
		RepositoryLink:          repoLink,
		Language:                issue.Language,
		Reactions:               reactions,
		GitOrganizationName:     gitOrgName,
		Repository: domain.RepoShortInfo{
			ID:   issue.Repository.ID,
			Name: issue.Repository.Name,
		},
	}, nil
}

type urlBuildData struct {
	Platform           domain.PlatformType
	RepositoryName     string
	RepositoryFullName string
	HTTPURL            string
	RepositoryURL      string
}

func (uc *GetIssueByIDUseCase) calculateReactions(issue *domain.Issue, feedbacks []domain.FeedbackItem) domain.ReactionStats {
	suggestionIDs := make(map[string]bool)
	for _, s := range issue.ContributingSuggestions {
		if s.ID != "" {
			suggestionIDs[s.ID] = true
		}
	}

	var up, down int
	for _, f := range feedbacks {
		if suggestionIDs[f.SuggestionID] {
			up += f.Reactions.ThumbsUp
			down += f.Reactions.ThumbsDown
		}
	}
	return domain.ReactionStats{ThumbsUp: up, ThumbsDown: down}
}

func (uc *GetIssueByIDUseCase) buildPRLinks(issue *domain.Issue, data urlBuildData) []domain.LinkInfo {
	prNumMap := make(map[int]bool)
	for _, s := range issue.ContributingSuggestions {
		if s.PRNumber > 0 {
			prNumMap[s.PRNumber] = true
		}
	}

	var prNums []int
	for n := range prNumMap {
		prNums = append(prNums, n)
	}
	sort.Ints(prNums)

	links := make([]domain.LinkInfo, len(prNums))
	for i, n := range prNums {
		strNum := strconv.Itoa(n)
		links[i] = domain.LinkInfo{
			Label: strNum,
			URL:   uc.buildPullRequestURL(data, strNum),
		}
	}
	return links
}

func (uc *GetIssueByIDUseCase) buildFileURL(data urlBuildData, filePath, branch string) string {
	cleanPath := strings.TrimPrefix(filePath, "/")
	repoURL := uc.buildRepositoryURL(data)

	switch data.Platform {
	case domain.PlatformGitHub:
		return fmt.Sprintf("%s/blob/%s/%s", repoURL, branch, cleanPath)
	case domain.PlatformGitLab:
		return fmt.Sprintf("%s/-/blob/%s/%s", repoURL, branch, cleanPath)
	case domain.PlatformAzureRepos:
		if data.HTTPURL != "" {
			return fmt.Sprintf("%s?path=/%s", data.HTTPURL, cleanPath)
		}
		return fmt.Sprintf("%s?path=/%s", repoURL, cleanPath)
	case domain.PlatformBitbucket:
		return fmt.Sprintf("https://bitbucket.org/%s/src/%s/%s", data.RepositoryFullName, branch, cleanPath)
	case domain.PlatformForgejo:
		return fmt.Sprintf("%s/src/branch/%s/%s", repoURL, branch, cleanPath)
	default:
		return fmt.Sprintf("%s/blob/%s/%s", repoURL, branch, cleanPath)
	}
}

func (uc *GetIssueByIDUseCase) buildPullRequestURL(data urlBuildData, prNumber string) string {
	repoURL := uc.buildRepositoryURL(data)

	switch data.Platform {
	case domain.PlatformGitHub:
		return fmt.Sprintf("%s/pull/%s", repoURL, prNumber)
	case domain.PlatformGitLab:
		return fmt.Sprintf("%s/-/merge_requests/%s", repoURL, prNumber)
	case domain.PlatformAzureRepos:
		if data.HTTPURL != "" {
			return fmt.Sprintf("%s/pullrequest/%s", data.HTTPURL, prNumber)
		}
		return fmt.Sprintf("%s/pullrequest/%s", repoURL, prNumber)
	case domain.PlatformBitbucket:
		return fmt.Sprintf("https://bitbucket.org/%s/pull-requests/%s", data.RepositoryFullName, prNumber)
	case domain.PlatformForgejo:
		return fmt.Sprintf("%s/pulls/%s", repoURL, prNumber)
	default:
		return fmt.Sprintf("%s/pull/%s", repoURL, prNumber)
	}
}

func (uc *GetIssueByIDUseCase) buildRepositoryURL(data urlBuildData) string {
	switch data.Platform {
	case domain.PlatformGitHub:
		if data.RepositoryURL != "" {
			if u, err := url.Parse(data.RepositoryURL); err == nil && u.Host != "" {
				return fmt.Sprintf("%s://%s/%s", u.Scheme, u.Host, data.RepositoryFullName)
			}
		}
		return fmt.Sprintf("https://github.com/%s", data.RepositoryFullName)
	case domain.PlatformGitLab:
		if data.RepositoryURL != "" {
			return data.RepositoryURL
		}
		return fmt.Sprintf("https://gitlab.com/%s", data.RepositoryFullName)
	case domain.PlatformAzureRepos:
		if data.HTTPURL != "" {
			return data.HTTPURL
		}
		return data.RepositoryURL
	case domain.PlatformBitbucket:
		return fmt.Sprintf("https://bitbucket.org/%s", data.RepositoryFullName)
	case domain.PlatformForgejo:
		if data.HTTPURL != "" {
			return data.HTTPURL
		}
		return data.RepositoryURL
	default:
		if data.RepositoryURL != "" {
			return data.RepositoryURL
		}
		return fmt.Sprintf("https://github.com/%s", data.RepositoryFullName)
	}
}
