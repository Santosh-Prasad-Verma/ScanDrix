package openai

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

// Client implements native inference against OpenAI and OpenAI-compatible API endpoints
// (supporting OpenAI GPT-4o/5.x, Moonshot Kimi, Alibaba Qwen, MiniMax, xAI Grok, and Mistral).
type Client struct {
	baseURL      string
	apiKey       string
	providerType orchestrator.LLMProviderType
	httpClient   *http.Client
}

// NewClient creates a new OpenAI connector.
func NewClient(apiKey string, baseURL ...string) *Client {
	endpoint := "https://api.openai.com/v1"
	if len(baseURL) > 0 && baseURL[0] != "" {
		endpoint = baseURL[0]
	}
	return &Client{
		baseURL:      strings.TrimRight(endpoint, "/"),
		apiKey:       apiKey,
		providerType: orchestrator.ProviderOpenAI,
		httpClient: &http.Client{
			Timeout: 180 * time.Second,
		},
	}
}

// NewCompatibleClient creates an OpenAI-compatible connector for direct frontier APIs.
func NewCompatibleClient(provider orchestrator.LLMProviderType, apiKey, baseURL string) *Client {
	return &Client{
		baseURL:      strings.TrimRight(baseURL, "/"),
		apiKey:       apiKey,
		providerType: provider,
		httpClient: &http.Client{
			Timeout: 180 * time.Second,
		},
	}
}

func (c *Client) Complete(ctx context.Context, req orchestrator.InferenceRequest, model string) (*orchestrator.InferenceResponse, error) {
	apiKey := c.apiKey
	if req.TenantAPIKey != "" {
		apiKey = req.TenantAPIKey
	}
	if apiKey == "" {
		return nil, fmt.Errorf("%s api key is required", c.providerType)
	}

	modelName := model
	if modelName == "" {
		modelName = "gpt-5.6-terra"
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

	// Flagship models with reasoning tokens (o1, o3, o4, o5, gpt-5.x)
	isReasoningModel := strings.HasPrefix(modelName, "o1") || strings.HasPrefix(modelName, "o3") ||
		strings.HasPrefix(modelName, "o4") || strings.HasPrefix(modelName, "o5") ||
		strings.Contains(modelName, "sol") || strings.Contains(modelName, "terra")

	if req.MaxTokens > 0 {
		if isReasoningModel {
			payload["max_completion_tokens"] = req.MaxTokens
		} else {
			payload["max_tokens"] = req.MaxTokens
		}
	}

	// Reasoning effort / temperature
	if isReasoningModel {
		if req.EnableReasoning {
			payload["reasoning_effort"] = "high"
		}
	} else if req.Temperature > 0 {
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
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%s request failed: %w", c.providerType, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s error (HTTP %d): %s", c.providerType, resp.StatusCode, string(body))
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
		return nil, fmt.Errorf("failed parsing %s response: %w", c.providerType, err)
	}

	if len(data.Choices) == 0 {
		return nil, fmt.Errorf("%s returned no completion choices", c.providerType)
	}

	return &orchestrator.InferenceResponse{
		Content:          data.Choices[0].Message.Content,
		ModelUsed:        modelName,
		ProviderUsed:     c.providerType,
		PromptTokens:     data.Usage.PromptTokens,
		CompletionTokens: data.Usage.CompletionTokens,
		TotalTokens:      data.Usage.TotalTokens,
	}, nil
}
