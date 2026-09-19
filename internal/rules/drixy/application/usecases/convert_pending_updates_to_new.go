// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: convert_pending_updates_to_new.go
// ═══════════════════════════════════════════════════════════════

package usecases

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/rules/drixy/domain/contracts"
	"github.com/scandrix/backend/internal/rules/drixy/domain/entities"
	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
	"github.com/scandrix/backend/internal/rules/drixy/dtos"
)

// ConvertPendingUpdatesToNewUseCase branches pending update requests into standalone new active rules.
type ConvertPendingUpdatesToNewUseCase struct {
	rulesService contracts.IDrixyRulesService
}

// NewConvertPendingUpdatesToNewUseCase initializes the use case.
func NewConvertPendingUpdatesToNewUseCase(rulesService contracts.IDrixyRulesService) *ConvertPendingUpdatesToNewUseCase {
	return &ConvertPendingUpdatesToNewUseCase{rulesService: rulesService}
}

// Execute branches pending update requests into distinct new rules.
func (uc *ConvertPendingUpdatesToNewUseCase) Execute(
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

	targetMap := make(map[string]bool)
	for _, id := range dto.RuleIDs {
		targetMap[id] = true
	}

	rules := entity.Rules()
	var createdRules []interfaces.DrixyRule
	now := time.Now().UTC()

	for i := range rules {
		if targetMap[rules[i].UUID] {
			// Clone into new rule
			newRule := rules[i]
			newRule.UUID = uuid.New().String()
			newRule.Status = interfaces.DrixyRulesStatusActive
			newRule.RequestType = ""
			newRule.TargetRuleUUID = ""
			newRule.CreatedAt = &now
			newRule.UpdatedAt = &now
			createdRules = append(createdRules, newRule)

			// Mark original update request discarded
			rules[i].Status = interfaces.DrixyRulesStatusRejected
			rules[i].UpdatedAt = &now
		}
	}

	rules = append(rules, createdRules...)

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

	return createdRules, nil
}
