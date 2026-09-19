// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package vertex

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
	return "google_vertex"
}

func (m *Module) Aliases() []string {
	return []string{"vertex"}
}

func (m *Module) Label() string {
	return "Google Vertex AI"
}

func (m *Module) Doc() string {
	return "https://cloud.google.com/vertex-ai/docs"
}

func (m *Module) Capabilities(model string) kernel.ModelCapabilities {
	mLower := strings.ToLower(strings.TrimSpace(model))
	var maxInputTokens int
	if strings.HasPrefix(mLower, "claude-3-5-sonnet") {
		maxInputTokens = 200_000
	}

	return kernel.ModelCapabilities{
		MaxInputTokens:      maxInputTokens,
		StructuredOutput:    "json_schema",
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
		{Key: "apiKey", Label: "Service Account JSON / Access Token", Type: "password", Required: true, Scope: "top"},
		{Key: "vertexLocation", Label: "Vertex Location", Type: "text", Required: false, Scope: "settings", Placeholder: "us-central1"},
	}
}

func (m *Module) ModelListing(providerID string) *kernel.ModelListing {
	if providerID == "google_vertex" || providerID == "vertex" {
		return &kernel.ModelListing{
			Kind: kernel.ListingStatic,
			StaticModels: []kernel.CatalogModel{
				kernel.CatalogWithReasoning("gemini-2.5-pro", "Vertex Gemini 2.5 Pro", "gemini-2.5-pro"),
				kernel.CatalogWithReasoning("gemini-2.5-flash", "Vertex Gemini 2.5 Flash", "gemini-2.5-flash"),
				kernel.CatalogWithReasoning("claude-3-5-sonnet-v2@20241022", "Vertex Claude 3.5 Sonnet", "claude-3-5-sonnet"),
				kernel.CatalogWithReasoning("claude-sonnet-4-5@20250929", "Vertex Claude Sonnet 4.5", "claude-sonnet-4-5"),
			},
		}
	}
	return nil
}

type vertexContentWire struct {
	Role  string           `json:"role"`
	Parts []map[string]any `json:"parts"`
}

type vertexRequestWire struct {
	Contents         []vertexContentWire `json:"contents"`
	GenerationConfig map[string]any      `json:"generationConfig,omitempty"`
}

type vertexResponseWire struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
			Role string `json:"role"`
		} `json:"content"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (m *Module) Execute(ctx context.Context, cfg byok.NormalizedModel, req kernel.ExecutionRequest) (*kernel.ExecutionResult, error) {
	wireReq := vertexRequestWire{
		GenerationConfig: make(map[string]any),
	}

	for _, msg := range req.Messages {
		role := "user"
		if msg.Role == "assistant" {
			role = "model"
		}
		wireReq.Contents = append(wireReq.Contents, vertexContentWire{
			Role:  role,
			Parts: []map[string]any{{"text": msg.Content}},
		})
	}

	if req.Temperature != nil {
		wireReq.GenerationConfig["temperature"] = *req.Temperature
	}
	if req.MaxTokens > 0 {
		wireReq.GenerationConfig["maxOutputTokens"] = req.MaxTokens
	}
	if req.ResponseSchema != nil {
		wireReq.GenerationConfig["responseMimeType"] = "application/json"
		wireReq.GenerationConfig["responseSchema"] = req.ResponseSchema
	}

	payloadBytes, err := json.Marshal(wireReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal vertex request: %w", err)
	}

	modelID := cfg.Model
	if modelID == "" {
		return nil, fmt.Errorf("vertex model ID is required")
	}

	location := "us-central1"
	if cfg.VertexLocation != "" {
		location = cfg.VertexLocation
	}

	endpointURL := fmt.Sprintf("https://%s-aiplatform.googleapis.com/v1/projects/default/locations/%s/publishers/google/models/%s:generateContent", location, location, modelID)
	if cfg.BaseURL != "" {
		endpointURL = strings.TrimRight(cfg.BaseURL, "/") + "/publishers/google/models/" + modelID + ":generateContent"
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if cfg.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}

	client := m.httpClient
	if client == nil {
		client = settings.NewSafeHTTPClient(90 * time.Second)
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("vertex request error: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read vertex response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errResp vertexResponseWire
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != nil {
			return nil, fmt.Errorf("vertex api error (status %d): %s", resp.StatusCode, errResp.Error.Message)
		}
		return nil, fmt.Errorf("vertex api error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var parsed vertexResponseWire
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("failed to decode vertex response: %w", err)
	}

	if len(parsed.Candidates) == 0 {
		return nil, fmt.Errorf("vertex returned zero candidates")
	}

	var textParts []string
	for _, p := range parsed.Candidates[0].Content.Parts {
		if p.Text != "" {
			textParts = append(textParts, p.Text)
		}
	}

	result := &kernel.ExecutionResult{
		Text: strings.Join(textParts, "\n"),
		Raw:  parsed,
		Usage: kernel.TokenUsage{
			InputTokens:  parsed.UsageMetadata.PromptTokenCount,
			OutputTokens: parsed.UsageMetadata.CandidatesTokenCount,
			TotalTokens:  parsed.UsageMetadata.TotalTokenCount,
		},
	}

	return result, nil
}
