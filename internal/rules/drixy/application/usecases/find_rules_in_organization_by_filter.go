// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: find_rules_in_organization_by_filter.go
// ═══════════════════════════════════════════════════════════════

package usecases

import (
	"context"
	"errors"

	"github.com/scandrix/backend/internal/rules/drixy/application/usecases/utils"
	"github.com/scandrix/backend/internal/rules/drixy/domain/contracts"
	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
	"github.com/scandrix/backend/internal/rules/drixy/infrastructure/adapters/services"
)

// FindRulesInOrganizationByFilterDrixyRulesUseCase filters rules by scope, status, and repository hierarchy.
type FindRulesInOrganizationByFilterDrixyRulesUseCase struct {
	rulesService    contracts.IDrixyRulesService
	referenceLoader *services.ExternalReferenceLoaderService
}

// NewFindRulesInOrganizationByFilterDrixyRulesUseCase creates a new filter use case.
func NewFindRulesInOrganizationByFilterDrixyRulesUseCase(
	rulesService contracts.IDrixyRulesService,
	referenceLoader *services.ExternalReferenceLoaderService,
) *FindRulesInOrganizationByFilterDrixyRulesUseCase {
	return &FindRulesInOrganizationByFilterDrixyRulesUseCase{
		rulesService:    rulesService,
		referenceLoader: referenceLoader,
	}
}

// Execute retrieves rules matching specified filters.
func (uc *FindRulesInOrganizationByFilterDrixyRulesUseCase) Execute(
	ctx context.Context,
	organizationID string,
	filter map[string]any,
	repositoryID string,
	directoryID string,
) ([]interfaces.DrixyRule, error) {
	if organizationID == "" {
		return nil, errors.New("organization ID is required")
	}

	entity, err := uc.rulesService.FindByOrganizationID(ctx, organizationID)
	if err != nil || entity == nil {
		return []interfaces.DrixyRule{}, nil
	}

	allRules := entity.Rules()
	var scoped []interfaces.DrixyRule

	for _, rule := range allRules {
		// Repository & Directory Scope Matching with global inheritance
		if repositoryID != "" && directoryID == "" {
			if rule.RepositoryID != "global" && (rule.RepositoryID != repositoryID || rule.DirectoryID != "") {
				continue
			}
		} else if repositoryID != "" && directoryID != "" {
			if rule.RepositoryID != "global" && (rule.RepositoryID != repositoryID || (rule.DirectoryID != "" && rule.DirectoryID != directoryID)) {
				continue
			}
		}

		// Status filter (exclude DELETED and APPLIED by default unless explicitly specified)
		statusFilter, hasStatus := filter["status"].(string)
		if hasStatus && statusFilter != "" {
			if string(rule.Status) != statusFilter {
				continue
			}
		} else {
			if rule.Status == interfaces.DrixyRulesStatusDeleted || rule.Status == interfaces.DrixyRulesStatusApplied {
				continue
			}
		}

		// Type filter
		typeFilter, hasType := filter["type"].(string)
		if hasType && typeFilter != "" {
			if string(rule.Type) != typeFilter {
				continue
			}
		}

		scoped = append(scoped, rule)
	}

	enriched := utils.EnrichRulesWithContextReferences(ctx, scoped, uc.referenceLoader)
	return enriched, nil
}
