// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: resync_rules_from_ide.go
// ═══════════════════════════════════════════════════════════════

package usecases

import (
	"context"
	"fmt"

	infraServices "github.com/scandrix/backend/internal/rules/drixy/infrastructure/adapters/services"
)

// ResyncRulesFromIdeParams contains arguments for manual IDE rules resynchronization.
type ResyncRulesFromIdeParams struct {
	TeamID          string   `json:"teamId"`
	RepositoriesIDs []string `json:"repositoriesIds"`
	Path            string   `json:"path,omitempty"`
}

// ResyncRulesFromIdeUseCase manually re-scans IDE rule files across target repositories.
type ResyncRulesFromIdeUseCase struct {
	syncService *infraServices.DrixyRulesSyncService
}

// NewResyncRulesFromIdeUseCase constructs the resync use case.
func NewResyncRulesFromIdeUseCase(
	syncService *infraServices.DrixyRulesSyncService,
) *ResyncRulesFromIdeUseCase {
	return &ResyncRulesFromIdeUseCase{
		syncService: syncService,
	}
}

// Execute performs rule re-synchronization across specified repositories.
func (uc *ResyncRulesFromIdeUseCase) Execute(
	ctx context.Context,
	organizationID string,
	params ResyncRulesFromIdeParams,
) error {
	if organizationID == "" {
		return fmt.Errorf("organization ID is required")
	}

	for _, repoID := range params.RepositoriesIDs {
		_ = uc.syncService.SyncRepositoryMain(ctx, infraServices.SyncRepositoryParams{
			OrganizationID: organizationID,
			TeamID:         params.TeamID,
			RepositoryID:   repoID,
		})
	}

	return nil
}
