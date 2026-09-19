// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Harness Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package policies_test

import (
	"context"
	"testing"

	"github.com/scandrix/backend/internal/agentharness/contracts"
	"github.com/scandrix/backend/internal/agentharness/infrastructure/policies"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBudgetPolicy(t *testing.T) {
	ctx := context.Background()
	policy := policies.NewBudgetPolicy(policies.BudgetPolicyOptions{
		ForceTextLeadSteps: 2,
		UrgentLeadSteps:    3,
		EncourageLeadSteps: 4,
	})

	// Step 1 of 12 -> free (no note)
	d1, err := policy.PrepareStep(ctx, contracts.StepView{StepNumber: 1, MaxSteps: 12})
	require.NoError(t, err)
	assert.Nil(t, d1.InjectNote)

	// Step 4 of 12 -> encourage band transition (injects note)
	d4, err := policy.PrepareStep(ctx, contracts.StepView{StepNumber: 4, MaxSteps: 12})
	require.NoError(t, err)
	require.NotNil(t, d4.InjectNote)
	assert.Equal(t, contracts.RoleUser, d4.InjectNote.Role)
	assert.Contains(t, d4.InjectNote.Content, "STEP BUDGET: you are on step 4/12")
	require.Len(t, d4.Emit, 1)
	assert.Equal(t, "budget.band", d4.Emit[0].Kind)

	// Step 5 of 12 -> still encourage band; no new note injected (cache-friendly!)
	d5, err := policy.PrepareStep(ctx, contracts.StepView{StepNumber: 5, MaxSteps: 12})
	require.NoError(t, err)
	assert.Nil(t, d5.InjectNote)

	// Step 7 of 12 -> urgent band transition (injects note)
	d7, err := policy.PrepareStep(ctx, contracts.StepView{StepNumber: 7, MaxSteps: 12})
	require.NoError(t, err)
	require.NotNil(t, d7.InjectNote)
	assert.Contains(t, d7.InjectNote.Content, "Final steps before the submit is forced")
}

type fakeLedger struct {
	summary contracts.ProgressSummary
	debt    *string
	marked  []string
}

func (f *fakeLedger) MarkFromToolCall(toolName string, input any, step int) {
	f.marked = append(f.marked, toolName)
}

func (f *fakeLedger) Summary() contracts.ProgressSummary {
	return f.summary
}

func (f *fakeLedger) DebtNote() *string {
	return f.debt
}

func TestCompletionGatePolicy(t *testing.T) {
	ctx := context.Background()
	debtMsg := "2 critical diff hunks remaining"
	ledger := &fakeLedger{
		debt: &debtMsg,
		summary: contracts.ProgressSummary{
			TotalTargets:    5,
			PendingTargets:  2,
			CriticalTotal:   2,
			CriticalPending: 2,
		},
	}

	gate := policies.NewCompletionGatePolicy(ledger, policies.CompletionGatePolicyOptions{
		DoneToolName: "submitResult",
	})

	// PrepareStep injects debt note
	directives, err := gate.PrepareStep(ctx, contracts.StepView{})
	require.NoError(t, err)
	require.NotNil(t, directives.InjectNote)
	assert.Contains(t, directives.InjectNote.Content, "2 critical diff hunks remaining")

	// Step finish records tool calls
	err = gate.OnStepFinish(ctx, contracts.StepView{
		Steps: []contracts.RunStep{
			{
				Index: 1,
				Message: contracts.AgentMessage{
					ToolCalls: []contracts.ToolCallRecord{
						{Name: "readFile"},
					},
				},
			},
		},
	})
	require.NoError(t, err)
	assert.Contains(t, ledger.marked, "readFile")

	// ShouldStop vetoes if doneTool called but critical targets remain
	vetoView := contracts.StepView{
		Steps: []contracts.RunStep{
			{
				Message: contracts.AgentMessage{
					ToolCalls: []contracts.ToolCallRecord{
						{Name: "submitResult"},
					},
				},
			},
		},
	}
	stop, err := gate.ShouldStop(ctx, vetoView)
	require.NoError(t, err)
	assert.False(t, stop, "must veto stop when critical targets are pending")

	// When critical targets are 0, ShouldStop honors doneTool
	ledger.summary.CriticalPending = 0
	stop, err = gate.ShouldStop(ctx, vetoView)
	require.NoError(t, err)
	assert.True(t, stop, "must honor stop when critical targets are resolved")
}

func TestForceFinalizePolicy(t *testing.T) {
	ctx := context.Background()
	policy := policies.NewForceFinalizePolicy(policies.ForceFinalizePolicyOptions{
		DoneToolName:    "submitResult",
		WithinLastSteps: 2,
	})

	// Before last 2 steps (step 7 of 10): no restriction
	d7, err := policy.PrepareStep(ctx, contracts.StepView{StepNumber: 7, MaxSteps: 10})
	require.NoError(t, err)
	assert.Empty(t, d7.ActiveTools)

	// Step 8 of 10 (10 - 2): restricts active tools to only submitResult
	d8, err := policy.PrepareStep(ctx, contracts.StepView{StepNumber: 8, MaxSteps: 10})
	require.NoError(t, err)
	assert.Equal(t, []string{"submitResult"}, d8.ActiveTools)
	require.NotNil(t, d8.InjectNote)
	assert.Contains(t, d8.InjectNote.Content, "Call submitResult now")
}

type fakeCompressor struct {
	result *contracts.CompressionResult
}

func (f *fakeCompressor) MaybeCompress(messages []contracts.AgentMessage) *contracts.CompressionResult {
	return f.result
}

func TestCompressionPolicy(t *testing.T) {
	ctx := context.Background()
	comp := &fakeCompressor{
		result: &contracts.CompressionResult{
			Messages:     []contracts.AgentMessage{{Role: contracts.RoleUser, Content: "compressed"}},
			BeforeTokens: 1000,
			AfterTokens:  400,
		},
	}

	policy := policies.NewCompressionPolicy(comp)
	directives, err := policy.PrepareStep(ctx, contracts.StepView{
		Messages: []contracts.AgentMessage{{Role: contracts.RoleUser, Content: "long messages..."}},
	})
	require.NoError(t, err)
	assert.Len(t, directives.Messages, 1)
	require.Len(t, directives.Emit, 1)
	assert.Equal(t, "context.compress", directives.Emit[0].Kind)
}
