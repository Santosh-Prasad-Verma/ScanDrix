package usecases

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/platform/domain/contracts"
	"github.com/scandrix/backend/internal/platform/domain/types"
)

// IRepositoryConfigReader reads configured repositories for an organization and team.
type IRepositoryConfigReader interface {
	FindConfiguredRepositories(ctx context.Context, organizationID, teamID string) ([]*types.Repositories, error)
}

// FilteredPRResponse represents a normalized pull request response.
// Translates the returned structure of get-prs.use-case.ts and get-prs-repo.use-case.ts.
type FilteredPRResponse struct {
	ID         string                     `json:"id"`
	Repository types.RepositoryDescriptor `json:"repository"`
	PullNumber int                        `json:"pull_number"`
	Title      string                     `json:"title"`
	URL        string                     `json:"url"`
}

// GetPRsFilters specifies optional search filters for pull requests.
type GetPRsFilters struct {
	Number    *int
	Title     *string
	URL       *string
	State     *string
	Author    *string
	Branch    *string
	StartDate *time.Time
	EndDate   *time.Time
}

// GetPRsParams defines the parameters for GetPRsUseCase.
type GetPRsParams struct {
	OrganizationID string
	TeamID         string
	RepositoryID   *string
	RepositoryName *string
	Filters        GetPRsFilters
}

// GetPRsUseCase translates get-prs.use-case.ts.
// It retrieves, filters, groups and caps pull requests across repositories.
type GetPRsUseCase struct {
	codeManagement contracts.ICodeManagementService
	configReader   IRepositoryConfigReader
}

// NewGetPRsUseCase creates a new instance of GetPRsUseCase.
func NewGetPRsUseCase(
	codeManagement contracts.ICodeManagementService,
	configReader IRepositoryConfigReader,
) *GetPRsUseCase {
	return &GetPRsUseCase{
		codeManagement: codeManagement,
		configReader:   configReader,
	}
}

// Execute retrieves and normalizes pull requests matching criteria.
func (uc *GetPRsUseCase) Execute(ctx context.Context, params GetPRsParams) ([]FilteredPRResponse, error) {
	orgData := types.OrganizationAndTeamData{
		OrganizationID: params.OrganizationID,
		TeamID:         params.TeamID,
	}

	repoDescriptor, err := uc.resolveRepository(ctx, orgData, params.RepositoryID, params.RepositoryName)
	if err != nil {
		slog.WarnContext(ctx, "Repository resolution failed or filter did not match",
			"organizationId", params.OrganizationID,
			"teamId", params.TeamID,
			"repositoryId", params.RepositoryID,
			"repositoryName", params.RepositoryName,
			"error", err,
		)
		return []FilteredPRResponse{}, nil
	}

	state := "open"
	if params.Filters.State != nil && *params.Filters.State != "" {
		state = *params.Filters.State
	}

	author := ""
	if params.Filters.Author != nil {
		author = *params.Filters.Author
	}

	branch := ""
	if params.Filters.Branch != nil {
		branch = *params.Filters.Branch
	}

	pullRequests, err := uc.codeManagement.GetPullRequests(ctx, orgData, repoDescriptor, state, author, branch)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to retrieve pull requests from code management",
			"organizationId", params.OrganizationID,
			"teamId", params.TeamID,
			"error", err,
		)
		return []FilteredPRResponse{}, nil
	}

	if len(pullRequests) == 0 {
		return []FilteredPRResponse{}, nil
	}

	// Apply in-memory client filters (number, title, url, dates)
	filtered := uc.applyClientFilters(pullRequests, params.Filters)

	// Limit to 20 PRs per repository (matching getLimitedPrsByRepo)
	limited := uc.getLimitedPrsByRepo(filtered, 20)

	// Format response
	return uc.formatPRs(limited), nil
}

func (uc *GetPRsUseCase) resolveRepository(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repoID *string,
	repoName *string,
) (*types.RepositoryDescriptor, error) {
	if repoID == nil && repoName == nil {
		return nil, nil // Global / all repos
	}

	if repoID != nil && *repoID == "global" {
		return nil, nil
	}

	if uc.configReader == nil {
		desc := &types.RepositoryDescriptor{}
		if repoID != nil {
			desc.ID = *repoID
		}
		if repoName != nil {
			desc.Name = *repoName
		}
		return desc, nil
	}

	configured, err := uc.configReader.FindConfiguredRepositories(ctx, orgData.OrganizationID, orgData.TeamID)
	if err != nil {
		return nil, err
	}

	for _, r := range configured {
		if repoID != nil && fmt.Sprintf("%v", r.ID) == *repoID {
			return &types.RepositoryDescriptor{
				ID:   fmt.Sprintf("%v", r.ID),
				Name: r.Name,
			}, nil
		}
		if repoName != nil && strings.EqualFold(r.Name, *repoName) {
			return &types.RepositoryDescriptor{
				ID:   fmt.Sprintf("%v", r.ID),
				Name: r.Name,
			}, nil
		}
	}

	return nil, fmt.Errorf("repository not found for specified filter")
}

func (uc *GetPRsUseCase) applyClientFilters(prs []*types.PullRequest, filters GetPRsFilters) []*types.PullRequest {
	var result []*types.PullRequest

	startDate := time.Now().Add(-30 * 24 * time.Hour)
	if filters.StartDate != nil {
		startDate = *filters.StartDate
	}

	endDate := time.Now()
	if filters.EndDate != nil {
		endDate = *filters.EndDate
	}

	for _, pr := range prs {
		if pr == nil {
			continue
		}

		if filters.Number != nil && pr.Number != *filters.Number {
			continue
		}

		if filters.Title != nil && *filters.Title != "" {
			title := pr.Title
			if title == "" {
				title = pr.Message
			}
			if !strings.Contains(strings.ToLower(title), strings.ToLower(*filters.Title)) {
				continue
			}
		}

		if filters.URL != nil && *filters.URL != "" {
			normalizedPrUrl := strings.TrimRight(strings.ToLower(pr.PRURL), "/")
			normalizedFilterUrl := strings.TrimRight(strings.ToLower(*filters.URL), "/")
			if normalizedPrUrl != normalizedFilterUrl {
				continue
			}
		}

		if pr.CreatedAt != "" {
			if t, err := time.Parse(time.RFC3339, pr.CreatedAt); err == nil {
				if t.Before(startDate) || t.After(endDate) {
					continue
				}
			}
		}

		result = append(result, pr)
	}

	return result
}

func (uc *GetPRsUseCase) getLimitedPrsByRepo(prs []*types.PullRequest, limitPerRepo int) []*types.PullRequest {
	if limitPerRepo <= 0 {
		limitPerRepo = 20
	}

	grouped := make(map[string][]*types.PullRequest)
	for _, pr := range prs {
		repoKey := pr.RepositoryData.Name
		if repoKey == "" {
			repoKey = pr.Repository
		}
		if repoKey == "" {
			repoKey = "default"
		}
		grouped[repoKey] = append(grouped[repoKey], pr)
	}

	var limited []*types.PullRequest
	for _, list := range grouped {
		if len(list) > limitPerRepo {
			limited = append(limited, list[:limitPerRepo]...)
		} else {
			limited = append(limited, list...)
		}
	}

	return limited
}

func (uc *GetPRsUseCase) formatPRs(prs []*types.PullRequest) []FilteredPRResponse {
	results := make([]FilteredPRResponse, 0, len(prs))
	for _, pr := range prs {
		id := pr.ID
		if id == "" {
			id = pr.RepositoryData.ID
		}
		if id == "" {
			id = fmt.Sprintf("%d", pr.Number)
		}

		title := pr.Message
		if title == "" {
			title = pr.Title
		}

		results = append(results, FilteredPRResponse{
			ID:         id,
			Repository: pr.RepositoryData,
			PullNumber: pr.Number,
			Title:      title,
			URL:        pr.PRURL,
		})
	}
	return results
}
