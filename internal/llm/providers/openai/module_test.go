// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scandrix/backend/internal/llm/byok"
	"github.com/scandrix/backend/internal/llm/providers/kernel"
)

func TestOpenAIModuleExecution(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		resp := openAIChatResponse{
			ID: "chatcmpl-123",
			Choices: []struct {
				Index   int `json:"index"`
				Message struct {
					Role      string               `json:"role"`
					Content   *string              `json:"content"`
					ToolCalls []openAIToolCallWire `json:"tool_calls,omitempty"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			}{
				{
					Index: 0,
					Message: struct {
						Role      string               `json:"role"`
						Content   *string              `json:"content"`
						ToolCalls []openAIToolCallWire `json:"tool_calls,omitempty"`
					}{
						Role: "assistant",
						Content: func() *string {
							s := `{"status":"ok"}`
							return &s
						}(),
					},
					FinishReason: "stop",
				},
			},
		}
		resp.Usage.PromptTokens = 15
		resp.Usage.CompletionTokens = 8
		resp.Usage.TotalTokens = 23

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	mod := New(WithHTTPClient(server.Client()))
	cfg := byok.NormalizedModel{
		Provider: "openai",
		Model:    "gpt-4o",
		APIKey:   "test-key",
		BaseURL:  server.URL,
	}

	req := kernel.ExecutionRequest{
		Messages: []kernel.ChatMessage{
			{Role: "user", Content: "hello"},
		},
		ResponseSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status": map[string]any{"type": "string"},
			},
		},
	}

	res, err := mod.Execute(context.Background(), cfg, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Text != `{"status":"ok"}` {
		t.Fatalf("expected text `{\"status\":\"ok\"}`, got %s", res.Text)
	}

	if res.Usage.TotalTokens != 23 {
		t.Fatalf("expected 23 total tokens, got %d", res.Usage.TotalTokens)
	}
}
