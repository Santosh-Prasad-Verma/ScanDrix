// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: get_rules_limit_status.go
// ═══════════════════════════════════════════════════════════════

package usecases

import (
	"context"
	"fmt"

	"github.com/scandrix/backend/internal/rules/drixy/domain/contracts"
)

// RulesLimitStatusResponse contains current rule counts against subscription capacity.
type RulesLimitStatusResponse struct {
	Total int `json:"total"`
}

// GetRulesLimitStatusUseCase checks the rule limit utilization for an organization.
type GetRulesLimitStatusUseCase struct {
	rulesService contracts.IDrixyRulesService
}

// NewGetRulesLimitStatusUseCase constructs the limit status use case.
func NewGetRulesLimitStatusUseCase(
	rulesService contracts.IDrixyRulesService,
) *GetRulesLimitStatusUseCase {
	return &GetRulesLimitStatusUseCase{
		rulesService: rulesService,
	}
}

// Execute returns the count of active rules contributing to quota limits.
func (uc *GetRulesLimitStatusUseCase) Execute(
	ctx context.Context,
	organizationID, teamID string,
) (*RulesLimitStatusResponse, error) {
	if organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	total, err := uc.rulesService.GetRulesLimitStatus(ctx, organizationID, teamID)
	if err != nil {
		return nil, fmt.Errorf("failed to get rules limit status: %w", err)
	}

	return &RulesLimitStatusResponse{
		Total: total,
	}, nil
}
