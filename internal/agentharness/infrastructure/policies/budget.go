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
	"sync"
	"time"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

// BudgetBand categorizes the current step depth of an agent execution.
type BudgetBand string

const (
	BudgetBandFree      BudgetBand = "free"
	BudgetBandEncourage BudgetBand = "encourage"
	BudgetBandUrgent    BudgetBand = "urgent"
)

// BudgetPolicyOptions allows fine-tuning budget escalation thresholds.
type BudgetPolicyOptions struct {
	ForceTextLeadSteps int
	UrgentLeadSteps    int
	EncourageLeadSteps int
}

// DefaultBudgetPolicyOptions returns production-calibrated defaults.
func DefaultBudgetPolicyOptions() BudgetPolicyOptions {
	forceText := 2
	if v := os.Getenv("AGENT_BUDGET_FORCE_TEXT_LEAD"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			forceText = parsed
		}
	}

	urgentLead := 3
	if v := os.Getenv("AGENT_BUDGET_URGENT_LEAD"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			urgentLead = parsed
		}
	}

	encourageLead := 4
	if v := os.Getenv("AGENT_BUDGET_ENCOURAGE_LEAD"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			encourageLead = parsed
		}
	}

	return BudgetPolicyOptions{
		ForceTextLeadSteps: forceText,
		UrgentLeadSteps:    urgentLead,
		EncourageLeadSteps: encourageLead,
	}
}

// ComputeBudgetBand calculates the active band and guidance note for a given step.
func ComputeBudgetBand(stepNumber, maxSteps int, opts BudgetPolicyOptions) (BudgetBand, string) {
	forceTextAfter := maxSteps - opts.ForceTextLeadSteps

	if maxSteps < 6 || stepNumber >= forceTextAfter {
		return BudgetBandFree, ""
	}

	urgentFrom := forceTextAfter - opts.UrgentLeadSteps
	if urgentFrom < 3 {
		urgentFrom = 3
	}

	encourageFrom := urgentFrom - opts.EncourageLeadSteps
	if encourageFrom < 2 {
		encourageFrom = 2
	}

	if stepNumber >= urgentFrom {
		return BudgetBandUrgent, fmt.Sprintf(
			"STEP BUDGET: you are on step %d/%d. Final steps before the submit is forced. "+
				"Synthesize findings from the evidence already collected. "+
				"Do NOT start new exploration threads unless verifying a specific named hypothesis.",
			stepNumber, maxSteps,
		)
	}

	if stepNumber >= encourageFrom {
		return BudgetBandEncourage, fmt.Sprintf(
			"STEP BUDGET: you are on step %d/%d. Start forming concrete hypotheses from the evidence collected so far. "+
				"Avoid new reads unless they answer a specific question you can state upfront.",
			stepNumber, maxSteps,
		)
	}

	return BudgetBandFree, ""
}

// BudgetPolicy injects escalating guidance as an agent approaches its step ceiling.
// It is cache-friendly: a note is injected ONLY when the band changes, preserving
// implicit prefix caching on Anthropic and Google Gemini.
type BudgetPolicy struct {
	contracts.BasePolicy
	opts     BudgetPolicyOptions
	mu       sync.Mutex
	lastBand BudgetBand
}

// NewBudgetPolicy constructs a thread-safe BudgetPolicy instance.
func NewBudgetPolicy(opts ...BudgetPolicyOptions) *BudgetPolicy {
	var options BudgetPolicyOptions
	if len(opts) > 0 {
		options = opts[0]
	} else {
		options = DefaultBudgetPolicyOptions()
	}

	return &BudgetPolicy{
		BasePolicy: contracts.BasePolicy{PolicyName: "budget"},
		opts:       options,
		lastBand:   BudgetBandFree,
	}
}

func (p *BudgetPolicy) PrepareStep(ctx context.Context, view contracts.StepView) (contracts.StepDirectives, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	band, note := ComputeBudgetBand(view.StepNumber, view.MaxSteps, p.opts)

	// Only inject when the band actually transitions (prevents invalidating provider prefix cache).
	if band == p.lastBand || note == "" {
		p.lastBand = band
		return contracts.StepDirectives{}, nil
	}

	p.lastBand = band

	return contracts.StepDirectives{
		InjectNote: &contracts.InjectNote{
			Role:    contracts.RoleUser,
			Content: note,
		},
		Emit: []contracts.TraceEvent{
			{
				At:     time.Now(),
				Source: p.Name(),
				Kind:   "budget.band",
				Detail: map[string]any{
					"band": string(band),
					"step": view.StepNumber,
				},
			},
		},
	}, nil
}
