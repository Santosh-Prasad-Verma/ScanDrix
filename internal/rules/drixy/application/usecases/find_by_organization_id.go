// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: find_by_organization_id.go
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

// FindByOrganizationIdDrixyRulesUseCase retrieves all active and paused rules for an organization.
type FindByOrganizationIdDrixyRulesUseCase struct {
	rulesService    contracts.IDrixyRulesService
	referenceLoader *services.ExternalReferenceLoaderService
}

// FindByOrganizationIDDrixyRulesUseCase aliases FindByOrganizationIdDrixyRulesUseCase
type FindByOrganizationIDDrixyRulesUseCase = FindByOrganizationIdDrixyRulesUseCase

// NewFindByOrganizationIdDrixyRulesUseCase constructs a new use case instance.
func NewFindByOrganizationIdDrixyRulesUseCase(
	rulesService contracts.IDrixyRulesService,
	referenceLoader *services.ExternalReferenceLoaderService,
) *FindByOrganizationIdDrixyRulesUseCase {
	return &FindByOrganizationIdDrixyRulesUseCase{
		rulesService:    rulesService,
		referenceLoader: referenceLoader,
	}
}

// NewFindByOrganizationIDDrixyRulesUseCase is an alias for NewFindByOrganizationIdDrixyRulesUseCase.
func NewFindByOrganizationIDDrixyRulesUseCase(
	rulesService contracts.IDrixyRulesService,
	referenceLoader *services.ExternalReferenceLoaderService,
) *FindByOrganizationIdDrixyRulesUseCase {
	return NewFindByOrganizationIdDrixyRulesUseCase(rulesService, referenceLoader)
}

// Execute returns the visible rules for an organization.
func (uc *FindByOrganizationIdDrixyRulesUseCase) Execute(ctx context.Context, organizationID string) (*interfaces.DrixyRules, error) {
	if organizationID == "" {
		return nil, errors.New("organization ID is required")
	}

	entity, err := uc.rulesService.FindByOrganizationID(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return &interfaces.DrixyRules{
			OrganizationID: organizationID,
			Rules:          []interfaces.DrixyRule{},
		}, nil
	}

	var visible []interfaces.DrixyRule
	for _, rule := range entity.Rules() {
		if rule.Status != interfaces.DrixyRulesStatusDeleted && rule.Status != interfaces.DrixyRulesStatusApplied {
			visible = append(visible, rule)
		}
	}

	enriched := utils.EnrichRulesWithContextReferences(ctx, visible, uc.referenceLoader)

	return &interfaces.DrixyRules{
		UUID:           entity.UUID(),
		OrganizationID: entity.OrganizationID(),
		Rules:          enriched,
		CreatedAt:      entity.CreatedAt(),
		UpdatedAt:      entity.UpdatedAt(),
	}, nil
}
