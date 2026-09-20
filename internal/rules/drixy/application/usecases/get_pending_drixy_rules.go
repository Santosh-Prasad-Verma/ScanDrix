// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: get_pending_drixy_rules.go
// ═══════════════════════════════════════════════════════════════

package usecases

import (
	"context"
	"errors"

	"github.com/scandrix/backend/internal/rules/drixy/domain/contracts"
	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
	"github.com/scandrix/backend/internal/rules/drixy/dtos"
)

// GetPendingDrixyRulesUseCase queries all pending rules and memories awaiting team approval.
type GetPendingDrixyRulesUseCase struct {
	rulesService contracts.IDrixyRulesService
}

// NewGetPendingDrixyRulesUseCase initializes the use case.
func NewGetPendingDrixyRulesUseCase(rulesService contracts.IDrixyRulesService) *GetPendingDrixyRulesUseCase {
	return &GetPendingDrixyRulesUseCase{rulesService: rulesService}
}

// Execute returns pending rules with category counts.
func (uc *GetPendingDrixyRulesUseCase) Execute(ctx context.Context, organizationID string, repositoryID string) (*dtos.PendingRulesResponse, error) {
	if organizationID == "" {
		return nil, errors.New("organization ID is required")
	}

	entity, err := uc.rulesService.FindByOrganizationID(ctx, organizationID)
	if err != nil || entity == nil {
		return &dtos.PendingRulesResponse{
			Items: []interfaces.DrixyRule{},
		}, nil
	}

	var items []interfaces.DrixyRule
	rulesCount := 0
	memoriesCount := 0

	for _, rule := range entity.Rules() {
		if rule.Status != interfaces.DrixyRulesStatusPending {
			continue
		}
		if repositoryID != "" && rule.RepositoryID != repositoryID {
			continue
		}

		if rule.Type == interfaces.DrixyRulesTypeMemory {
			memoriesCount++
		} else {
			rulesCount++
		}
		items = append(items, rule)
	}

	res := &dtos.PendingRulesResponse{
		Items: items,
	}
	res.Counts.Total = len(items)
	res.Counts.Rules = rulesCount
	res.Counts.Memories = memoriesCount

	return res, nil
}
