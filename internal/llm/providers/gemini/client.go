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

	"github.com/scandrix/backend/internal/llm/orchestrator"
)

// Client implements native inference against the Google Gemini API.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a new Google Gemini provider connector.
func NewClient(apiKey string, baseURL ...string) *Client {
	endpoint := "https://generativelanguage.googleapis.com/v1beta"
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
		return nil, fmt.Errorf("gemini api key is required")
	}

	modelName := model
	if modelName == "" {
		modelName = "gemini-3.1-pro"
	}

	// Separate system prompt and build contents payload
	var systemText string
	var contents []map[string]any

	for _, m := range req.Messages {
		if m.Role == orchestrator.RoleSystem {
			if systemText != "" {
				systemText += "\n\n"
			}
			systemText += m.Content
		} else {
			role := "user"
			if m.Role == orchestrator.RoleAssistant {
				role = "model"
			}
			contents = append(contents, map[string]any{
				"role": role,
				"parts": []map[string]string{
					{"text": m.Content},
				},
			})
		}
	}

	genConfig := map[string]any{
		"responseMimeType": "application/json",
	}
	if req.MaxTokens > 0 {
		genConfig["maxOutputTokens"] = req.MaxTokens
	}
	if req.Temperature > 0 {
		genConfig["temperature"] = req.Temperature
	}
	if req.EnableReasoning {
		genConfig["thinkingConfig"] = map[string]any{
			"thinkingBudget": 8192,
		}
	}

	payload := map[string]any{
		"contents":         contents,
		"generationConfig": genConfig,
	}

	if systemText != "" {
		payload["systemInstruction"] = map[string]any{
			"parts": []map[string]string{
				{"text": systemText},
			},
		}
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", c.baseURL, modelName, apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var geminiResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		UsageMetadata struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
			TotalTokenCount      int `json:"totalTokenCount"`
		} `json:"usageMetadata"`
	}

	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return nil, fmt.Errorf("failed parsing gemini response: %w", err)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("gemini returned no content candidates")
	}

	var contentBuilder strings.Builder
	for _, part := range geminiResp.Candidates[0].Content.Parts {
		contentBuilder.WriteString(part.Text)
	}

	return &orchestrator.InferenceResponse{
		Content:          contentBuilder.String(),
		ModelUsed:        modelName,
		ProviderUsed:     orchestrator.ProviderGemini,
		PromptTokens:     geminiResp.UsageMetadata.PromptTokenCount,
		CompletionTokens: geminiResp.UsageMetadata.CandidatesTokenCount,
		TotalTokens:      geminiResp.UsageMetadata.TotalTokenCount,
	}, nil
}
