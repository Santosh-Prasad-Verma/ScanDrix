package openai_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scandrix/backend/internal/llm/orchestrator"
	"github.com/scandrix/backend/internal/llm/providers/openai"
)

func TestOpenAIComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]string{
						"role":    "assistant",
						"content": `{"findings":[]}`,
					},
				},
			},
			"usage": map[string]int{
				"prompt_tokens":     120,
				"completion_tokens": 40,
				"total_tokens":      160,
			},
		})
	}))
	defer server.Close()

	client := openai.NewClient("test-key", server.URL)
	resp, err := client.Complete(context.Background(), orchestrator.InferenceRequest{
		Messages: []orchestrator.ChatMessage{
			{Role: orchestrator.RoleUser, Content: "Analyze code"},
		},
		EnableReasoning: true,
	}, "gpt-5.6-sol")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != `{"findings":[]}` {
		t.Fatalf("unexpected content: %s", resp.Content)
	}
	if resp.PromptTokens != 120 || resp.CompletionTokens != 40 || resp.TotalTokens != 160 {
		t.Fatalf("unexpected token counts: %+v", resp)
	}
	if resp.ProviderUsed != orchestrator.ProviderOpenAI {
		t.Fatalf("unexpected provider: %s", resp.ProviderUsed)
	}
}

func TestOpenAICompatibleComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]string{
						"role":    "assistant",
						"content": `{"findings":[{"severity":"HIGH"}]}`,
					},
				},
			},
			"usage": map[string]int{
				"prompt_tokens":     200,
				"completion_tokens": 80,
				"total_tokens":      280,
			},
		})
	}))
	defer server.Close()

	// Test with Moonshot Kimi
	client := openai.NewCompatibleClient(orchestrator.ProviderMoonshot, "kimi-key", server.URL)
	resp, err := client.Complete(context.Background(), orchestrator.InferenceRequest{
		Messages: []orchestrator.ChatMessage{
			{Role: orchestrator.RoleUser, Content: "Audit repo"},
		},
	}, "kimi-k3")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ProviderUsed != orchestrator.ProviderMoonshot {
		t.Fatalf("expected ProviderMoonshot, got: %s", resp.ProviderUsed)
	}
	if resp.TotalTokens != 280 {
		t.Fatalf("expected 280 tokens, got: %d", resp.TotalTokens)
	}
}
