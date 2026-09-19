// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: import_fast_drixy_rules.go
// ═══════════════════════════════════════════════════════════════

package usecases

import (
	"context"
	"fmt"

	"github.com/scandrix/backend/internal/rules/drixy/domain/contracts"
	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
	"github.com/scandrix/backend/internal/rules/drixy/dtos"
)

// ImportFastDrixyRulesUseCase saves rules discovered through fast repository scanning.
type ImportFastDrixyRulesUseCase struct {
	createOrUpdateUseCase *CreateOrUpdateDrixyRuleUseCase
}

// NewImportFastDrixyRulesUseCase constructs the use case.
func NewImportFastDrixyRulesUseCase(
	createOrUpdateUseCase *CreateOrUpdateDrixyRuleUseCase,
) *ImportFastDrixyRulesUseCase {
	return &ImportFastDrixyRulesUseCase{
		createOrUpdateUseCase: createOrUpdateUseCase,
	}
}

// Execute parses and registers batch imported fast-sync rules.
func (uc *ImportFastDrixyRulesUseCase) Execute(
	ctx context.Context,
	organizationID string,
	dto dtos.ImportFastDrixyRulesDto,
	userInfo *contracts.UserAuditInfo,
) ([]*interfaces.DrixyRule, error) {
	if organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	// If userInfo is nil, pass nil directly without fabricating mock audit data.

	results := make([]*interfaces.DrixyRule, 0, len(dto.Rules))

	for _, rule := range dto.Rules {
		path := rule.Path
		if path == "" {
			path = "**/*"
		}

		severity := "MEDIUM"
		if rule.Severity != "" {
			severity = string(interfaces.ResolveDrixyRuleSeverityLevelFromString(rule.Severity))
		}

		scope := rule.Scope
		if scope == "" {
			scope = interfaces.DrixyRulesScopeFile
		}

		createDto := dtos.CreateDrixyRuleDto{
			Title:        rule.Title,
			Rule:         rule.Rule,
			Path:         path,
			Severity:     severity,
			Scope:        scope,
			RepositoryID: rule.RepositoryID,
			Examples:     rule.Examples,
			Origin:       interfaces.DrixyRulesOriginRepoFileSync,
			Status:       interfaces.DrixyRulesStatusActive,
			Type:         interfaces.DrixyRulesTypeStandard,
		}

		created, err := uc.createOrUpdateUseCase.Execute(ctx, createDto, organizationID, userInfo)
		if err != nil {
			continue
		}
		results = append(results, created)
	}

	return results, nil
}
