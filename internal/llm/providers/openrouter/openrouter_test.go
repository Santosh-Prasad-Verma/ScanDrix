package openrouter_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/llm/orchestrator"
	"github.com/scandrix/backend/internal/llm/providers/openrouter"
)

func TestOpenRouterClientEndToEnd(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test_openrouter_key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "gen-openrouter-nemotron-123",
			"choices": [
				{
					"message": {
						"role": "assistant",
						"content": "{\"summary\": \"Nemotron Review Analysis OK\", \"findings\": []}"
					}
				}
			],
			"usage": {
				"prompt_tokens": 175,
				"completion_tokens": 65,
				"total_tokens": 240
			}
		}`))
	}))
	defer server.Close()

	client := openrouter.NewClient("test_openrouter_key", server.URL)

	req := orchestrator.InferenceRequest{
		WorkspaceID: uuid.New(),
		Messages: []orchestrator.ChatMessage{
			{Role: orchestrator.RoleSystem, Content: "You are a code reviewer."},
			{Role: orchestrator.RoleUser, Content: "Analyze pull request with Nemotron."},
		},
		PreferredModel: "nvidia/nemotron-3-ultra-550b-a55b:free",
	}

	resp, err := client.Complete(ctx, req, "nvidia/nemotron-3-ultra-550b-a55b:free")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ProviderUsed != orchestrator.ProviderOpenRouter || resp.PromptTokens != 175 || resp.CompletionTokens != 65 {
		t.Fatalf("unexpected metrics: %+v", resp)
	}

	if resp.Content != "{\"summary\": \"Nemotron Review Analysis OK\", \"findings\": []}" {
		t.Fatalf("unexpected content: %s", resp.Content)
	}
}
