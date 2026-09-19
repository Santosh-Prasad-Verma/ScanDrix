// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

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

	"github.com/scandrix/backend/internal/llm/byok"
	"github.com/scandrix/backend/internal/llm/providers/kernel"
	"github.com/scandrix/backend/internal/llm/structured"
	"github.com/scandrix/backend/internal/usecases/settings"
)

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
	return "open_router"
}

func (m *Module) Aliases() []string {
	return []string{"openrouter"}
}

func (m *Module) Label() string {
	return "OpenRouter"
}

func (m *Module) Doc() string {
	return "https://openrouter.ai/docs"
}

func (m *Module) Capabilities(model string) kernel.ModelCapabilities {
	structOut := "json_object"
	if structured.OpenRouterHonorsJSONSchema(model) {
		structOut = "json_schema"
	}
	return kernel.ModelCapabilities{
		MaxInputTokens:      128000,
		StructuredOutput:    structOut,
		ToolCalling:         "native",
		SupportsStreaming:   true,
		PromptCaching:       true,
		SupportsReasoning:   true,
		SupportsTemperature: true,
	}
}

func (m *Module) ReasoningTraits(cfg byok.NormalizedModel) kernel.ModelReasoningTraits {
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
	}
}

func (m *Module) ModelListing(providerID string) *kernel.ModelListing {
	if providerID == "open_router" || providerID == "openrouter" {
		return &kernel.ModelListing{
			Kind:      kernel.ListingHTTP,
			APIKeyEnv: "API_OPEN_ROUTER_API_KEY",
			URL: func(creds kernel.ResolvedListingCreds) string {
				return "https://openrouter.ai/api/v1/models"
			},
			Headers: func(creds kernel.ResolvedListingCreds) map[string]string {
				return kernel.BearerHeaders(creds.APIKey)
			},
			Parse: kernel.ParseOpenAIIDs,
		}
	}
	return nil
}

type openRouterChatRequest struct {
	Model          string                 `json:"model"`
	Messages       []map[string]any       `json:"messages"`
	Tools          []map[string]any       `json:"tools,omitempty"`
	ToolChoice     any                    `json:"tool_choice,omitempty"`
	ResponseFormat any                    `json:"response_format,omitempty"`
	Temperature    *float64               `json:"temperature,omitempty"`
	MaxTokens      int                    `json:"max_tokens,omitempty"`
	Transforms     []string               `json:"transforms,omitempty"`
	Route          string                 `json:"route,omitempty"`
	Provider       map[string]any         `json:"provider,omitempty"`
	Reasoning      map[string]any         `json:"reasoning,omitempty"`
}

type openRouterChatResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role      string `json:"role"`
			Content   *string `json:"content"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error,omitempty"`
}

func (m *Module) Execute(ctx context.Context, cfg byok.NormalizedModel, req kernel.ExecutionRequest) (*kernel.ExecutionResult, error) {
	wireReq := openRouterChatRequest{
		Model:     cfg.Model,
		MaxTokens: req.MaxTokens,
	}

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
			var tcs []map[string]any
			for _, tc := range msg.ToolCalls {
				tcs = append(tcs, map[string]any{
					"id":   tc.ID,
					"type": "function",
					"function": map[string]any{
						"name":      tc.Name,
						"arguments": tc.Arguments,
					},
				})
			}
			wireMsg["tool_calls"] = tcs
		}
		wireReq.Messages = append(wireReq.Messages, wireMsg)
	}

	for _, t := range req.Tools {
		wireReq.Tools = append(wireReq.Tools, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  t.Parameters,
			},
		})
	}

	if len(wireReq.Tools) > 0 && req.ToolChoice != nil {
		wireReq.ToolChoice = req.ToolChoice
	}

	if req.ResponseSchema != nil {
		if structured.OpenRouterHonorsJSONSchema(cfg.Model) {
			wireReq.ResponseFormat = map[string]any{
				"type": "json_schema",
				"json_schema": map[string]any{
					"name":   "scandrix_structured_response",
					"strict": true,
					"schema": structured.ToStrictWireSchema(req.ResponseSchema),
				},
			}
		} else {
			wireReq.ResponseFormat = map[string]any{
				"type": "json_object",
			}
		}
	}

	if len(cfg.OpenRouterProviderOrder) > 0 || cfg.OpenRouterAllowFallback != nil {
		prov := make(map[string]any)
		if len(cfg.OpenRouterProviderOrder) > 0 {
			prov["order"] = cfg.OpenRouterProviderOrder
		}
		if cfg.OpenRouterAllowFallback != nil {
			prov["allow_fallbacks"] = *cfg.OpenRouterAllowFallback
		}
		wireReq.Provider = prov
	}

	if req.ReasoningEffort != "" && req.ReasoningEffort != kernel.ReasoningNone {
		wireReq.Reasoning = map[string]any{
			"effort": string(req.ReasoningEffort),
		}
	}

	wireReq.Temperature = req.Temperature

	payloadBytes, err := json.Marshal(wireReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal openrouter request: %w", err)
	}

	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://openrouter.ai/api/v1"
	}
	endpointURL := baseURL + "/chat/completions"

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	httpReq.Header.Set("HTTP-Referer", "https://scandrix.dev")
	httpReq.Header.Set("X-Title", "ScanDrix AI")

	client := m.httpClient
	if client == nil {
		client = settings.NewSafeHTTPClient(90 * time.Second)
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openrouter request error: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read openrouter response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errResp openRouterChatResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != nil {
			return nil, fmt.Errorf("openrouter api error (status %d): %s", resp.StatusCode, errResp.Error.Message)
		}
		return nil, fmt.Errorf("openrouter api returned error status %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed openRouterChatResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("failed to decode openrouter response: %w", err)
	}

	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("openrouter returned zero choices")
	}

	choice := parsed.Choices[0]
	result := &kernel.ExecutionResult{
		Raw: parsed,
		Usage: kernel.TokenUsage{
			InputTokens:  parsed.Usage.PromptTokens,
			OutputTokens: parsed.Usage.CompletionTokens,
			TotalTokens:  parsed.Usage.TotalTokens,
		},
	}

	if choice.Message.Content != nil {
		result.Text = *choice.Message.Content
	}

	for _, tc := range choice.Message.ToolCalls {
		result.ToolCalls = append(result.ToolCalls, kernel.ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}

	return result, nil
}
