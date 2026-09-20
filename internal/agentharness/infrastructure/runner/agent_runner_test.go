// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Harness Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package runner_test

import (
	"context"
	"errors"
	"testing"

	"github.com/scandrix/backend/internal/agentharness/contracts"
	"github.com/scandrix/backend/internal/agentharness/infrastructure/runner"
	"github.com/scandrix/backend/internal/agentharness/infrastructure/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockTool struct {
	name   string
	output string
}

func (m *mockTool) Name() string {
	return m.name
}

func (m *mockTool) Description() string {
	return "mock tool"
}

func (m *mockTool) InputSchema() contracts.JSONSchema {
	return contracts.JSONSchema{Type: "object"}
}

func (m *mockTool) Strict() bool {
	return false
}

func (m *mockTool) Execute(ctx contracts.ToolContext, input any) (contracts.ToolResult, error) {
	return contracts.ToolResult{
		Output: m.output,
	}, nil
}

func TestGoAgentRunner_BasicExecution(t *testing.T) {
	turn := 0
	invoker := func(ctx context.Context, req runner.ModelTurnRequest) (*runner.ModelTurnResult, error) {
		turn++
		if turn == 1 {
			// Model calls a tool
			return &runner.ModelTurnResult{
				Text: "I will read the file.",
				ToolCalls: []contracts.ToolCallRecord{
					{
						ID:    "call-1",
						Name:  "readFile",
						Input: map[string]any{"path": "main.go"},
					},
				},
				Usage: &contracts.TokenUsage{InputTokens: 100, OutputTokens: 20},
			}, nil
		}

		// Model answers and submits result
		return &runner.ModelTurnResult{
			Text: "Analysis complete.",
			ToolCalls: []contracts.ToolCallRecord{
				{
					ID:    "call-2",
					Name:  "submitResult",
					Input: map[string]any{"finding": "null pointer dereference"},
				},
			},
			Usage: &contracts.TokenUsage{InputTokens: 150, OutputTokens: 40},
		}, nil
	}

	readFileTool := &mockTool{name: "readFile", output: "package main\nfunc main() {}"}
	submitTool := &mockTool{name: "submitResult", output: "submitted"}
	registry := tools.NewInMemoryToolRegistry(readFileTool, submitTool)

	agentRunner := runner.NewGoAgentRunner(invoker)

	spec := contracts.AgentSpec{
		ID:             "code-reviewer",
		SystemPrompt:   "Review code for vulnerabilities.",
		Tools:          registry,
		MaxSteps:       5,
		ResultToolName: "submitResult",
	}

	toolCtx := contracts.ToolContext{
		RunID:   "test-run-1",
		Context: context.Background(),
	}

	state, err := agentRunner.Run(context.Background(), spec, contracts.AgentRunInput{
		Prompt: "Review main.go",
	}, toolCtx)

	require.NoError(t, err)
	assert.Equal(t, "test-run-1", state.RunID)
	assert.Equal(t, "code-reviewer", state.AgentID)
	assert.Equal(t, contracts.StatusCompleted, state.Status)
	assert.Len(t, state.Steps, 2)

	// Verify token usage accumulation
	assert.Equal(t, 250, state.Usage.InputTokens)
	assert.Equal(t, 60, state.Usage.OutputTokens)

	// Verify artifact materialization for ResultToolName
	require.Len(t, state.Artifacts, 1)
	assert.Equal(t, "submitResult", state.Artifacts[0].Type)
	payloadMap := state.Artifacts[0].Payload.(map[string]any)
	assert.Equal(t, "null pointer dereference", payloadMap["finding"])
}

type stopPolicy struct {
	contracts.BasePolicy
	stopAtStep int
}

func (s *stopPolicy) ShouldStop(ctx context.Context, view contracts.StepView) (bool, error) {
	return view.StepNumber >= s.stopAtStep, nil
}

func TestGoAgentRunner_PolicyInterception(t *testing.T) {
	invoker := func(ctx context.Context, req runner.ModelTurnRequest) (*runner.ModelTurnResult, error) {
		return &runner.ModelTurnResult{
			Text: "Exploring...",
			ToolCalls: []contracts.ToolCallRecord{
				{Name: "noop", Input: nil},
			},
		}, nil
	}

	registry := tools.NewInMemoryToolRegistry(&mockTool{name: "noop", output: "ok"})
	agentRunner := runner.NewGoAgentRunner(invoker)

	spec := contracts.AgentSpec{
		ID:    "explorer",
		Tools: registry,
		Policies: []contracts.AgentPolicy{
			&stopPolicy{
				BasePolicy: contracts.BasePolicy{PolicyName: "early-stop"},
				stopAtStep: 1,
			},
		},
		MaxSteps: 10,
	}

	state, err := agentRunner.Run(context.Background(), spec, contracts.AgentRunInput{
		Prompt: "start",
	}, contracts.ToolContext{RunID: "stop-run", Context: context.Background()})

	require.NoError(t, err)
	assert.Equal(t, contracts.StatusStopped, state.Status)
	assert.Equal(t, "early-stop", state.StopReason)
	assert.Len(t, state.Steps, 1)
}

func TestGoAgentRunner_ProviderErrorCapture(t *testing.T) {
	invoker := func(ctx context.Context, req runner.ModelTurnRequest) (*runner.ModelTurnResult, error) {
		return nil, errors.New("upstream provider rate limit 429")
	}

	agentRunner := runner.NewGoAgentRunner(invoker)
	spec := contracts.AgentSpec{
		ID:       "faulty",
		Tools:    tools.NewInMemoryToolRegistry(),
		MaxSteps: 3,
	}

	state, err := agentRunner.Run(context.Background(), spec, contracts.AgentRunInput{
		Prompt: "start",
	}, contracts.ToolContext{RunID: "error-run", Context: context.Background()})

	require.Error(t, err)
	assert.Equal(t, contracts.StatusError, state.Status)
	assert.Equal(t, "error", state.StopReason)
	require.NotEmpty(t, state.Trace)

	// Check trace has error event
	var foundError bool
	for _, tr := range state.Trace {
		if tr.Kind == "error" {
			foundError = true
			assert.Contains(t, tr.Detail["message"], "429")
		}
	}
	assert.True(t, foundError)
}

type mockToolWithInput struct {
	name   string
	execFn func(ctx contracts.ToolContext, input any) (contracts.ToolResult, error)
}

func (m *mockToolWithInput) Name() string                        { return m.name }
func (m *mockToolWithInput) Description() string                 { return "mock tool" }
func (m *mockToolWithInput) InputSchema() contracts.JSONSchema   { return contracts.JSONSchema{Type: "object"} }
func (m *mockToolWithInput) Strict() bool                        { return false }
func (m *mockToolWithInput) Execute(ctx contracts.ToolContext, input any) (contracts.ToolResult, error) {
	return m.execFn(ctx, input)
}

func TestGoAgentRunner_ToolCallSelfHealing(t *testing.T) {
	var receivedInput any
	recordingTool := &mockToolWithInput{
		name: "grep",
		execFn: func(ctx contracts.ToolContext, input any) (contracts.ToolResult, error) {
			receivedInput = input
			return contracts.ToolResult{Output: "matched"}, nil
		},
	}
	registry := tools.NewInMemoryToolRegistry(recordingTool)

	// Model provides prose-wrapped JSON: "I'll run grep with arguments: {\"pattern\": \"auth\", \"path\": \"internal/\"}"
	invoker := func(ctx context.Context, req runner.ModelTurnRequest) (*runner.ModelTurnResult, error) {
		return &runner.ModelTurnResult{
			Text: "Searching...",
			ToolCalls: []contracts.ToolCallRecord{
				{
					ID:    "heal-call",
					Name:  "grep",
					Input: "I'll run grep with arguments: {\"pattern\": \"auth\", \"path\": \"internal/\"}",
				},
			},
			Usage: &contracts.TokenUsage{InputTokens: 50, OutputTokens: 10},
		}, nil
	}

	agentRunner := runner.NewGoAgentRunner(invoker)
	spec := contracts.AgentSpec{
		ID:       "healer",
		Tools:    registry,
		MaxSteps: 1,
	}

	state, err := agentRunner.Run(context.Background(), spec, contracts.AgentRunInput{
		Prompt: "grep auth",
	}, contracts.ToolContext{RunID: "heal-run", Context: context.Background()})

	require.NoError(t, err)
	assert.Equal(t, contracts.StatusBudgetExhausted, state.Status)
	inputMap, ok := receivedInput.(map[string]any)
	require.True(t, ok, "expected self-healed JSON map, got %T: %v", receivedInput, receivedInput)
	assert.Equal(t, "auth", inputMap["pattern"])
	assert.Equal(t, "internal/", inputMap["path"])

	// Verify billing span and telemetry span in trace
	var hasBillingSpan, hasTurnSpan bool
	for _, tr := range state.Trace {
		if tr.Kind == "telemetry.billing_span" {
			hasBillingSpan = true
			assert.Equal(t, "healer", tr.Detail["agent_id"])
			assert.Equal(t, 50, tr.Detail["input_tokens"])
			assert.Equal(t, 10, tr.Detail["output_tokens"])
		}
		if tr.Kind == "telemetry.span" {
			hasTurnSpan = true
		}
	}
	assert.True(t, hasBillingSpan, "expected telemetry.billing_span event in trace")
	assert.True(t, hasTurnSpan, "expected telemetry.span event in trace")
}
