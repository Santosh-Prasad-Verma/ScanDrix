// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package specialists

import (
	"context"
	"fmt"
	"time"

	"github.com/scandrix/backend/internal/review/orchestrator"
)

// GeneralistSpecialistAgent performs a comprehensive, unified review across all engineering disciplines
// (bugs, security, performance, and architecture) for fast and normal review modes.
type GeneralistSpecialistAgent struct {
	llm LLMClient
}

// NewGeneralistSpecialistAgent constructs a new generalist review persona.
func NewGeneralistSpecialistAgent(llm LLMClient) *GeneralistSpecialistAgent {
	return &GeneralistSpecialistAgent{llm: llm}
}

func (a *GeneralistSpecialistAgent) Name() string {
	return "generalist"
}

func (a *GeneralistSpecialistAgent) Category() string {
	return "generalist"
}

func (a *GeneralistSpecialistAgent) Review(ctx context.Context, input orchestrator.ReviewAgentInput) (*orchestrator.ReviewAgentOutput, error) {
	startTime := time.Now()

	if a.llm == nil {
		return nil, fmt.Errorf("llm client is not configured for generalist specialist")
	}

	sysPrompt := a.systemPrompt()
	userPrompt := BuildSpecialistUserPrompt(input, "General Engineering: Reliability, Core Security, Algorithmic Efficiency, and Structural Hygiene")

	raw, err := a.llm.GenerateResponse(ctx, sysPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("generalist specialist execution failed: %w", err)
	}

	findings, err := ParseSpecialistResponse(raw, a.Name(), a.Category())
	if err != nil {
		return nil, err
	}

	return &orchestrator.ReviewAgentOutput{
		AgentName:      a.Name(),
		Category:       a.Category(),
		Findings:       findings,
		TotalTurns:     1,
		DurationMs:     time.Since(startTime).Milliseconds(),
		FinishReason:   "completed",
		TokensConsumed: len(raw) / 3,
	}, nil
}

func (a *GeneralistSpecialistAgent) systemPrompt() string {
	return `You are ScanDrix's Generalist Review Persona, an elite principal engineer evaluating pull request diffs.
Your mandate is to provide a balanced, high-fidelity assessment covering:
1. Reliability & Correctness: Race conditions, nil pointer dereferences, error handling, off-by-one errors.
2. Core Security: OWASP Top 10 vulnerabilities, unauthorized data exposure, unvalidated inputs, insecure deserialization.
3. Performance: N+1 database queries, unbounded collections, excessive heap allocations, redundant computations.
4. Structural Hygiene: Layering violations, dead code, leaky abstractions, regression risks.

CRITICAL OPERATING RULES:
1. Ground every finding in concrete line numbers from the provided unified diff.
2. Never flag stylistic preferences, formatting choices, or subjective refactors.
3. Avoid speculative words ("might", "could potentially", "consider if"). Flag only definitive, actionable flaws.
4. If a suggested fix is clear, supply a concrete committable unified diff or replacement code.

Output your findings strictly in the following JSON format:
{
  "findings": [
    {
      "file_path": "path/to/file.go",
      "start_line": 15,
      "end_line": 20,
      "severity": "CRITICAL" | "HIGH" | "MEDIUM" | "LOW" | "INFO",
      "confidence": "HIGH" | "MEDIUM" | "LOW",
      "category": "bug" | "security" | "performance" | "architecture",
      "title": "Clear concise summary of the issue",
      "description": "Thorough technical analysis explaining the failure mode or risk",
      "remediation": "Step-by-step actionable advice on how to resolve the problem",
      "suggested_diff": "@@ -15,3 +15,4 @@\n-oldLine\n+newLine\n",
      "existing_code": "code being modified",
      "improved_code": "corrected code replacement",
      "blocking": true | false
    }
  ]
}`
}
