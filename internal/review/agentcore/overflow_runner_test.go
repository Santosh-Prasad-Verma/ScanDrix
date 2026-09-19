package agentcore

import (
	"context"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/agentharness/contracts"
	"github.com/scandrix/backend/internal/agentharness/infrastructure/tools"
)

type mockRunner struct {
	runFn func(ctx context.Context, spec contracts.AgentSpec, input contracts.AgentRunInput, toolCtx contracts.ToolContext) (*contracts.RunState, error)
}

func (m *mockRunner) Run(ctx context.Context, spec contracts.AgentSpec, input contracts.AgentRunInput, toolCtx contracts.ToolContext) (*contracts.RunState, error) {
	return m.runFn(ctx, spec, input, toolCtx)
}

func TestOverflowRecoveringRunner_CleanRun(t *testing.T) {
	mock := &mockRunner{
		runFn: func(ctx context.Context, spec contracts.AgentSpec, input contracts.AgentRunInput, toolCtx contracts.ToolContext) (*contracts.RunState, error) {
			return &contracts.RunState{
				Status: contracts.StatusCompleted,
				Steps:  []contracts.RunStep{},
			}, nil
		},
	}

	tightenCalled := false
	tightener := func(spec contracts.AgentSpec, scale float64) contracts.AgentSpec {
		tightenCalled = true
		return spec
	}

	runner := NewOverflowRecoveringRunner(mock, tightener)
	state, err := runner.Run(context.Background(), contracts.AgentSpec{ID: "test"}, contracts.AgentRunInput{}, contracts.ToolContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.Status != contracts.StatusCompleted {
		t.Errorf("expected status completed, got %v", state.Status)
	}
	if tightenCalled {
		t.Errorf("tightener should not be called on clean run")
	}

	metrics := runner.Metrics()
	if metrics.RunsExecuted != 1 || metrics.OverflowsDetected != 0 {
		t.Errorf("unexpected metrics: %+v", metrics)
	}
}

func TestOverflowRecoveringRunner_NonOverflowError(t *testing.T) {
	mock := &mockRunner{
		runFn: func(ctx context.Context, spec contracts.AgentSpec, input contracts.AgentRunInput, toolCtx contracts.ToolContext) (*contracts.RunState, error) {
			return &contracts.RunState{
				Status: contracts.StatusError,
				Trace: []contracts.TraceEvent{
					{
						At:   time.Now(),
						Kind: "error",
						Detail: map[string]any{
							"message": "rate limit exceeded: 429 Too Many Requests",
						},
					},
				},
			}, nil
		},
	}

	tightenCalled := false
	tightener := func(spec contracts.AgentSpec, scale float64) contracts.AgentSpec {
		tightenCalled = true
		return spec
	}

	runner := NewOverflowRecoveringRunner(mock, tightener)
	state, err := runner.Run(context.Background(), contracts.AgentSpec{ID: "test"}, contracts.AgentRunInput{}, contracts.ToolContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.Status != contracts.StatusError {
		t.Errorf("expected status error, got %v", state.Status)
	}
	if tightenCalled {
		t.Errorf("tightener should not be called on rate limit error")
	}

	metrics := runner.Metrics()
	if metrics.RunsExecuted != 1 || metrics.OverflowsDetected != 0 {
		t.Errorf("unexpected metrics: %+v", metrics)
	}
}

func TestOverflowRecoveringRunner_ContextOverflowRecovery(t *testing.T) {
	callCount := 0
	mock := &mockRunner{
		runFn: func(ctx context.Context, spec contracts.AgentSpec, input contracts.AgentRunInput, toolCtx contracts.ToolContext) (*contracts.RunState, error) {
			callCount++
			if callCount == 1 {
				// First run overflows context window
				return &contracts.RunState{
					Status: contracts.StatusError,
					Trace: []contracts.TraceEvent{
						{
							At:   time.Now(),
							Kind: "error",
							Detail: map[string]any{
								"message":      "maximum context length is 262144 tokens, however you requested 421869 tokens",
								"responseBody": "context_length_exceeded",
							},
						},
					},
				}, nil
			}

			// Retry run succeeds
			return &contracts.RunState{
				Status: contracts.StatusCompleted,
				Steps:  []contracts.RunStep{},
			}, nil
		},
	}

	var observedScale float64
	tightener := func(spec contracts.AgentSpec, scale float64) contracts.AgentSpec {
		observedScale = scale
		spec.MaxSteps = 10
		return spec
	}

	runner := NewOverflowRecoveringRunner(mock, tightener)
	state, err := runner.Run(context.Background(), contracts.AgentSpec{ID: "test"}, contracts.AgentRunInput{}, contracts.ToolContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.Status != contracts.StatusCompleted {
		t.Errorf("expected recovered status completed, got %v", state.Status)
	}
	if callCount != 2 {
		t.Errorf("expected 2 runs (1 fail + 1 retry), got %d", callCount)
	}
	if observedScale != DefaultOverflowRetryScale {
		t.Errorf("expected retry scale %v, got %v", DefaultOverflowRetryScale, observedScale)
	}

	// Verify trace event added
	hasRecoveryEvent := false
	for _, tr := range state.Trace {
		if tr.Kind == "context.overflow.recovered" {
			hasRecoveryEvent = true
			break
		}
	}
	if !hasRecoveryEvent {
		t.Errorf("expected context.overflow.recovered trace event")
	}

	metrics := runner.Metrics()
	if metrics.RunsExecuted != 1 || metrics.OverflowsDetected != 1 || metrics.RecoveriesSucceeded != 1 {
		t.Errorf("unexpected metrics: %+v", metrics)
	}
}

func TestOverflowRecoveringRunner_DoubleOverflow(t *testing.T) {
	callCount := 0
	mock := &mockRunner{
		runFn: func(ctx context.Context, spec contracts.AgentSpec, input contracts.AgentRunInput, toolCtx contracts.ToolContext) (*contracts.RunState, error) {
			callCount++
			return &contracts.RunState{
				Status: contracts.StatusError,
				Trace: []contracts.TraceEvent{
					{
						At:   time.Now(),
						Kind: "error",
						Detail: map[string]any{
							"message": "context_length_exceeded: prompt exceeds model limit",
						},
					},
				},
			}, nil
		},
	}

	tightener := func(spec contracts.AgentSpec, scale float64) contracts.AgentSpec {
		return spec
	}

	runner := NewOverflowRecoveringRunner(mock, tightener)
	state, err := runner.Run(context.Background(), contracts.AgentSpec{ID: "test"}, contracts.AgentRunInput{}, contracts.ToolContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.Status != contracts.StatusError {
		t.Errorf("expected status error, got %v", state.Status)
	}
	if callCount != 2 {
		t.Errorf("expected exactly 2 runs, got %d", callCount)
	}

	metrics := runner.Metrics()
	if metrics.RecoveriesFailed != 1 {
		t.Errorf("expected 1 recovery failed, got %d", metrics.RecoveriesFailed)
	}
}

func TestTokenEstimator(t *testing.T) {
	// Empty string
	if tokens := EstimateTokens(""); tokens != 0 {
		t.Errorf("expected 0 tokens for empty string, got %d", tokens)
	}

	// Short string
	text := "func CalculateTotal(price float64) float64 { return price * 1.2 }"
	tokens := EstimateTokens(text)
	if tokens <= 0 {
		t.Errorf("expected positive token estimate, got %d", tokens)
	}

	chars := TokensToChars(tokens)
	if chars < len(text)-5 {
		t.Errorf("tokensToChars too small: %d vs original len %d", chars, len(text))
	}

	// Value tokens
	val := map[string]any{
		"path":      "src/auth.go",
		"startLine": 10,
		"endLine":   25,
	}
	valTokens := EstimateValueTokens(val)
	if valTokens <= 0 {
		t.Errorf("expected positive token estimate for map, got %d", valTokens)
	}

	// Overhead tokens
	spec := contracts.AgentSpec{
		SystemPrompt: "You are a code reviewer.",
		Tools:        tools.NewInMemoryToolRegistry(),
	}
	overhead := EstimateAgentSpecOverhead(spec)
	if overhead <= 0 {
		t.Errorf("expected positive overhead tokens, got %d", overhead)
	}
}
