// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

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
	return "openai"
}

func (m *Module) Aliases() []string {
	return []string{"openai_compatible"}
}

func (m *Module) Label() string {
	return "OpenAI"
}

func (m *Module) Doc() string {
	return "https://platform.openai.com/docs"
}

func (m *Module) Capabilities(model string) kernel.ModelCapabilities {
	mLower := strings.ToLower(model)
	isReasoning := strings.HasPrefix(mLower, "o1") || strings.HasPrefix(mLower, "o3") || strings.HasPrefix(mLower, "o4")

	return kernel.ModelCapabilities{
		MaxInputTokens:      128000,
		StructuredOutput:    "json_schema",
		ToolCalling:         "native",
		SupportsStreaming:   true,
		PromptCaching:       true,
		SupportsReasoning:   isReasoning,
		SupportsTemperature: !isReasoning,
	}
}

func (m *Module) ReasoningTraits(cfg byok.NormalizedModel) kernel.ModelReasoningTraits {
	mLower := strings.ToLower(cfg.Model)
	if strings.HasPrefix(mLower, "o1") || strings.HasPrefix(mLower, "o3") || strings.HasPrefix(mLower, "o4") {
		return kernel.ModelReasoningTraits{
			ThinksByDefault:                true,
			CanDisableThinking:             false,
			ForcedToolChoiceSupported:      true,
			RejectsThinkingWhenToolsForced: false,
			BudgetMode:                     "effort_only",
		}
	}
	return kernel.ModelReasoningTraits{
		ThinksByDefault: false,
	}
}

func (m *Module) TemperaturePolicy(cfg byok.NormalizedModel) *kernel.TemperaturePolicy {
	mLower := strings.ToLower(cfg.Model)
	if strings.HasPrefix(mLower, "o1") || strings.HasPrefix(mLower, "o3") || strings.HasPrefix(mLower, "o4") {
		return &kernel.TemperaturePolicy{
			Mode: "unsupported",
		}
	}
	return &kernel.TemperaturePolicy{
		Mode: "free",
	}
}

func (m *Module) SystemCacheControl(cfg byok.NormalizedModel) map[string]any {
	// OpenAI caches prompt prefixes automatically without requiring special headers
	return nil
}

func (m *Module) UIFields() []kernel.FieldDescriptor {
	return []kernel.FieldDescriptor{
		{Key: "apiKey", Label: "API Key", Type: "password", Required: true, Scope: "top"},
		{Key: "baseURL", Label: "Base URL", Type: "url", Required: false, Scope: "top", Placeholder: "https://api.openai.com/v1"},
	}
}

func (m *Module) ModelListing(providerID string) *kernel.ModelListing {
	if providerID == "openai" {
		return &kernel.ModelListing{
			Kind:      kernel.ListingHTTP,
			APIKeyEnv: "API_OPEN_AI_API_KEY",
			URL: func(creds kernel.ResolvedListingCreds) string {
				return "https://api.openai.com/v1/models"
			},
			Headers: func(creds kernel.ResolvedListingCreds) map[string]string {
				return kernel.BearerHeaders(creds.APIKey)
			},
			Parse: kernel.ParseOpenAIIDs,
		}
	}
	if providerID == "openai_compatible" {
		return &kernel.ModelListing{
			Kind:            kernel.ListingHTTP,
			RequiresBaseURL: true,
			URL: func(creds kernel.ResolvedListingCreds) string {
				return kernel.OpenAICompatibleModelsURL(creds.BaseURL)
			},
			Headers: func(creds kernel.ResolvedListingCreds) map[string]string {
				return kernel.BearerHeaders(creds.APIKey)
			},
			Parse: kernel.ParseOpenAIIDs,
		}
	}
	return nil
}

type openAIMessage struct {
	Role       string               `json:"role"`
	Content    string               `json:"content"`
	Name       string               `json:"name,omitempty"`
	ToolCallID string               `json:"tool_call_id,omitempty"`
	ToolCalls  []openAIToolCallWire `json:"tool_calls,omitempty"`
}

type openAIToolCallWire struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Function openAIFunctionCallWire `json:"function"`
}

type openAIFunctionCallWire struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAIToolWire struct {
	Type     string             `json:"type"`
	Function openAIFunctionWire `json:"function"`
}

type openAIFunctionWire struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
	Strict      bool           `json:"strict,omitempty"`
}

type openAIResponseFormat struct {
	Type       string            `json:"type"`
	JSONSchema *openAIJSONSchema `json:"json_schema,omitempty"`
}

type openAIJSONSchema struct {
	Name   string         `json:"name"`
	Strict bool           `json:"strict"`
	Schema map[string]any `json:"schema"`
}

type openAIChatRequest struct {
	Model          string                `json:"model"`
	Messages       []openAIMessage       `json:"messages"`
	Tools          []openAIToolWire      `json:"tools,omitempty"`
	ToolChoice     any                   `json:"tool_choice,omitempty"`
	ResponseFormat *openAIResponseFormat `json:"response_format,omitempty"`
	Temperature    *float64              `json:"temperature,omitempty"`
	MaxTokens      int                   `json:"max_tokens,omitempty"`
	ReasoningEffort string               `json:"reasoning_effort,omitempty"`
}

type openAIChatResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role      string               `json:"role"`
			Content   *string              `json:"content"`
			ToolCalls []openAIToolCallWire `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
		CompletionDetails *struct {
			ReasoningTokens int `json:"reasoning_tokens"`
		} `json:"completion_tokens_details,omitempty"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
	} `json:"error,omitempty"`
}

func (m *Module) Execute(ctx context.Context, cfg byok.NormalizedModel, req kernel.ExecutionRequest) (*kernel.ExecutionResult, error) {
	wireReq := openAIChatRequest{
		Model:     cfg.Model,
		MaxTokens: req.MaxTokens,
	}

	for _, msg := range req.Messages {
		wireMsg := openAIMessage{
			Role:       msg.Role,
			Content:    msg.Content,
			Name:       msg.Name,
			ToolCallID: msg.ToolCallID,
		}
		for _, tc := range msg.ToolCalls {
			wireMsg.ToolCalls = append(wireMsg.ToolCalls, openAIToolCallWire{
				ID:   tc.ID,
				Type: "function",
				Function: openAIFunctionCallWire{
					Name:      tc.Name,
					Arguments: tc.Arguments,
				},
			})
		}
		wireReq.Messages = append(wireReq.Messages, wireMsg)
	}

	for _, t := range req.Tools {
		wireReq.Tools = append(wireReq.Tools, openAIToolWire{
			Type: "function",
			Function: openAIFunctionWire{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
				Strict:      true,
			},
		})
	}

	if len(wireReq.Tools) > 0 && req.ToolChoice != nil {
		wireReq.ToolChoice = req.ToolChoice
	}

	if req.ResponseSchema != nil {
		strictSchema := structured.ToStrictWireSchema(req.ResponseSchema)
		supportsStrict := isNativeOpenAIModel(cfg.Model) ||
			structured.IsNeverDowngradeModel(cfg.Model) ||
			structured.OpenAICompatibleHonorsJSONSchema(cfg.BaseURL)

		if supportsStrict {
			wireReq.ResponseFormat = &openAIResponseFormat{
				Type: "json_schema",
				JSONSchema: &openAIJSONSchema{
					Name:   "scandrix_structured_response",
					Strict: true,
					Schema: strictSchema,
				},
			}
		} else {
			wireReq.ResponseFormat = &openAIResponseFormat{
				Type: "json_object",
			}
		}
	}

	isReasoning := strings.HasPrefix(strings.ToLower(cfg.Model), "o1") || strings.HasPrefix(strings.ToLower(cfg.Model), "o3") || strings.HasPrefix(strings.ToLower(cfg.Model), "o4")
	if isReasoning {
		if req.ReasoningEffort != "" && req.ReasoningEffort != kernel.ReasoningNone {
			wireReq.ReasoningEffort = string(req.ReasoningEffort)
		}
	} else {
		wireReq.Temperature = req.Temperature
	}

	payloadBytes, err := json.Marshal(wireReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal openai request: %w", err)
	}

	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	endpointURL := baseURL + "/chat/completions"

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
		return nil, fmt.Errorf("openai request error: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read openai response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errResp openAIChatResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != nil {
			return nil, fmt.Errorf("openai api error (status %d): %s", resp.StatusCode, errResp.Error.Message)
		}
		return nil, fmt.Errorf("openai api returned error status %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed openAIChatResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("failed to decode openai response: %w", err)
	}

	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("openai returned zero choices")
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

	if parsed.Usage.CompletionDetails != nil {
		result.Usage.ReasoningTokens = parsed.Usage.CompletionDetails.ReasoningTokens
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

func isNativeOpenAIModel(model string) bool {
	m := strings.ToLower(model)
	return strings.HasPrefix(m, "gpt-") ||
		strings.HasPrefix(m, "o1") ||
		strings.HasPrefix(m, "o3") ||
		strings.HasPrefix(m, "o4") ||
		strings.HasPrefix(m, "chatgpt-")
}

