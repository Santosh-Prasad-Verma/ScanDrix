// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Harness Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package policies

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

// ForceFinalizePolicyOptions configures terminal step enforcement.
type ForceFinalizePolicyOptions struct {
	DoneToolName    string
	WithinLastSteps int
}

// DefaultForceFinalizePolicyOptions returns options with runtime environment fallbacks.
func DefaultForceFinalizePolicyOptions(doneToolName string) ForceFinalizePolicyOptions {
	within := 2
	if v := os.Getenv("AGENT_FORCE_FINALIZE_LAST_STEPS"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			within = parsed
		}
	}

	return ForceFinalizePolicyOptions{
		DoneToolName:    doneToolName,
		WithinLastSteps: within,
	}
}

// ForceFinalizePolicy restricts available tools to ONLY the result submission tool
// in the final steps before maxSteps. This prevents the agent from running out of steps
// while still exploring, ensuring collected findings are never discarded.
type ForceFinalizePolicy struct {
	contracts.BasePolicy
	opts ForceFinalizePolicyOptions
}

// NewForceFinalizePolicy constructs a ForceFinalizePolicy instance.
func NewForceFinalizePolicy(opts ForceFinalizePolicyOptions) *ForceFinalizePolicy {
	if opts.WithinLastSteps <= 0 {
		opts.WithinLastSteps = 2
	}
	return &ForceFinalizePolicy{
		BasePolicy: contracts.BasePolicy{PolicyName: "force-finalize"},
		opts:       opts,
	}
}

func (p *ForceFinalizePolicy) PrepareStep(ctx context.Context, view contracts.StepView) (contracts.StepDirectives, error) {
	within := p.opts.WithinLastSteps
	if view.StepNumber < view.MaxSteps-within {
		return contracts.StepDirectives{}, nil
	}

	return contracts.StepDirectives{
		ActiveTools: []string{p.opts.DoneToolName},
		InjectNote: &contracts.InjectNote{
			Role: contracts.RoleUser,
			Content: fmt.Sprintf(
				"You are at the final step. Call %s now with the findings you have — do not investigate further.",
				p.opts.DoneToolName,
			),
		},
		Emit: []contracts.TraceEvent{
			{
				At:     time.Now(),
				Source: p.Name(),
				Kind:   "force-finalize",
				Detail: map[string]any{
					"stepNumber": view.StepNumber,
					"maxSteps":   view.MaxSteps,
				},
			},
		},
	}, nil
}
