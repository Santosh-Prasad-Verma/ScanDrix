// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package openrouter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scandrix/backend/internal/llm/byok"
	"github.com/scandrix/backend/internal/llm/providers/kernel"
)

func TestOpenRouterModuleExecution(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-openrouter-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Header.Get("X-Title") != "ScanDrix AI" {
			http.Error(w, "missing scandrix header", http.StatusBadRequest)
			return
		}

		resp := openRouterChatResponse{
			ID: "gen-123",
			Choices: []struct {
				Index   int `json:"index"`
				Message struct {
					Role      string `json:"role"`
					Content   *string `json:"content"`
					ToolCalls []struct {
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls,omitempty"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			}{
				{
					Index: 0,
					Message: struct {
						Role      string `json:"role"`
						Content   *string `json:"content"`
						ToolCalls []struct {
							ID       string `json:"id"`
							Type     string `json:"type"`
							Function struct {
								Name      string `json:"name"`
								Arguments string `json:"arguments"`
							} `json:"function"`
						} `json:"tool_calls,omitempty"`
					}{
						Role: "assistant",
						Content: func() *string {
							s := "OpenRouter response for ScanDrix"
							return &s
						}(),
					},
					FinishReason: "stop",
				},
			},
		}
		resp.Usage.PromptTokens = 8
		resp.Usage.CompletionTokens = 4
		resp.Usage.TotalTokens = 12

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	mod := New(WithHTTPClient(server.Client()))
	cfg := byok.NormalizedModel{
		Provider: "openrouter",
		Model:    "anthropic/claude-3.5-sonnet",
		APIKey:   "test-openrouter-key",
		BaseURL:  server.URL,
	}

	req := kernel.ExecutionRequest{
		Messages: []kernel.ChatMessage{
			{Role: "user", Content: "test prompt"},
		},
	}

	res, err := mod.Execute(context.Background(), cfg, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Text != "OpenRouter response for ScanDrix" {
		t.Fatalf("expected 'OpenRouter response for ScanDrix', got %s", res.Text)
	}

	if res.Usage.TotalTokens != 12 {
		t.Fatalf("expected 12 total tokens, got %d", res.Usage.TotalTokens)
	}
}
