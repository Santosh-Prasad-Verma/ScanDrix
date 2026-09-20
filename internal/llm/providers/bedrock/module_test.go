// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package bedrock

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scandrix/backend/internal/llm/byok"
	"github.com/scandrix/backend/internal/llm/providers/kernel"
)

func TestBedrockModuleExecution(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := bedrockConverseResponseWire{}
		resp.Output.Message.Role = "assistant"
		resp.Output.Message.Content = []bedrockContentWire{
			{Text: "Bedrock synthesis for ScanDrix"},
		}
		resp.Usage.InputTokens = 18
		resp.Usage.OutputTokens = 9
		resp.Usage.TotalTokens = 27

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	mod := New(WithHTTPClient(server.Client()))
	cfg := byok.NormalizedModel{
		Provider: "bedrock",
		Model:    "anthropic.claude-3-5-sonnet-20241022-v2:0",
		APIKey:   "aws-bearer-token",
		BaseURL:  server.URL,
	}

	req := kernel.ExecutionRequest{
		Messages: []kernel.ChatMessage{
			{Role: "user", Content: "review diff"},
		},
	}

	res, err := mod.Execute(context.Background(), cfg, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Text != "Bedrock synthesis for ScanDrix" {
		t.Fatalf("expected 'Bedrock synthesis for ScanDrix', got %s", res.Text)
	}

	if res.Usage.TotalTokens != 27 {
		t.Fatalf("expected 27 total tokens, got %d", res.Usage.TotalTokens)
	}
}
