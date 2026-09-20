package usecases

import (
	"context"
	"sort"

	"github.com/scandrix/backend/internal/cockpit/domain"
	"github.com/scandrix/backend/internal/cockpit/domain/helpers"
)

// DrixyRuleMeta models custom review rule definition.
type DrixyRuleMeta struct {
	UUID         string  `json:"uuid"`
	Title        string  `json:"title"`
	Severity     *string `json:"severity"`
	RepositoryID *string `json:"repository_id"`
	DirectoryID  *string `json:"directory_id"`
	Status       string  `json:"status"` // "ACTIVE"
}

// DrixyRulesDirectoryService retrieves configured rules and scope mappings.
type DrixyRulesDirectoryService interface {
	FindByOrganizationID(ctx context.Context, organizationID string) ([]DrixyRuleMeta, error)
	ResolveScopeMaps(ctx context.Context, organizationID string) (dirFolders map[string][]string, repoNames map[string]string, err error)
}

// GetDrixyRulesHealthUseCase aggregates review analytics and custom rule configurations.
type GetDrixyRulesHealthUseCase struct {
	reviewAnalytics domain.CockpitReviewAnalyticsService
	rulesDirectory  DrixyRulesDirectoryService
}

// NewGetDrixyRulesHealthUseCase creates an initialized use case.
func NewGetDrixyRulesHealthUseCase(
	reviewAnalytics domain.CockpitReviewAnalyticsService,
	rulesDirectory DrixyRulesDirectoryService,
) *GetDrixyRulesHealthUseCase {
	return &GetDrixyRulesHealthUseCase{
		reviewAnalytics: reviewAnalytics,
		rulesDirectory:  rulesDirectory,
	}
}

// Execute combines warehouse usage stats with rule definitions to categorize health states.
func (uc *GetDrixyRulesHealthUseCase) Execute(ctx context.Context, q domain.CockpitRangeQuery) ([]domain.DrixyRuleHealthRow, error) {
	usageRows, err := uc.reviewAnalytics.GetDrixyRulesUsage(ctx, q)
	if err != nil {
		return nil, err
	}

	rules, err := uc.rulesDirectory.FindByOrganizationID(ctx, q.OrganizationID)
	if err != nil {
		rules = nil
	}

	warehouseRepoNames, err := uc.reviewAnalytics.GetRepositoryNames(ctx, q.OrganizationID)
	if err != nil {
		warehouseRepoNames = make(map[string]string)
	}

	dirFolders, configRepoNames, _ := uc.rulesDirectory.ResolveScopeMaps(ctx, q.OrganizationID)
	if dirFolders == nil {
		dirFolders = make(map[string][]string)
	}
	if configRepoNames == nil {
		configRepoNames = make(map[string]string)
	}

	usageByRule := make(map[string]domain.DrixyRuleUsageRow, len(usageRows))
	for _, u := range usageRows {
		usageByRule[u.RuleID] = u
	}

	var results []domain.DrixyRuleHealthRow
	for _, rule := range rules {
		if rule.Status != "ACTIVE" && rule.Status != "active" && rule.Status != "" {
			continue
		}

		u, hasUsage := usageByRule[rule.UUID]
		var usagePtr *domain.DrixyRuleUsageRow
		if hasUsage {
			usagePtr = &u
		}

		state, evaluatedUsage := helpers.ComputeRuleState(usagePtr)

		var repoID *string
		if rule.RepositoryID != nil && *rule.RepositoryID != "" && *rule.RepositoryID != "global" {
			repoID = rule.RepositoryID
		}

		var repoName *string
		if repoID != nil {
			if name, ok := warehouseRepoNames[*repoID]; ok && name != "" {
				repoName = &name
			} else if name, ok := configRepoNames[*repoID]; ok && name != "" {
				repoName = &name
			}
		}

		var folders []string
		if rule.DirectoryID != nil {
			folders = dirFolders[*rule.DirectoryID]
		}

		results = append(results, domain.DrixyRuleHealthRow{
			DrixyRuleUsageRow: evaluatedUsage,
			Title:            rule.Title,
			Severity:         rule.Severity,
			RepositoryID:     repoID,
			RepositoryName:   repoName,
			DirectoryID:      rule.DirectoryID,
			DirectoryFolders: folders,
			State:            state,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Triggers > results[j].Triggers
	})

	return results, nil
}
