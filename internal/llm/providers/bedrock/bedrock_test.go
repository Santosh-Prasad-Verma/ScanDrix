package bedrock_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/llm/orchestrator"
	"github.com/scandrix/backend/internal/llm/providers/bedrock"
)

func TestBedrockClientEndToEnd(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test_bedrock_bearer" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"output": {
				"message": {
					"role": "assistant",
					"content": [
						{"text": "{\"summary\": \"Bedrock Review OK\", \"findings\": []}"}
					]
				}
			},
			"usage": {
				"inputTokens": 300,
				"outputTokens": 100,
				"totalTokens": 400
			}
		}`))
	}))
	defer server.Close()

	client := bedrock.NewClient("us-east-1", "test_bedrock_bearer", server.URL)

	req := orchestrator.InferenceRequest{
		WorkspaceID: uuid.New(),
		Messages: []orchestrator.ChatMessage{
			{Role: orchestrator.RoleSystem, Content: "You are a code analyzer."},
			{Role: orchestrator.RoleUser, Content: "Analyze pull request"},
		},
		PreferredModel: "bedrock-claude-3-7-sonnet",
	}

	resp, err := client.Complete(ctx, req, "bedrock-claude-3-7-sonnet")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ProviderUsed != orchestrator.ProviderBedrock || resp.PromptTokens != 300 || resp.CompletionTokens != 100 {
		t.Fatalf("unexpected metrics: %+v", resp)
	}

	if resp.Content != "{\"summary\": \"Bedrock Review OK\", \"findings\": []}" {
		t.Fatalf("unexpected content: %s", resp.Content)
	}
}
