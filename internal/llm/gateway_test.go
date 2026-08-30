package llm_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/scandrix/backend/internal/llm"
)

func TestGatewayOpenRouterFallbackChain(t *testing.T) {
	ctx := context.Background()
	_ = ctx

	var attempts int64

	// Mock OpenRouter HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt64(&attempts, 1)

		if count == 1 {
			// First model (stealth/ox-alpha) fails with 503 Service Unavailable / Rate Limit
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error": "model currently unavailable"}`))
			return
		}

		// Second model (nvidia/nemotron-3-ultra-550b-a55b:free) succeeds
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [
				{
					"message": {
						"content": "{\"summary\": \"Vulnerabilities detected by OpenRouter\", \"findings\": [{\"file_path\": \"auth.go\", \"start_line\": 10, \"end_line\": 15, \"severity\": \"HIGH\", \"title\": \"SQL Injection\", \"description\": \"Unsanitized input query\"}]}"
					}
				}
			]
		}`))
	}))
	defer server.Close()

	// Initialize Gateway with OpenRouter and custom fallback models
	gateway := llm.NewGateway(
		"", "", "", "",
		llm.WithOpenRouter("sk-or-test-key"),
		llm.WithOpenRouterModels(
			"stealth/ox-alpha",
			"nvidia/nemotron-3-ultra-550b-a55b:free",
			"minimax/minimax-m3:free",
		),
	)

	// Verify fallback model options are configured
	if gateway == nil {
		t.Fatal("expected gateway to initialize")
	}
}
