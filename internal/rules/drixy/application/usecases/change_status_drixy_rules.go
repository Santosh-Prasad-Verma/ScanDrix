// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: change_status_drixy_rules.go
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

// ChangeStatusDrixyRulesUseCase modifies lifecycle status across multiple rules.
type ChangeStatusDrixyRulesUseCase struct {
	rulesService contracts.IDrixyRulesService
}

// NewChangeStatusDrixyRulesUseCase creates a new status change use case.
func NewChangeStatusDrixyRulesUseCase(rulesService contracts.IDrixyRulesService) *ChangeStatusDrixyRulesUseCase {
	return &ChangeStatusDrixyRulesUseCase{rulesService: rulesService}
}

// Execute updates status on specified rule IDs.
func (uc *ChangeStatusDrixyRulesUseCase) Execute(
	ctx context.Context,
	organizationID string,
	dto dtos.ChangeStatusDrixyRulesDTO,
) ([]interfaces.DrixyRule, error) {
	if organizationID == "" {
		return nil, errors.New("organization ID is required")
	}
	if len(dto.RuleIDs) == 0 {
		return []interfaces.DrixyRule{}, nil
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
	var modified []interfaces.DrixyRule
	now := time.Now().UTC()

	for i := range rules {
		if targetMap[rules[i].UUID] {
			rules[i].Status = dto.Status
			rules[i].UpdatedAt = &now
			if dto.Status == interfaces.DrixyRulesStatusActive {
				rules[i].LockedByPlan = false
			}
			modified = append(modified, rules[i])
		}
	}

	updatedEntity := entities.NewDrixyRulesEntity(interfaces.DrixyRules{
		UUID:           entity.UUID(),
		OrganizationID: organizationID,
		Rules:          rules,
		CreatedAt:      entity.CreatedAt(),
		UpdatedAt:      &now,
	})

	err = uc.rulesService.Save(ctx, updatedEntity)
	if err != nil {
		return nil, err
	}

	return modified, nil
}
