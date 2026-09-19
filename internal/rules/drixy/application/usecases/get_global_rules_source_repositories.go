// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: get_global_rules_source_repositories.go
// ═══════════════════════════════════════════════════════════════

package usecases

import (
	"context"

	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
)

// IGlobalRulesConfigStore provides access to global source repository configuration.
type IGlobalRulesConfigStore interface {
	GetGlobalRulesSourceRepositories(ctx context.Context, organizationID, teamID string) ([]interfaces.GlobalRulesSourceRepository, error)
	SetGlobalRulesSourceRepositories(ctx context.Context, organizationID, teamID string, repos []interfaces.GlobalRulesSourceRepository) error
}

// GetGlobalRulesSourceRepositoriesUseCase queries source repositories providing global rules.
type GetGlobalRulesSourceRepositoriesUseCase struct {
	configStore IGlobalRulesConfigStore
}

// NewGetGlobalRulesSourceRepositoriesUseCase constructs the use case.
func NewGetGlobalRulesSourceRepositoriesUseCase(
	configStore IGlobalRulesConfigStore,
) *GetGlobalRulesSourceRepositoriesUseCase {
	return &GetGlobalRulesSourceRepositoriesUseCase{
		configStore: configStore,
	}
}

// Execute returns configured global rules source repositories.
func (uc *GetGlobalRulesSourceRepositoriesUseCase) Execute(
	ctx context.Context,
	organizationID, teamID string,
) ([]interfaces.GlobalRulesSourceRepository, error) {
	if uc.configStore == nil {
		return []interfaces.GlobalRulesSourceRepository{}, nil
	}

	repos, err := uc.configStore.GetGlobalRulesSourceRepositories(ctx, organizationID, teamID)
	if err != nil {
		return []interfaces.GlobalRulesSourceRepository{}, nil
	}
	if repos == nil {
		return []interfaces.GlobalRulesSourceRepository{}, nil
	}

	return repos, nil
}
