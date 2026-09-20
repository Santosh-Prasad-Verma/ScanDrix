// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Application Layer
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package usecases

import (
	"context"
	"fmt"

	"github.com/scandrix/backend/internal/agents/businessrules"
)

// BusinessRulesValidationAgentUseCase coordinates execution of the business rules validation agent.
type BusinessRulesValidationAgentUseCase struct {
	pipeline *businessrules.BlueprintPipeline
	provider *businessrules.BusinessRulesValidationAgentProvider
}

// NewBusinessRulesValidationAgentUseCase creates a new BusinessRulesValidationAgentUseCase.
func NewBusinessRulesValidationAgentUseCase(
	provider *businessrules.BusinessRulesValidationAgentProvider,
) *BusinessRulesValidationAgentUseCase {
	return &BusinessRulesValidationAgentUseCase{
		pipeline: businessrules.NewBlueprintPipeline(provider),
		provider: provider,
	}
}

// Execute coordinates the complete business rules validation pipeline.
func (uc *BusinessRulesValidationAgentUseCase) Execute(
	ctx context.Context,
	bctx businessrules.BusinessRulesContext,
) (*businessrules.ValidationResult, error) {
	if uc.pipeline == nil {
		return nil, fmt.Errorf("business rules validation pipeline is not initialized")
	}

	result, err := uc.pipeline.Run(ctx, bctx)
	if err != nil {
		return nil, fmt.Errorf("failed to process business rules validation: %w", err)
	}

	return result, nil
}
