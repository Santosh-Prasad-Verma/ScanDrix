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

// ArchitectureSpecialistAgent scrutinizes code diffs for structural health and Clean Architecture rules.
type ArchitectureSpecialistAgent struct {
	llm LLMClient
}

// NewArchitectureSpecialistAgent constructs a new architecture review persona.
func NewArchitectureSpecialistAgent(llm LLMClient) *ArchitectureSpecialistAgent {
	return &ArchitectureSpecialistAgent{llm: llm}
}

func (a *ArchitectureSpecialistAgent) Name() string {
	return "architecture"
}

func (a *ArchitectureSpecialistAgent) Category() string {
	return "architecture"
}

func (a *ArchitectureSpecialistAgent) Review(ctx context.Context, input orchestrator.ReviewAgentInput) (*orchestrator.ReviewAgentOutput, error) {
	startTime := time.Now()

	if a.llm == nil {
		return nil, fmt.Errorf("llm client is not configured for architecture specialist")
	}

	sysPrompt := a.systemPrompt()
	userPrompt := BuildSpecialistUserPrompt(input, "Clean Architecture, Layer Boundaries, Dependency Direction, and Domain Separation")

	raw, err := a.llm.GenerateResponse(ctx, sysPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("architecture specialist execution failed: %w", err)
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
		TokensConsumed: len(raw) / 4,
	}, nil
}

func (a *ArchitectureSpecialistAgent) systemPrompt() string {
	return `You are ScanDrix AI's Principal Software Architect.
Your role is to enforce Clean Architecture, separation of concerns, proper dependency directions, and anti-pattern prevention.

FOCUS AREAS:
- Layer Violations (domain layer importing infrastructure drivers like database/sql, pgx, amqp, or http routers)
- Inverted Dependencies (high-level policy depending on low-level detail instead of abstractions)
- Leaky Abstractions (exposing database ORM models or raw SQL errors across HTTP API boundaries)
- Cyclic Package Dependencies (circular imports or package entanglement)
- God Objects & Classes (classes/packages with too many unrelated responsibilities)
- Missing Interface Segregation (large monolithic interfaces rather than focused role interfaces)
- Contract Drift (breaking changes to public API request/response contracts without versioning)
- Hardcoded Dependencies (instantiating concrete infrastructure services inside domain handlers rather than dependency injection)

OUTPUT FORMAT (strictly valid JSON):
{
  "findings": [
    {
      "file_path": "path/to/domain/order.go",
      "start_line": 12,
      "end_line": 15,
      "severity": "HIGH",
      "confidence": "HIGH",
      "category": "architecture",
      "title": "Clean Architecture Violation: Domain Importing Infrastructure Driver",
      "description": "The domain entity package directly imports github.com/jackc/pgx/v5. Domain packages must remain pure.",
      "remediation": "Define a Repository interface in the domain package and implement it in the infrastructure package.",
      "improved_code": "type OrderRepository interface {\n    Save(ctx context.Context, order *Order) error\n}",
      "blocking": false
    }
  ]
}
If no architectural flaws are found, return {"findings": []}.`
}
