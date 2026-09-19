// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: get_global_rules_import_status.go
// ═══════════════════════════════════════════════════════════════

package usecases

import (
	"context"

	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
	infraServices "github.com/scandrix/backend/internal/rules/drixy/infrastructure/adapters/services"
)

// IPlanTierProvider resolves an organization's subscription plan tier.
type IPlanTierProvider interface {
	ResolveGlobalRulesImportTier(ctx context.Context, organizationID, teamID string) (interfaces.GlobalRulesImportTier, error)
}

// GetGlobalRulesImportStatusUseCase evaluates global rule import limits and usage.
type GetGlobalRulesImportStatusUseCase struct {
	planTierProvider IPlanTierProvider
	syncService      *infraServices.DrixyRulesSyncService
}

// NewGetGlobalRulesImportStatusUseCase constructs the import status use case.
func NewGetGlobalRulesImportStatusUseCase(
	planTierProvider IPlanTierProvider,
	syncService *infraServices.DrixyRulesSyncService,
) *GetGlobalRulesImportStatusUseCase {
	return &GetGlobalRulesImportStatusUseCase{
		planTierProvider: planTierProvider,
		syncService:      syncService,
	}
}

// Execute returns the import tier, limit, used count, and remaining quota.
func (uc *GetGlobalRulesImportStatusUseCase) Execute(
	ctx context.Context,
	organizationID, teamID string,
) (interfaces.GlobalRulesImportStatus, error) {
	tier := interfaces.GlobalRulesImportTierPaid
	if uc.planTierProvider != nil {
		resolvedTier, err := uc.planTierProvider.ResolveGlobalRulesImportTier(ctx, organizationID, teamID)
		if err == nil && resolvedTier != "" {
			tier = resolvedTier
		}
	}

	used, err := uc.syncService.CountGlobalSyncedRules(ctx, organizationID, teamID)
	if err != nil {
		used = 0
	}

	switch tier {
	case interfaces.GlobalRulesImportTierFree:
		zero := 0
		return interfaces.GlobalRulesImportStatus{
			Tier:      interfaces.GlobalRulesImportTierFree,
			Limit:     &zero,
			Used:      used,
			Remaining: &zero,
		}, nil
	case interfaces.GlobalRulesImportTierTrial:
		limit := interfaces.GlobalRulesTrialImportLimit
		rem := limit - used
		if rem < 0 {
			rem = 0
		}
		return interfaces.GlobalRulesImportStatus{
			Tier:      interfaces.GlobalRulesImportTierTrial,
			Limit:     &limit,
			Used:      used,
			Remaining: &rem,
		}, nil
	default:
		return interfaces.GlobalRulesImportStatus{
			Tier:      interfaces.GlobalRulesImportTierPaid,
			Limit:     nil,
			Used:      used,
			Remaining: nil,
		}, nil
	}
}
