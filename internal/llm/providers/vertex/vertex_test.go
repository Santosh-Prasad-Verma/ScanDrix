package vertex_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/llm/orchestrator"
	"github.com/scandrix/backend/internal/llm/providers/vertex"
)

func TestVertexClientEndToEnd(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test_vertex_token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"candidates": [
				{
					"content": {
						"role": "model",
						"parts": [
							{"text": "{\"summary\": \"Vertex AI Review OK\", \"findings\": []}"}
						]
					},
					"finishReason": "STOP"
				}
			],
			"usageMetadata": {
				"promptTokenCount": 250,
				"candidatesTokenCount": 80,
				"totalTokenCount": 330
			}
		}`))
	}))
	defer server.Close()

	client := vertex.NewClient("my-project-123", "us-central1", "test_vertex_token", server.URL)

	req := orchestrator.InferenceRequest{
		WorkspaceID: uuid.New(),
		Messages: []orchestrator.ChatMessage{
			{Role: orchestrator.RoleSystem, Content: "You are a code reviewer."},
			{Role: orchestrator.RoleUser, Content: "Inspect unified diff."},
		},
		PreferredModel: "vertex-gemini-2.5-pro",
	}

	resp, err := client.Complete(ctx, req, "vertex-gemini-2.5-pro")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ProviderUsed != orchestrator.ProviderVertex || resp.PromptTokens != 250 || resp.CompletionTokens != 80 {
		t.Fatalf("unexpected response metrics: %+v", resp)
	}

	if resp.Content != "{\"summary\": \"Vertex AI Review OK\", \"findings\": []}" {
		t.Fatalf("unexpected content: %s", resp.Content)
	}
}
