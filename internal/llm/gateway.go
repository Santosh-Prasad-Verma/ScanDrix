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

// Gateway provides multi-model AI synthesis across Anthropic, OpenAI, and local endpoints.
type Gateway struct {
	anthropicKey   string
	openAIKey      string
	geminiKey      string
	localEndpoint  string
	httpClient     *http.Client
	circuitBreaker *CircuitBreaker
}

// NewGateway initializes the multi-provider LLM gateway.
func NewGateway(anthropicKey, openAIKey, geminiKey, localEndpoint string) *Gateway {
	return &Gateway{
		anthropicKey:   anthropicKey,
		openAIKey:      openAIKey,
		geminiKey:      geminiKey,
		localEndpoint:  localEndpoint,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
		circuitBreaker: NewCircuitBreaker(5, 30*time.Second),
	}
}

// AnalyzeDiff routes the pull request diff to an available AI provider and parses structured recommendations.
func (g *Gateway) AnalyzeDiff(ctx context.Context, req ReviewRequest) (*ReviewResponse, error) {
	prompt := buildSystemPrompt(req)
	var resp *ReviewResponse

	err := g.circuitBreaker.Execute(ctx, 2, func(callCtx context.Context) error {
		var callErr error
		if g.anthropicKey != "" {
			resp, callErr = g.callAnthropic(callCtx, prompt)
		} else if g.openAIKey != "" {
			resp, callErr = g.callOpenAI(callCtx, prompt)
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
