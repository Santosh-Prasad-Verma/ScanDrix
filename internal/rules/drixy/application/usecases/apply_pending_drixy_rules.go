// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: apply_pending_drixy_rules.go
// ═══════════════════════════════════════════════════════════════

package usecases

import (
	"context"
	"errors"
	"time"

	"github.com/scandrix/backend/internal/rules/drixy/domain/contracts"
	"github.com/scandrix/backend/internal/rules/drixy/domain/entities"
	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
	"github.com/scandrix/backend/internal/rules/drixy/dtos"
)

// ApplyPendingDrixyRulesUseCase approves pending rule requests into active enforcement.
type ApplyPendingDrixyRulesUseCase struct {
	rulesService contracts.IDrixyRulesService
}

// NewApplyPendingDrixyRulesUseCase constructs a new use case.
func NewApplyPendingDrixyRulesUseCase(rulesService contracts.IDrixyRulesService) *ApplyPendingDrixyRulesUseCase {
	return &ApplyPendingDrixyRulesUseCase{rulesService: rulesService}
}

// Execute applies pending rules by IDs.
func (uc *ApplyPendingDrixyRulesUseCase) Execute(
	ctx context.Context,
	organizationID string,
	dto dtos.RuleIdsDto,
	userInfo *contracts.UserAuditInfo,
) ([]interfaces.DrixyRule, error) {
	if organizationID == "" {
		return nil, errors.New("organization ID is required")
	}

	entity, err := uc.rulesService.FindByOrganizationID(ctx, organizationID)
	if err != nil || entity == nil {
		return nil, errors.New("organization rules not found")
	}

	rules := entity.Rules()
	rulesByUUID := make(map[string]int)
	for idx, r := range rules {
		rulesByUUID[r.UUID] = idx
	}

	var applied []interfaces.DrixyRule
	now := time.Now().UTC()

	for _, pendingID := range dto.RuleIDs {
		idx, found := rulesByUUID[pendingID]
		if !found {
			continue
		}
		pendingRule := rules[idx]

		if pendingRule.RequestType == interfaces.DrixyRuleRequestTypeUpdate && pendingRule.TargetRuleUUID != "" {
			// Apply update to target rule
			if targetIdx, tFound := rulesByUUID[pendingRule.TargetRuleUUID]; tFound {
				rules[targetIdx].Title = pendingRule.Title
				rules[targetIdx].Rule = pendingRule.Rule
				rules[targetIdx].Severity = pendingRule.Severity
				rules[targetIdx].Examples = pendingRule.Examples
				rules[targetIdx].UpdatedAt = &now
				applied = append(applied, rules[targetIdx])
			}
			rules[idx].Status = interfaces.DrixyRulesStatusApplied
			rules[idx].UpdatedAt = &now
		} else {
			// Activate new rule
			rules[idx].Status = interfaces.DrixyRulesStatusActive
			rules[idx].LockedByPlan = false
			rules[idx].UpdatedAt = &now
			applied = append(applied, rules[idx])
		}
	}

	updated := entities.NewDrixyRulesEntity(interfaces.DrixyRules{
		UUID:           entity.UUID(),
		OrganizationID: organizationID,
		Rules:          rules,
		CreatedAt:      entity.CreatedAt(),
		UpdatedAt:      &now,
	})

	err = uc.rulesService.Save(ctx, updated)
	if err != nil {
		return nil, err
	}

	return applied, nil
}
