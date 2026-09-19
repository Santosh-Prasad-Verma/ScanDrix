// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: fast_sync_ide_rules.go
// ═══════════════════════════════════════════════════════════════

package usecases

import (
	"context"
	"fmt"

	infraServices "github.com/scandrix/backend/internal/rules/drixy/infrastructure/adapters/services"
)

// FastSyncIdeRulesParams defines arguments for triggered IDE fast synchronization.
type FastSyncIdeRulesParams struct {
	TeamID           string `json:"teamId"`
	RepositoryID     string `json:"repositoryId"`
	RepositoryName   string `json:"repositoryName,omitempty"`
	MaxFiles         int    `json:"maxFiles,omitempty"`
	MaxFileSizeBytes int64  `json:"maxFileSizeBytes,omitempty"`
	MaxTotalBytes    int64  `json:"maxTotalBytes,omitempty"`
	MaxConcurrent    int    `json:"maxConcurrent,omitempty"`
}

// FastSyncIdeRulesUseCase synchronizes IDE rule files in fast mode.
type FastSyncIdeRulesUseCase struct {
	syncService *infraServices.DrixyRulesSyncService
}

// NewFastSyncIdeRulesUseCase constructs the use case.
func NewFastSyncIdeRulesUseCase(
	syncService *infraServices.DrixyRulesSyncService,
) *FastSyncIdeRulesUseCase {
	return &FastSyncIdeRulesUseCase{
		syncService: syncService,
	}
}

// Execute triggers the fast file sync over the repository.
func (uc *FastSyncIdeRulesUseCase) Execute(
	ctx context.Context,
	organizationID string,
	params FastSyncIdeRulesParams,
) (*infraServices.SyncRepositoryResult, error) {
	if organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}
	if params.RepositoryID == "" {
		return nil, fmt.Errorf("repository ID is required")
	}

	res, err := uc.syncService.SyncRepositoryMainFast(ctx, infraServices.SyncRepositoryParams{
		OrganizationID:   organizationID,
		TeamID:           params.TeamID,
		RepositoryID:     params.RepositoryID,
		RepositoryName:   params.RepositoryName,
		MaxFiles:         params.MaxFiles,
		MaxFileSizeBytes: params.MaxFileSizeBytes,
		MaxTotalBytes:    params.MaxTotalBytes,
		MaxConcurrent:    params.MaxConcurrent,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fast sync rules: %w", err)
	}

	return res, nil
}
