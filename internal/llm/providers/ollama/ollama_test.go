package ollama_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/llm/orchestrator"
	"github.com/scandrix/backend/internal/llm/providers/ollama"
)

func TestOllamaAndVLLMEndToEnd(t *testing.T) {
	ctx := context.Background()

	// 1. Mock Ollama Server
	ollamaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"model": "deepseek-r1:70b",
			"message": {
				"role": "assistant",
				"content": "{\"summary\": \"Ollama Review Passed\", \"findings\": []}"
			},
			"prompt_eval_count": 180,
			"eval_count": 60,
			"total_duration": 500000000
		}`))
	}))
	defer ollamaServer.Close()

	// 2. Mock vLLM Server
	vllmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "cmpl-vllm-456",
			"choices": [
				{
					"message": {
						"role": "assistant",
						"content": "{\"summary\": \"vLLM Fast Inference OK\", \"findings\": []}"
					}
				}
			],
			"usage": {
				"prompt_tokens": 220,
				"completion_tokens": 75,
				"total_tokens": 295
			}
		}`))
	}))
	defer vllmServer.Close()

	client := ollama.NewClient(ollamaServer.URL, vllmServer.URL)

	// Test Ollama invocation
	ollamaReq := orchestrator.InferenceRequest{
		WorkspaceID: uuid.New(),
		Messages: []orchestrator.ChatMessage{
			{Role: orchestrator.RoleSystem, Content: "You are a code reviewer."},
			{Role: orchestrator.RoleUser, Content: "Review diff on-prem."},
		},
		PreferredModel: "ollama-deepseek-r1-70b",
	}

	ollamaResp, err := client.Complete(ctx, ollamaReq, "ollama-deepseek-r1-70b")
	if err != nil {
		t.Fatalf("unexpected ollama error: %v", err)
	}

	if ollamaResp.ProviderUsed != orchestrator.ProviderOllama || ollamaResp.PromptTokens != 180 || ollamaResp.CompletionTokens != 60 {
		t.Fatalf("unexpected ollama metrics: %+v", ollamaResp)
	}

	// Test vLLM invocation
	vllmReq := orchestrator.InferenceRequest{
		WorkspaceID: uuid.New(),
		Messages: []orchestrator.ChatMessage{
			{Role: orchestrator.RoleUser, Content: "Analyze diff on vllm cluster."},
		},
		PreferredModel: "vllm-qwen-2.5-coder",
	}

	vllmResp, err := client.Complete(ctx, vllmReq, "vllm-qwen-2.5-coder")
	if err != nil {
		t.Fatalf("unexpected vllm error: %v", err)
	}

	if vllmResp.ProviderUsed != orchestrator.ProviderVLLM || vllmResp.PromptTokens != 220 || vllmResp.TotalTokens != 295 {
		t.Fatalf("unexpected vllm metrics: %+v", vllmResp)
	}
}
