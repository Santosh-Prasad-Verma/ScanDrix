// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package invocation_test

import (
	"testing"

	"github.com/scandrix/backend/internal/llm/byok"
	"github.com/scandrix/backend/internal/llm/invocation"
	_ "github.com/scandrix/backend/internal/llm/providers/all"
)

func TestAgentModelIdentity(t *testing.T) {
	// Nil slot -> system default
	idNil := invocation.AgentModelIdentity(nil)
	if idNil.IsBYOK {
		t.Fatal("expected IsBYOK false for nil slot")
	}
	if idNil.Model != "system:default" {
		t.Fatalf("expected system:default, got %s", idNil.Model)
	}

	// BYOK slot
	slot := &byok.NormalizedModel{
		Provider:     byok.ProviderAnthropic,
		Model:        "claude-3-7-sonnet-20250219",
		BYOKModelID:  "model-1",
		CredentialID: "cred-1",
	}
	idSlot := invocation.AgentModelIdentity(slot)
	if !idSlot.IsBYOK {
		t.Fatal("expected IsBYOK true for slot")
	}
	if idSlot.Model != "anthropic:claude-3-7-sonnet-20250219" {
		t.Fatalf("expected anthropic:claude-3-7-sonnet-20250219, got %s", idSlot.Model)
	}
	if idSlot.BYOKModelID != "model-1" || idSlot.CredentialID != "cred-1" {
		t.Fatalf("unexpected attribution IDs: %v", idSlot)
	}
}

func TestResolveModelConfig(t *testing.T) {
	temp := 0.5
	slot := &byok.NormalizedModel{
		Provider:        byok.ProviderOpenAI,
		Model:           "gpt-4o",
		Temperature:     &temp,
		MaxOutputTokens: 2048,
		ReasoningEffort: "medium",
	}

	inv := invocation.ResolveModelConfig(slot, invocation.ResolveModelInvocationOptions{
		RunName: "code-review",
	})

	if inv.ModelName != "openai:gpt-4o" {
		t.Fatalf("expected openai:gpt-4o, got %s", inv.ModelName)
	}
	if inv.CallOptions.Temperature == nil || *inv.CallOptions.Temperature != 0.5 {
		t.Fatalf("expected temp 0.5, got %v", inv.CallOptions.Temperature)
	}
	if inv.CallOptions.MaxOutputTokens != 2048 {
		t.Fatalf("expected 2048 max output tokens, got %d", inv.CallOptions.MaxOutputTokens)
	}
}

func TestResolveTaskInvocation(t *testing.T) {
	cfg := &byok.BYOKConfig{
		Version: 2,
		Credentials: []byok.BYOKCredential{
			{ID: "cred-1", Provider: "openai"},
		},
		Models: []byok.BYOKModelConfig{
			{ID: "m-1", CredentialID: "cred-1", Model: "gpt-4o"},
		},
		Routing: byok.BYOKRouting{
			DefaultModelID: "m-1",
		},
	}

	taskInv := invocation.ResolveTaskInvocation(cfg, byok.TaskCodeReview, invocation.ResolveTaskInvocationOptions{
		ResolveModelInvocationOptions: invocation.ResolveModelInvocationOptions{
			RunName: "review-test",
		},
	})

	if taskInv.Slot == nil {
		t.Fatal("expected resolved slot")
	}
	if taskInv.UsageIdentity.Model != taskInv.ModelName {
		t.Fatalf("expected usage identity model to match model name: %s vs %s", taskInv.UsageIdentity.Model, taskInv.ModelName)
	}
}
