// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: backfill_rule_detectors.go
// ═══════════════════════════════════════════════════════════════

package usecases

import (
	"context"

	"github.com/scandrix/backend/internal/rules/drixy/domain/contracts"
	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
)

// BackfillDetectorsOptions controls filtering and batching for detector generation.
type BackfillDetectorsOptions struct {
	OnlyMissing bool `json:"onlyMissing"`
	Limit       int  `json:"limit,omitempty"`
	Concurrency int  `json:"concurrency,omitempty"`
}

// BackfillDetectorsResult tracks outcomes across processed rules.
type BackfillDetectorsResult struct {
	Total     int `json:"total"`
	Processed int `json:"processed"`
	Compiled  int `json:"compiled"`
	Declined  int `json:"declined"`
	Errored   int `json:"errored"`
	Skipped   int `json:"skipped"`
}

// BackfillRuleDetectorsUseCase activates T0 deterministic regex detectors on existing rules.
type BackfillRuleDetectorsUseCase struct {
	rulesService     contracts.IDrixyRulesService
	detectorCompiler contracts.IDrixyRuleDetectorCompiler
}

// NewBackfillRuleDetectorsUseCase constructs the backfill use case.
func NewBackfillRuleDetectorsUseCase(
	rulesService contracts.IDrixyRulesService,
	detectorCompiler contracts.IDrixyRuleDetectorCompiler,
) *BackfillRuleDetectorsUseCase {
	return &BackfillRuleDetectorsUseCase{
		rulesService:     rulesService,
		detectorCompiler: detectorCompiler,
	}
}

// Execute sweeps an organization's rules and builds verified mechanical detectors.
func (uc *BackfillRuleDetectorsUseCase) Execute(
	ctx context.Context,
	organizationID, teamID string,
	opts BackfillDetectorsOptions,
) (BackfillDetectorsResult, error) {
	result := BackfillDetectorsResult{}

	entity, err := uc.rulesService.FindByOrganizationID(ctx, organizationID)
	if err != nil || entity == nil {
		return result, err
	}

	allRules := entity.Rules()
	result.Total = len(allRules)

	var eligible []interfaces.DrixyRule
	for _, r := range allRules {
		if r.UUID == "" || r.Status != interfaces.StatusActive || r.Type == interfaces.TypeMemory {
			continue
		}
		if opts.OnlyMissing && r.Detector != nil {
			continue
		}
		eligible = append(eligible, r)
	}

	target := eligible
	if opts.Limit > 0 && len(target) > opts.Limit {
		target = target[:opts.Limit]
	}

	result.Skipped = result.Total - len(target)

	for _, rule := range target {
		result.Processed++
		cRes, err := uc.detectorCompiler.CompileAndSave(ctx, organizationID, teamID, rule.UUID, &rule)
		if err != nil || cRes.DeclineReason == "error" {
			result.Errored++
		} else if cRes.Compiled {
			result.Compiled++
		} else {
			result.Declined++
		}
	}

	return result, nil
}
