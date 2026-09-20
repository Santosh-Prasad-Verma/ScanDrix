// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package anthropic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scandrix/backend/internal/llm/byok"
	"github.com/scandrix/backend/internal/llm/providers/kernel"
)

func TestAnthropicModuleExecution(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-anthropic-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		resp := anthropicResponseWire{
			ID:   "msg_123",
			Type: "message",
			Role: "assistant",
			Content: []anthropicContentBlockWire{
				{
					Type: "text",
					Text: "Hello from Drixy AI",
				},
			},
		}
		resp.Usage.InputTokens = 10
		resp.Usage.OutputTokens = 5

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	mod := New(WithHTTPClient(server.Client()))
	cfg := byok.NormalizedModel{
		Provider: "anthropic",
		Model:    "claude-3-7-sonnet-20250219",
		APIKey:   "test-anthropic-key",
		BaseURL:  server.URL,
	}

	req := kernel.ExecutionRequest{
		Messages: []kernel.ChatMessage{
			{Role: "user", Content: "hello drixy"},
		},
	}

	res, err := mod.Execute(context.Background(), cfg, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Text != "Hello from Drixy AI" {
		t.Fatalf("expected 'Hello from Drixy AI', got %s", res.Text)
	}

	if res.Usage.TotalTokens != 15 {
		t.Fatalf("expected 15 total tokens, got %d", res.Usage.TotalTokens)
	}
}
