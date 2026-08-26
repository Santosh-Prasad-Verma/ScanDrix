package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/codehound/codehound/shared/domain"
	"github.com/google/uuid"
)

// OpenRouter API Constants and Model Identifiers
const (
	OpenRouterBaseURL = "https://openrouter.ai/api/v1/chat/completions"

	// Tier 1: Fast Triage
	ModelGeminiFlash = "google/gemini-2.5-flash"
	ModelGPT4oMini   = "openai/gpt-4o-mini"

	// Tier 2: Dual Specialized Reasoning
	ModelDeepSeekR1    = "deepseek/deepseek-r1"
	ModelClaudeSonnet  = "anthropic/claude-3.7-sonnet"

	// Tier 3: Arbiter & Conflict Judge
	ModelOpenAIo3Mini = "openai/o3-mini"
	ModelClaudeOpus   = "anthropic/claude-3-opus"
)

// ModelPricing defines USD costs per million tokens.
type ModelPricing struct {
	InputCostPer1M  float64
	OutputCostPer1M float64
}

var PricingTable = map[string]ModelPricing{
	ModelGeminiFlash:  {InputCostPer1M: 0.15, OutputCostPer1M: 0.60},
	ModelGPT4oMini:    {InputCostPer1M: 0.15, OutputCostPer1M: 0.60},
	ModelDeepSeekR1:   {InputCostPer1M: 0.55, OutputCostPer1M: 2.19},
	ModelClaudeSonnet: {InputCostPer1M: 3.00, OutputCostPer1M: 15.00},
	ModelOpenAIo3Mini: {InputCostPer1M: 1.10, OutputCostPer1M: 4.40},
	ModelClaudeOpus:   {InputCostPer1M: 15.00, OutputCostPer1M: 75.00},
}

// CalculateCostUSD computes the USD cost of an inference call given prompt/completion token counts.
func CalculateCostUSD(model string, promptTokens, completionTokens int) float64 {
	pricing, ok := PricingTable[model]
	if !ok {
		pricing = ModelPricing{InputCostPer1M: 1.00, OutputCostPer1M: 3.00}
	}
	inputCost := (float64(promptTokens) / 1_000_000.0) * pricing.InputCostPer1M
	outputCost := (float64(completionTokens) / 1_000_000.0) * pricing.OutputCostPer1M
	return inputCost + outputCost
}

// ChatMessage represents a prompt message in OpenAI format.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest represents the OpenRouter payload.
type ChatRequest struct {
	Model          string        `json:"model"`
	Messages       []ChatMessage `json:"messages"`
	Temperature    float64       `json:"temperature"`
	ResponseFormat *FormatSpec   `json:"response_format,omitempty"`
}

type FormatSpec struct {
	Type string `json:"type"` // "json_object"
}

// ChatResponse represents the OpenRouter completion response.
type ChatResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// Client communicates with the OpenRouter AI Gateway with local mock fallback.
type Client struct {
	apiKey     string
	httpClient *http.Client
	isMock     bool
}

// NewClient initializes the AI Client. If apiKey is empty, runs in deterministic offline mock mode.
func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 45 * time.Second,
		},
		isMock: apiKey == "",
	}
}

// Complete executes an LLM query, returns the generated text, and records a domain.ModelRun entry.
func (c *Client) Complete(ctx context.Context, tenantID, projectID, scanID uuid.UUID, role, model, systemPrompt, userPrompt string, jsonMode bool) (string, domain.ModelRun, error) {
	startTime := time.Now()

	// If in mock mode (offline / no API key), return simulated deterministic AI response
	if c.isMock {
		time.Sleep(10 * time.Millisecond)
		mockContent := c.generateMockResponse(role, userPrompt)
		latency := int(time.Since(startTime).Milliseconds())
		promptTokens := len(systemPrompt+userPrompt) / 4
		completionTokens := len(mockContent) / 4
		cost := CalculateCostUSD(model, promptTokens, completionTokens)

		modelRun := domain.ModelRun{
			ID:               uuid.New(),
			TenantID:         tenantID,
			ProjectID:        projectID,
			ScanID:           scanID,
			Role:             role,
			Provider:         "OpenRouter (Mock)",
			ModelName:        model,
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalCostUSD:     cost,
			LatencyMS:        latency,
			Status:           "SUCCESS",
			CreatedAt:        time.Now().UTC(),
		}
		return mockContent, modelRun, nil
	}

	// Live OpenRouter HTTP Call
	reqBody := ChatRequest{
		Model: model,
		Messages: []ChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.1,
	}
	if jsonMode {
		reqBody.ResponseFormat = &FormatSpec{Type: "json_object"}
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", domain.ModelRun{}, fmt.Errorf("failed to marshal chat request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", OpenRouterBaseURL, bytes.NewReader(jsonBytes))
	if err != nil {
		return "", domain.ModelRun{}, fmt.Errorf("failed to create http request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("HTTP-Referer", "https://codehound.dev")
	req.Header.Set("X-Title", "CodeHound Autonomous Verification")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", domain.ModelRun{}, fmt.Errorf("openrouter request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", domain.ModelRun{}, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", domain.ModelRun{}, fmt.Errorf("openrouter API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(bodyBytes, &chatResp); err != nil {
		return "", domain.ModelRun{}, fmt.Errorf("failed to decode openrouter response: %w", err)
	}

	content := ""
	if len(chatResp.Choices) > 0 {
		content = chatResp.Choices[0].Message.Content
	}

	latency := int(time.Since(startTime).Milliseconds())
	cost := CalculateCostUSD(model, chatResp.Usage.PromptTokens, chatResp.Usage.CompletionTokens)

	modelRun := domain.ModelRun{
		ID:               uuid.New(),
		TenantID:         tenantID,
		ProjectID:        projectID,
		ScanID:           scanID,
		Role:             role,
		Provider:         "OpenRouter",
		ModelName:        model,
		PromptTokens:     chatResp.Usage.PromptTokens,
		CompletionTokens: chatResp.Usage.CompletionTokens,
		TotalCostUSD:     cost,
		LatencyMS:        latency,
		Status:           "SUCCESS",
		CreatedAt:        time.Now().UTC(),
	}

	return content, modelRun, nil
}

func (c *Client) generateMockResponse(role, userPrompt string) string {
	switch role {
	case "TRIAGE_AGENT":
		return `{"is_clean": false, "anomaly_detected": true, "confidence": 0.88, "summary": "Found unvalidated user query formatting."}`
	case "LOGIC_BUG_HUNTER":
		return `{"has_bug": true, "bug_type": "OFF_BY_ONE", "file": "server.go", "line": 14, "description": "Array bounds check uses <= instead of < causing panic.", "suggested_input": "len=0"}`
	case "SECURITY_ANALYST":
		return `{"has_vulnerability": true, "cwe_id": "CWE-89", "title": "SQL Injection in User Query", "severity": "CRITICAL", "taint_path": ["req.Query", "fmt.Sprintf", "db.Query"]}`
	case "ARBITER_JUDGE":
		return `{"ruling": "CONFIRMED_VULNERABILITY", "consensus_score": 0.96, "final_severity": "CRITICAL", "justification": "Dual agents verified taint flow from unescaped user input into SQL database sink."}`
	default:
		return `{"status": "ANALYZED", "confidence": 0.95}`
	}
}
