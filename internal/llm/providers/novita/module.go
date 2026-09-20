// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package novita

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/llm/byok"
	"github.com/scandrix/backend/internal/llm/providers/kernel"
	"github.com/scandrix/backend/internal/usecases/settings"
)

// Module implements the Novita AI provider module for ScanDrix.
type Module struct {
	httpClient *http.Client
}

type Option func(*Module)

func WithHTTPClient(client *http.Client) Option {
	return func(m *Module) {
		m.httpClient = client
	}
}

func New(opts ...Option) *Module {
	m := &Module{}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

func (m *Module) ID() string {
	return "novita"
}

func (m *Module) Aliases() []string {
	return []string{}
}

func (m *Module) Label() string {
	return "Novita"
}

func (m *Module) Doc() string {
	return "https://novita.ai/docs/guides/introduction"
}

func (m *Module) Capabilities(model string) kernel.ModelCapabilities {
	mLower := strings.ToLower(model)
	isReasoner := strings.Contains(mLower, "r1") || strings.Contains(mLower, "thinking") || strings.Contains(mLower, "reasoner")

	return kernel.ModelCapabilities{
		MaxInputTokens:      128000,
		StructuredOutput:    "json_object",
		ToolCalling:         "native",
		SupportsStreaming:   true,
		PromptCaching:       false,
		SupportsReasoning:   isReasoner,
		SupportsTemperature: true,
	}
}

func (m *Module) ReasoningTraits(cfg byok.NormalizedModel) kernel.ModelReasoningTraits {
	mLower := strings.ToLower(cfg.Model)
	if strings.Contains(mLower, "r1") || strings.Contains(mLower, "thinking") {
		return kernel.ModelReasoningTraits{
			ThinksByDefault:                true,
			CanDisableThinking:             false,
			ForcedToolChoiceSupported:      true,
			RejectsThinkingWhenToolsForced: false,
			BudgetMode:                     "fixed",
		}
	}
	return kernel.ModelReasoningTraits{
		ThinksByDefault: false,
	}
}

func (m *Module) TemperaturePolicy(cfg byok.NormalizedModel) *kernel.TemperaturePolicy {
	return &kernel.TemperaturePolicy{
		Mode: "free",
	}
}

func (m *Module) SystemCacheControl(cfg byok.NormalizedModel) map[string]any {
	return nil
}

func (m *Module) UIFields() []kernel.FieldDescriptor {
	return []kernel.FieldDescriptor{
		{Key: "apiKey", Label: "API Key", Type: "password", Required: true, Scope: "top"},
		{Key: "baseURL", Label: "Base URL", Type: "url", Required: false, Scope: "top", Placeholder: "https://api.novita.ai/v3/openai"},
	}
}

func (m *Module) ModelListing(providerID string) *kernel.ModelListing {
	if providerID == "novita" {
		return &kernel.ModelListing{
			Kind:      kernel.ListingHTTP,
			APIKeyEnv: "API_NOVITA_AI_API_KEY",
			URL: func(creds kernel.ResolvedListingCreds) string {
				return "https://api.novita.ai/v3/openai/models"
			},
			Headers: func(creds kernel.ResolvedListingCreds) map[string]string {
				return kernel.BearerHeaders(creds.APIKey)
			},
			Parse: kernel.ParseOpenAIIDs,
		}
	}
	return nil
}

func (m *Module) Execute(ctx context.Context, cfg byok.NormalizedModel, req kernel.ExecutionRequest) (*kernel.ExecutionResult, error) {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.novita.ai/v3/openai"
	}
	endpointURL := baseURL + "/chat/completions"

	messages := make([]map[string]any, 0, len(req.Messages))
	for _, msg := range req.Messages {
		wireMsg := map[string]any{
			"role":    msg.Role,
			"content": msg.Content,
		}
		if msg.Name != "" {
			wireMsg["name"] = msg.Name
		}
		if msg.ToolCallID != "" {
			wireMsg["tool_call_id"] = msg.ToolCallID
		}
		if len(msg.ToolCalls) > 0 {
			var tcList []map[string]any
			for _, tc := range msg.ToolCalls {
				tcList = append(tcList, map[string]any{
					"id":   tc.ID,
					"type": "function",
					"function": map[string]any{
						"name":      tc.Name,
						"arguments": tc.Arguments,
					},
				})
			}
			wireMsg["tool_calls"] = tcList
		}
		messages = append(messages, wireMsg)
	}

	payload := map[string]any{
		"model":    cfg.Model,
		"messages": messages,
	}

	if req.MaxTokens > 0 {
		payload["max_tokens"] = req.MaxTokens
	}
	if req.Temperature != nil {
		payload["temperature"] = req.Temperature
	}
	if req.ResponseSchema != nil {
		payload["response_format"] = map[string]any{
			"type": "json_object",
		}
	}

	if len(req.Tools) > 0 {
		var toolsList []map[string]any
		for _, t := range req.Tools {
			toolsList = append(toolsList, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        t.Name,
					"description": t.Description,
					"parameters":  t.Parameters,
				},
			})
		}
		payload["tools"] = toolsList
		if req.ToolChoice != nil {
			payload["tool_choice"] = req.ToolChoice
		}
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal novita request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	client := m.httpClient
	if client == nil {
		client = settings.NewSafeHTTPClient(90 * time.Second)
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("novita request error: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read novita response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("novita api returned error status %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Role      string `json:"role"`
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("failed to decode novita response: %w", err)
	}

	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("novita returned empty choices")
	}

	choice := parsed.Choices[0].Message
	result := &kernel.ExecutionResult{
		Text: choice.Content,
		Raw:  parsed,
		Usage: kernel.TokenUsage{
			InputTokens:  parsed.Usage.PromptTokens,
			OutputTokens: parsed.Usage.CompletionTokens,
			TotalTokens:  parsed.Usage.TotalTokens,
		},
	}

	for _, tc := range choice.ToolCalls {
		result.ToolCalls = append(result.ToolCalls, kernel.ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}

	return result, nil
}
