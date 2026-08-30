package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/llm/orchestrator"
)

// Client implements native inference against Ollama and vLLM self-hosted backends.
type Client struct {
	ollamaEndpoint string
	vllmEndpoint   string
	httpClient     *http.Client
}

// NewClient creates an Ollama / vLLM local inference connector.
func NewClient(ollamaEndpoint, vllmEndpoint string) *Client {
	if ollamaEndpoint == "" {
		ollamaEndpoint = "http://localhost:11434"
	}
	if vllmEndpoint == "" {
		vllmEndpoint = "http://localhost:8000"
	}
	return &Client{
		ollamaEndpoint: strings.TrimRight(ollamaEndpoint, "/"),
		vllmEndpoint:   strings.TrimRight(vllmEndpoint, "/"),
		httpClient: &http.Client{
			Timeout: 180 * time.Second,
		},
	}
}

func (c *Client) Complete(ctx context.Context, req orchestrator.InferenceRequest, model string) (*orchestrator.InferenceResponse, error) {
	if strings.HasPrefix(model, "vllm") || strings.Contains(c.vllmEndpoint, "8000") && strings.HasPrefix(model, "qwen") {
		return c.completeVLLM(ctx, req, model)
	}
	return c.completeOllama(ctx, req, model)
}

func (c *Client) completeOllama(ctx context.Context, req orchestrator.InferenceRequest, model string) (*orchestrator.InferenceResponse, error) {
	modelName := strings.TrimPrefix(model, "ollama-")
	if modelName == "" {
		modelName = "deepseek-r1:70b"
	}

	var messages []map[string]string
	for _, m := range req.Messages {
		messages = append(messages, map[string]string{
			"role":    string(m.Role),
			"content": m.Content,
		})
	}

	payload := map[string]any{
		"model":    modelName,
		"messages": messages,
		"format":   "json",
		"stream":   false,
		"options": map[string]any{
			"temperature": 0.2,
		},
	}
	if req.Temperature > 0 {
		payload["options"].(map[string]any)["temperature"] = req.Temperature
	}
	if req.MaxTokens > 0 {
		payload["options"].(map[string]any)["num_predict"] = req.MaxTokens
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/api/chat", c.ollamaEndpoint)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var data struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		PromptEvalCount int `json:"prompt_eval_count"`
		EvalCount       int `json:"eval_count"`
	}

	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("failed parsing ollama response: %w", err)
	}

	return &orchestrator.InferenceResponse{
		Content:          data.Message.Content,
		ModelUsed:        model,
		ProviderUsed:     orchestrator.ProviderOllama,
		PromptTokens:     data.PromptEvalCount,
		CompletionTokens: data.EvalCount,
		TotalTokens:      data.PromptEvalCount + data.EvalCount,
	}, nil
}

func (c *Client) completeVLLM(ctx context.Context, req orchestrator.InferenceRequest, model string) (*orchestrator.InferenceResponse, error) {
	modelName := strings.TrimPrefix(model, "vllm-")
	if modelName == "" {
		modelName = "Qwen/Qwen2.5-Coder-32B-Instruct"
	}

	var messages []map[string]string
	for _, m := range req.Messages {
		messages = append(messages, map[string]string{
			"role":    string(m.Role),
			"content": m.Content,
		})
	}

	payload := map[string]any{
		"model":           modelName,
		"messages":        messages,
		"response_format": map[string]string{"type": "json_object"},
		"stream":          false,
	}
	if req.MaxTokens > 0 {
		payload["max_tokens"] = req.MaxTokens
	}
	if req.Temperature > 0 {
		payload["temperature"] = req.Temperature
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/v1/chat/completions", c.vllmEndpoint)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if req.TenantAPIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+req.TenantAPIKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("vllm request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vllm error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var data struct {
		Choices []struct {
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("failed parsing vllm response: %w", err)
	}

	if len(data.Choices) == 0 {
		return nil, fmt.Errorf("vllm returned no completion choices")
	}

	return &orchestrator.InferenceResponse{
		Content:          data.Choices[0].Message.Content,
		ModelUsed:        model,
		ProviderUsed:     orchestrator.ProviderVLLM,
		PromptTokens:     data.Usage.PromptTokens,
		CompletionTokens: data.Usage.CompletionTokens,
		TotalTokens:      data.Usage.TotalTokens,
	}, nil
}
