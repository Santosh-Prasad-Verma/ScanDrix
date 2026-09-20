// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Application Layer Tests
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package usecases

import (
	"context"
	"testing"

	"github.com/scandrix/backend/internal/agentharness/contracts"
	"github.com/scandrix/backend/internal/agents/businessrules"
	"github.com/scandrix/backend/internal/agents/conversation"
	"github.com/scandrix/backend/internal/agents/persistence"
)

type mockAgentRunner struct {
	runFn func(ctx context.Context, spec contracts.AgentSpec, input contracts.AgentRunInput, toolCtx contracts.ToolContext) (*contracts.RunState, error)
}

func (m *mockAgentRunner) Run(ctx context.Context, spec contracts.AgentSpec, input contracts.AgentRunInput, toolCtx contracts.ToolContext) (*contracts.RunState, error) {
	return m.runFn(ctx, spec, input, toolCtx)
}

func TestBusinessRulesValidationAgentUseCase(t *testing.T) {
	ctx := context.Background()

	runner := &mockAgentRunner{
		runFn: func(ctx context.Context, spec contracts.AgentSpec, input contracts.AgentRunInput, toolCtx contracts.ToolContext) (*contracts.RunState, error) {
			return &contracts.RunState{
				Status: contracts.StatusCompleted,
				Steps: []contracts.RunStep{
					{
						Index: 0,
						Message: contracts.AgentMessage{
							Role:    contracts.RoleAssistant,
							Content: `{"is_compliant": true, "summary": "All rules verified."}`,
						},
					},
				},
			}, nil
		},
	}

	provider := businessrules.NewBusinessRulesValidationAgentProvider(runner)
	uc := NewBusinessRulesValidationAgentUseCase(provider)

	// Test limitation response path (missing task context & pr diff)
	res, err := uc.Execute(ctx, businessrules.BusinessRulesContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.NeedsMoreInfo {
		t.Errorf("expected NeedsMoreInfo=true for empty context")
	}

	// Test full analysis path
	bctx := businessrules.BusinessRulesContext{
		TaskContext: "Task: PROJ-1\nDescription: Add auth",
		PRDiff:      "+ func Auth() {}",
		TaskQuality: "COMPLETE",
	}

	res, err = uc.Execute(ctx, bctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsCompliant || res.Summary != "All rules verified." {
		t.Errorf("unexpected validation result: %+v", res)
	}
}

func TestConversationAgentUseCase(t *testing.T) {
	ctx := context.Background()
	store := persistence.NewAgentSessionStore(10)

	runner := &mockAgentRunner{
		runFn: func(ctx context.Context, spec contracts.AgentSpec, input contracts.AgentRunInput, toolCtx contracts.ToolContext) (*contracts.RunState, error) {
			return &contracts.RunState{
				Status: contracts.StatusCompleted,
				Steps: []contracts.RunStep{
					{
						Index: 0,
						Message: contracts.AgentMessage{
							Role:    contracts.RoleAssistant,
							Content: "Here is the architectural review explanation.",
						},
					},
				},
			}, nil
		},
	}

	provider := conversation.NewConversationAgentProvider(runner, store)
	uc := NewConversationAgentUseCase(provider)

	req := conversation.ConversationRequest{
		Prompt:   "Review this diff",
		ThreadID: "thread-uc-1",
	}

	res, err := uc.Execute(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Response != "Here is the architectural review explanation." {
		t.Errorf("unexpected response: %s", res.Response)
	}
	if res.ThreadID != "thread-uc-1" {
		t.Errorf("expected thread-uc-1, got %s", res.ThreadID)
	}

	// Verify persistence in session store
	history, err := store.Load(ctx, "thread-uc-1")
	if err != nil || len(history) != 2 {
		t.Fatalf("expected 2 turns in store, got %d, err: %v", len(history), err)
	}
}
