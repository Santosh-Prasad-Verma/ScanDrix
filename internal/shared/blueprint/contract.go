// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package blueprint

// ValidateContract evaluates an input or output contract against the given context.
func ValidateContract[T any](stepName, stage string, contract *StepContract[T], ctx T) (T, error) {
	if contract == nil {
		return ctx, nil
	}

	var validator Validator[T]
	if stage == "input" {
		validator = contract.Input
	} else {
		validator = contract.Output
	}

	if validator == nil {
		return ctx, nil
	}

	if err := validator.Validate(ctx); err != nil {
		return ctx, &BlueprintStepContractViolationError{
			StepName: stepName,
			Stage:    stage,
			Details:  err.Error(),
		}
	}

	return ctx, nil
}
