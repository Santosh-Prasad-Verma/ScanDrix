// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Business Rules Validation Agent
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package businessrules

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/scandrix/backend/internal/agentharness/contracts"
	"github.com/scandrix/backend/internal/agentharness/domain"
	"github.com/scandrix/backend/internal/agentharness/infrastructure/tools"
)

const verifierSystemPrompt = `You audit a business-rules validation verdict produced by another agent.
The analyzer claims the PR does NOT correctly implement the task requirements.
Your ONLY job: decide whether that claimed violation genuinely holds.

Default to keep=true (trust the analyzer). Set keep=false ONLY when you can
concretely REFUTE the claim — i.e. the diff actually satisfies the task, or
the claimed gap is unsupported by the diff/task you were given. When unsure,
keep=true. Return a strict JSON response with: {"keep": true|false, "rationale": "...", "confidence": "high"|"medium"|"low"}`

// BusinessRulesVerifier executes an independent verification pass (doer != checker).
type BusinessRulesVerifier struct {
	runner contracts.AgentRunner
}

// NewBusinessRulesVerifier constructs the verifier pass with the harness runner.
func NewBusinessRulesVerifier(runner contracts.AgentRunner) *BusinessRulesVerifier {
	return &BusinessRulesVerifier{runner: runner}
}

// Verify evaluates whether an alleged violation should be kept or refuted.
func (v *BusinessRulesVerifier) Verify(ctx context.Context, result ValidationResult, bctx BusinessRulesContext) (Verdict, error) {
	maxTokens := 1000
	spec := contracts.AgentSpec{
		ID:              "business-rules-verifier",
		AgentName:       "BusinessRulesValidation",
		RunName:         "businessRulesVerify",
		Phase:           "businessRulesVerify",
		SpanName:        "BusinessRulesValidation::businessRulesVerify",
		SystemPrompt:    verifierSystemPrompt,
		Tools:           tools.NewInMemoryToolRegistry(),
		MaxSteps:        2,
		MaxOutputTokens: &maxTokens,
	}

	prompt := fmt.Sprintf(`Refute the analyzer claim below if the diff actually satisfies the task.

## Task Requirements:
%s

## PR Diff:
%s

## Analyzer's Claimed Violation:
Summary: %s
Violated Rules: %s
Missing Requirements: %s

Submit verdict in JSON: {"keep": true|false, "rationale": "...", "confidence": "high"|"medium"|"low"}`,
		bctx.TaskContext,
		bctx.PRDiff,
		result.Summary,
		strings.Join(result.ViolatedRules, "; "),
		strings.Join(result.MissingRequirements, "; "),
	)

	toolCtx := contracts.ToolContext{
		RunID:   "business-rules-verify",
		Context: ctx,
	}

	state, err := v.runner.Run(ctx, spec, contracts.AgentRunInput{Prompt: prompt}, toolCtx)
	if err != nil {
		// Fail-open: on verify error, keep original analyzer result
		return Verdict{Keep: true, Rationale: "verifier error; keeping analyzer result"}, nil
	}

	rawText := domain.FinalText(state)
	stripped := stripCodeFence(strings.TrimSpace(rawText))

	var parsed Verdict
	if err := json.Unmarshal([]byte(stripped), &parsed); err == nil {
		return parsed, nil
	}

	// Fallback heuristic if not strict JSON
	if strings.Contains(strings.ToLower(stripped), `"keep": false`) || strings.Contains(strings.ToLower(stripped), `"keep":false`) {
		return Verdict{Keep: false, Rationale: "refuted by verifier"}, nil
	}

	return Verdict{Keep: true, Rationale: "verification inconclusive; keeping analyzer result"}, nil
}

// ApplyBusinessRulesVerdict updates the validation result based on the verifier's judgment.
// If refuted (keep=false), clears violations and marks compliant.
func ApplyBusinessRulesVerdict(result ValidationResult, verdict Verdict) ValidationResult {
	if verdict.Keep {
		return result
	}

	res := result
	res.IsCompliant = true
	res.NeedsMoreInfo = false
	res.ViolatedRules = nil
	res.MissingRequirements = nil
	res.Confidence = verdict.Confidence
	if verdict.Rationale != "" {
		res.Summary = verdict.Rationale
	} else {
		res.Summary = "Verified clean: requirements have been satisfied in the diff."
	}
	return res
}

// ShouldVerifyValidationResult determines whether the candidate result warrants verification.
func ShouldVerifyValidationResult(result ValidationResult, verifyEnabled bool) bool {
	return verifyEnabled && !result.NeedsMoreInfo && !result.IsCompliant && len(result.MissingRequirements)+len(result.ViolatedRules) > 0
}
