package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/scandrix/backend/pkg/models"
)

// ReviewRequest encapsulates contextual inputs provided to the AI model.
type ReviewRequest struct {
	RepoNamespace string
	PullTitle     string
	DiffContent   string
	CustomRules   string
}

// ReviewResponse contains structured findings returned by the AI provider.
type ReviewResponse struct {
	Summary  string                 `json:"summary"`
	Findings []CandidateFindingJSON `json:"findings"`
}

// CandidateFindingJSON represents raw JSON output from the model.
type CandidateFindingJSON struct {
	FilePath      string `json:"file_path"`
	StartLine     int    `json:"start_line"`
	EndLine       int    `json:"end_line"`
	Severity      string `json:"severity"`
	Category      string `json:"category"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	Remediation   string `json:"remediation"`
	SuggestedDiff string `json:"suggested_diff"`
}

// Gateway provides multi-model AI synthesis across Anthropic, OpenAI, DeepSeek, Bedrock, Vertex, and local endpoints.
type Gateway struct {
	anthropicKey   string
	openAIKey      string
	geminiKey      string
	deepseekKey    string
	bedrockToken   string
	bedrockRegion  string
	vertexToken    string
	vertexProject  string
	vertexRegion   string
	openrouterKey    string
	openrouterModels []string
	ollamaEndpoint   string
	vllmEndpoint     string
	localEndpoint    string
	httpClient       *http.Client
	circuitBreaker   *CircuitBreaker
}

// GatewayOption configures optional credentials on Gateway.
type GatewayOption func(*Gateway)

func WithDeepSeek(key string) GatewayOption {
	return func(g *Gateway) { g.deepseekKey = key }
}

func WithOpenRouter(key string) GatewayOption {
	return func(g *Gateway) { g.openrouterKey = key }
}

func WithOpenRouterModels(models ...string) GatewayOption {
	return func(g *Gateway) {
		clean := make([]string, 0, len(models))
		for _, m := range models {
			if strings.TrimSpace(m) != "" {
				clean = append(clean, strings.TrimSpace(m))
			}
		}
		g.openrouterModels = clean
	}
}

func WithBedrock(region, token string) GatewayOption {
	return func(g *Gateway) { g.bedrockRegion = region; g.bedrockToken = token }
}

func WithVertex(project, region, token string) GatewayOption {
	return func(g *Gateway) { g.vertexProject = project; g.vertexRegion = region; g.vertexToken = token }
}

func WithOllama(endpoint string) GatewayOption {
	return func(g *Gateway) { g.ollamaEndpoint = endpoint }
}

func WithVLLM(endpoint string) GatewayOption {
	return func(g *Gateway) { g.vllmEndpoint = endpoint }
}

// NewGateway initializes the multi-provider LLM gateway.
func NewGateway(anthropicKey, openAIKey, geminiKey, localEndpoint string, opts ...GatewayOption) *Gateway {
	g := &Gateway{
		anthropicKey:   anthropicKey,
		openAIKey:      openAIKey,
		geminiKey:      geminiKey,
		localEndpoint:  localEndpoint,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
		circuitBreaker: NewCircuitBreaker(5, 30*time.Second),
	}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// AnalyzeDiff routes the pull request diff to an available AI provider and parses structured recommendations.
func (g *Gateway) AnalyzeDiff(ctx context.Context, req ReviewRequest) (*ReviewResponse, error) {
	prompt := buildSystemPrompt(req)
	var resp *ReviewResponse

	err := g.circuitBreaker.Execute(ctx, 2, func(callCtx context.Context) error {
		var callErr error
		if g.openrouterKey != "" {
			resp, callErr = g.callOpenRouter(callCtx, prompt)
		} else if g.anthropicKey != "" {
			resp, callErr = g.callAnthropic(callCtx, prompt)
		} else if g.openAIKey != "" {
			resp, callErr = g.callOpenAI(callCtx, prompt)
		} else if g.deepseekKey != "" {
			resp, callErr = g.callDeepSeek(callCtx, prompt)
		} else if g.vertexToken != "" {
			resp, callErr = g.callVertex(callCtx, prompt)
		} else if g.bedrockToken != "" {
			resp, callErr = g.callBedrock(callCtx, prompt)
		} else if g.ollamaEndpoint != "" {
			resp, callErr = g.callOllama(callCtx, prompt)
		} else if g.vllmEndpoint != "" {
			resp, callErr = g.callOpenAICompatible(callCtx, g.vllmEndpoint+"/v1", prompt)
		} else if g.localEndpoint != "" {
			resp, callErr = g.callOpenAICompatible(callCtx, g.localEndpoint, prompt)
		} else {
			return fmt.Errorf("no valid AI provider credentials configured")
		}
		return callErr
	})

	if err != nil {
		return nil, fmt.Errorf("ai review synthesis failed: %w", err)
	}

	return resp, nil
}

func buildSystemPrompt(req ReviewRequest) string {
	return fmt.Sprintf(`You are an elite Staff Software Engineer conducting an automated pull request review.
Repository: %s
Pull Request Title: %s

Inspect the provided unified diff and identify real software bugs, race conditions, security vulnerabilities, and architectural defects.
Do NOT flag stylistic preferences or trivial formatting.

Return your analysis strictly as valid JSON matching this schema:
{
  "summary": "High-level summary of changes and review verdict",
  "findings": [
    {
      "file_path": "path/to/file.ext",
      "start_line": 10,
      "end_line": 15,
      "severity": "CRITICAL|HIGH|MEDIUM|LOW|INFO",
      "category": "SECURITY|BUG|PERFORMANCE|CORRECTNESS",
      "title": "Clear concise summary of the issue",
      "description": "Technical root cause explanation",
      "remediation": "How to remediate the defect",
      "suggested_diff": "Optional code fix replacement"
    }
  ]
}

Custom Policy Directives:
%s

Unified Git Diff:
%s
`, req.RepoNamespace, req.PullTitle, req.CustomRules, req.DiffContent)
}

func (g *Gateway) callAnthropic(ctx context.Context, prompt string) (*ReviewResponse, error) {
	reqBody := map[string]any{
		"model":      "claude-3-5-sonnet-20241022",
		"max_tokens": 4096,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	jsonBytes, _ := json.Marshal(reqBody)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.anthropic.com/v1/messages", bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", g.anthropicKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := g.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var anthropicResp struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &anthropicResp); err != nil || len(anthropicResp.Content) == 0 {
		return nil, fmt.Errorf("failed parsing anthropic response: %w", err)
	}

	return parseStructuredJSON(anthropicResp.Content[0].Text)
}

func (g *Gateway) callOpenAI(ctx context.Context, prompt string) (*ReviewResponse, error) {
	return g.callOpenAICompatible(ctx, "https://api.openai.com/v1", prompt)
}

func (g *Gateway) callOpenAICompatible(ctx context.Context, baseURL, prompt string) (*ReviewResponse, error) {
	reqBody := map[string]any{
		"model": "gpt-4o",
		"messages": []map[string]string{
			{"role": "system", "content": "You are a code review analysis engine that exclusively outputs structured JSON."},
			{"role": "user", "content": prompt},
		},
		"response_format": map[string]string{"type": "json_object"},
	}
	jsonBytes, _ := json.Marshal(reqBody)

	url := strings.TrimRight(baseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if g.openAIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+g.openAIKey)
	}

	resp, err := g.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var openAIResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &openAIResp); err != nil || len(openAIResp.Choices) == 0 {
		return nil, fmt.Errorf("failed parsing openai response: %w", err)
	}

	return parseStructuredJSON(openAIResp.Choices[0].Message.Content)
}

func (g *Gateway) callDeepSeek(ctx context.Context, prompt string) (*ReviewResponse, error) {
	reqBody := map[string]any{
		"model": "deepseek-chat",
		"messages": []map[string]string{
			{"role": "system", "content": "You are a code review analysis engine that exclusively outputs structured JSON."},
			{"role": "user", "content": prompt},
		},
		"response_format": map[string]string{"type": "json_object"},
	}
	jsonBytes, _ := json.Marshal(reqBody)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.deepseek.com/chat/completions", bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+g.deepseekKey)

	resp, err := g.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("deepseek request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("deepseek error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var deepseekResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &deepseekResp); err != nil || len(deepseekResp.Choices) == 0 {
		return nil, fmt.Errorf("failed parsing deepseek response: %w", err)
	}
	return parseStructuredJSON(deepseekResp.Choices[0].Message.Content)
}

func (g *Gateway) callBedrock(ctx context.Context, prompt string) (*ReviewResponse, error) {
	region := g.bedrockRegion
	if region == "" {
		region = "us-east-1"
	}
	reqBody := map[string]any{
		"messages": []map[string]any{
			{
				"role": "user",
				"content": []map[string]string{
					{"text": prompt},
				},
			},
		},
		"inferenceConfig": map[string]any{
			"maxTokens":   4096,
			"temperature": 0.2,
		},
	}
	jsonBytes, _ := json.Marshal(reqBody)

	url := fmt.Sprintf("https://bedrock-runtime.%s.amazonaws.com/model/anthropic.claude-3-5-sonnet-20241022-v2:0/converse", region)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+g.bedrockToken)

	resp, err := g.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("bedrock request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bedrock error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var bedrockResp struct {
		Output struct {
			Message struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"message"`
		} `json:"output"`
	}
	if err := json.Unmarshal(body, &bedrockResp); err != nil || len(bedrockResp.Output.Message.Content) == 0 {
		return nil, fmt.Errorf("failed parsing bedrock response: %w", err)
	}
	return parseStructuredJSON(bedrockResp.Output.Message.Content[0].Text)
}

func (g *Gateway) callVertex(ctx context.Context, prompt string) (*ReviewResponse, error) {
	region := g.vertexRegion
	if region == "" {
		region = "us-central1"
	}
	project := g.vertexProject
	if project == "" {
		project = "scandrix-prod"
	}

	reqBody := map[string]any{
		"contents": []map[string]any{
			{
				"role": "user",
				"parts": []map[string]string{
					{"text": prompt},
				},
			},
		},
		"generationConfig": map[string]any{
			"temperature":      0.2,
			"responseMimeType": "application/json",
		},
	}
	jsonBytes, _ := json.Marshal(reqBody)

	url := fmt.Sprintf("https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/google/models/gemini-2.5-pro:generateContent", region, project, region)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+g.vertexToken)

	resp, err := g.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("vertex request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vertex error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var vertexResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(body, &vertexResp); err != nil || len(vertexResp.Candidates) == 0 || len(vertexResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("failed parsing vertex response: %w", err)
	}
	return parseStructuredJSON(vertexResp.Candidates[0].Content.Parts[0].Text)
}

func (g *Gateway) callOllama(ctx context.Context, prompt string) (*ReviewResponse, error) {
	reqBody := map[string]any{
		"model": "deepseek-r1:70b",
		"messages": []map[string]string{
			{"role": "system", "content": "You are a code review analysis engine that exclusively outputs structured JSON."},
			{"role": "user", "content": prompt},
		},
		"format": "json",
		"stream": false,
	}
	jsonBytes, _ := json.Marshal(reqBody)

	url := strings.TrimRight(g.ollamaEndpoint, "/") + "/api/chat"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var ollamaResp struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		return nil, fmt.Errorf("failed parsing ollama response: %w", err)
	}
	return parseStructuredJSON(ollamaResp.Message.Content)
}

func (g *Gateway) callOpenRouter(ctx context.Context, prompt string) (*ReviewResponse, error) {
	modelsToTry := g.openrouterModels
	if len(modelsToTry) == 0 {
		modelsToTry = []string{
			"stealth/ox-alpha",
			"nvidia/nemotron-3-ultra-550b-a55b:free",
			"minimax/minimax-m3:free",
			"thinkingmachines/inkling:free",
		}
	}

	var lastErr error
	for _, modelName := range modelsToTry {
		if strings.TrimSpace(modelName) == "" {
			continue
		}
		reqBody := map[string]any{
			"model": modelName,
			"messages": []map[string]string{
				{"role": "system", "content": "You are a code review analysis engine that exclusively outputs structured JSON."},
				{"role": "user", "content": prompt},
			},
			"response_format": map[string]string{"type": "json_object"},
			"stream":          false,
		}
		jsonBytes, _ := json.Marshal(reqBody)

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(jsonBytes))
		if err != nil {
			lastErr = err
			continue
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+g.openrouterKey)
		httpReq.Header.Set("HTTP-Referer", "https://scandrix.dev")
		httpReq.Header.Set("X-Title", "ScanDrix")

		resp, err := g.httpClient.Do(httpReq)
		if err != nil {
			lastErr = fmt.Errorf("openrouter request for model %s failed: %w", modelName, err)
			continue
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("openrouter model %s error (HTTP %d): %s", modelName, resp.StatusCode, string(body))
			continue
		}

		var openRouterResp struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(body, &openRouterResp); err != nil || len(openRouterResp.Choices) == 0 {
			lastErr = fmt.Errorf("failed parsing openrouter response for model %s: %w", modelName, err)
			continue
		}

		parsed, err := parseStructuredJSON(openRouterResp.Choices[0].Message.Content)
		if err != nil {
			lastErr = fmt.Errorf("failed parsing JSON findings for model %s: %w", modelName, err)
			continue
		}
		return parsed, nil
	}

	return nil, fmt.Errorf("all openrouter models in fallback chain failed: %w", lastErr)
}

func parseStructuredJSON(raw string) (*ReviewResponse, error) {
	cleaned := strings.TrimSpace(raw)
	if strings.HasPrefix(cleaned, "```json") {
		cleaned = strings.TrimPrefix(cleaned, "```json")
		cleaned = strings.TrimSuffix(cleaned, "```")
	} else if strings.HasPrefix(cleaned, "```") {
		cleaned = strings.TrimPrefix(cleaned, "```")
		cleaned = strings.TrimSuffix(cleaned, "```")
	}
	cleaned = strings.TrimSpace(cleaned)

	var res ReviewResponse
	if err := json.Unmarshal([]byte(cleaned), &res); err != nil {
		return nil, fmt.Errorf("failed decoding AI structured JSON: %w (raw content: %s)", err, raw)
	}
	return &res, nil
}

// ConvertToModelFindings converts candidate JSON findings to database models.
func (r *ReviewResponse) ConvertToModelFindings(reviewID, workspaceID models.Workspace) []models.CodeFinding {
	var results []models.CodeFinding
	for _, f := range r.Findings {
		sev := models.SeverityMedium
		switch strings.ToUpper(f.Severity) {
		case "CRITICAL":
			sev = models.SeverityCritical
		case "HIGH":
			sev = models.SeverityHigh
		case "LOW":
			sev = models.SeverityLow
		case "INFO":
			sev = models.SeverityInfo
		}

		results = append(results, models.CodeFinding{
			ReviewID:      reviewID.ID,
			WorkspaceID:   workspaceID.ID,
			FilePath:      f.FilePath,
			StartLine:     f.StartLine,
			EndLine:       f.EndLine,
			Severity:      sev,
			Category:      f.Category,
			Title:         f.Title,
			Description:   f.Description,
			Remediation:   f.Remediation,
			SuggestedDiff: f.SuggestedDiff,
		})
	}
	return results
}
