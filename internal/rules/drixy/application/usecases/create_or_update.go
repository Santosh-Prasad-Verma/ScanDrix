// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: create_or_update.go
// ═══════════════════════════════════════════════════════════════

package usecases

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/rules/drixy/domain/contracts"
	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
	"github.com/scandrix/backend/internal/rules/drixy/dtos"
	"github.com/scandrix/backend/internal/rules/drixy/infrastructure/adapters/services"
)

// CreateOrUpdateDrixyRulesUseCase handles persisting new or modified rules.
type CreateOrUpdateDrixyRulesUseCase struct {
	rulesService      contracts.IDrixyRulesService
	detectorCompiler  contracts.IDrixyRuleDetectorCompiler
	summaryService    *services.DrixyRuleSummaryService
	referenceLoader   *services.ExternalReferenceLoaderService
}

// CreateOrUpdateDrixyRuleUseCase is an alias for CreateOrUpdateDrixyRulesUseCase.
type CreateOrUpdateDrixyRuleUseCase = CreateOrUpdateDrixyRulesUseCase

// NewCreateOrUpdateDrixyRulesUseCase initializes the use case.
func NewCreateOrUpdateDrixyRulesUseCase(
	rulesService contracts.IDrixyRulesService,
	compiler contracts.IDrixyRuleDetectorCompiler,
	summaryService *services.DrixyRuleSummaryService,
	referenceLoader *services.ExternalReferenceLoaderService,
) *CreateOrUpdateDrixyRulesUseCase {
	return &CreateOrUpdateDrixyRulesUseCase{
		rulesService:     rulesService,
		detectorCompiler: compiler,
		summaryService:   summaryService,
		referenceLoader:  referenceLoader,
	}
}

// NewCreateOrUpdateDrixyRuleUseCase is an alias constructor for NewCreateOrUpdateDrixyRulesUseCase.
func NewCreateOrUpdateDrixyRuleUseCase(
	rulesService contracts.IDrixyRulesService,
	compiler contracts.IDrixyRuleDetectorCompiler,
	summaryService *services.DrixyRuleSummaryService,
	referenceLoader *services.ExternalReferenceLoaderService,
) *CreateOrUpdateDrixyRulesUseCase {
	return NewCreateOrUpdateDrixyRulesUseCase(rulesService, compiler, summaryService, referenceLoader)
}

// Execute persists the rule, compiling detectors and summaries where applicable.
func (uc *CreateOrUpdateDrixyRulesUseCase) Execute(
	ctx context.Context,
	dto dtos.CreateDrixyRuleDto,
	organizationID string,
	userInfo *contracts.UserAuditInfo,
) (*interfaces.DrixyRule, error) {
	if organizationID == "" {
		return nil, errors.New("organization ID is required")
	}

	rule := dto.ToRule()
	if rule.UUID == "" {
		rule.UUID = uuid.New().String()
		now := time.Now().UTC()
		rule.CreatedAt = &now
	}
	now := time.Now().UTC()
	rule.UpdatedAt = &now

	// 1. Enrich external references
	if uc.referenceLoader != nil {
		uc.referenceLoader.EnrichRule(ctx, &rule)
	}

	// 2. Generate structured validation summaries and atoms for long rules
	if uc.summaryService != nil && len(rule.Rule) >= services.LongRuleThresholdChars {
		if summary, err := uc.summaryService.GenerateSummary(ctx, &rule); err == nil && summary != nil {
			rule.Summary = summary
		}
		if atoms, err := uc.summaryService.DecomposeRule(ctx, &rule); err == nil && atoms != nil {
			rule.Atoms = atoms
		}
	}

	// 3. Compile deterministic T0 detector if rule is mechanical
	if uc.detectorCompiler != nil {
		res, err := uc.detectorCompiler.CompileAndSave(ctx, organizationID, dto.TeamID, rule.UUID, &rule)
		if err == nil && res.Compiled && res.Detector != nil {
			rule.Detector = res.Detector
		}
	}

	// 4. Save to persistence layer
	saved, err := uc.rulesService.UpdateRuleWithLogging(ctx, organizationID, dto.TeamID, &rule, userInfo)
	if err != nil {
		return nil, err
	}

	return saved, nil
}
