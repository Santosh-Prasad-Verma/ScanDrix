// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package conversation

import (
	"context"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/agentharness/contracts"
	"github.com/scandrix/backend/internal/agentharness/infrastructure/persistence"
)

type mockAgentRunner struct {
	runFn func(ctx context.Context, spec contracts.AgentSpec, input contracts.AgentRunInput, toolCtx contracts.ToolContext) (*contracts.RunState, error)
}

func (m *mockAgentRunner) Run(ctx context.Context, spec contracts.AgentSpec, input contracts.AgentRunInput, toolCtx contracts.ToolContext) (*contracts.RunState, error) {
	return m.runFn(ctx, spec, input, toolCtx)
}

func TestNormalizeConversationResponse(t *testing.T) {
	cases := []struct {
		name     string
		input    any
		expected string
	}{
		{
			name:     "Plain string",
			input:    "Hello engineer",
			expected: "Hello engineer",
		},
		{
			name:     "JSON content envelope",
			input:    `{"content": "This is unwrapped answer"}`,
			expected: "This is unwrapped answer",
		},
		{
			name:     "Markdown code-fenced JSON envelope",
			input:    "```json\n{\"content\": \"Fenced answer\"}\n```",
			expected: "Fenced answer",
		},
		{
			name:     "Map input",
			input:    map[string]any{"content": "From map"},
			expected: "From map",
		},
		{
			name:     "Nested envelope",
			input:    `{"content": "{\"answer\": \"Double unwrapped\"}"}`,
			expected: "Double unwrapped",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := NormalizeConversationResponse(tc.input)
			if actual != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, actual)
			}
		})
	}
}

func TestConversationAgentProvider_Execute(t *testing.T) {
	ctx := context.Background()
	store := persistence.NewInMemoryConversationStore(10)

	runner := &mockAgentRunner{
		runFn: func(ctx context.Context, spec contracts.AgentSpec, input contracts.AgentRunInput, toolCtx contracts.ToolContext) (*contracts.RunState, error) {
			return &contracts.RunState{
				Status: contracts.StatusCompleted,
				Steps: []contracts.RunStep{
					{
						Index: 0,
						Message: contracts.AgentMessage{
							Role:    contracts.RoleAssistant,
							Content: `{"content": "Refactored user controller to use dependency injection."}`,
						},
					},
				},
				Usage: contracts.TokenUsage{InputTokens: 100, OutputTokens: 50},
			}, nil
		},
	}

	provider := NewConversationAgentProvider(runner, store)

	req := ConversationRequest{
		Prompt:         "What changed in the auth module?",
		OrganizationID: "org-1",
		TeamID:         "team-1",
		ThreadID:       "cmc:org-1:team-1:user-1:conv-1",
	}

	res, err := provider.Execute(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Response != "Refactored user controller to use dependency injection." {
		t.Errorf("unexpected response: %s", res.Response)
	}
	if res.Usage.Total() != 150 {
		t.Errorf("unexpected usage: %v", res.Usage)
	}

	// Verify that history was appended to the conversation store
	history, err := store.Load(ctx, req.ThreadID)
	if err != nil {
		t.Fatalf("store load error: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 turns in store, got %d", len(history))
	}
	if history[0].Role != contracts.RoleUser || history[0].Content != req.Prompt {
		t.Errorf("unexpected user message in store: %v", history[0])
	}
	if history[1].Role != contracts.RoleAssistant || !strings.Contains(history[1].Content, "Refactored user controller") {
		t.Errorf("unexpected assistant message in store: %v", history[1])
	}
}

func TestConversationAgentProvider_AutoSandboxTools(t *testing.T) {
	ctx := context.Background()
	store := persistence.NewInMemoryConversationStore(10)

	var recordedSpec contracts.AgentSpec
	runner := &mockAgentRunner{
		runFn: func(ctx context.Context, spec contracts.AgentSpec, input contracts.AgentRunInput, toolCtx contracts.ToolContext) (*contracts.RunState, error) {
			recordedSpec = spec
			return &contracts.RunState{
				Status: contracts.StatusCompleted,
				Steps: []contracts.RunStep{
					{
						Index: 0,
						Message: contracts.AgentMessage{
							Role:    contracts.RoleAssistant,
							Content: "Found matching files.",
						},
					},
				},
			}, nil
		},
	}

	tempDir := t.TempDir()
	provider := NewConversationAgentProvider(runner, store, ConversationAgentOptions{
		DefaultSandboxRoot: tempDir,
	})

	req := ConversationRequest{
		Prompt:         "Find all references to auth",
		OrganizationID: "org-1",
		TeamID:         "team-1",
		ThreadID:       "thread-1",
	}

	_, err := provider.Execute(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	toolsList := recordedSpec.Tools.List()
	if len(toolsList) < 4 {
		t.Fatalf("expected at least 4 sandbox tools registered, got %d", len(toolsList))
	}

	toolNames := make(map[string]bool)
	for _, tl := range toolsList {
		toolNames[tl.Name()] = true
	}

	for _, expectedTool := range []string{"grep", "readFile", "listDir", "exec"} {
		if !toolNames[expectedTool] {
			t.Errorf("expected auto-attached sandbox tool %s, but missing", expectedTool)
		}
	}
}

