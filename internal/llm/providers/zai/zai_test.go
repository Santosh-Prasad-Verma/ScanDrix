// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package zai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scandrix/backend/internal/llm/byok"
	"github.com/scandrix/backend/internal/llm/providers/kernel"
)

func TestZaiModuleExecution(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-zai-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"id": "msg_456",
			"type": "message",
			"role": "assistant",
			"content": [{"type": "text", "text": "Z.ai GLM response"}],
			"usage": {"input_tokens": 60, "output_tokens": 40}
		}`))
	}))
	defer ts.Close()

	client := ts.Client()
	mod := New(WithHTTPClient(client))

	if mod.ID() != "zai" {
		t.Fatalf("expected ID zai, got %s", mod.ID())
	}

	listing := mod.ModelListing("zai")
	if listing == nil || listing.Kind != kernel.ListingManual {
		t.Fatalf("expected manual model listing for zai")
	}

	cfg := byok.NormalizedModel{
		APIKey:  "test-zai-key",
		BaseURL: ts.URL,
		Model:   "glm-4-plus",
	}

	res, err := mod.Execute(context.Background(), cfg, kernel.ExecutionRequest{
		Messages: []kernel.ChatMessage{
			{Role: "user", Content: "Hello GLM"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Text != "Z.ai GLM response" {
		t.Fatalf("expected 'Z.ai GLM response', got %q", res.Text)
	}
}
