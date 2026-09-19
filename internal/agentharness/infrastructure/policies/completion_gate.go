// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Harness Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package policies

import (
	"context"
	"fmt"
	"time"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

// CompletionGatePolicyOptions configures the completion gate interceptor.
type CompletionGatePolicyOptions struct {
	DoneToolName string
}

// CompletionGatePolicy enforces target completion by:
// 1. Steering the agent via progress debt injection when critical targets remain unaddressed.
// 2. Marking coverage from executed tool calls on each step finish.
// 3. Vetoing early termination via ShouldStop if critical targets are still pending.
type CompletionGatePolicy struct {
	contracts.BasePolicy
	ledger contracts.ProgressLedger
	opts   CompletionGatePolicyOptions
}

// NewCompletionGatePolicy constructs an enterprise CompletionGatePolicy.
func NewCompletionGatePolicy(ledger contracts.ProgressLedger, opts CompletionGatePolicyOptions) *CompletionGatePolicy {
	return &CompletionGatePolicy{
		BasePolicy: contracts.BasePolicy{PolicyName: "completion-gate"},
		ledger:     ledger,
		opts:       opts,
	}
}

func (p *CompletionGatePolicy) PrepareStep(ctx context.Context, view contracts.StepView) (contracts.StepDirectives, error) {
	if p.ledger == nil {
		return contracts.StepDirectives{}, nil
	}

	debt := p.ledger.DebtNote()
	if debt == nil || *debt == "" {
		return contracts.StepDirectives{}, nil
	}

	summary := p.ledger.Summary()
	noteContent := fmt.Sprintf("%s\nPrioritize the pending critical targets before anything else.", *debt)

	return contracts.StepDirectives{
		InjectNote: &contracts.InjectNote{
			Role:    contracts.RoleUser,
			Content: noteContent,
		},
		Emit: []contracts.TraceEvent{
			{
				At:     time.Now(),
				Source: p.Name(),
				Kind:   "progress.debt",
				Detail: map[string]any{
					"criticalPending": summary.CriticalPending,
					"criticalTotal":   summary.CriticalTotal,
					"pending":         summary.PendingTargets,
				},
			},
		},
	}, nil
}

func (p *CompletionGatePolicy) OnStepFinish(ctx context.Context, view contracts.StepView) error {
	if p.ledger == nil || len(view.Steps) == 0 {
		return nil
	}

	lastStep := view.Steps[len(view.Steps)-1]
	for _, tc := range lastStep.Message.ToolCalls {
		p.ledger.MarkFromToolCall(tc.Name, tc.Input, lastStep.Index)
	}

	return nil
}

func (p *CompletionGatePolicy) ShouldStop(ctx context.Context, view contracts.StepView) (bool, error) {
	if len(view.Steps) == 0 {
		return false, nil
	}

	lastStep := view.Steps[len(view.Steps)-1]
	doneCalled := false
	for _, tc := range lastStep.Message.ToolCalls {
		if tc.Name == p.opts.DoneToolName {
			doneCalled = true
			break
		}
	}

	if !doneCalled {
		return false, nil
	}

	if p.ledger != nil {
		summary := p.ledger.Summary()
		// Gate ONLY on critical-tier pending — never require 100% of non-critical items.
		if summary.CriticalTotal > 0 && summary.CriticalPending > 0 {
			// Veto finalize: agent must keep investigating critical items.
			return false, nil
		}
	}

	return true, nil
}
