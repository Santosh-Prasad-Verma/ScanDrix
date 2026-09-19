// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scandrix/backend/internal/llm/byok"
	"github.com/scandrix/backend/internal/llm/providers/kernel"
)

func TestGeminiModuleExecution(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-goog-api-key") != "test-gemini-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		resp := geminiGenerateResponseWire{
			Candidates: []struct {
				Content struct {
					Parts []geminiPartWire `json:"parts"`
					Role  string           `json:"role"`
				} `json:"content"`
				FinishReason string `json:"finishReason"`
			}{
				{
					Content: struct {
						Parts []geminiPartWire `json:"parts"`
						Role  string           `json:"role"`
					}{
						Parts: []geminiPartWire{
							{Text: "Gemini review from ScanDrix"},
						},
						Role: "model",
					},
					FinishReason: "STOP",
				},
			},
		}
		resp.UsageMetadata.PromptTokenCount = 12
		resp.UsageMetadata.CandidatesTokenCount = 7
		resp.UsageMetadata.TotalTokenCount = 19

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	mod := New(WithHTTPClient(server.Client()))
	cfg := byok.NormalizedModel{
		Provider: "google-gemini",
		Model:    "gemini-2.5-pro",
		APIKey:   "test-gemini-key",
		BaseURL:  server.URL,
	}

	req := kernel.ExecutionRequest{
		Messages: []kernel.ChatMessage{
			{Role: "user", Content: "review this code"},
		},
	}

	res, err := mod.Execute(context.Background(), cfg, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Text != "Gemini review from ScanDrix" {
		t.Fatalf("expected 'Gemini review from ScanDrix', got %s", res.Text)
	}

	if res.Usage.TotalTokens != 19 {
		t.Fatalf("expected 19 total tokens, got %d", res.Usage.TotalTokens)
	}
}
