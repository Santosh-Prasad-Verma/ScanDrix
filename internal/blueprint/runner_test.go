// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package blueprint

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testLogger struct {
	logs []string
	mu   sync.Mutex
}

func (l *testLogger) Log(msg string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logs = append(l.logs, fmt.Sprintf(msg, args...))
}

func (l *testLogger) Error(msg string, err error, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logs = append(l.logs, fmt.Sprintf("%s: %v", fmt.Sprintf(msg, args...), err))
}

func TestRunBlueprint_DeterministicAndFormatFlow(t *testing.T) {
	ctx := context.Background()
	logger := &testLogger{}
	var metrics []StepMetric

	initialCtx := NewDefaultContext(map[string]any{"org_id": "test-org"}, "en-US")
	initialCtx.Set("counter", 1)

	steps := []BlueprintStep[*DefaultContext]{
		DeterministicStep[*DefaultContext]{
			StepName: "step-increment",
			Fn: func(ctx context.Context, c *DefaultContext) (*DefaultContext, error) {
				val, _ := c.Get("counter")
				c.Set("counter", val.(int)+1)
				return c, nil
			},
		},
		FormatStep[*DefaultContext]{
			StepName: "step-format",
			Fn: func(c *DefaultContext) (*DefaultContext, error) {
				val, _ := c.Get("counter")
				c.SetResult(fmt.Sprintf("final_count_%d", val.(int)))
				return c, nil
			},
		},
	}

	res, err := RunBlueprint(ctx, RunnerOptions[*DefaultContext]{
		Steps:          steps,
		InitialContext: initialCtx,
		OnStepMetric: func(m StepMetric) {
			metrics = append(metrics, m)
		},
		Logger: logger,
	})

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, []string{"step-increment", "step-format"}, res.CompletedSteps)
	assert.Empty(t, res.SkippedAt)
	assert.Equal(t, "final_count_2", res.Context.GetResult())

	require.Len(t, metrics, 2)
	assert.Equal(t, "step-increment", metrics[0].StepName)
	assert.Equal(t, StatusSuccess, metrics[0].Status)
	assert.Equal(t, "step-format", metrics[1].StepName)
	assert.Equal(t, StatusSuccess, metrics[1].Status)
}

func TestRunBlueprint_GateShortCircuitsZeroLLMCalls(t *testing.T) {
	ctx := context.Background()
	logger := &testLogger{}
	var metrics []StepMetric
	llmCalled := false

	initialCtx := NewDefaultContext(nil, "en-US")
	initialCtx.Set("is_eligible", false)

	steps := []BlueprintStep[*DefaultContext]{
		DeterministicStep[*DefaultContext]{
			StepName: "extract-context",
			Fn: func(ctx context.Context, c *DefaultContext) (*DefaultContext, error) {
				c.Set("extracted", true)
				return c, nil
			},
		},
		GateStep[*DefaultContext]{
			StepName: "eligibility-gate",
			Condition: func(ctx context.Context, c *DefaultContext) bool {
				val, _ := c.Get("is_eligible")
				return val == true
			},
			OnFail: func(ctx context.Context, c *DefaultContext) (*DefaultContext, error) {
				c.SetResult("Gate check failed: task requirements not satisfied")
				return c, nil
			},
		},
		LLMStep[*DefaultContext]{
			StepName:  "expensive-llm-analysis",
			Skill:     "deep-architectural-review",
			AgentName: "arbiter-agent",
		},
		FormatStep[*DefaultContext]{
			StepName: "final-formatter",
			Fn: func(c *DefaultContext) (*DefaultContext, error) {
				c.Set("formatted", true)
				return c, nil
			},
		},
	}

	res, err := RunBlueprint(ctx, RunnerOptions[*DefaultContext]{
		Steps:          steps,
		InitialContext: initialCtx,
		RunLLMStep: func(ctx context.Context, step LLMStep[*DefaultContext], c *DefaultContext) (*DefaultContext, error) {
			llmCalled = true
			return c, nil
		},
		OnStepMetric: func(m StepMetric) {
			metrics = append(metrics, m)
		},
		Logger: logger,
	})

	require.NoError(t, err)
	require.NotNil(t, res)

	// Verify short-circuiting
	assert.False(t, llmCalled, "CRITICAL: LLM step must NOT be executed when gate condition fails")
	assert.Equal(t, "eligibility-gate", res.SkippedAt)
	assert.Equal(t, []string{"extract-context"}, res.CompletedSteps)
	assert.Equal(t, "Gate check failed: task requirements not satisfied", res.Context.GetResult())

	// Formatter should not have executed
	_, hasFormatted := res.Context.Get("formatted")
	assert.False(t, hasFormatted)

	require.Len(t, metrics, 2)
	assert.Equal(t, "extract-context", metrics[0].StepName)
	assert.Equal(t, StatusSuccess, metrics[0].Status)
	assert.Equal(t, "eligibility-gate", metrics[1].StepName)
	assert.Equal(t, StatusSkipped, metrics[1].Status)
}

func TestRunBlueprint_LLMStepDelegation(t *testing.T) {
	ctx := context.Background()
	llmExecuted := false

	initialCtx := NewDefaultContext(nil, "en-US")

	steps := []BlueprintStep[*DefaultContext]{
		LLMStep[*DefaultContext]{
			StepName:  "llm-review",
			Skill:     "security-auditor",
			AgentName: "sec-agent",
		},
	}

	res, err := RunBlueprint(ctx, RunnerOptions[*DefaultContext]{
		Steps:          steps,
		InitialContext: initialCtx,
		RunLLMStep: func(ctx context.Context, step LLMStep[*DefaultContext], c *DefaultContext) (*DefaultContext, error) {
			assert.Equal(t, "security-auditor", step.Skill)
			assert.Equal(t, "sec-agent", step.AgentName)
			llmExecuted = true
			c.SetResult("security review passed with zero findings")
			return c, nil
		},
	})

	require.NoError(t, err)
	assert.True(t, llmExecuted)
	assert.Equal(t, "security review passed with zero findings", res.Context.GetResult())
	assert.Equal(t, []string{"llm-review"}, res.CompletedSteps)
}

func TestRunBlueprint_ContractValidation(t *testing.T) {
	ctx := context.Background()

	initialCtx := NewDefaultContext(nil, "en-US")
	// "diff" is missing, which violates input contract

	steps := []BlueprintStep[*DefaultContext]{
		DeterministicStep[*DefaultContext]{
			StepName: "step-with-contract",
			StepContract: NewContract[*DefaultContext](
				RequireNonEmptyString[*DefaultContext]("diff", func(c *DefaultContext) string {
					val, ok := c.Get("diff")
					if !ok || val == nil {
						return ""
					}
					return val.(string)
				}),
				nil,
			),
			Fn: func(ctx context.Context, c *DefaultContext) (*DefaultContext, error) {
				return c, nil
			},
		},
	}

	res, err := RunBlueprint(ctx, RunnerOptions[*DefaultContext]{
		Steps:          steps,
		InitialContext: initialCtx,
	})

	require.Error(t, err)
	assert.Nil(t, res)

	var contractErr *BlueprintStepContractViolationError
	require.True(t, errors.As(err, &contractErr))
	assert.Equal(t, "step-with-contract", contractErr.StepName)
	assert.Equal(t, "input", contractErr.Stage)
	assert.Contains(t, contractErr.Details, "field 'diff' is required")
}

func TestRunBlueprint_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	initialCtx := NewDefaultContext(nil, "en-US")
	steps := []BlueprintStep[*DefaultContext]{
		DeterministicStep[*DefaultContext]{
			StepName: "step-never-run",
			Fn: func(ctx context.Context, c *DefaultContext) (*DefaultContext, error) {
				return c, nil
			},
		},
	}

	res, err := RunBlueprint(ctx, RunnerOptions[*DefaultContext]{
		Steps:          steps,
		InitialContext: initialCtx,
	})

	require.Error(t, err)
	assert.Nil(t, res)
	assert.Equal(t, context.Canceled, err)
}

func TestRunBlueprint_ParallelStep(t *testing.T) {
	ctx := context.Background()
	initialCtx := NewDefaultContext(nil, "en-US")

	parallelStep := ParallelStep[*DefaultContext]{
		StepName: "parallel-skills",
		Skills:   []string{"security", "performance", "styling"},
	}

	// 1. Without RunParallelStep handler -> error
	_, err := RunBlueprint(ctx, RunnerOptions[*DefaultContext]{
		Steps:          []BlueprintStep[*DefaultContext]{parallelStep},
		InitialContext: initialCtx,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be executed by RunBlueprint directly")

	// 2. With RunParallelStep handler -> success
	res, err := RunBlueprint(ctx, RunnerOptions[*DefaultContext]{
		Steps:          []BlueprintStep[*DefaultContext]{parallelStep},
		InitialContext: initialCtx,
		RunParallelStep: func(ctx context.Context, step ParallelStep[*DefaultContext], c *DefaultContext) (*DefaultContext, error) {
			assert.Equal(t, []string{"security", "performance", "styling"}, step.Skills)
			c.SetResult("parallel skills finished")
			return c, nil
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "parallel skills finished", res.Context.GetResult())
	assert.Equal(t, []string{"parallel-skills"}, res.CompletedSteps)
}

func TestStepMetric_Timing(t *testing.T) {
	ctx := context.Background()
	initialCtx := NewDefaultContext(nil, "en-US")

	var recordedMetric StepMetric
	steps := []BlueprintStep[*DefaultContext]{
		DeterministicStep[*DefaultContext]{
			StepName: "sleep-step",
			Fn: func(ctx context.Context, c *DefaultContext) (*DefaultContext, error) {
				time.Sleep(15 * time.Millisecond)
				return c, nil
			},
		},
	}

	_, err := RunBlueprint(ctx, RunnerOptions[*DefaultContext]{
		Steps:          steps,
		InitialContext: initialCtx,
		OnStepMetric: func(metric StepMetric) {
			recordedMetric = metric
		},
	})

	require.NoError(t, err)
	assert.Equal(t, "sleep-step", recordedMetric.StepName)
	assert.Equal(t, StatusSuccess, recordedMetric.Status)
	assert.GreaterOrEqual(t, recordedMetric.DurationMs, int64(10))
}
