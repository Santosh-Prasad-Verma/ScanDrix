// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: resync_global_rules.go
// ═══════════════════════════════════════════════════════════════

package usecases

import (
	"context"

	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
	infraServices "github.com/scandrix/backend/internal/rules/drixy/infrastructure/adapters/services"
)

// ResyncGlobalRulesResult summarizes the dispatched resync operation.
type ResyncGlobalRulesResult struct {
	Repositories int `json:"repositories"`
}

// ResyncGlobalRulesUseCase triggers a re-scan of all configured source repositories into global scope.
type ResyncGlobalRulesUseCase struct {
	configStore IGlobalRulesConfigStore
	syncService *infraServices.DrixyRulesSyncService
}

// NewResyncGlobalRulesUseCase constructs the global resync use case.
func NewResyncGlobalRulesUseCase(
	configStore IGlobalRulesConfigStore,
	syncService *infraServices.DrixyRulesSyncService,
) *ResyncGlobalRulesUseCase {
	return &ResyncGlobalRulesUseCase{
		configStore: configStore,
		syncService: syncService,
	}
}

// Execute initiates asynchronous background resync of all global rule sources.
func (uc *ResyncGlobalRulesUseCase) Execute(
	ctx context.Context,
	organizationID, teamID string,
) (*ResyncGlobalRulesResult, error) {
	var repos []interfaces.GlobalRulesSourceRepository
	if uc.configStore != nil {
		r, err := uc.configStore.GetGlobalRulesSourceRepositories(ctx, organizationID, teamID)
		if err == nil {
			repos = r
		}
	}

	go func(targetRepos []interfaces.GlobalRulesSourceRepository) {
		for _, repo := range targetRepos {
			_ = uc.syncService.SyncRepositoryGlobal(context.Background(), infraServices.SyncRepositoryParams{
				OrganizationID: organizationID,
				TeamID:         teamID,
				RepositoryID:   repo.ID,
				RepositoryName: repo.Name,
			})
		}
	}(repos)

	return &ResyncGlobalRulesResult{
		Repositories: len(repos),
	}, nil
}
