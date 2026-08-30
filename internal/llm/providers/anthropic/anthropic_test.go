package anthropic_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scandrix/backend/internal/llm/orchestrator"
	"github.com/scandrix/backend/internal/llm/providers/anthropic"
)

func TestAnthropicComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]string{
				{"type": "text", "text": `{"verdict":"APPROVED"}`},
			},
			"usage": map[string]int{
				"input_tokens":  150,
				"output_tokens": 50,
			},
		})
	}))
	defer server.Close()

	client := anthropic.NewClient("test-key", server.URL)
	resp, err := client.Complete(context.Background(), orchestrator.InferenceRequest{
		Messages: []orchestrator.ChatMessage{
			{Role: orchestrator.RoleSystem, Content: "You are a code reviewer"},
			{Role: orchestrator.RoleUser, Content: "Review this diff"},
		},
		EnableReasoning: true,
	}, "claude-sonnet-5")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != `{"verdict":"APPROVED"}` {
		t.Fatalf("unexpected content: %s", resp.Content)
	}
	if resp.PromptTokens != 150 || resp.CompletionTokens != 50 || resp.TotalTokens != 200 {
		t.Fatalf("unexpected tokens: %+v", resp)
	}
	if resp.ProviderUsed != orchestrator.ProviderAnthropic {
		t.Fatalf("unexpected provider: %s", resp.ProviderUsed)
	}
}
