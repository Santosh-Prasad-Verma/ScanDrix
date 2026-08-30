package bedrock

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

// Client implements native inference against AWS Bedrock Runtime Converse API.
type Client struct {
	region         string
	bearerToken    string
	customEndpoint string
	httpClient     *http.Client
}

// NewClient creates a new AWS Bedrock connector.
func NewClient(region, bearerToken string, customEndpoint ...string) *Client {
	if region == "" {
		region = "us-east-1"
	}
	endpoint := ""
	if len(customEndpoint) > 0 {
		endpoint = strings.TrimRight(customEndpoint[0], "/")
	}
	return &Client{
		region:         region,
		bearerToken:    bearerToken,
		customEndpoint: endpoint,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (c *Client) Complete(ctx context.Context, req orchestrator.InferenceRequest, model string) (*orchestrator.InferenceResponse, error) {
	token := c.bearerToken
	if req.TenantAPIKey != "" {
		token = req.TenantAPIKey
	}
	if token == "" {
		return nil, fmt.Errorf("aws bedrock authentication token is required")
	}

	modelID := model
	if modelID == "" {
		modelID = "anthropic.claude-3-5-sonnet-20241022-v2:0"
	}
	if modelID == "bedrock-claude-3-7-sonnet" {
		modelID = "anthropic.claude-3-7-sonnet-20250219-v1:0"
	} else if modelID == "bedrock-nova-pro" {
		modelID = "amazon.nova-pro-v1:0"
	} else if modelID == "bedrock-llama-3-70b" {
		modelID = "meta.llama3-70b-instruct-v1:0"
	}

	type bedrockContent struct {
		Text string `json:"text"`
	}
	type bedrockMessage struct {
		Role    string           `json:"role"`
		Content []bedrockContent `json:"content"`
	}

	var messages []bedrockMessage
	var systemPrompts []bedrockContent

	for _, m := range req.Messages {
		if m.Role == orchestrator.RoleSystem {
			systemPrompts = append(systemPrompts, bedrockContent{Text: m.Content})
		} else {
			messages = append(messages, bedrockMessage{
				Role:    string(m.Role),
				Content: []bedrockContent{{Text: m.Content}},
			})
		}
	}

	if len(messages) == 0 {
		messages = append(messages, bedrockMessage{
			Role:    "user",
			Content: []bedrockContent{{Text: "Analyze the provided code diff."}},
		})
	}

	payload := map[string]any{
		"messages": messages,
		"inferenceConfig": map[string]any{
			"maxTokens":   4096,
			"temperature": 0.2,
		},
	}

	if len(systemPrompts) > 0 {
		payload["system"] = systemPrompts
	}
	if req.MaxTokens > 0 {
		payload["inferenceConfig"].(map[string]any)["maxTokens"] = req.MaxTokens
	}
	if req.Temperature > 0 {
		payload["inferenceConfig"].(map[string]any)["temperature"] = req.Temperature
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://bedrock-runtime.%s.amazonaws.com/model/%s/converse", c.region, modelID)
	if c.customEndpoint != "" {
		url = fmt.Sprintf("%s/model/%s/converse", c.customEndpoint, modelID)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("aws bedrock request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("aws bedrock error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var data struct {
		Output struct {
			Message struct {
				Role    string `json:"role"`
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"message"`
		} `json:"output"`
		Usage struct {
			InputTokens  int `json:"inputTokens"`
			OutputTokens int `json:"outputTokens"`
			TotalTokens  int `json:"totalTokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("failed parsing bedrock response: %w", err)
	}

	if len(data.Output.Message.Content) == 0 {
		return nil, fmt.Errorf("bedrock returned empty response content")
	}

	content := data.Output.Message.Content[0].Text
	return &orchestrator.InferenceResponse{
		Content:          content,
		ModelUsed:        modelID,
		ProviderUsed:     orchestrator.ProviderBedrock,
		PromptTokens:     data.Usage.InputTokens,
		CompletionTokens: data.Usage.OutputTokens,
		TotalTokens:      data.Usage.TotalTokens,
	}, nil
}
