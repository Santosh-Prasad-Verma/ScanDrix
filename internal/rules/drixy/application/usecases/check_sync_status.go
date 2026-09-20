// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: check_sync_status.go
// ═══════════════════════════════════════════════════════════════

package usecases

import (
	"context"

	"github.com/scandrix/backend/internal/rules/drixy/domain/contracts"
	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
)

// SyncStatusFlags indicates onboarding/initial sync states for rules sync and rule learning.
type SyncStatusFlags struct {
	IdeRulesSyncEnabledFirstTime        bool `json:"ideRulesSyncEnabledFirstTime"`
	DrixyRulesGeneratorEnabledFirstTime bool `json:"drixyRulesGeneratorEnabledFirstTime"`
}

// CheckSyncStatusUseCase determines if a repository is undergoing rule sync for the first time.
type CheckSyncStatusUseCase struct {
	rulesService contracts.IDrixyRulesService
}

// NewCheckSyncStatusUseCase constructs the check use case.
func NewCheckSyncStatusUseCase(
	rulesService contracts.IDrixyRulesService,
) *CheckSyncStatusUseCase {
	return &CheckSyncStatusUseCase{
		rulesService: rulesService,
	}
}

// Execute checks whether IDE file sync and review-learning generator have previously seeded rules.
func (uc *CheckSyncStatusUseCase) Execute(
	ctx context.Context,
	organizationID, teamID string,
	repositoryID string,
) (SyncStatusFlags, error) {
	flags := SyncStatusFlags{
		IdeRulesSyncEnabledFirstTime:        true,
		DrixyRulesGeneratorEnabledFirstTime: true,
	}

	entity, err := uc.rulesService.FindByOrganizationID(ctx, organizationID)
	if err != nil || entity == nil {
		return flags, nil
	}

	hasIdeRules := false
	hasPastReviewRules := false

	for _, rule := range entity.Rules() {
		if repositoryID != "" && rule.RepositoryID != repositoryID {
			continue
		}

		if rule.SourcePath != "" || rule.Origin == interfaces.DrixyRulesOriginRepoFileSync {
			hasIdeRules = true
		}
		if rule.Origin == interfaces.DrixyRulesOriginPastReviews {
			hasPastReviewRules = true
		}
	}

	flags.IdeRulesSyncEnabledFirstTime = !hasIdeRules
	flags.DrixyRulesGeneratorEnabledFirstTime = !hasPastReviewRules

	return flags, nil
}
