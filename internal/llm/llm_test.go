// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package llm

import (
	"context"
	"testing"

	"github.com/scandrix/backend/internal/llm/byok"
	"github.com/scandrix/backend/internal/llm/providers/kernel"
)

type mockTestProvider struct {
	id string
}

func (m *mockTestProvider) ID() string                                                    { return m.id }
func (m *mockTestProvider) Aliases() []string                                             { return nil }
func (m *mockTestProvider) Label() string                                                 { return "Mock Test" }
func (m *mockTestProvider) Doc() string                                                   { return "" }
func (m *mockTestProvider) Capabilities(model string) kernel.ModelCapabilities           { return kernel.ModelCapabilities{} }
func (m *mockTestProvider) ReasoningTraits(cfg byok.NormalizedModel) kernel.ModelReasoningTraits {
	return kernel.ModelReasoningTraits{}
}
func (m *mockTestProvider) TemperaturePolicy(cfg byok.NormalizedModel) *kernel.TemperaturePolicy {
	return nil
}
func (m *mockTestProvider) SystemCacheControl(cfg byok.NormalizedModel) map[string]any { return nil }
func (m *mockTestProvider) UIFields() []kernel.FieldDescriptor                         { return nil }
func (m *mockTestProvider) ModelListing(providerID string) *kernel.ModelListing         { return nil }
func (m *mockTestProvider) Execute(ctx context.Context, cfg byok.NormalizedModel, req kernel.ExecutionRequest) (*kernel.ExecutionResult, error) {
	if req.ResponseSchema != nil {
		return &kernel.ExecutionResult{
			Text: `{"approved": true, "reason": "Looks good"}`,
			Usage: kernel.TokenUsage{
				TotalTokens: 50,
			},
		}, nil
	}
	return &kernel.ExecutionResult{
		Text: "Plain text review result",
		Usage: kernel.TokenUsage{
			TotalTokens: 30,
		},
	}, nil
}

func TestLLMRun(t *testing.T) {
	kernel.Register(&mockTestProvider{id: "mock_test_provider"})

	slot := &byok.NormalizedModel{
		Provider: "mock_test_provider",
		Model:    "mock-v1",
	}

	ctx := context.Background()

	// 1. Text Call Test
	resText, err := Run(ctx, LLMRequest{
		ByokConfig: slot,
		User:       "Review this change",
	})
	if err != nil {
		t.Fatalf("Run text failed: %v", err)
	}
	if resText.Text != "Plain text review result" {
		t.Fatalf("unexpected text: %s", resText.Text)
	}

	// 2. Structured Call Test
	type reviewResult struct {
		Approved bool   `json:"approved"`
		Reason   string `json:"reason"`
	}
	var target reviewResult
	resStruct, err := Run(ctx, LLMRequest{
		ByokConfig: slot,
		User:       "Review structured",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"approved": map[string]any{"type": "boolean"},
				"reason":   map[string]any{"type": "string"},
			},
		},
		Target: &target,
	})
	if err != nil {
		t.Fatalf("Run structured failed: %v", err)
	}
	if !target.Approved || target.Reason != "Looks good" {
		t.Fatalf("failed unmarshaling structured response: %+v", target)
	}
	if resStruct.Usage.TotalTokens != 50 {
		t.Fatalf("expected 50 tokens, got %d", resStruct.Usage.TotalTokens)
	}
}
