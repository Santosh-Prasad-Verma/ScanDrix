// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: find_recommended_drixy_rules.go
// ═══════════════════════════════════════════════════════════════

package usecases

import (
	"context"

	"github.com/scandrix/backend/internal/rules/drixy/domain/contracts"
)

// RepositoryInfo provides basic repository metadata for recommendations.
type RepositoryInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Language string `json:"language"`
	Selected bool   `json:"selected"`
}

// IRepositoryProvider supplies configured repositories for an organization.
type IRepositoryProvider interface {
	GetRepositories(ctx context.Context, organizationID, teamID string) ([]RepositoryInfo, error)
}

// FindRecommendedDrixyRulesUseCase recommends catalog rules based on repository characteristics and historical PR suggestions.
type FindRecommendedDrixyRulesUseCase struct {
	rulesService       contracts.IDrixyRulesService
	repositoryProvider IRepositoryProvider
}

// NewFindRecommendedDrixyRulesUseCase constructs the use case.
func NewFindRecommendedDrixyRulesUseCase(
	rulesService contracts.IDrixyRulesService,
	repositoryProvider IRepositoryProvider,
) *FindRecommendedDrixyRulesUseCase {
	return &FindRecommendedDrixyRulesUseCase{
		rulesService:       rulesService,
		repositoryProvider: repositoryProvider,
	}
}

// Execute retrieves recommended rules for the organization's connected repositories.
func (uc *FindRecommendedDrixyRulesUseCase) Execute(
	ctx context.Context,
	organizationID, teamID string,
	limit int,
) ([]contracts.LibraryDrixyRule, error) {
	if limit <= 0 {
		limit = 10
	}

	var selectedRepos []RepositoryInfo
	if uc.repositoryProvider != nil {
		repos, err := uc.repositoryProvider.GetRepositories(ctx, organizationID, teamID)
		if err == nil {
			for _, r := range repos {
				if r.Selected {
					selectedRepos = append(selectedRepos, r)
				}
			}
		}
	}

	// Fallback to sample query if no repos provider configured
	if len(selectedRepos) == 0 {
		selectedRepos = []RepositoryInfo{{ID: "default", Selected: true}}
	}

	uniqueMap := make(map[string]contracts.LibraryDrixyRule)
	for _, repo := range selectedRepos {
		rules, err := uc.rulesService.GetRecommendedRulesBySuggestions(
			ctx, organizationID, teamID, repo.ID, repo.Language,
		)
		if err != nil {
			continue
		}
		for _, r := range rules {
			if _, exists := uniqueMap[r.UUID]; !exists {
				uniqueMap[r.UUID] = r
			}
		}
	}

	result := make([]contracts.LibraryDrixyRule, 0, len(uniqueMap))
	for _, r := range uniqueMap {
		result = append(result, r)
		if len(result) >= limit {
			break
		}
	}

	return result, nil
}
