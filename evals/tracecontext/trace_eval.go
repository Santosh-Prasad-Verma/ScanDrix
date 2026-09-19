// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Evaluation Suite
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package tracecontext

import (
	"github.com/scandrix/backend/internal/agentharness/contracts"
)

// TraceEfficiencyResult measures token overhead and reasoning density in an agent run.
type TraceEfficiencyResult struct {
	TotalSteps          int     `json:"total_steps"`
	TotalTokens         int     `json:"total_tokens"`
	AverageTokensPerStep float64 `json:"avg_tokens_per_step"`
	ToolCallCount       int     `json:"tool_call_count"`
	CompressionEvents   int     `json:"compression_events"`
	EfficiencyScore     float64 `json:"efficiency_score"`
	Passed              bool    `json:"passed"`
}

// EvaluateTraceEfficiency evaluates whether an agent trace executed efficiently
// without looping, excessive token bloat, or thrashing.
func EvaluateTraceEfficiency(state *contracts.RunState, maxAvgTokensPerStep ...float64) TraceEfficiencyResult {
	if state == nil || len(state.Steps) == 0 {
		return TraceEfficiencyResult{Passed: false}
	}

	maxAvg := 4000.0
	if len(maxAvgTokensPerStep) > 0 && maxAvgTokensPerStep[0] > 0 {
		maxAvg = maxAvgTokensPerStep[0]
	}

	totalSteps := len(state.Steps)
	totalTokens := state.Usage.InputTokens + state.Usage.OutputTokens
	avgTokens := float64(totalTokens) / float64(totalSteps)

	toolCount := 0
	for _, s := range state.Steps {
		toolCount += len(s.Message.ToolCalls)
	}

	compressionCount := 0
	for _, tr := range state.Trace {
		if tr.Kind == "compression" {
			compressionCount++
		}
	}

	// Efficiency score decreases if avg tokens exceeds target
	efficiency := 1.0
	if avgTokens > maxAvg {
		efficiency = maxAvg / avgTokens
	}

	passed := efficiency >= 0.70 && state.Status == contracts.StatusCompleted

	return TraceEfficiencyResult{
		TotalSteps:           totalSteps,
		TotalTokens:          totalTokens,
		AverageTokensPerStep: avgTokens,
		ToolCallCount:        toolCount,
		CompressionEvents:    compressionCount,
		EfficiencyScore:      efficiency,
		Passed:               passed,
	}
}
