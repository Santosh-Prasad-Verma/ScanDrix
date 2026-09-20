// Package agentcore provides the deep core agent loop and execution components for code reviews.
package agentcore

import (
	"context"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/scandrix/backend/internal/agentharness/contracts"
	"github.com/scandrix/backend/internal/llm"
)

const (
	// DefaultOverflowRetryScale is the fraction of the original context window/budget to retry at (60%).
	DefaultOverflowRetryScale = 0.60
)

// SpecTightener rebuilds an AgentSpec with its compression or token budget scaled down by scale (< 1.0 tightens).
type SpecTightener func(spec contracts.AgentSpec, scale float64) contracts.AgentSpec

// OverflowRunnerMetrics records operational counters for context overflow recovery.
type OverflowRunnerMetrics struct {
	RunsExecuted        int64 `json:"runsExecuted"`
	OverflowsDetected   int64 `json:"overflowsDetected"`
	RecoveriesSucceeded int64 `json:"recoveriesSucceeded"`
	RecoveriesFailed    int64 `json:"recoveriesFailed"`
}

// OverflowRecoveringRunner decorates contracts.AgentRunner to recover from mid-loop context overflows.
// When proactive window compression mis-estimates dense code tokens (issue #1574) and a provider
// fails with context window overflow, this decorator re-runs the pass ONCE with tighter compression.
type OverflowRecoveringRunner struct {
	inner      contracts.AgentRunner
	tighten    SpecTightener
	retryScale float64

	runsExecuted        atomic.Int64
	overflowsDetected   atomic.Int64
	recoveriesSucceeded atomic.Int64
	recoveriesFailed    atomic.Int64
}

// NewOverflowRecoveringRunner creates an enterprise overflow recovering runner.
func NewOverflowRecoveringRunner(inner contracts.AgentRunner, tighten SpecTightener) *OverflowRecoveringRunner {
	scale := DefaultOverflowRetryScale
	if val := os.Getenv("SCANDRIX_OVERFLOW_RETRY_SCALE"); val != "" {
		if parsed, err := strconv.ParseFloat(val, 64); err == nil && parsed > 0 && parsed < 1.0 {
			scale = parsed
		}
	}

	return &OverflowRecoveringRunner{
		inner:      inner,
		tighten:    tighten,
		retryScale: scale,
	}
}

// Run executes the agent spec and re-runs once with tighter compression if context overflow occurs.
func (o *OverflowRecoveringRunner) Run(
	ctx context.Context,
	spec contracts.AgentSpec,
	input contracts.AgentRunInput,
	toolCtx contracts.ToolContext,
) (*contracts.RunState, error) {
	o.runsExecuted.Add(1)

	state, err := o.inner.Run(ctx, spec, input, toolCtx)
	if state == nil {
		return nil, err
	}

	// Check if the run failed because the prompt overflowed the context window
	if !llm.IsContextOverflowResult(*state) {
		return state, err
	}

	o.overflowsDetected.Add(1)

	// If no tightener is provided, return the original failed state
	if o.tighten == nil {
		o.recoveriesFailed.Add(1)
		return state, err
	}

	// Single retry at tighter window — never recurses
	tighterSpec := o.tighten(spec, o.retryScale)
	recoveredState, retryErr := o.inner.Run(ctx, tighterSpec, input, toolCtx)

	if recoveredState != nil {
		// Append recovery audit event to state trace
		recoveryEvent := contracts.TraceEvent{
			At:     time.Now(),
			Source: "overflow_recovering_runner",
			Kind:   "context.overflow.recovered",
			Detail: map[string]any{
				"retryScale": o.retryScale,
				"specId":     spec.ID,
				"success":    recoveredState.Status != contracts.StatusError,
			},
		}
		recoveredState.Trace = append(recoveredState.Trace, recoveryEvent)

		if recoveredState.Status != contracts.StatusError {
			o.recoveriesSucceeded.Add(1)
		} else {
			o.recoveriesFailed.Add(1)
		}
	} else {
		o.recoveriesFailed.Add(1)
	}

	return recoveredState, retryErr
}

// Metrics returns snapshot of operational counters.
func (o *OverflowRecoveringRunner) Metrics() OverflowRunnerMetrics {
	return OverflowRunnerMetrics{
		RunsExecuted:        o.runsExecuted.Load(),
		OverflowsDetected:   o.overflowsDetected.Load(),
		RecoveriesSucceeded: o.recoveriesSucceeded.Load(),
		RecoveriesFailed:    o.recoveriesFailed.Load(),
	}
}
