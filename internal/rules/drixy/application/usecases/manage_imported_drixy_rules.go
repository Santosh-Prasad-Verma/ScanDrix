// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: manage_imported_drixy_rules.go
// ═══════════════════════════════════════════════════════════════

package usecases

import (
	"context"
	"fmt"

	"github.com/scandrix/backend/internal/rules/drixy/dtos"
	infraServices "github.com/scandrix/backend/internal/rules/drixy/infrastructure/adapters/services"
)

// ManageImportedDrixyRulesUseCase controls batch operations (pause, resume, delete) on IDE-synced rules.
type ManageImportedDrixyRulesUseCase struct {
	syncService *infraServices.DrixyRulesSyncService
}

// NewManageImportedDrixyRulesUseCase constructs the management use case.
func NewManageImportedDrixyRulesUseCase(
	syncService *infraServices.DrixyRulesSyncService,
) *ManageImportedDrixyRulesUseCase {
	return &ManageImportedDrixyRulesUseCase{
		syncService: syncService,
	}
}

// Execute performs bulk lifecycle transitions on imported rules for a given repository.
func (uc *ManageImportedDrixyRulesUseCase) Execute(
	ctx context.Context,
	organizationID, teamID string,
	dto dtos.ManageImportedDrixyRulesDto,
) (*dtos.ManageImportedRulesResult, error) {
	if organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}
	if dto.RepositoryID == "" {
		return nil, fmt.Errorf("repository ID is required")
	}

	switch dto.Action {
	case dtos.ManageImportedRulesActionPause:
		if err := uc.syncService.PauseAllIdeSyncRulesForRepository(ctx, organizationID, teamID, dto.RepositoryID); err != nil {
			return nil, fmt.Errorf("failed to pause rules: %w", err)
		}
	case dtos.ManageImportedRulesActionResume:
		if err := uc.syncService.ResumeAllIdeSyncRulesForRepository(ctx, organizationID, teamID, dto.RepositoryID); err != nil {
			return nil, fmt.Errorf("failed to resume rules: %w", err)
		}
	case dtos.ManageImportedRulesActionDelete:
		if err := uc.syncService.PurgeAllIdeSyncRulesForRepository(ctx, organizationID, teamID, dto.RepositoryID); err != nil {
			return nil, fmt.Errorf("failed to delete rules: %w", err)
		}
	default:
		return nil, fmt.Errorf("invalid action: %s", dto.Action)
	}

	counts, err := uc.syncService.CountIdeSyncRulesForRepository(ctx, organizationID, teamID, dto.RepositoryID)
	if err != nil {
		return nil, fmt.Errorf("failed to count rules: %w", err)
	}

	res := &dtos.ManageImportedRulesResult{
		Action: dto.Action,
	}
	res.Counts.Active = counts.Active
	res.Counts.Paused = counts.Paused
	res.Counts.Deleted = counts.Deleted
	res.Counts.Pinned = counts.Pinned

	return res, nil
}

// Count returns rule metrics for the specified repository.
func (uc *ManageImportedDrixyRulesUseCase) Count(
	ctx context.Context,
	organizationID, teamID, repositoryID string,
) (struct {
	Active  int
	Paused  int
	Deleted int
	Pinned  int
}, error) {
	return uc.syncService.CountIdeSyncRulesForRepository(ctx, organizationID, teamID, repositoryID)
}
