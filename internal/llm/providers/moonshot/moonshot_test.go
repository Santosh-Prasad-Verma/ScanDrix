// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package moonshot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scandrix/backend/internal/llm/byok"
	"github.com/scandrix/backend/internal/llm/providers/kernel"
)

func TestMoonshotModuleExecution(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-moonshot-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"id": "msg_123",
			"type": "message",
			"role": "assistant",
			"content": [{"type": "text", "text": "Moonshot Kimi response"}],
			"usage": {"input_tokens": 50, "output_tokens": 30}
		}`))
	}))
	defer ts.Close()

	client := ts.Client()
	mod := New(WithHTTPClient(client))

	if mod.ID() != "moonshot" {
		t.Fatalf("expected ID moonshot, got %s", mod.ID())
	}

	cfg := byok.NormalizedModel{
		APIKey:  "test-moonshot-key",
		BaseURL: ts.URL,
		Model:   "kimi-k2.5",
	}

	res, err := mod.Execute(context.Background(), cfg, kernel.ExecutionRequest{
		Messages: []kernel.ChatMessage{
			{Role: "user", Content: "Hello Kimi"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Text != "Moonshot Kimi response" {
		t.Fatalf("expected 'Moonshot Kimi response', got %q", res.Text)
	}
}
