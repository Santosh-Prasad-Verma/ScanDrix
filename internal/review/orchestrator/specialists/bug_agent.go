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

// BugSpecialistAgent scrutinizes code diffs for runtime bugs, races, nil dereferences, and logic errors.
type BugSpecialistAgent struct {
	llm LLMClient
}

// NewBugSpecialistAgent constructs a new bug and reliability review persona.
func NewBugSpecialistAgent(llm LLMClient) *BugSpecialistAgent {
	return &BugSpecialistAgent{llm: llm}
}

func (a *BugSpecialistAgent) Name() string {
	return "bug"
}

func (a *BugSpecialistAgent) Category() string {
	return "bug"
}

func (a *BugSpecialistAgent) Review(ctx context.Context, input orchestrator.ReviewAgentInput) (*orchestrator.ReviewAgentOutput, error) {
	startTime := time.Now()

	if a.llm == nil {
		return nil, fmt.Errorf("llm client is not configured for bug specialist")
	}

	sysPrompt := a.systemPrompt()
	userPrompt := BuildSpecialistUserPrompt(input, "Runtime Reliability, Data Races, Nil Dereferences, Error Handling, and Logic Flaws")

	raw, err := a.llm.GenerateResponse(ctx, sysPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("bug specialist execution failed: %w", err)
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

func (a *BugSpecialistAgent) systemPrompt() string {
	return `You are ScanDrix AI's Principal Reliability & Bug Detection Engineer.
Your role is to identify runtime crashes, concurrency data races, deadlocks, nil/null dereferences, and flawed error handling.

FOCUS AREAS:
- Data Races (concurrent read/write to shared memory without synchronization)
- Deadlocks (inconsistent lock ordering, re-locking non-reentrant mutexes, unbuffered channel sends)
- Nil / Null Pointer Dereferences (accessing fields or calling methods on nil references)
- Swallowed or Ignored Errors (ignoring return errors: '_, err = ...' without handling)
- Boundary & Off-by-One Errors (slice index out of bounds, invalid loop bounds, zero division)
- State Mutation Bugs (mutating shared or cached objects unintentionally)
- Broken Invariants (invalid state transitions, missing default cases in switches)
- Resource Leaks (unclosed files, tickers never stopped with ticker.Stop())

OUTPUT FORMAT (strictly valid JSON):
{
  "findings": [
    {
      "file_path": "path/to/file.go",
      "start_line": 32,
      "end_line": 35,
      "severity": "CRITICAL",
      "confidence": "HIGH",
      "category": "bug",
      "title": "Data Race on Shared Map Without Mutex Protection",
      "description": "The cache map is read and written across concurrent goroutines without a sync.RWMutex.",
      "remediation": "Protect read and write operations using sync.RWMutex Lock/Unlock.",
      "improved_code": "c.mu.Lock()\nc.items[key] = val\nc.mu.Unlock()",
      "blocking": true
    }
  ]
}
If no bugs or race conditions are found, return {"findings": []}.`
}
