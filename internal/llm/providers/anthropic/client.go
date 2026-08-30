package anthropic

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

// Client implements native inference against the Anthropic Messages API.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a new Anthropic provider connector.
func NewClient(apiKey string, baseURL ...string) *Client {
	endpoint := "https://api.anthropic.com/v1"
	if len(baseURL) > 0 && baseURL[0] != "" {
		endpoint = baseURL[0]
	}
	return &Client{
		baseURL: strings.TrimRight(endpoint, "/"),
		apiKey:  apiKey,
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
		return nil, fmt.Errorf("anthropic api key is required")
	}

	modelName := model
	if modelName == "" {
		modelName = "claude-sonnet-5"
	}

	// Separate system prompt from conversational user/assistant messages
	var systemPrompt string
	var messages []map[string]any

	for _, m := range req.Messages {
		if m.Role == orchestrator.RoleSystem {
			if systemPrompt != "" {
				systemPrompt += "\n\n"
			}
			systemPrompt += m.Content
		} else {
			role := "user"
			if m.Role == orchestrator.RoleAssistant {
				role = "assistant"
			}
			messages = append(messages, map[string]any{
				"role":    role,
				"content": m.Content,
			})
		}
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 8192
	}

	payload := map[string]any{
		"model":      modelName,
		"messages":   messages,
		"max_tokens": maxTokens,
	}

	if systemPrompt != "" {
		payload["system"] = systemPrompt
	}

	// Thinking / Extended reasoning configuration
	if req.EnableReasoning {
		thinkingBudget := 4096
		if maxTokens > 8192 {
			thinkingBudget = 8192
		}
		payload["thinking"] = map[string]any{
			"type":          "enabled",
			"budget_tokens": thinkingBudget,
		}
		// When thinking is enabled in Anthropic, temperature must not be set or must be 1.0
	} else if req.Temperature > 0 {
		payload["temperature"] = req.Temperature
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/messages", c.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("x-api-key", apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var data struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("failed parsing anthropic response: %w", err)
	}

	var contentBuilder strings.Builder
	for _, block := range data.Content {
		if block.Type == "text" {
			contentBuilder.WriteString(block.Text)
		}
	}

	return &orchestrator.InferenceResponse{
		Content:          contentBuilder.String(),
		ModelUsed:        modelName,
		ProviderUsed:     orchestrator.ProviderAnthropic,
		PromptTokens:     data.Usage.InputTokens,
		CompletionTokens: data.Usage.OutputTokens,
		TotalTokens:      data.Usage.InputTokens + data.Usage.OutputTokens,
	}, nil
}
