package deepseek

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

// Client implements native inference against DeepSeek API (deepseek-chat, deepseek-reasoner).
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a new DeepSeek connector.
func NewClient(apiKey string, baseURL ...string) *Client {
	endpoint := "https://api.deepseek.com"
	if len(baseURL) > 0 && baseURL[0] != "" {
		endpoint = baseURL[0]
	}
	return &Client{
		baseURL: strings.TrimRight(endpoint, "/"),
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (c *Client) Complete(ctx context.Context, req orchestrator.InferenceRequest, model string) (*orchestrator.InferenceResponse, error) {
	apiKey := c.apiKey
	if req.TenantAPIKey != "" {
		apiKey = req.TenantAPIKey
	}
	if apiKey == "" {
		return nil, fmt.Errorf("deepseek api key is required")
	}

	modelName := model
	if modelName == "" {
		modelName = "deepseek-chat"
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
		"stream":   false,
	}

	if req.MaxTokens > 0 {
		payload["max_tokens"] = req.MaxTokens
	}
	if req.Temperature > 0 {
		payload["temperature"] = req.Temperature
	}
	// If reasoning is requested or model is deepseek-reasoner, it provides reasoning_content
	if modelName == "deepseek-chat" && !req.EnableReasoning {
		payload["response_format"] = map[string]string{"type": "json_object"}
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/chat/completions", c.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("deepseek request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("deepseek error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var data struct {
		Choices []struct {
			Message struct {
				Role             string `json:"role"`
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content,omitempty"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens        int `json:"prompt_tokens"`
			CompletionTokens    int `json:"completion_tokens"`
			TotalTokens         int `json:"total_tokens"`
			PromptCacheHitTokens int `json:"prompt_cache_hit_tokens,omitempty"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("failed parsing deepseek response: %w", err)
	}

	if len(data.Choices) == 0 {
		return nil, fmt.Errorf("deepseek returned no completion choices")
	}

	content := data.Choices[0].Message.Content
	return &orchestrator.InferenceResponse{
		Content:          content,
		ModelUsed:        modelName,
		ProviderUsed:     orchestrator.ProviderDeepSeek,
		PromptTokens:     data.Usage.PromptTokens,
		CompletionTokens: data.Usage.CompletionTokens,
		TotalTokens:      data.Usage.TotalTokens,
	}, nil
}
