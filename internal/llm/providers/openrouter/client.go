package openrouter

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

// Client implements native inference against OpenRouter API for Nemotron & other models.
type Client struct {
	baseURL    string
	apiKey     string
	siteURL    string
	siteName   string
	httpClient *http.Client
}

// NewClient creates a new OpenRouter connector.
func NewClient(apiKey string, baseURL ...string) *Client {
	endpoint := "https://openrouter.ai/api/v1"
	if len(baseURL) > 0 && baseURL[0] != "" {
		endpoint = baseURL[0]
	}
	return &Client{
		baseURL:  strings.TrimRight(endpoint, "/"),
		apiKey:   apiKey,
		siteURL:  "https://scandrix.dev",
		siteName: "ScanDrix Autonomous Code Review",
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
		return nil, fmt.Errorf("openrouter api key is required")
	}

	modelName := model
	if modelName == "" {
		modelName = "nvidia/nemotron-3-ultra-550b-a55b:free"
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

	url := fmt.Sprintf("%s/chat/completions", c.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("HTTP-Referer", c.siteURL)
	httpReq.Header.Set("X-Title", c.siteName)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openrouter request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openrouter error (HTTP %d): %s", resp.StatusCode, string(body))
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
		return nil, fmt.Errorf("failed parsing openrouter response: %w", err)
	}

	if len(data.Choices) == 0 {
		return nil, fmt.Errorf("openrouter returned no completion choices")
	}

	return &orchestrator.InferenceResponse{
		Content:          data.Choices[0].Message.Content,
		ModelUsed:        modelName,
		ProviderUsed:     orchestrator.ProviderOpenRouter,
		PromptTokens:     data.Usage.PromptTokens,
		CompletionTokens: data.Usage.CompletionTokens,
		TotalTokens:      data.Usage.TotalTokens,
	}, nil
}
