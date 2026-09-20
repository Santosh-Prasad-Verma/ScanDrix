// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package agentloop_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/llm/agentloop"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockFailoverControl struct {
	markedUnsafe bool
}

func (m *mockFailoverControl) MarkUnsafeToRetry() {
	m.markedUnsafe = true
}

func TestApplyCacheBreakpoints(t *testing.T) {
	messages := []map[string]any{
		{"role": "user", "content": "hello 1"},
		{"role": "assistant", "content": "hi 1"},
		{"role": "user", "content": "hello 2"},
	}
	tools := []map[string]any{
		{"name": "tool_1"},
		{"name": "tool_2"},
	}

	sys, msgs, tls := agentloop.ApplyCacheBreakpoints("System prompt", messages, tools, 5, "anthropic", "claude-sonnet-4.5")

	// System prompt marked
	sysSlice, ok := sys.([]map[string]any)
	require.True(t, ok)
	assert.Contains(t, sysSlice[0], "cache_control")

	// Last user message marked
	assert.Contains(t, msgs[2], "cache_control")
	assert.NotContains(t, msgs[0], "cache_control")

	// Last tool marked
	assert.Contains(t, tls[1], "cache_control")
	assert.NotContains(t, tls[0], "cache_control")
}

func TestRepairInvalidToolInput(t *testing.T) {
	ctx := context.Background()

	invoker := func(ctx context.Context, prompt string) (string, error) {
		assert.Contains(t, prompt, "search_files")
		// Returns corrected JSON wrapped in markdown
		return "```json\n{\"query\": \"fixed_search\", \"max_results\": 10}\n```", nil
	}

	repaired := agentloop.RepairInvalidToolInput(
		ctx,
		invoker,
		"search_files",
		`{"query": 123}`,
		errors.New("expected string, got number"),
	)

	assert.Equal(t, `{"query": "fixed_search", "max_results": 10}`, repaired)
}

func TestExecuteLoopWithToolExecutionAndFailoverVeto(t *testing.T) {
	ctx := context.Background()
	ctrl := &mockFailoverControl{}

	stepCount := 0
	runner := func(ctx context.Context, stepNum int, history []map[string]any) (*agentloop.StepEvent, error) {
		stepCount++
		if stepNum == 1 {
			return &agentloop.StepEvent{
				ToolCalls: []agentloop.ToolCall{
					{ID: "call-1", ToolName: "list_rules", Arguments: "{}"},
				},
				TokensUsed: 150,
			}, nil
		}
		return &agentloop.StepEvent{
			TextContent: "Here is your review complete.",
			TokensUsed:  200,
		}, nil
	}

	seams := agentloop.AgentLoopSeams{
		MaxSteps: 5,
		ToolExecutor: func(ctx context.Context, call agentloop.ToolCall) (agentloop.ToolResult, error) {
			assert.Equal(t, "list_rules", call.ToolName)
			return agentloop.ToolResult{
				ToolCallID: call.ID,
				Content:    `{"rules": ["rule-1", "rule-2"]}`,
			}, nil
		},
	}

	res, err := agentloop.ExecuteLoop(ctx, runner, seams, ctrl, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, len(res.Steps))
	assert.Equal(t, "Here is your review complete.", res.FinalAnswer)
	assert.Equal(t, 350, res.TotalTokens)
	assert.True(t, ctrl.markedUnsafe, "Control must be marked unsafe to retry once a tool executes")
}

func TestRunAgentLoopCall_TransientRetry(t *testing.T) {
	ctx := context.Background()
	attempts := 0

	runner := func(ctx context.Context, stepNum int, history []map[string]any) (*agentloop.StepEvent, error) {
		attempts++
		if attempts == 1 {
			return nil, errors.New("transient HTTP 502 bad gateway")
		}
		return &agentloop.StepEvent{
			StepNumber:  stepNum,
			TextContent: "Recovered successfully",
			TokensUsed:  100,
		}, nil
	}

	params := agentloop.AgentLoopParams{
		Runner: runner,
		Config: agentloop.AgentLoopConfig{
			MaxSteps:       3,
			MaxRetries:     2,
			RetryBaseDelay: 5 * time.Millisecond,
		},
	}

	res, err := agentloop.RunAgentLoopCall(ctx, params)
	require.NoError(t, err)
	assert.Equal(t, 2, attempts, "Should succeed on second attempt after transient error")
	assert.Equal(t, "Recovered successfully", res.FinalAnswer)
	assert.Equal(t, "natural_completion", res.StopReason)
}

func TestRunAgentLoopCall_StopCondition(t *testing.T) {
	ctx := context.Background()

	runner := func(ctx context.Context, stepNum int, history []map[string]any) (*agentloop.StepEvent, error) {
		return &agentloop.StepEvent{
			StepNumber:  stepNum,
			TextContent: "Step answer",
			ToolCalls: []agentloop.ToolCall{
				{ID: "tool-1", ToolName: "keep_going", Arguments: "{}"},
			},
			TokensUsed: 50,
		}, nil
	}

	params := agentloop.AgentLoopParams{
		Runner: runner,
		Seams: agentloop.AgentLoopSeams{
			MaxSteps: 5,
			StopWhen: func(event agentloop.StepEvent) bool {
				return event.StepNumber >= 2
			},
		},
		Config: agentloop.AgentLoopConfig{
			MaxSteps:       5,
			MaxRetries:     0,
			RetryBaseDelay: time.Millisecond,
		},
	}

	res, err := agentloop.RunAgentLoopCall(ctx, params)
	require.NoError(t, err)
	assert.Equal(t, 2, len(res.Steps))
	assert.Equal(t, "stop_condition_met", res.StopReason)
}
