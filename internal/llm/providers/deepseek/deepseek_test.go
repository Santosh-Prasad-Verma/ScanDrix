package deepseek_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/llm/orchestrator"
	"github.com/scandrix/backend/internal/llm/providers/deepseek"
)

func TestDeepSeekClientEndToEnd(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "Bearer ds_test_key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "cmpl-deepseek-123",
			"choices": [
				{
					"message": {
						"role": "assistant",
						"content": "{\"summary\": \"Analysis passed\", \"findings\": []}",
						"reasoning_content": "Inspected AST and memory bounds."
					}
				}
			],
			"usage": {
				"prompt_tokens": 150,
				"completion_tokens": 50,
				"total_tokens": 200,
				"prompt_cache_hit_tokens": 100
			}
		}`))
	}))
	defer server.Close()

	client := deepseek.NewClient("ds_test_key", server.URL)

	req := orchestrator.InferenceRequest{
		WorkspaceID: uuid.New(),
		Messages: []orchestrator.ChatMessage{
			{Role: orchestrator.RoleSystem, Content: "You are a code reviewer."},
			{Role: orchestrator.RoleUser, Content: "Review diff"},
		},
		PreferredModel:  "deepseek-reasoner",
		EnableReasoning: true,
	}

	resp, err := client.Complete(ctx, req, "deepseek-reasoner")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ProviderUsed != orchestrator.ProviderDeepSeek || resp.PromptTokens != 150 || resp.TotalTokens != 200 {
		t.Fatalf("unexpected response metrics: %+v", resp)
	}

	if resp.Content != "{\"summary\": \"Analysis passed\", \"findings\": []}" {
		t.Fatalf("unexpected content: %s", resp.Content)
	}
}
