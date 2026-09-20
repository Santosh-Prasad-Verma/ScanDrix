// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package blueprint

import (
	"fmt"
	"strings"
)

// ValidateContract evaluates the given stage contract ("input" or "output").
// Returns a BlueprintStepContractViolationError if validation fails.
func ValidateContract[T any](stepName string, stage string, contract *StepContract[T], ctx T) (T, error) {
	if contract == nil {
		return ctx, nil
	}

	var validator Validator[T]
	switch stage {
	case "input":
		validator = contract.Input
	case "output":
		validator = contract.Output
	default:
		return ctx, fmt.Errorf("unknown contract validation stage '%s'", stage)
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

// NewContract builds a StepContract from input and output validator functions.
func NewContract[T any](input ValidatorFunc[T], output ValidatorFunc[T]) *StepContract[T] {
	var inValidator Validator[T]
	if input != nil {
		inValidator = input
	}
	var outValidator Validator[T]
	if output != nil {
		outValidator = output
	}
	return &StepContract[T]{
		Input:  inValidator,
		Output: outValidator,
	}
}

// RequireNonEmptyString returns a ValidatorFunc that asserts a field string is non-empty.
func RequireNonEmptyString[T any](fieldName string, getter func(c T) string) ValidatorFunc[T] {
	return func(c T) error {
		if getter == nil {
			return fmt.Errorf("getter function is nil for field '%s'", fieldName)
		}
		val := strings.TrimSpace(getter(c))
		if val == "" {
			return fmt.Errorf("field '%s' is required and cannot be empty", fieldName)
		}
		return nil
	}
}

// RequireNonNull returns a ValidatorFunc that asserts a field is not nil.
func RequireNonNull[T any](fieldName string, getter func(c T) any) ValidatorFunc[T] {
	return func(c T) error {
		if getter == nil {
			return fmt.Errorf("getter function is nil for field '%s'", fieldName)
		}
		if val := getter(c); val == nil {
			return fmt.Errorf("field '%s' cannot be nil", fieldName)
		}
		return nil
	}
}

// CombineValidators merges multiple validators into a single ValidatorFunc, executing each in sequence.
func CombineValidators[T any](validators ...ValidatorFunc[T]) ValidatorFunc[T] {
	return func(c T) error {
		var errs []string
		for _, v := range validators {
			if v != nil {
				if err := v(c); err != nil {
					errs = append(errs, err.Error())
				}
			}
		}
		if len(errs) > 0 {
			return fmt.Errorf("%s", strings.Join(errs, "; "))
		}
		return nil
	}
}
