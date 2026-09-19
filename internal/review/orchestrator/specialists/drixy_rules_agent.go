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

// DrixyRulesSpecialistAgent evaluates custom organization and repo rules via the Deterministic Sharded Judge.
type DrixyRulesSpecialistAgent struct {
	judge *orchestrator.DeterministicShardedJudge
}

// NewDrixyRulesSpecialistAgent constructs a new custom rules review specialist.
func NewDrixyRulesSpecialistAgent(executor orchestrator.ShardedJudgeExecutor) *DrixyRulesSpecialistAgent {
	return &DrixyRulesSpecialistAgent{
		judge: orchestrator.NewDeterministicShardedJudge(executor),
	}
}

func (a *DrixyRulesSpecialistAgent) Name() string {
	return "drixy_rules"
}

func (a *DrixyRulesSpecialistAgent) Category() string {
	return "custom_rule"
}

func (a *DrixyRulesSpecialistAgent) Review(ctx context.Context, input orchestrator.ReviewAgentInput) (*orchestrator.ReviewAgentOutput, error) {
	startTime := time.Now()

	if len(input.DrixyRules) == 0 {
		return &orchestrator.ReviewAgentOutput{
			AgentName:    a.Name(),
			Category:     a.Category(),
			FinishReason: "completed",
			DurationMs:   time.Since(startTime).Milliseconds(),
		}, nil
	}

	findings, err := a.judge.EvaluateAll(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("custom rules evaluation failed: %w", err)
	}

	return &orchestrator.ReviewAgentOutput{
		AgentName:      a.Name(),
		Category:       a.Category(),
		Findings:       findings,
		TotalTurns:     len(input.ChangedFiles),
		DurationMs:     time.Since(startTime).Milliseconds(),
		FinishReason:   "completed",
		TokensConsumed: len(findings) * 150,
	}, nil
}
