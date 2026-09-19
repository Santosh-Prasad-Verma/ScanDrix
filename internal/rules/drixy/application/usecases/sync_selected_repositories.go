// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: sync_selected_repositories.go
// ═══════════════════════════════════════════════════════════════

package usecases

import (
	"context"

	infraServices "github.com/scandrix/backend/internal/rules/drixy/infrastructure/adapters/services"
)

// SyncSelectedRepositoriesDrixyRulesUseCase synchronizes rules for a set of selected repositories.
type SyncSelectedRepositoriesDrixyRulesUseCase struct {
	syncService *infraServices.DrixyRulesSyncService
}

// NewSyncSelectedRepositoriesDrixyRulesUseCase constructs the use case.
func NewSyncSelectedRepositoriesDrixyRulesUseCase(
	syncService *infraServices.DrixyRulesSyncService,
) *SyncSelectedRepositoriesDrixyRulesUseCase {
	return &SyncSelectedRepositoriesDrixyRulesUseCase{
		syncService: syncService,
	}
}

// Execute performs rule synchronization for specified repositories.
func (uc *SyncSelectedRepositoriesDrixyRulesUseCase) Execute(
	ctx context.Context,
	organizationID, teamID string,
	repositoriesIDs []string,
) error {
	for _, repoID := range repositoriesIDs {
		_ = uc.syncService.SyncRepositoryMain(ctx, infraServices.SyncRepositoryParams{
			OrganizationID: organizationID,
			TeamID:         teamID,
			RepositoryID:   repoID,
		})
	}
	return nil
}
