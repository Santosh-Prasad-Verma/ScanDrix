// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package novita

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scandrix/backend/internal/llm/byok"
	"github.com/scandrix/backend/internal/llm/providers/kernel"
)

func TestNovitaModuleExecution(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-novita-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"choices": [{"message": {"role": "assistant", "content": "Novita DeepSeek response"}}],
			"usage": {"prompt_tokens": 40, "completion_tokens": 25, "total_tokens": 65}
		}`))
	}))
	defer ts.Close()

	client := ts.Client()
	mod := New(WithHTTPClient(client))
	if mod.ID() != "novita" {
		t.Fatalf("expected ID novita, got %s", mod.ID())
	}

	listing := mod.ModelListing("novita")
	if listing == nil || listing.Kind != kernel.ListingHTTP {
		t.Fatalf("expected http model listing for novita")
	}

	cfg := byok.NormalizedModel{
		APIKey:  "test-novita-key",
		BaseURL: ts.URL,
		Model:   "deepseek/deepseek-r1",
	}

	res, err := mod.Execute(context.Background(), cfg, kernel.ExecutionRequest{
		Messages: []kernel.ChatMessage{
			{Role: "user", Content: "Hello Novita"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Text != "Novita DeepSeek response" {
		t.Fatalf("expected 'Novita DeepSeek response', got %q", res.Text)
	}
}
