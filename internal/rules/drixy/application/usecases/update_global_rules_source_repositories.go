// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: update_global_rules_source_repositories.go
// ═══════════════════════════════════════════════════════════════

package usecases

import (
	"context"
	"fmt"

	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
	infraServices "github.com/scandrix/backend/internal/rules/drixy/infrastructure/adapters/services"
)

// UpdateGlobalRulesSourceRepositoriesUseCase configures source repositories for global rules.
type UpdateGlobalRulesSourceRepositoriesUseCase struct {
	configStore IGlobalRulesConfigStore
	syncService *infraServices.DrixyRulesSyncService
}

// NewUpdateGlobalRulesSourceRepositoriesUseCase constructs the update use case.
func NewUpdateGlobalRulesSourceRepositoriesUseCase(
	configStore IGlobalRulesConfigStore,
	syncService *infraServices.DrixyRulesSyncService,
) *UpdateGlobalRulesSourceRepositoriesUseCase {
	return &UpdateGlobalRulesSourceRepositoriesUseCase{
		configStore: configStore,
		syncService: syncService,
	}
}

// Execute reconciles and updates the list of repositories providing global rules.
func (uc *UpdateGlobalRulesSourceRepositoriesUseCase) Execute(
	ctx context.Context,
	organizationID, teamID string,
	repositories []interfaces.GlobalRulesSourceRepository,
) ([]interfaces.GlobalRulesSourceRepository, error) {
	if organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	var previous []interfaces.GlobalRulesSourceRepository
	if uc.configStore != nil {
		prev, err := uc.configStore.GetGlobalRulesSourceRepositories(ctx, organizationID, teamID)
		if err == nil {
			previous = prev
		}
	}

	prevMap := make(map[string]bool)
	for _, p := range previous {
		prevMap[p.ID] = true
	}

	nextMap := make(map[string]bool)
	for _, n := range repositories {
		nextMap[n.ID] = true
	}

	// Purge removed repositories
	for _, p := range previous {
		if !nextMap[p.ID] {
			_ = uc.syncService.PurgeGlobalRulesForSourceRepository(ctx, organizationID, teamID, p.ID)
		}
	}

	// Save new config
	if uc.configStore != nil {
		if err := uc.configStore.SetGlobalRulesSourceRepositories(ctx, organizationID, teamID, repositories); err != nil {
			return nil, fmt.Errorf("failed to save global source repositories: %w", err)
		}
	}

	// Sync added repositories in background
	for _, n := range repositories {
		if !prevMap[n.ID] {
			go func(repo interfaces.GlobalRulesSourceRepository) {
				_ = uc.syncService.SyncRepositoryGlobal(context.Background(), infraServices.SyncRepositoryParams{
					OrganizationID: organizationID,
					TeamID:         teamID,
					RepositoryID:   repo.ID,
					RepositoryName: repo.Name,
				})
			}(n)
		}
	}

	return repositories, nil
}
