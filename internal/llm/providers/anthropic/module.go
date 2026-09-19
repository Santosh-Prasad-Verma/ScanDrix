// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

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

	"github.com/scandrix/backend/internal/llm/byok"
	"github.com/scandrix/backend/internal/llm/providers/kernel"
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
	return "anthropic"
}

func (m *Module) Aliases() []string {
	return []string{"anthropic_compatible"}
}

func (m *Module) Label() string {
	return "Anthropic"
}

func (m *Module) Doc() string {
	return "https://docs.anthropic.com"
}

func (m *Module) Capabilities(model string) kernel.ModelCapabilities {
	mLower := strings.ToLower(model)
	supportsReasoning := strings.Contains(mLower, "3-7") || strings.Contains(mLower, "3.7") || strings.Contains(mLower, "4")

	return kernel.ModelCapabilities{
		MaxInputTokens:      200000,
		StructuredOutput:    "json_schema",
		ToolCalling:         "native",
		SupportsStreaming:   true,
		PromptCaching:       true,
		SupportsReasoning:   supportsReasoning,
		SupportsTemperature: true,
	}
}

func (m *Module) ReasoningTraits(cfg byok.NormalizedModel) kernel.ModelReasoningTraits {
	mLower := strings.ToLower(cfg.Model)
	if strings.Contains(mLower, "3-7") || strings.Contains(mLower, "3.7") || strings.Contains(mLower, "4") {
		return kernel.ModelReasoningTraits{
			ThinksByDefault:                false,
			CanDisableThinking:             true,
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
	return map[string]any{
		"type": "ephemeral",
	}
}

func (m *Module) UIFields() []kernel.FieldDescriptor {
	return []kernel.FieldDescriptor{
		{Key: "apiKey", Label: "API Key", Type: "password", Required: true, Scope: "top"},
		{Key: "baseURL", Label: "Base URL", Type: "url", Required: false, Scope: "top", Placeholder: "https://api.anthropic.com/v1"},
	}
}

func (m *Module) ModelListing(providerID string) *kernel.ModelListing {
	if providerID == "anthropic" {
		return &kernel.ModelListing{
			Kind:      kernel.ListingHTTP,
			APIKeyEnv: "API_ANTHROPIC_API_KEY",
			URL: func(creds kernel.ResolvedListingCreds) string {
				return "https://api.anthropic.com/v1/models"
			},
			Headers: func(creds kernel.ResolvedListingCreds) map[string]string {
				return map[string]string{
					"x-api-key":         creds.APIKey,
					"anthropic-version": "2023-06-01",
					"Content-Type":      "application/json",
				}
			},
			Parse: func(body []byte) ([]kernel.CatalogModel, error) {
				var resp struct {
					Data []struct {
						ID          string `json:"id"`
						DisplayName string `json:"display_name"`
					} `json:"data"`
				}
				if err := json.Unmarshal(body, &resp); err != nil {
					return nil, err
				}
				result := make([]kernel.CatalogModel, 0, len(resp.Data))
				for _, d := range resp.Data {
					name := d.DisplayName
					if name == "" {
						name = kernel.FormatModelLabel(d.ID)
					}
					result = append(result, kernel.CatalogModel{
						ID:   d.ID,
						Name: name,
					})
				}
				return result, nil
			},
		}
	}
	if providerID == "anthropic_compatible" {
		return &kernel.ModelListing{Kind: kernel.ListingManual}
	}
	return nil
}

type anthropicContentBlockWire struct {
	Type         string         `json:"type"`
	Text         string         `json:"text,omitempty"`
	ID           string         `json:"id,omitempty"`
	Name         string         `json:"name,omitempty"`
	Input        map[string]any `json:"input,omitempty"`
	ToolUseID    string         `json:"tool_use_id,omitempty"`
	Content      string         `json:"content,omitempty"`
	CacheControl map[string]any `json:"cache_control,omitempty"`
}

type anthropicMessageWire struct {
	Role    string                      `json:"role"`
	Content []anthropicContentBlockWire `json:"content"`
}

type anthropicToolWire struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema"`
}

type anthropicRequestWire struct {
	Model       string                      `json:"model"`
	Messages    []anthropicMessageWire      `json:"messages"`
	System      []anthropicContentBlockWire `json:"system,omitempty"`
	Tools       []anthropicToolWire         `json:"tools,omitempty"`
	ToolChoice  any                         `json:"tool_choice,omitempty"`
	Thinking    map[string]any              `json:"thinking,omitempty"`
	MaxTokens   int                         `json:"max_tokens"`
	Temperature *float64                    `json:"temperature,omitempty"`
}

type anthropicResponseWire struct {
	ID      string                      `json:"id"`
	Type    string                      `json:"type"`
	Role    string                      `json:"role"`
	Content []anthropicContentBlockWire `json:"content"`
	Usage   struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (m *Module) Execute(ctx context.Context, cfg byok.NormalizedModel, req kernel.ExecutionRequest) (*kernel.ExecutionResult, error) {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 8192
	}

	wireReq := anthropicRequestWire{
		Model:     cfg.Model,
		MaxTokens: maxTokens,
	}

	for _, msg := range req.Messages {
		if msg.Role == "system" {
			systemBlock := anthropicContentBlockWire{
				Type: "text",
				Text: msg.Content,
			}
			if req.SystemCacheHint {
				systemBlock.CacheControl = map[string]any{"type": "ephemeral"}
			}
			wireReq.System = append(wireReq.System, systemBlock)
			continue
		}

		wireMsg := anthropicMessageWire{
			Role: msg.Role,
		}
		if msg.Role == "tool" {
			wireMsg.Role = "user"
			wireMsg.Content = append(wireMsg.Content, anthropicContentBlockWire{
				Type:      "tool_result",
				ToolUseID: msg.ToolCallID,
				Content:   msg.Content,
			})
		} else {
			if msg.Content != "" {
				wireMsg.Content = append(wireMsg.Content, anthropicContentBlockWire{
					Type: "text",
					Text: msg.Content,
				})
			}
			for _, tc := range msg.ToolCalls {
				var inputArgs map[string]any
				_ = json.Unmarshal([]byte(tc.Arguments), &inputArgs)
				wireMsg.Content = append(wireMsg.Content, anthropicContentBlockWire{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Name,
					Input: inputArgs,
				})
			}
		}
		wireReq.Messages = append(wireReq.Messages, wireMsg)
	}

	for _, t := range req.Tools {
		wireReq.Tools = append(wireReq.Tools, anthropicToolWire{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.Parameters,
		})
	}

	if req.ResponseSchema != nil {
		wireReq.Tools = append(wireReq.Tools, anthropicToolWire{
			Name:        "scandrix_structured_response",
			Description: "Submit structured response conforming to the strict schema",
			InputSchema: req.ResponseSchema,
		})
		wireReq.ToolChoice = map[string]any{
			"type": "tool",
			"name": "scandrix_structured_response",
		}
	} else if len(wireReq.Tools) > 0 && req.ToolChoice != nil {
		wireReq.ToolChoice = req.ToolChoice
	}

	if req.ReasoningEffort != "" && req.ReasoningEffort != kernel.ReasoningNone {
		budget := 4096
		if maxTokens > 8192 {
			budget = 8192
		}
		wireReq.Thinking = map[string]any{
			"type":          "enabled",
			"budget_tokens": budget,
		}
	} else if req.Temperature != nil {
		wireReq.Temperature = req.Temperature
	}

	payloadBytes, err := json.Marshal(wireReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal anthropic request: %w", err)
	}

	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.anthropic.com/v1"
	}
	endpointURL := baseURL + "/messages"

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", cfg.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("anthropic-beta", "prompt-caching-2024-07-31")

	client := m.httpClient
	if client == nil {
		client = settings.NewSafeHTTPClient(90 * time.Second)
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic request error: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read anthropic response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errResp anthropicResponseWire
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != nil {
			return nil, fmt.Errorf("anthropic api error (status %d): %s", resp.StatusCode, errResp.Error.Message)
		}
		return nil, fmt.Errorf("anthropic api returned error status %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed anthropicResponseWire
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("failed to decode anthropic response: %w", err)
	}

	result := &kernel.ExecutionResult{
		Raw: parsed,
		Usage: kernel.TokenUsage{
			InputTokens:  parsed.Usage.InputTokens,
			OutputTokens: parsed.Usage.OutputTokens,
			TotalTokens:  parsed.Usage.InputTokens + parsed.Usage.OutputTokens,
		},
	}

	var textParts []string
	for _, block := range parsed.Content {
		switch block.Type {
		case "text":
			textParts = append(textParts, block.Text)
		case "tool_use":
			if block.Name == "scandrix_structured_response" {
				jsonBytes, _ := json.Marshal(block.Input)
				result.Text = string(jsonBytes)
			} else {
				argBytes, _ := json.Marshal(block.Input)
				result.ToolCalls = append(result.ToolCalls, kernel.ToolCall{
					ID:        block.ID,
					Name:      block.Name,
					Arguments: string(argBytes),
				})
			}
		}
	}

	if result.Text == "" && len(textParts) > 0 {
		result.Text = strings.Join(textParts, "\n")
	}

	return result, nil
}
