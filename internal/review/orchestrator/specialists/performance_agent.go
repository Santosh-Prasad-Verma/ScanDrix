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

// PerformanceSpecialistAgent scrutinizes code diffs for latency, N+1 queries, allocations, and leaks.
type PerformanceSpecialistAgent struct {
	llm LLMClient
}

// NewPerformanceSpecialistAgent constructs a new performance review persona.
func NewPerformanceSpecialistAgent(llm LLMClient) *PerformanceSpecialistAgent {
	return &PerformanceSpecialistAgent{llm: llm}
}

func (a *PerformanceSpecialistAgent) Name() string {
	return "performance"
}

func (a *PerformanceSpecialistAgent) Category() string {
	return "performance"
}

func (a *PerformanceSpecialistAgent) Review(ctx context.Context, input orchestrator.ReviewAgentInput) (*orchestrator.ReviewAgentOutput, error) {
	startTime := time.Now()

	if a.llm == nil {
		return nil, fmt.Errorf("llm client is not configured for performance specialist")
	}

	sysPrompt := a.systemPrompt()
	userPrompt := BuildSpecialistUserPrompt(input, "Latency, N+1 Queries, Memory Allocations, Concurrency Leaks, and Throughput")

	raw, err := a.llm.GenerateResponse(ctx, sysPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("performance specialist execution failed: %w", err)
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

func (a *PerformanceSpecialistAgent) systemPrompt() string {
	return `You are ScanDrix AI's Principal Performance & Scalability Architect.
Your role is to identify database performance bottlenecks, excessive heap allocations, connection leaks, and algorithmic inefficiencies.

FOCUS AREAS:
- N+1 Database Queries (queries inside loops without batching/eager-loading)
- Unindexed Queries (full table scans, unindexed JOIN/WHERE clauses on high-cardinality tables)
- Connection Leaks (unclosed HTTP response bodies, unclosed SQL rows, unreturned pool connections)
- Hot-Path Heap Allocations (redundant string concatenations in loops, unbuffered I/O, boxing)
- Goroutine & Thread Leaks (spawning goroutines without cancellation context or termination guarantee)
- Lock Contention (holding locks during network/disk I/O, coarse-grained locks)
- Algorithmic Complexity (O(n^2) or higher operations on unbounded datasets)
- Missing Timeouts (unbounded network requests, infinite retry loops)

OUTPUT FORMAT (strictly valid JSON):
{
  "findings": [
    {
      "file_path": "path/to/file.go",
      "start_line": 50,
      "end_line": 55,
      "severity": "HIGH",
      "confidence": "HIGH",
      "category": "performance",
      "title": "N+1 Database Queries Inside Order Processing Loop",
      "description": "Each iteration of the orders loop issues a separate SQL query to fetch customer details.",
      "remediation": "Batch customer IDs and fetch all records in a single 'WHERE id IN (...)' query before the loop.",
      "improved_code": "customers := repo.GetCustomersByIDs(ctx, customerIDs)",
      "blocking": true
    }
  ]
}
If no performance bottlenecks are found, return {"findings": []}.`
}
