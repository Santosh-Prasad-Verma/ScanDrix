// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package vertex

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scandrix/backend/internal/llm/byok"
	"github.com/scandrix/backend/internal/llm/providers/kernel"
)

func TestVertexModuleExecution(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := vertexResponseWire{}
		resp.Candidates = []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
				Role string `json:"role"`
			} `json:"content"`
		}{
			{
				Content: struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
					Role string `json:"role"`
				}{
					Parts: []struct {
						Text string `json:"text"`
					}{
						{Text: "Vertex AI analysis for ScanDrix"},
					},
					Role: "model",
				},
			},
		}
		resp.UsageMetadata.PromptTokenCount = 20
		resp.UsageMetadata.CandidatesTokenCount = 10
		resp.UsageMetadata.TotalTokenCount = 30

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	mod := New(WithHTTPClient(server.Client()))
	cfg := byok.NormalizedModel{
		Provider: "vertex",
		Model:    "gemini-2.5-pro",
		APIKey:   "vertex-token",
		BaseURL:  server.URL,
	}

	req := kernel.ExecutionRequest{
		Messages: []kernel.ChatMessage{
			{Role: "user", Content: "vertex test"},
		},
	}

	res, err := mod.Execute(context.Background(), cfg, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Text != "Vertex AI analysis for ScanDrix" {
		t.Fatalf("expected 'Vertex AI analysis for ScanDrix', got %s", res.Text)
	}

	if res.Usage.TotalTokens != 30 {
		t.Fatalf("expected 30 total tokens, got %d", res.Usage.TotalTokens)
	}
}
