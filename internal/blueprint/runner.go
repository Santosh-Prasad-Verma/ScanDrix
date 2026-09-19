// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package blueprint

import (
	"context"
	"fmt"
	"time"
)

// RunBlueprint executes a skill blueprint against an initial context in strict sequential order.
// It enforces input/output contracts, records step metrics, and guarantees immediate short-circuiting
// when a gate step condition fails (zero subsequent steps or LLM calls are made).
func RunBlueprint[T any](ctx context.Context, opts RunnerOptions[T]) (*BlueprintResult[T], error) {
	currentCtx := opts.InitialContext
	var completedSteps []string

	for _, step := range opts.Steps {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		stepName := step.Name()
		stepType := step.Type()
		startedAt := time.Now()

		emitMetric := func(status StepExecutionStatus, errMsg string) {
			if opts.OnStepMetric != nil {
				opts.OnStepMetric(StepMetric{
					StepName:     stepName,
					StepType:     stepType,
					Status:       status,
					DurationMs:   time.Since(startedAt).Milliseconds(),
					ErrorMessage: errMsg,
				})
			}
		}

		if opts.Logger != nil {
			opts.Logger.Log("[blueprint] running step: %s (%s)", stepName, stepType)
		}

		// 1. Validate Input Contract
		var err error
		currentCtx, err = ValidateContract(stepName, "input", step.GetContract(), currentCtx)
		if err != nil {
			emitMetric(StatusFailed, err.Error())
			if opts.Logger != nil {
				opts.Logger.Error(fmt.Sprintf("[blueprint] step '%s' input contract violation", stepName), err)
			}
			return nil, err
		}

		// 2. Execute Step
		newCtx, status, execErr := step.Execute(ctx, currentCtx, &opts)
		if execErr != nil {
			emitMetric(StatusFailed, execErr.Error())
			if opts.Logger != nil {
				opts.Logger.Error(fmt.Sprintf("[blueprint] step '%s' execution failed", stepName), execErr)
			}
			return nil, execErr
		}

		// 3. Handle Gate Failure Short-Circuit
		if status == StatusSkipped {
			if opts.Logger != nil {
				opts.Logger.Log("[blueprint] gate '%s' failed — short-circuiting", stepName)
			}

			// Validate Output Contract on early-exit context
			validatedSkipCtx, outErr := ValidateContract(stepName, "output", step.GetContract(), newCtx)
			if outErr != nil {
				emitMetric(StatusFailed, outErr.Error())
				return nil, outErr
			}

			emitMetric(StatusSkipped, "")
			return &BlueprintResult[T]{
				Context:        validatedSkipCtx,
				CompletedSteps: completedSteps,
				SkippedAt:      stepName,
			}, nil
		}

		// 4. Validate Output Contract
		validatedCtx, outErr := ValidateContract(stepName, "output", step.GetContract(), newCtx)
		if outErr != nil {
			emitMetric(StatusFailed, outErr.Error())
			if opts.Logger != nil {
				opts.Logger.Error(fmt.Sprintf("[blueprint] step '%s' output contract violation", stepName), outErr)
			}
			return nil, outErr
		}

		currentCtx = validatedCtx
		completedSteps = append(completedSteps, stepName)
		emitMetric(StatusSuccess, "")
	}

	return &BlueprintResult[T]{
		Context:        currentCtx,
		CompletedSteps: completedSteps,
	}, nil
}
