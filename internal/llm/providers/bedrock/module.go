// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

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
	return "amazon_bedrock"
}

func (m *Module) Aliases() []string {
	return []string{"bedrock", "aws_bedrock"}
}

func (m *Module) Label() string {
	return "Amazon Bedrock"
}

func (m *Module) Doc() string {
	return "https://docs.aws.amazon.com/bedrock"
}

func (m *Module) Capabilities(model string) kernel.ModelCapabilities {
	return kernel.ModelCapabilities{
		MaxInputTokens:      200000,
		StructuredOutput:    "json_schema",
		ToolCalling:         "native",
		SupportsStreaming:   true,
		PromptCaching:       false,
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
		{Key: "awsBearerToken", Label: "Bedrock API Key (Bearer Token)", Type: "password", Required: false, Scope: "settings"},
		{Key: "awsRegion", Label: "AWS Region", Type: "text", Required: true, Scope: "settings", Placeholder: "us-east-1"},
		{Key: "awsAccessKeyId", Label: "AWS Access Key ID", Type: "text", Required: false, Scope: "settings"},
		{Key: "awsSecretAccessKey", Label: "AWS Secret Access Key", Type: "password", Required: false, Scope: "settings"},
		{Key: "awsSessionToken", Label: "AWS Session Token", Type: "password", Required: false, Scope: "settings"},
	}
}

var bedrockCuratedCatalog = []kernel.CatalogModel{
	kernel.CatalogWithReasoning("us.anthropic.claude-sonnet-4-5-20250929-v1:0", "Claude Sonnet 4.5 (us, cross-region)", "claude-sonnet-4-5"),
	kernel.CatalogWithReasoning("us.anthropic.claude-3-7-sonnet-20250219-v1:0", "Claude 3.7 Sonnet (us, cross-region)", "claude-3-7-sonnet"),
	kernel.CatalogWithReasoning("us.anthropic.claude-3-5-sonnet-20241022-v2:0", "Claude 3.5 Sonnet v2 (us, cross-region)", "claude-3-5-sonnet"),
}

func (m *Module) ModelListing(providerID string) *kernel.ModelListing {
	if providerID == "amazon_bedrock" || providerID == "bedrock" || providerID == "aws_bedrock" {
		return &kernel.ModelListing{
			Kind:           kernel.ListingHTTP,
			TimeoutMs:      10000,
			FallbackModels: bedrockCuratedCatalog,
			URL: func(creds kernel.ResolvedListingCreds) string {
				region := creds.AWSRegion
				if region == "" {
					region = "us-east-1"
				}
				return fmt.Sprintf("https://bedrock.%s.amazonaws.com/inference-profiles?maxResults=1000&typeEquals=SYSTEM_DEFINED", region)
			},
			Headers: func(creds kernel.ResolvedListingCreds) map[string]string {
				return map[string]string{
					"Authorization": "Bearer " + creds.AWSBearerToken,
					"Accept":        "application/json",
				}
			},
			Parse: func(body []byte) ([]kernel.CatalogModel, error) {
				var resp struct {
					Summaries []struct {
						ProfileID   string `json:"inferenceProfileId"`
						ProfileName string `json:"inferenceProfileName"`
						Status      string `json:"status"`
					} `json:"inferenceProfileSummaries"`
				}
				if err := json.Unmarshal(body, &resp); err != nil {
					return nil, err
				}
				var list []kernel.CatalogModel
				for _, s := range resp.Summaries {
					if s.Status == "" || s.Status == "ACTIVE" {
						name := s.ProfileName
						if name == "" {
							name = s.ProfileID
						}
						list = append(list, kernel.CatalogWithReasoning(s.ProfileID, name, s.ProfileID))
					}
				}
				return list, nil
			},
		}
	}
	return nil
}

type bedrockContentWire struct {
	Text string `json:"text"`
}

type bedrockMessageWire struct {
	Role    string               `json:"role"`
	Content []bedrockContentWire `json:"content"`
}

type bedrockConverseRequestWire struct {
	Messages        []bedrockMessageWire   `json:"messages"`
	System          []bedrockContentWire   `json:"system,omitempty"`
	InferenceConfig map[string]any         `json:"inferenceConfig,omitempty"`
}

type bedrockConverseResponseWire struct {
	Output struct {
		Message struct {
			Role    string               `json:"role"`
			Content []bedrockContentWire `json:"content"`
		} `json:"message"`
	} `json:"output"`
	Usage struct {
		InputTokens  int `json:"inputTokens"`
		OutputTokens int `json:"outputTokens"`
		TotalTokens  int `json:"totalTokens"`
	} `json:"usage"`
	Message string `json:"message,omitempty"`
}

func (m *Module) Execute(ctx context.Context, cfg byok.NormalizedModel, req kernel.ExecutionRequest) (*kernel.ExecutionResult, error) {
	wireReq := bedrockConverseRequestWire{
		InferenceConfig: make(map[string]any),
	}

	for _, msg := range req.Messages {
		if msg.Role == "system" {
			wireReq.System = append(wireReq.System, bedrockContentWire{Text: msg.Content})
			continue
		}

		wireReq.Messages = append(wireReq.Messages, bedrockMessageWire{
			Role:    msg.Role,
			Content: []bedrockContentWire{{Text: msg.Content}},
		})
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	wireReq.InferenceConfig["maxTokens"] = maxTokens

	if req.Temperature != nil {
		wireReq.InferenceConfig["temperature"] = *req.Temperature
	} else {
		wireReq.InferenceConfig["temperature"] = 0.2
	}

	payloadBytes, err := json.Marshal(wireReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal bedrock request: %w", err)
	}

	modelID := cfg.Model
	if modelID == "" {
		return nil, fmt.Errorf("bedrock model ID is required")
	}

	region := "us-east-1"
	if cfg.AWSRegion != "" {
		region = cfg.AWSRegion
	}

	endpointURL := fmt.Sprintf("https://bedrock-runtime.%s.amazonaws.com/model/%s/converse", region, modelID)
	if cfg.BaseURL != "" {
		endpointURL = strings.TrimRight(cfg.BaseURL, "/") + "/model/" + modelID + "/converse"
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	bearer := cfg.APIKey
	if bearer == "" {
		bearer = cfg.AWSBearerToken
	}
	if bearer != "" {
		httpReq.Header.Set("Authorization", "Bearer "+bearer)
	}

	client := m.httpClient
	if client == nil {
		client = settings.NewSafeHTTPClient(90 * time.Second)
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("bedrock request error: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read bedrock response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("bedrock api error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var parsed bedrockConverseResponseWire
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("failed to decode bedrock response: %w", err)
	}

	var textParts []string
	for _, c := range parsed.Output.Message.Content {
		if c.Text != "" {
			textParts = append(textParts, c.Text)
		}
	}

	result := &kernel.ExecutionResult{
		Text: strings.Join(textParts, "\n"),
		Raw:  parsed,
		Usage: kernel.TokenUsage{
			InputTokens:  parsed.Usage.InputTokens,
			OutputTokens: parsed.Usage.OutputTokens,
			TotalTokens:  parsed.Usage.TotalTokens,
		},
	}

	return result, nil
}
