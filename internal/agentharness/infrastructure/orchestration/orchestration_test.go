// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Harness Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package orchestration_test

import (
	"context"
	"errors"
	"testing"

	"github.com/scandrix/backend/internal/agentharness/contracts"
	"github.com/scandrix/backend/internal/agentharness/infrastructure/orchestration"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockRunner struct {
	runFn func(ctx context.Context, spec contracts.AgentSpec, input contracts.AgentRunInput, toolCtx contracts.ToolContext) (*contracts.RunState, error)
}

func (m *mockRunner) Run(ctx context.Context, spec contracts.AgentSpec, input contracts.AgentRunInput, toolCtx contracts.ToolContext) (*contracts.RunState, error) {
	return m.runFn(ctx, spec, input, toolCtx)
}

func TestDefaultSubAgentFactory(t *testing.T) {
	runner := &mockRunner{
		runFn: func(ctx context.Context, spec contracts.AgentSpec, input contracts.AgentRunInput, toolCtx contracts.ToolContext) (*contracts.RunState, error) {
			return &contracts.RunState{
				Status: contracts.StatusCompleted,
				Steps: []contracts.RunStep{
					{
						Message: contracts.AgentMessage{
							Role:    contracts.RoleAssistant,
							Content: "Sub-agent result for: " + input.Prompt,
						},
					},
				},
			}, nil
		},
	}

	factory := orchestration.NewDefaultSubAgentFactory(runner)
	tool := factory.AsTool(contracts.SubAgentParams{
		Name:        "exploreFiles",
		Description: "Explores files",
		Spec: contracts.AgentSpec{
			ID: "explorer-subagent",
		},
		ToPrompt: func(input any) string {
			return input.(map[string]any)["query"].(string)
		},
		Summarize: func(state *contracts.RunState) string {
			return state.Steps[0].Message.Content.(string)
		},
	})

	ctx := contracts.ToolContext{
		RunID:   "parent-run",
		Context: context.Background(),
	}

	res, err := tool.Execute(ctx, map[string]any{"query": "find auth handlers"})
	require.NoError(t, err)
	assert.Equal(t, "Sub-agent result for: find auth handlers", res.Output)
	assert.Equal(t, "explorer-subagent", res.Meta["subAgentId"])
	assert.Equal(t, "completed", res.Meta["status"])
}

type mockVerifier struct {
	shouldRefute map[string]bool
	panicOn      string
	errorOn      string
}

func (m *mockVerifier) Verify(ctx context.Context, candidate string, toolCtx contracts.ToolContext) (contracts.Verdict, error) {
	if candidate == m.panicOn {
		panic("simulated verifier panic")
	}
	if candidate == m.errorOn {
		return contracts.Verdict{}, errors.New("simulated network failure")
	}
	if m.shouldRefute[candidate] {
		return contracts.Verdict{
			Keep:      false,
			Rationale: "candidate refuted",
		}, nil
	}
	return contracts.Verdict{
		Keep:      true,
		Rationale: "valid candidate",
	}, nil
}

func TestRunVerificationPass(t *testing.T) {
	verifier := &mockVerifier{
		shouldRefute: map[string]bool{
			"bad-finding-1": true,
		},
		panicOn: "panicking-finding",
		errorOn: "error-finding",
	}

	candidates := []string{
		"good-finding-1",
		"bad-finding-1",
		"good-finding-2",
		"panicking-finding",
		"error-finding",
	}

	ctx := contracts.ToolContext{
		RunID:   "run-verify",
		Context: context.Background(),
	}

	res, err := orchestration.RunVerificationPass(context.Background(), orchestration.VerificationPassParams[string]{
		Candidates:  candidates,
		Verifier:    verifier,
		Concurrency: 2,
	}, ctx)

	require.NoError(t, err)
	// Kept must include: good-finding-1, good-finding-2, plus the 2 fail-open candidates (panicking & error)
	assert.Contains(t, res.Kept, "good-finding-1")
	assert.Contains(t, res.Kept, "good-finding-2")
	assert.Contains(t, res.Kept, "panicking-finding", "must fail-open on panic")
	assert.Contains(t, res.Kept, "error-finding", "must fail-open on error")
	assert.Len(t, res.Kept, 4)

	// Dropped must contain bad-finding-1
	require.Len(t, res.Dropped, 1)
	assert.Equal(t, "bad-finding-1", res.Dropped[0].Candidate)
	assert.False(t, res.Dropped[0].Verdict.Keep)
}
