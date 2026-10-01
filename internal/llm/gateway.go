// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/llm/byok"
	"github.com/scandrix/backend/internal/llm/providers/kernel"
	"github.com/scandrix/backend/internal/prompts"
	"github.com/scandrix/backend/internal/scandrix/agentfirewall"
	"github.com/scandrix/backend/pkg/models"
)

// ═══════════════════════════════════════════════════════════════
// 1. LLM REQUEST & RESPONSE SCHEMAS
// ═══════════════════════════════════════════════════════════════

// ReviewRequest encapsulates contextual inputs provided to the AI model.
type ReviewRequest struct {
	WorkspaceID        uuid.UUID
	RepoNamespace      string
	PullTitle          string
	DiffContent        string
	CustomRules        string
	Model              string // User-selected model name (takes priority over defaults)
	BaseURL            string // Custom endpoint URL for OpenAI-compatible providers
	BYOKConfig         *byok.BYOKConfig
	BYOKOpenRouterKey  string
	BYOKAnthropicKey   string
	BYOKOpenAIKey      string
	BYOKGeminiKey      string
	BYOKDeepSeekKey    string
	BYOKCustomEndpoint string
}

// ReviewResponse contains structured findings returned by the AI provider.
type ReviewResponse struct {
	EngineVersion        string                 `json:"engine_version,omitempty"`
	ReviewVerdict        string                 `json:"review_verdict,omitempty"`
	RiskScore            int                    `json:"risk_score,omitempty"`
	Summary              string                 `json:"summary"`
	PositiveObservations []string               `json:"positive_observations,omitempty"`
	Statistics           map[string]int         `json:"statistics,omitempty"`
	Findings             []CandidateFindingJSON `json:"findings"`
	PolicyCompliance     map[string]any         `json:"policy_compliance,omitempty"`
}

// CandidateFindingJSON represents raw JSON output from the model.
type CandidateFindingJSON struct {
	ID                         string   `json:"id,omitempty"`
	FilePath                   string   `json:"file_path"`
	StartLine                  int      `json:"start_line"`
	EndLine                    int      `json:"end_line"`
	Severity                   string   `json:"severity"`
	Category                   string   `json:"category"`
	Title                      string   `json:"title"`
	Description                string   `json:"description"`
	Remediation                string   `json:"remediation,omitempty"`
	SuggestedDiff              string   `json:"suggested_diff,omitempty"`
	SuggestedFix               string   `json:"suggested_fix,omitempty"`
	Evidence                   string   `json:"evidence,omitempty"`
	Impact                     string   `json:"impact,omitempty"`
	ExploitScenario            string   `json:"exploit_scenario,omitempty"`
	RootCause                  string   `json:"root_cause,omitempty"`
	Preconditions              string   `json:"preconditions,omitempty"`
	ExistingMitigationsChecked string   `json:"existing_mitigations_checked,omitempty"`
	SecurityThreatTags         []string `json:"security_threat_tags,omitempty"`
	ConfidenceScore            float64  `json:"confidence_score,omitempty"`
	RuleViolated               string   `json:"rule_violated,omitempty"`
	PredictedFalsePositiveProb float64  `json:"predicted_false_positive_prob,omitempty"`
	RuleID                     string   `json:"rule_id,omitempty"`
	RuleTitle                  string   `json:"rule_title,omitempty"`
	Confidence                 string   `json:"confidence,omitempty"`
	Blocking                   bool     `json:"blocking,omitempty"`
	BlockingJustification      string   `json:"blocking_justification,omitempty"`
	References                 []string `json:"references,omitempty"`
}

// ChatMessage represents a single message in an LLM conversation.
type ChatMessage = kernel.ChatMessage

// ═══════════════════════════════════════════════════════════════
// 2. GATEWAY STRUCT & OPTIONS
// ═══════════════════════════════════════════════════════════════

// Gateway provides multi-model AI synthesis across cloud frontier providers.
type Gateway struct {
	anthropicKey     string
	openAIKey        string
	openAIBaseURL    string
	openAIModels     []string
	geminiKey        string
	deepseekKey      string
	bedrockToken     string
	bedrockRegion    string
	vertexToken      string
	vertexProject    string
	vertexRegion     string
	openrouterKey    string
	openrouterModels []string
	localEndpoint    string
	novitaKey        string
	mistralKey       string
	groqKey          string
	cohereKey        string
	xaiKey           string
	minimaxKey       string
	moonshotKey      string
	alibabaKey       string
	httpClient       *http.Client
	circuitBreaker   *CircuitBreaker
	breakers         *ProviderBreakerRegistry
	tokenLimiter     *TokenBudgetLimiter
	maxContextTokens int
	firewall         *agentfirewall.Firewall
	engine           *Engine
}

// GatewayOption configures optional credentials on Gateway.
type GatewayOption func(*Gateway)

func WithDeepSeek(key string) GatewayOption {
	return func(g *Gateway) { g.deepseekKey = key }
}

func WithNovita(key string) GatewayOption {
	return func(g *Gateway) { g.novitaKey = key }
}

func WithMistral(key string) GatewayOption {
	return func(g *Gateway) { g.mistralKey = key }
}

func WithGroq(key string) GatewayOption {
	return func(g *Gateway) { g.groqKey = key }
}

func WithCohere(key string) GatewayOption {
	return func(g *Gateway) { g.cohereKey = key }
}

func WithXAI(key string) GatewayOption {
	return func(g *Gateway) { g.xaiKey = key }
}

func WithMiniMax(key string) GatewayOption {
	return func(g *Gateway) { g.minimaxKey = key }
}

func WithMoonshot(key string) GatewayOption {
	return func(g *Gateway) { g.moonshotKey = key }
}

func WithAlibaba(key string) GatewayOption {
	return func(g *Gateway) { g.alibabaKey = key }
}

func WithOpenRouter(key string) GatewayOption {
	return func(g *Gateway) { g.openrouterKey = key }
}

func WithOpenRouterModels(models ...string) GatewayOption {
	return func(g *Gateway) {
		for _, m := range models {
			m = strings.TrimSpace(m)
			if m != "" {
				g.openrouterModels = append(g.openrouterModels, m)
			}
		}
	}
}

func WithOpenAIBaseURL(baseURL string) GatewayOption {
	return func(g *Gateway) { g.openAIBaseURL = baseURL }
}

func WithOpenAIModels(models ...string) GatewayOption {
	return func(g *Gateway) {
		for _, m := range models {
			m = strings.TrimSpace(m)
			if m != "" {
				g.openAIModels = append(g.openAIModels, m)
			}
		}
	}
}

func WithBedrock(region, token string) GatewayOption {
	return func(g *Gateway) { g.bedrockRegion = region; g.bedrockToken = token }
}

func WithVertex(project, region, token string) GatewayOption {
	return func(g *Gateway) {
		g.vertexProject = project
		g.vertexRegion = region
		g.vertexToken = token
	}
}

// Deprecated: WithVLLM is a legacy option maintained for backwards compatibility.
func WithVLLM(endpoint string) GatewayOption {
	return func(g *Gateway) {
		if g.localEndpoint == "" && endpoint != "" {
			g.localEndpoint = endpoint
		}
	}
}

func WithTokenBudgetLimiter(limiter *TokenBudgetLimiter) GatewayOption {
	return func(g *Gateway) { g.tokenLimiter = limiter }
}

func WithMaxContextTokens(tokens int) GatewayOption {
	return func(g *Gateway) { g.maxContextTokens = tokens }
}

func WithProviderBreakers(breakers *ProviderBreakerRegistry) GatewayOption {
	return func(g *Gateway) {
		if breakers != nil {
			g.breakers = breakers
			g.circuitBreaker = breakers.GetOrCreate("default")
		}
	}
}

func (g *Gateway) SetCustomModel(modelName string) {
	if strings.TrimSpace(modelName) != "" {
		g.openrouterModels = []string{strings.TrimSpace(modelName)}
		g.openAIModels = []string{strings.TrimSpace(modelName)}
	}
}

// NewGateway initializes the multi-provider LLM gateway.
func NewGateway(anthropicKey, openAIKey, geminiKey, localEndpoint string, opts ...GatewayOption) *Gateway {
	breakers := NewProviderBreakerRegistry(5, 30*time.Second)
	openAIBaseURL := ""
	if localEndpoint != "" {
		openAIBaseURL = localEndpoint
		if openAIKey == "" {
			openAIKey = "local"
		}
	}
	g := &Gateway{
		anthropicKey:  anthropicKey,
		openAIKey:     openAIKey,
		geminiKey:     geminiKey,
		localEndpoint: localEndpoint,
		openAIBaseURL: openAIBaseURL,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
		breakers:         breakers,
		circuitBreaker:   breakers.GetOrCreate("default"),
		maxContextTokens: 128_000,
		firewall:         agentfirewall.NewFirewall(),
		engine:           NewEngine(),
	}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// Engine returns the unified production AI orchestration engine.
func (g *Gateway) Engine() *Engine {
	if g.engine == nil {
		g.engine = NewEngine()
	}
	return g.engine
}

// ResolveDefaultSlot returns the active normalized model slot for default execution.
func (g *Gateway) ResolveDefaultSlot() *byok.NormalizedModel {
	return g.resolveDefaultSlot()
}

func (g *Gateway) resolveDefaultSlot() *byok.NormalizedModel {
	env := byok.LoadEnvLLMConfig()
	if g.openAIKey != "" {
		env.OpenAIKey = g.openAIKey
	}
	if g.openAIBaseURL != "" {
		env.OpenAIBaseURL = g.openAIBaseURL
	}
	if g.localEndpoint != "" && env.OpenAIBaseURL == "" {
		env.OpenAIBaseURL = g.localEndpoint
	}
	if env.OpenAIBaseURL != "" && env.OpenAIKey == "" {
		env.OpenAIKey = "local"
	}
	if g.anthropicKey != "" {
		env.AnthropicKey = g.anthropicKey
	}
	if g.geminiKey != "" {
		env.GeminiKey = g.geminiKey
	}
	if g.deepseekKey != "" {
		env.NovitaKey = g.deepseekKey
	}
	if g.openrouterKey != "" {
		env.OpenRouterKey = g.openrouterKey
	}
	if g.bedrockToken != "" {
		env.BedrockToken = g.bedrockToken
		env.BedrockRegion = g.bedrockRegion
	}
	if g.vertexToken != "" {
		env.VertexKey = g.vertexToken
		env.VertexProject = g.vertexProject
		env.VertexLocation = g.vertexRegion
	}
	if g.mistralKey != "" {
		env.MistralKey = g.mistralKey
	}
	if g.groqKey != "" {
		env.GroqKey = g.groqKey
	}
	if g.cohereKey != "" {
		env.CohereKey = g.cohereKey
	}
	return byok.ResolveManagedSlotFromConfig(env)
}

// ═══════════════════════════════════════════════════════════════
// 3. DIFF SYNTHESIS & REVIEW ORCHESTRATION
// ═══════════════════════════════════════════════════════════════

// AnalyzeDiff routes the pull request diff to an available AI provider and parses structured recommendations.
func (g *Gateway) AnalyzeDiff(ctx context.Context, req ReviewRequest) (*ReviewResponse, error) {
	if g.firewall != nil {
		if g.firewall.DetectPromptInjection(req.DiffContent) ||
			g.firewall.DetectPromptInjection(req.PullTitle) ||
			g.firewall.DetectPromptInjection(req.CustomRules) {
			slog.Warn("AI Gateway firewall blocked prompt injection attempt in diff payload",
				"repo", req.RepoNamespace,
				"title", req.PullTitle,
			)
			return nil, errors.New("security violation: prompt injection payload detected by firewall")
		}
	}

	prompt := buildSystemPrompt(req)

	// Preflight context window enforcement
	if g.maxContextTokens > 0 {
		if err := AssertPromptFitsContext(prompt, g.maxContextTokens); err != nil {
			return nil, fmt.Errorf("context window preflight failed: %w", err)
		}
	}

	// Token consumption and quota check
	if g.tokenLimiter != nil && req.WorkspaceID != uuid.Nil {
		estTokens := int64((len(prompt) + 3) / 4)
		isBYOK := req.BYOKConfig != nil || req.BYOKAnthropicKey != "" || req.BYOKOpenAIKey != "" || req.BYOKGeminiKey != "" || req.BYOKOpenRouterKey != "" || req.BYOKDeepSeekKey != ""
		if err := g.tokenLimiter.ConsumeTokensWithBYOK(req.WorkspaceID, estTokens, isBYOK); err != nil {
			return nil, fmt.Errorf("token quota limit exceeded: %w", err)
		}
	}

	// 1. Resolve active slot from stored BYOKConfig (organization parameters)
	var slot *byok.NormalizedModel
	if req.BYOKConfig != nil {
		slot, _, _ = byok.ResolveTaskSlot(req.BYOKConfig, byok.TaskCodeReview, "", "")
	}

	// 2. Fallback to transient BYOK config if legacy flat keys or request parameters provided
	if slot == nil {
		env := byok.LoadEnvLLMConfig()
		if req.BYOKOpenAIKey != "" {
			env.OpenAIKey = req.BYOKOpenAIKey
		}
		if req.BYOKAnthropicKey != "" {
			env.AnthropicKey = req.BYOKAnthropicKey
		}
		if req.BYOKGeminiKey != "" {
			env.GeminiKey = req.BYOKGeminiKey
		}
		if req.BYOKDeepSeekKey != "" {
			env.NovitaKey = req.BYOKDeepSeekKey
		}
		if req.BYOKOpenRouterKey != "" {
			env.OpenRouterKey = req.BYOKOpenRouterKey
		}
		if req.BaseURL != "" {
			env.OpenAIBaseURL = req.BaseURL
		}
		if req.Model != "" {
			env.DefaultModel = req.Model
		}
		if g.openAIKey != "" && env.OpenAIKey == "" {
			env.OpenAIKey = g.openAIKey
		}
		if g.openAIBaseURL != "" && env.OpenAIBaseURL == "" {
			env.OpenAIBaseURL = g.openAIBaseURL
		}
		if g.localEndpoint != "" && env.OpenAIBaseURL == "" {
			env.OpenAIBaseURL = g.localEndpoint
		}
		if env.OpenAIBaseURL != "" && env.OpenAIKey == "" {
			env.OpenAIKey = "local"
		}
		if g.anthropicKey != "" && env.AnthropicKey == "" {
			env.AnthropicKey = g.anthropicKey
		}
		if g.geminiKey != "" && env.GeminiKey == "" {
			env.GeminiKey = g.geminiKey
		}
		if g.mistralKey != "" && env.MistralKey == "" {
			env.MistralKey = g.mistralKey
		}
		if g.groqKey != "" && env.GroqKey == "" {
			env.GroqKey = g.groqKey
		}
		if g.cohereKey != "" && env.CohereKey == "" {
			env.CohereKey = g.cohereKey
		}
		slot = byok.ResolveManagedSlotFromConfig(env)
	}

	// 3. Fallback to system managed default slot
	if slot == nil {
		slot = g.resolveDefaultSlot()
	}
	if slot != nil && req.Model != "" {
		slot.Model = req.Model
	}

	var reviewResp ReviewResponse
	userPrompt := fmt.Sprintf("Pull Request Title: %s\nRepository: %s\n\nDiff Content:\n%s", req.PullTitle, req.RepoNamespace, req.DiffContent)

	callRes, err := RunStructuredReviewCall(ctx, StructuredReviewCallParams{
		BaseReviewCallParams: BaseReviewCallParams{
			Slot:           slot,
			System:         prompt,
			User:           userPrompt,
			RunName:        "scandrix-diff-analysis",
			OrganizationID: req.WorkspaceID.String(),
		},
		Target: &reviewResp,
	})
	if err != nil {
		// If strict unmarshal failed, attempt fallback recovery on raw response text
		if callRes != nil && callRes.Text != "" {
			if parsed, parseErr := parseStructuredJSON(callRes.Text); parseErr == nil && parsed != nil {
				return parsed, nil
			}
		}
		return nil, fmt.Errorf("ai review synthesis failed: %w", err)
	}

	return &reviewResp, nil
}

// ═══════════════════════════════════════════════════════════════
// 4. CHAT GENERATION
// ═══════════════════════════════════════════════════════════════

// GenerateChatResponse routes conversational queries to the appropriate chat model.
func (g *Gateway) GenerateChatResponse(ctx context.Context, systemPrompt string, history []ChatMessage, userMessage string) (string, error) {
	return g.GenerateChatResponseWithModel(ctx, "", systemPrompt, history, userMessage)
}

// GenerateChatResponseWithModel routes conversational queries using a specific model override.
func (g *Gateway) GenerateChatResponseWithModel(ctx context.Context, modelName string, systemPrompt string, history []ChatMessage, userMessage string) (string, error) {
	var slot *byok.NormalizedModel
	if modelName != "" {
		for _, p := range kernel.DefaultRegistry.List() {
			caps := p.Capabilities(modelName)
			if caps.ToolCalling == "native" || caps.StructuredOutput != "none" {
				slot = &byok.NormalizedModel{
					Provider: byok.BYOKProvider(p.ID()),
					Model:    modelName,
				}
				break
			}
		}
	}
	if slot == nil {
		slot = g.resolveDefaultSlot()
	}

	var userBuilder strings.Builder
	for _, h := range history {
		userBuilder.WriteString(fmt.Sprintf("%s: %s\n", h.Role, h.Content))
	}
	userBuilder.WriteString(fmt.Sprintf("user: %s\n", userMessage))

	res, err := RunTextReviewCall(ctx, TextReviewCallParams{
		BaseReviewCallParams: BaseReviewCallParams{
			Slot:                 slot,
			System:               systemPrompt,
			User:                 userBuilder.String(),
			RunName:              "scandrix-chat",
			DefaultModelOverride: modelName,
		},
	})
	if err != nil {
		return "", err
	}
	return res.Text, nil
}

// ═══════════════════════════════════════════════════════════════
// 5. PROMPT SANITIZATION & ASSEMBLY
// ═══════════════════════════════════════════════════════════════

func sanitizeTag(input, tag string) string {
	closeTag := "</" + tag + ">"
	return strings.ReplaceAll(input, closeTag, "[ESCAPED_"+tag+"]")
}

func buildSystemPrompt(req ReviewRequest) string {
	var sb strings.Builder

	sb.WriteString("# ═══════════════════════════════════════════════════════════════════════════════\n")
	sb.WriteString("# SCANDRIX AI — AUTONOMOUS PR REVIEW ENGINE  ·  SYSTEM DIRECTIVE v3.0\n")
	sb.WriteString("# ═══════════════════════════════════════════════════════════════════════════════\n\n")

	sb.WriteString(prompts.FoundationScopeHierarchy)
	sb.WriteString("\n\n")
	sb.WriteString(prompts.FoundationPromptInjectionGuardrails)
	sb.WriteString("\n\n")
	sb.WriteString(prompts.FoundationEvidenceGate)
	sb.WriteString("\n\n")
	sb.WriteString(prompts.FoundationConfidenceCalibration)
	sb.WriteString("\n\n")
	sb.WriteString(prompts.FoundationBlockingPolicy)
	sb.WriteString("\n\n")
	sb.WriteString(prompts.FoundationSignalToNoise)
	sb.WriteString("\n\n")
	sb.WriteString(prompts.FoundationRootCauseDedup)
	sb.WriteString("\n\n")
	sb.WriteString(prompts.FoundationContextDiscipline)
	sb.WriteString("\n\n")
	sb.WriteString(prompts.FoundationPositiveSecurity)
	sb.WriteString("\n\n")
	sb.WriteString(prompts.FoundationRemediationSafety)
	sb.WriteString("\n\n")

	sb.WriteString(`# ── OUTPUT SCHEMA CONTRACT (STRICT — v3.0) ─────────────────────────
Respond ONLY with valid JSON conforming to this canonical contract:
{
  "engine_version": "3.0",
  "review_verdict": "APPROVE | COMMENT_ONLY | REQUEST_CHANGES",
  "risk_score": <int 0-100>,
  "summary": "<high-level executive summary of the review>",
  "positive_observations": ["<security/quality improvements observed>"],
  "statistics": {
    "files_analyzed": <int>,
    "total_findings": <int>,
    "blocking_findings": <int>,
    "critical": <int>,
    "high": <int>,
    "medium": <int>,
    "low": <int>,
    "info": <int>
  },
  "findings": [
    {{CANONICAL_FINDING_SCHEMA}}
  ],
  "policy_compliance": {
    "custom_policies_evaluated": <boolean>,
    "violations": [
      {
        "policy": "<policy name>",
        "violation": "<explanation>",
        "finding_ids": ["<PREFIX-NNN>"]
      }
    ]
  }
}
`)
	sb.WriteString("\n\n")
	sb.WriteString(prompts.FoundationQualityGate)
	sb.WriteString("\n\n")

	sb.WriteString("# ── PULL REQUEST ARTIFACTS (UNTRUSTED L1 INPUT) ────────────────────\n\n")
	sb.WriteString("<pull_request_context>\n")
	sb.WriteString("<repository>" + sanitizeTag(req.RepoNamespace, "repository") + "</repository>\n")
	sb.WriteString("<title>" + sanitizeTag(req.PullTitle, "title") + "</title>\n")
	sb.WriteString("</pull_request_context>\n\n")

	if strings.TrimSpace(req.CustomRules) != "" {
		sb.WriteString("<custom_policy_directives>\n")
		sb.WriteString(sanitizeTag(req.CustomRules, "custom_policy_directives"))
		sb.WriteString("\n</custom_policy_directives>\n\n")
	}

	if strings.TrimSpace(req.DiffContent) != "" {
		sb.WriteString("<diff_content>\n")
		sb.WriteString(sanitizeTag(req.DiffContent, "diff_content"))
		sb.WriteString("\n</diff_content>\n\n")
	}

	return prompts.ApplyFoundations(sb.String())
}

// ═══════════════════════════════════════════════════════════════
// 6. STRUCTURED JSON PARSER (Fallback recovery)
// ═══════════════════════════════════════════════════════════════

func parseStructuredJSON(raw string) (*ReviewResponse, error) {
	cleaned := strings.TrimSpace(raw)
	if start := strings.Index(cleaned, "```json"); start != -1 {
		rest := cleaned[start+7:]
		if end := strings.Index(rest, "```"); end != -1 {
			cleaned = strings.TrimSpace(rest[:end])
		}
	} else if start := strings.Index(cleaned, "```"); start != -1 {
		rest := cleaned[start+3:]
		if end := strings.Index(rest, "```"); end != -1 {
			cleaned = strings.TrimSpace(rest[:end])
		}
	} else if start := strings.Index(cleaned, "{"); start != -1 {
		if end := strings.LastIndex(cleaned, "}"); end != -1 && end > start {
			cleaned = strings.TrimSpace(cleaned[start : end+1])
		}
	}

	var strictRes ReviewResponse
	if err := json.Unmarshal([]byte(cleaned), &strictRes); err == nil && len(strictRes.Findings) > 0 {
		return &strictRes, nil
	}

	var rawMap map[string]any
	if err := json.Unmarshal([]byte(cleaned), &rawMap); err == nil {
		res := &ReviewResponse{
			EngineVersion: "3.0",
			ReviewVerdict: "COMMENT_ONLY",
		}
		if sumVal, ok := rawMap["summary"]; ok {
			if s, ok := sumVal.(string); ok {
				res.Summary = s
			}
		}
		if v, ok := rawMap["review_verdict"].(string); ok {
			res.ReviewVerdict = v
		}

		var rawFindings []any
		for _, key := range []string{"findings", "vulnerabilities", "issues", "results"} {
			if list, ok := rawMap[key].([]any); ok && len(list) > 0 {
				rawFindings = list
				break
			}
		}

		for _, item := range rawFindings {
			if fMap, ok := item.(map[string]any); ok {
				f := CandidateFindingJSON{
					FilePath:    getString(fMap, "file_path"),
					Title:       getString(fMap, "title"),
					Description: getString(fMap, "description"),
					Severity:    getString(fMap, "severity"),
					Category:    getString(fMap, "category"),
				}
				if f.Severity == "" {
					f.Severity = "MEDIUM"
				}
				if f.Category == "" {
					f.Category = "SECURITY"
				}
				res.Findings = append(res.Findings, f)
			}
		}
		return res, nil
	}

	return nil, errors.New("failed to parse structured JSON from LLM output")
}

func getString(m map[string]any, k string) string {
	if v, ok := m[k].(string); ok {
		return v
	}
	return ""
}

// ConvertToModelFindings translates parsed review findings into persistent domain findings.
func (r *ReviewResponse) ConvertToModelFindings(reviewID uuid.UUID, ws models.Workspace) []models.CodeFinding {
	findings := make([]models.CodeFinding, 0, len(r.Findings))
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

		cat := strings.ToUpper(f.Category)
		if cat == "" {
			cat = "SECURITY"
		}

		rem := f.Remediation
		if rem == "" {
			rem = f.SuggestedFix
		}

		findings = append(findings, models.CodeFinding{
			ID:            uuid.New(),
			WorkspaceID:   ws.ID,
			ReviewID:      reviewID,
			FilePath:      f.FilePath,
			StartLine:     f.StartLine,
			EndLine:       f.EndLine,
			Severity:      sev,
			Category:      cat,
			Title:         f.Title,
			Description:   f.Description,
			Remediation:   rem,
			SuggestedDiff: f.SuggestedDiff,
			CreatedAt:     time.Now().UTC(),
		})
	}
	return findings
}
