// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package gemini

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
	return "google_gemini"
}

func (m *Module) Aliases() []string {
	return []string{"gemini", "google", "google-gemini"}
}

func (m *Module) Label() string {
	return "Google Gemini"
}

func (m *Module) Doc() string {
	return "https://ai.google.dev/docs"
}

var geminiMaxInputTokens = map[string]int{
	"gemini-2.5-pro":                1_000_000,
	"gemini-3.1-flash-lite-preview": 1_048_576,
}

func (m *Module) Capabilities(model string) kernel.ModelCapabilities {
	mLower := strings.ToLower(strings.TrimSpace(model))
	supportsReasoning := strings.Contains(mLower, "thinking") || strings.Contains(mLower, "2.5") || strings.Contains(mLower, "3.")

	return kernel.ModelCapabilities{
		MaxInputTokens:      geminiMaxInputTokens[mLower],
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
	if strings.Contains(mLower, "thinking") || strings.Contains(mLower, "2.5") || strings.Contains(mLower, "3.") {
		return kernel.ModelReasoningTraits{
			ThinksByDefault:                true,
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
	return nil
}

func (m *Module) UIFields() []kernel.FieldDescriptor {
	return []kernel.FieldDescriptor{
		{Key: "apiKey", Label: "API Key", Type: "password", Required: true, Scope: "top"},
	}
}

func (m *Module) ModelListing(providerID string) *kernel.ModelListing {
	if providerID == "google_gemini" || providerID == "gemini" || providerID == "google" || providerID == "google-gemini" {
		return &kernel.ModelListing{
			Kind:      kernel.ListingHTTP,
			APIKeyEnv: "API_GOOGLE_AI_API_KEY",
			TimeoutMs: 10000,
			URL: func(creds kernel.ResolvedListingCreds) string {
				return "https://generativelanguage.googleapis.com/v1beta/models"
			},
			Headers: func(creds kernel.ResolvedListingCreds) map[string]string {
				return map[string]string{
					"x-goog-api-key": creds.APIKey,
				}
			},
			Parse: func(body []byte) ([]kernel.CatalogModel, error) {
				var resp struct {
					Models []struct {
						Name string `json:"name"`
					} `json:"models"`
				}
				if err := json.Unmarshal(body, &resp); err != nil {
					return nil, err
				}
				var result []kernel.CatalogModel
				for _, mod := range resp.Models {
					if strings.Contains(strings.ToLower(mod.Name), "gemini") {
						parts := strings.Split(mod.Name, "/")
						mID := mod.Name
						if len(parts) > 1 {
							mID = parts[1]
						}
						result = append(result, kernel.CatalogWithReasoning(mID, "", mID))
					}
				}
				return result, nil
			},
		}
	}
	return nil
}

type geminiPartWire struct {
	Text         string                  `json:"text,omitempty"`
	FunctionCall *geminiFunctionCallWire `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResponseWire `json:"functionResponse,omitempty"`
}

type geminiFunctionCallWire struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

type geminiFunctionResponseWire struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type geminiContentWire struct {
	Role  string           `json:"role"`
	Parts []geminiPartWire `json:"parts"`
}

type geminiFunctionDeclarationWire struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
}

type geminiToolWire struct {
	FunctionDeclarations []geminiFunctionDeclarationWire `json:"functionDeclarations"`
}

type geminiGenerateRequestWire struct {
	Contents          []geminiContentWire `json:"contents"`
	SystemInstruction *geminiContentWire  `json:"systemInstruction,omitempty"`
	Tools             []geminiToolWire    `json:"tools,omitempty"`
	GenerationConfig  map[string]any      `json:"generationConfig,omitempty"`
}

type geminiGenerateResponseWire struct {
	Candidates []struct {
		Content struct {
			Parts []geminiPartWire `json:"parts"`
			Role  string           `json:"role"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

func (m *Module) Execute(ctx context.Context, cfg byok.NormalizedModel, req kernel.ExecutionRequest) (*kernel.ExecutionResult, error) {
	wireReq := geminiGenerateRequestWire{
		GenerationConfig: make(map[string]any),
	}

	for _, msg := range req.Messages {
		if msg.Role == "system" {
			wireReq.SystemInstruction = &geminiContentWire{
				Role: "user",
				Parts: []geminiPartWire{
					{Text: msg.Content},
				},
			}
			continue
		}

		role := "user"
		if msg.Role == "assistant" {
			role = "model"
		}

		var parts []geminiPartWire
		if msg.Role == "tool" {
			role = "user"
			var toolOutput map[string]any
			if err := json.Unmarshal([]byte(msg.Content), &toolOutput); err != nil {
				toolOutput = map[string]any{"output": msg.Content}
			}
			parts = append(parts, geminiPartWire{
				FunctionResponse: &geminiFunctionResponseWire{
					Name:     msg.Name,
					Response: toolOutput,
				},
			})
		} else {
			if msg.Content != "" {
				parts = append(parts, geminiPartWire{Text: msg.Content})
			}
			for _, tc := range msg.ToolCalls {
				var args map[string]any
				_ = json.Unmarshal([]byte(tc.Arguments), &args)
				parts = append(parts, geminiPartWire{
					FunctionCall: &geminiFunctionCallWire{
						Name: tc.Name,
						Args: args,
					},
				})
			}
		}

		wireReq.Contents = append(wireReq.Contents, geminiContentWire{
			Role:  role,
			Parts: parts,
		})
	}

	if len(req.Tools) > 0 {
		var decls []geminiFunctionDeclarationWire
		for _, t := range req.Tools {
			decls = append(decls, geminiFunctionDeclarationWire{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			})
		}
		wireReq.Tools = append(wireReq.Tools, geminiToolWire{
			FunctionDeclarations: decls,
		})
	}

	if req.ResponseSchema != nil {
		wireReq.GenerationConfig["responseMimeType"] = "application/json"
		wireReq.GenerationConfig["responseSchema"] = req.ResponseSchema
	}

	if req.Temperature != nil {
		wireReq.GenerationConfig["temperature"] = *req.Temperature
	}
	if req.MaxTokens > 0 {
		wireReq.GenerationConfig["maxOutputTokens"] = req.MaxTokens
	}

	if req.ReasoningEffort != "" && req.ReasoningEffort != kernel.ReasoningNone {
		wireReq.GenerationConfig["thinkingConfig"] = map[string]any{
			"thinkingBudget": 4096,
		}
	}

	payloadBytes, err := json.Marshal(wireReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal gemini request: %w", err)
	}

	modelName := cfg.Model
	if modelName == "" {
		return nil, fmt.Errorf("gemini model name is required")
	}

	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}
	endpointURL := fmt.Sprintf("%s/v1beta/models/%s:generateContent", baseURL, modelName)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", cfg.APIKey)

	client := m.httpClient
	if client == nil {
		client = settings.NewSafeHTTPClient(90 * time.Second)
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini request error: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read gemini response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errResp geminiGenerateResponseWire
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != nil {
			return nil, fmt.Errorf("gemini api error (status %d): %s", resp.StatusCode, errResp.Error.Message)
		}
		return nil, fmt.Errorf("gemini api returned error status %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed geminiGenerateResponseWire
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("failed to decode gemini response: %w", err)
	}

	if len(parsed.Candidates) == 0 {
		return nil, fmt.Errorf("gemini returned zero candidates")
	}

	candidate := parsed.Candidates[0]
	result := &kernel.ExecutionResult{
		Raw: parsed,
		Usage: kernel.TokenUsage{
			InputTokens:  parsed.UsageMetadata.PromptTokenCount,
			OutputTokens: parsed.UsageMetadata.CandidatesTokenCount,
			TotalTokens:  parsed.UsageMetadata.TotalTokenCount,
		},
	}

	var textParts []string
	for idx, part := range candidate.Content.Parts {
		if part.Text != "" {
			textParts = append(textParts, part.Text)
		}
		if part.FunctionCall != nil {
			argBytes, _ := json.Marshal(part.FunctionCall.Args)
			result.ToolCalls = append(result.ToolCalls, kernel.ToolCall{
				ID:        fmt.Sprintf("call_%d", idx),
				Name:      part.FunctionCall.Name,
				Arguments: string(argBytes),
			})
		}
	}

	result.Text = strings.Join(textParts, "\n")
	return result, nil
}
