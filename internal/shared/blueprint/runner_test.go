// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package blueprint

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunBlueprint_DeterministicSequential(t *testing.T) {
	ctx := context.Background()
	initial := NewDefaultContext("org-123", "en-US")
	initial.Set("val", 10)

	step1 := DeterministicStep[*DefaultContext]{
		StepName: "add-5",
		Fn: func(_ context.Context, c *DefaultContext) (*DefaultContext, error) {
			v, _ := c.Get("val")
			c.Set("val", v.(int)+5)
			return c, nil
		},
	}

	step2 := DeterministicStep[*DefaultContext]{
		StepName: "multiply-2",
		Fn: func(_ context.Context, c *DefaultContext) (*DefaultContext, error) {
			v, _ := c.Get("val")
			c.Set("val", v.(int)*2)
			return c, nil
		},
	}

	var metrics []StepMetric
	opts := RunnerOptions[*DefaultContext]{
		Steps:          []BlueprintStep[*DefaultContext]{step1, step2},
		InitialContext: initial,
		OnStepMetric: func(m StepMetric) {
			metrics = append(metrics, m)
		},
	}

	result, err := RunBlueprint(ctx, opts)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, []string{"add-5", "multiply-2"}, result.CompletedSteps)
	assert.Empty(t, result.SkippedAt)

	val, _ := result.Context.Get("val")
	assert.Equal(t, 30, val)
	assert.Len(t, metrics, 2)
	assert.Equal(t, StatusSuccess, metrics[0].Status)
	assert.Equal(t, StatusSuccess, metrics[1].Status)
}

func TestRunBlueprint_GateShortCircuit(t *testing.T) {
	ctx := context.Background()
	initial := NewDefaultContext("org-123", "en-US")
	initial.Set("eligible", false)

	llmExecuted := false
	gate := GateStep[*DefaultContext]{
		StepName: "check-eligibility",
		Condition: func(_ context.Context, c *DefaultContext) bool {
			el, _ := c.Get("eligible")
			return el.(bool)
		},
		OnFail: func(_ context.Context, c *DefaultContext) (*DefaultContext, error) {
			c.SetResult("ineligible-skipped")
			return c, nil
		},
	}

	llm := LLMStep[*DefaultContext]{
		StepName:  "expensive-analysis",
		Skill:     "deep-review",
		AgentName: "finder",
	}

	opts := RunnerOptions[*DefaultContext]{
		Steps:          []BlueprintStep[*DefaultContext]{gate, llm},
		InitialContext: initial,
		RunLLMStep: func(_ context.Context, _ LLMStep[*DefaultContext], c *DefaultContext) (*DefaultContext, error) {
			llmExecuted = true
			return c, nil
		},
	}

	result, err := RunBlueprint(ctx, opts)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.False(t, llmExecuted, "Gate short-circuit MUST prevent any subsequent LLM execution")
	assert.Equal(t, "check-eligibility", result.SkippedAt)
	assert.Empty(t, result.CompletedSteps)
	assert.Equal(t, "ineligible-skipped", result.Context.GetResult())
}

func TestRunBlueprint_ContractValidation(t *testing.T) {
	ctx := context.Background()
	initial := NewDefaultContext("org-123", "en-US")

	contractFailStep := DeterministicStep[*DefaultContext]{
		StepName: "failing-contract-step",
		StepContract: &StepContract[*DefaultContext]{
			Input: ValidatorFunc[*DefaultContext](func(c *DefaultContext) error {
				if _, ok := c.Get("required-key"); !ok {
					return errors.New("missing required-key")
				}
				return nil
			}),
		},
		Fn: func(_ context.Context, c *DefaultContext) (*DefaultContext, error) {
			return c, nil
		},
	}

	opts := RunnerOptions[*DefaultContext]{
		Steps:          []BlueprintStep[*DefaultContext]{contractFailStep},
		InitialContext: initial,
	}

	_, err := RunBlueprint(ctx, opts)
	require.Error(t, err)
	var contractErr *BlueprintStepContractViolationError
	assert.True(t, errors.As(err, &contractErr))
	assert.Equal(t, "failing-contract-step", contractErr.StepName)
	assert.Equal(t, "input", contractErr.Stage)
}
