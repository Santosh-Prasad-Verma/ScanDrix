// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package llm

import (
	"context"
	"testing"

	"github.com/scandrix/backend/internal/llm/byok"
	"github.com/scandrix/backend/internal/llm/providers/kernel"
)

type sampleReview struct {
	Verdict   string `json:"verdict"`
	RiskScore int    `json:"risk_score"`
}

type mockEngineProvider struct {
	response *kernel.ExecutionResult
}

func (m *mockEngineProvider) ID() string                                                    { return "mock-engine-provider" }
func (m *mockEngineProvider) Aliases() []string                                             { return []string{"mock"} }
func (m *mockEngineProvider) Label() string                                                 { return "Mock" }
func (m *mockEngineProvider) Doc() string                                                   { return "https://scandrix.dev" }
func (m *mockEngineProvider) Capabilities(model string) kernel.ModelCapabilities           { return kernel.ModelCapabilities{} }
func (m *mockEngineProvider) ReasoningTraits(cfg byok.NormalizedModel) kernel.ModelReasoningTraits {
	return kernel.ModelReasoningTraits{}
}
func (m *mockEngineProvider) TemperaturePolicy(cfg byok.NormalizedModel) *kernel.TemperaturePolicy {
	return nil
}
func (m *mockEngineProvider) SystemCacheControl(cfg byok.NormalizedModel) map[string]any {
	return nil
}
func (m *mockEngineProvider) UIFields() []kernel.FieldDescriptor                 { return nil }
func (m *mockEngineProvider) ModelListing(providerID string) *kernel.ModelListing { return nil }
func (m *mockEngineProvider) Execute(ctx context.Context, cfg byok.NormalizedModel, req kernel.ExecutionRequest) (*kernel.ExecutionResult, error) {
	return m.response, nil
}

func TestEngineRunStructuredWithRepair(t *testing.T) {
	mockProv := &mockEngineProvider{
		response: &kernel.ExecutionResult{
			// Return markdown code fence with trailing comma to test unwrap + repair
			Text: "```json\n{\n  \"verdict\": \"APPROVE\",\n  \"risk_score\": 10,\n}\n```",
			Usage: kernel.TokenUsage{
				InputTokens:  10,
				OutputTokens: 15,
				TotalTokens:  25,
			},
		},
	}
	kernel.Register(mockProv)

	engine := NewEngine()

	slot := byok.NormalizedModel{
		Provider: "mock-engine-provider",
		Model:    "gpt-4o",
	}

	msgs := []kernel.ChatMessage{
		{Role: "system", Content: "You are Drixy AI code reviewer."},
		{Role: "user", Content: "Review this diff."},
	}

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"verdict":    map[string]any{"type": "string"},
			"risk_score": map[string]any{"type": "integer"},
		},
		"required": []any{"verdict", "risk_score"},
	}

	var review sampleReview
	res, err := engine.RunStructured(context.Background(), slot, msgs, schema, &review)
	if err != nil {
		t.Fatalf("unexpected error running structured review: %v", err)
	}

	if review.Verdict != "APPROVE" {
		t.Fatalf("expected verdict APPROVE, got %s", review.Verdict)
	}
	if review.RiskScore != 10 {
		t.Fatalf("expected risk score 10, got %d", review.RiskScore)
	}
	if res.Usage.TotalTokens != 25 {
		t.Fatalf("expected 25 tokens, got %d", res.Usage.TotalTokens)
	}
}

func TestEngineRunText(t *testing.T) {
	mockProv := &mockEngineProvider{
		response: &kernel.ExecutionResult{
			Text: "Hello from ScanDrix Engine",
			Usage: kernel.TokenUsage{
				TotalTokens: 12,
			},
		},
	}
	kernel.Register(mockProv)

	engine := NewEngine()
	slot := byok.NormalizedModel{
		Provider: "mock-engine-provider",
		Model:    "claude-3-7-sonnet",
	}

	msgs := []kernel.ChatMessage{
		{Role: "user", Content: "hi"},
	}

	res, err := engine.RunText(context.Background(), slot, msgs)
	if err != nil {
		t.Fatalf("unexpected error running text: %v", err)
	}

	if res.Text != "Hello from ScanDrix Engine" {
		t.Fatalf("expected 'Hello from ScanDrix Engine', got %s", res.Text)
	}
}
