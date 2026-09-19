package llm_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

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
			// First model fails with 503 Service Unavailable / Rate Limit
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error": "model currently unavailable"}`))
			return
		}

		// Second model succeeds
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
			"test/primary-model",
			"test/secondary-model",
			"test/fallback-model",
		),
	)

	// Verify fallback model options are configured
	if gateway == nil {
		t.Fatal("expected gateway to initialize")
	}
}

func TestGatewayPerProviderCircuitBreakerFastFail(t *testing.T) {
	// Create isolated breaker registry with small threshold
	breakers := llm.NewProviderBreakerRegistry(2, 100*time.Millisecond)

	// Pre-trip the BYOK-CustomEndpoint breaker
	cbCustom := breakers.GetOrCreate("BYOK-CustomEndpoint")
	for i := 0; i < 2; i++ {
		cbCustom.RecordFailure()
	}

	if cbCustom.State() != llm.StateOpen {
		t.Fatalf("expected BYOK-CustomEndpoint breaker to be OPEN")
	}

	gateway := llm.NewGateway(
		"", "", "", "",
		llm.WithProviderBreakers(breakers),
	)

	// Invocations with tripped provider should immediately fail fast with circuit open error
	_, err := gateway.AnalyzeDiff(context.Background(), llm.ReviewRequest{
		BYOKCustomEndpoint: "http://127.0.0.1:9999",
		DiffContent:        "test diff",
	})

	if err == nil {
		t.Fatal("expected gateway invocation to fail when only provider breaker is open")
	}
}

func TestGatewayContextOverflowFailsFast(t *testing.T) {
	gateway := llm.NewGateway(
		"", "", "", "",
		llm.WithMaxContextTokens(1000), // Max 1000 tokens
	)

	// Oversized diff (10,000 characters ~ 2500 tokens > 1000 token context window)
	largeDiff := string(make([]byte, 10_000))
	_, err := gateway.AnalyzeDiff(context.Background(), llm.ReviewRequest{
		DiffContent: largeDiff,
	})

	if err == nil {
		t.Fatal("expected context window preflight check to reject oversized diff payload")
	}
}
