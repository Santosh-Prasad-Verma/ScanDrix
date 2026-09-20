// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: delete_rule_in_organization_by_id.go
// ═══════════════════════════════════════════════════════════════

package usecases

import (
	"context"
	"errors"

	"github.com/scandrix/backend/internal/rules/drixy/domain/contracts"
)

// DeleteRuleInOrganizationByIdDrixyRulesUseCase handles deleting or soft-deleting a rule.
type DeleteRuleInOrganizationByIdDrixyRulesUseCase struct {
	rulesService contracts.IDrixyRulesService
}

// DeleteRuleInOrganizationByIDDrixyRulesUseCase aliases DeleteRuleInOrganizationByIdDrixyRulesUseCase
type DeleteRuleInOrganizationByIDDrixyRulesUseCase = DeleteRuleInOrganizationByIdDrixyRulesUseCase

// NewDeleteRuleInOrganizationByIdDrixyRulesUseCase initializes the use case.
func NewDeleteRuleInOrganizationByIdDrixyRulesUseCase(rulesService contracts.IDrixyRulesService) *DeleteRuleInOrganizationByIdDrixyRulesUseCase {
	return &DeleteRuleInOrganizationByIdDrixyRulesUseCase{rulesService: rulesService}
}

// NewDeleteRuleInOrganizationByIDDrixyRulesUseCase is an alias for NewDeleteRuleInOrganizationByIdDrixyRulesUseCase.
func NewDeleteRuleInOrganizationByIDDrixyRulesUseCase(rulesService contracts.IDrixyRulesService) *DeleteRuleInOrganizationByIdDrixyRulesUseCase {
	return NewDeleteRuleInOrganizationByIdDrixyRulesUseCase(rulesService)
}

// Execute removes or marks a rule deleted.
func (uc *DeleteRuleInOrganizationByIdDrixyRulesUseCase) Execute(
	ctx context.Context,
	organizationID string,
	teamID string,
	ruleID string,
	userInfo *contracts.UserAuditInfo,
) (bool, error) {
	if ruleID == "" {
		return false, errors.New("rule ID is required")
	}
	return uc.rulesService.DeleteRuleWithLogging(ctx, organizationID, teamID, ruleID, userInfo)
}
