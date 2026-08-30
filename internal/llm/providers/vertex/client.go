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

	"github.com/scandrix/backend/internal/llm/orchestrator"
)

// Client implements native inference against Google Cloud Vertex AI REST API.
type Client struct {
	projectID      string
	location       string
	accessToken    string
	customEndpoint string
	httpClient     *http.Client
}

// NewClient creates a new Google Vertex AI connector.
func NewClient(projectID, location, accessToken string, customEndpoint ...string) *Client {
	if location == "" {
		location = "us-central1"
	}
	endpoint := ""
	if len(customEndpoint) > 0 {
		endpoint = strings.TrimRight(customEndpoint[0], "/")
	}
	return &Client{
		projectID:      projectID,
		location:       location,
		accessToken:    accessToken,
		customEndpoint: endpoint,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (c *Client) Complete(ctx context.Context, req orchestrator.InferenceRequest, model string) (*orchestrator.InferenceResponse, error) {
	token := c.accessToken
	if req.TenantAPIKey != "" {
		token = req.TenantAPIKey
	}
	if token == "" {
		return nil, fmt.Errorf("google vertex ai access token is required")
	}

	modelID := model
	if modelID == "" || modelID == "vertex-gemini-2.5-pro" {
		modelID = "gemini-2.5-pro"
	} else if modelID == "vertex-gemini-2.0-flash" {
		modelID = "gemini-2.0-flash"
	}

	type vertexPart struct {
		Text string `json:"text"`
	}
	type vertexContent struct {
		Role  string       `json:"role"`
		Parts []vertexPart `json:"parts"`
	}

	var contents []vertexContent
	var systemInstructions *vertexContent

	for _, m := range req.Messages {
		if m.Role == orchestrator.RoleSystem {
			systemInstructions = &vertexContent{
				Parts: []vertexPart{{Text: m.Content}},
			}
		} else {
			role := "user"
			if m.Role == orchestrator.RoleAssistant {
				role = "model"
			}
			contents = append(contents, vertexContent{
				Role:  role,
				Parts: []vertexPart{{Text: m.Content}},
			})
		}
	}

	if len(contents) == 0 {
		contents = append(contents, vertexContent{
			Role:  "user",
			Parts: []vertexPart{{Text: "Analyze the provided code diff."}},
		})
	}

	generationConfig := map[string]any{
		"temperature":      0.2,
		"responseMimeType": "application/json",
	}
	if req.MaxTokens > 0 {
		generationConfig["maxOutputTokens"] = req.MaxTokens
	}
	if req.Temperature > 0 {
		generationConfig["temperature"] = req.Temperature
	}

	payload := map[string]any{
		"contents":         contents,
		"generationConfig": generationConfig,
	}
	if systemInstructions != nil {
		payload["systemInstruction"] = systemInstructions
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	project := c.projectID
	if project == "" {
		project = "scandrix-production"
	}

	url := fmt.Sprintf("https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/google/models/%s:generateContent",
		c.location, project, c.location, modelID)
	if c.customEndpoint != "" {
		url = fmt.Sprintf("%s/models/%s:generateContent", c.customEndpoint, modelID)
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
		return nil, fmt.Errorf("vertex ai request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vertex ai error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var data struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
				Role string `json:"role"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
		UsageMetadata struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
			TotalTokenCount      int `json:"totalTokenCount"`
		} `json:"usageMetadata"`
	}

	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("failed parsing vertex ai response: %w", err)
	}

	if len(data.Candidates) == 0 || len(data.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("vertex ai returned no candidate text output")
	}

	content := data.Candidates[0].Content.Parts[0].Text
	return &orchestrator.InferenceResponse{
		Content:          content,
		ModelUsed:        modelID,
		ProviderUsed:     orchestrator.ProviderVertex,
		PromptTokens:     data.UsageMetadata.PromptTokenCount,
		CompletionTokens: data.UsageMetadata.CandidatesTokenCount,
		TotalTokens:      data.UsageMetadata.TotalTokenCount,
	}, nil
}
