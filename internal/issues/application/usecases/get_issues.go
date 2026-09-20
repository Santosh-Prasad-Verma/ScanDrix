package usecases

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/scandrix/backend/internal/issues/domain"
)

const (
	issuesCacheTTL = 15 * time.Minute
)

// GetIssuesUseCase retrieves organization issues with caching and repository scoping.
type GetIssuesUseCase struct {
	issuesService domain.IssuesService
	cacheService  domain.IssueCacheService
	authService   domain.IssuesAuthorizationService
}

// NewGetIssuesUseCase creates an initialized use case.
func NewGetIssuesUseCase(
	issuesService domain.IssuesService,
	cacheService domain.IssueCacheService,
	authService domain.IssuesAuthorizationService,
) *GetIssuesUseCase {
	return &GetIssuesUseCase{
		issuesService: issuesService,
		cacheService:  cacheService,
		authService:   authService,
	}
}

// Execute retrieves issues according to filters and caller permissions.
func (uc *GetIssuesUseCase) Execute(ctx context.Context, filter domain.GetIssuesFilter, user domain.UserRef) ([]*domain.Issue, error) {
	if filter.OrganizationID == "" {
		return nil, fmt.Errorf("organizationId is required")
	}

	cacheKey := fmt.Sprintf("issues_%s", filter.OrganizationID)
	var allIssues []*domain.Issue

	if uc.cacheService != nil {
		cached, found, err := uc.cacheService.Get(ctx, cacheKey)
		if err == nil && found {
			allIssues = cached
		}
	}

	if allIssues == nil {
		fetched, err := uc.issuesService.Find(ctx, filter.OrganizationID)
		if err != nil {
			return nil, fmt.Errorf("failed fetching issues: %w", err)
		}

		if len(fetched) == 0 {
			return []*domain.Issue{}, nil
		}

		allIssues = make([]*domain.Issue, len(fetched))
		for i, issue := range fetched {
			cp := *issue
			prNums := uc.selectAllPRNumbers(issue)
			cp.PRNumbers = prNums
			cp.ContributingSuggestions = nil // Project without raw suggestion bloat
			allIssues[i] = &cp
		}

		// Sort newest first
		sort.Slice(allIssues, func(i, j int) bool {
			return allIssues[i].CreatedAt.After(allIssues[j].CreatedAt)
		})

		if uc.cacheService != nil {
			_ = uc.cacheService.Set(ctx, cacheKey, allIssues, issuesCacheTTL)
		}
	}

	if len(allIssues) == 0 {
		return []*domain.Issue{}, nil
	}

	// Filter by repository access if restricted
	if uc.authService != nil {
		assignedRepoIDs, err := uc.authService.GetRepositoryScope(ctx, user, "read", "issues")
		if err == nil && assignedRepoIDs != nil {
			allowedMap := make(map[string]bool, len(assignedRepoIDs))
			for _, id := range assignedRepoIDs {
				allowedMap[id] = true
			}

			var filtered []*domain.Issue
			for _, iss := range allIssues {
				if allowedMap[iss.Repository.ID] {
					filtered = append(filtered, iss)
				}
			}
			return filtered, nil
		}
	}

	return allIssues, nil
}

func (uc *GetIssuesUseCase) selectAllPRNumbers(issue *domain.Issue) []string {
	prNumMap := make(map[int]bool)
	for _, s := range issue.ContributingSuggestions {
		if s.PRNumber > 0 {
			prNumMap[s.PRNumber] = true
		}
	}

	var nums []int
	for n := range prNumMap {
		nums = append(nums, n)
	}
	sort.Ints(nums)

	result := make([]string, len(nums))
	for i, n := range nums {
		result[i] = strconv.Itoa(n)
	}
	return result
}
