// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package azure

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scandrix/backend/internal/llm/byok"
	"github.com/scandrix/backend/internal/llm/providers/kernel"
)

func TestAzureModuleExecution(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("api-key") != "test-azure-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"choices": [{"message": {"role": "assistant", "content": "Azure response"}}],
			"usage": {"prompt_tokens": 20, "completion_tokens": 15, "total_tokens": 35}
		}`))
	}))
	defer ts.Close()

	client := ts.Client()
	mod := New(WithHTTPClient(client))
	if mod.ID() != "azure" {
		t.Fatalf("expected ID azure, got %s", mod.ID())
	}

	listing := mod.ModelListing("azure")
	if listing == nil || listing.Kind != kernel.ListingManual {
		t.Fatalf("expected manual model listing for azure")
	}

	cfg := byok.NormalizedModel{
		APIKey:  "test-azure-key",
		BaseURL: ts.URL,
		Model:   "gpt-4o-deployment",
	}

	res, err := mod.Execute(context.Background(), cfg, kernel.ExecutionRequest{
		Messages: []kernel.ChatMessage{
			{Role: "user", Content: "Hello Azure"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Text != "Azure response" {
		t.Fatalf("expected 'Azure response', got %q", res.Text)
	}
	if res.Usage.TotalTokens != 35 {
		t.Fatalf("expected 35 tokens, got %d", res.Usage.TotalTokens)
	}
}
