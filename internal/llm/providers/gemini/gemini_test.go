package gemini_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scandrix/backend/internal/llm/orchestrator"
	"github.com/scandrix/backend/internal/llm/providers/gemini"
)

func TestGeminiComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("key") != "gemini-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{
				{
					"content": map[string]any{
						"parts": []map[string]string{
							{"text": `{"verdict":"CLEAN"}`},
						},
					},
				},
			},
			"usageMetadata": map[string]int{
				"promptTokenCount":     90,
				"candidatesTokenCount": 30,
				"totalTokenCount":      120,
			},
		})
	}))
	defer server.Close()

	client := gemini.NewClient("gemini-key", server.URL)
	resp, err := client.Complete(context.Background(), orchestrator.InferenceRequest{
		Messages: []orchestrator.ChatMessage{
			{Role: orchestrator.RoleSystem, Content: "You are a code reviewer"},
			{Role: orchestrator.RoleUser, Content: "Inspect this PR"},
		},
		EnableReasoning: true,
	}, "gemini-3.1-pro")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != `{"verdict":"CLEAN"}` {
		t.Fatalf("unexpected content: %s", resp.Content)
	}
	if resp.PromptTokens != 90 || resp.CompletionTokens != 30 || resp.TotalTokens != 120 {
		t.Fatalf("unexpected token counts: %+v", resp)
	}
	if resp.ProviderUsed != orchestrator.ProviderGemini {
		t.Fatalf("unexpected provider: %s", resp.ProviderUsed)
	}
}
