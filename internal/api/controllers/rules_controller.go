package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/drixy"
	"github.com/scandrix/backend/internal/llm"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/pkg/models"
)

// RulesRepository abstracts rule persistence operations adhering to clean architecture.
type RulesRepository interface {
	ListReviewRules(ctx context.Context, wsID uuid.UUID) ([]models.ReviewRule, error)
	CreateReviewRule(ctx context.Context, rule *models.ReviewRule) error
	DeleteReviewRule(ctx context.Context, wsID, ruleID uuid.UUID) error
}

// RulesController manages security rules, database persistence, and Drixy AI rule synthesis.
type RulesController struct {
	evaluator  *rules.Evaluator
	repo       RulesRepository
	llmGateway *llm.Gateway
}

// NewRulesController initializes the rules controller.
func NewRulesController(evaluator *rules.Evaluator, repo ...RulesRepository) *RulesController {
	c := &RulesController{evaluator: evaluator}
	if len(repo) > 0 && !isNilInterface(repo[0]) {
		c.repo = repo[0]
	}
	return c
}

// SetRepository configures the database repository.
func (c *RulesController) SetRepository(repo RulesRepository) {
	if isNilInterface(repo) {
		repo = nil
	}
	c.repo = repo
}

// SetLLMGateway configures the AI gateway for rule generation.
func (c *RulesController) SetLLMGateway(gw *llm.Gateway) {
	c.llmGateway = gw
}

// Routes mounts rules endpoints.
func (c *RulesController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/catalog", c.handleGetCatalog)
	r.Post("/test", c.handleTestRule)
	r.Post("/generate", c.handleGenerateRule)
	r.Get("/", c.handleListRules)
	r.Post("/", c.handleCreateRule)
	r.Delete("/{ruleID}", c.handleDeleteRule)

	return r
}

func (c *RulesController) handleGetCatalog(w http.ResponseWriter, r *http.Request) {
	catalog := rules.DefaultCatalog()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(catalog)
}

func (c *RulesController) handleListRules(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	res := make([]dtos.RuleResponse, 0)
	if c.repo != nil {
		stored, err := c.repo.ListReviewRules(r.Context(), wsID)
		if err == nil {
			for _, sr := range stored {
				res = append(res, dtos.RuleResponse{
					ID:          sr.ID,
					WorkspaceID: sr.WorkspaceID,
					Name:        sr.Name,
					PathPattern: sr.PathPattern,
					RegexRule:   sr.RuleContent,
					Severity:    sr.Severity,
					Category:    "CUSTOM",
					Description: sr.Description,
					Remediation: sr.Remediation,
					Enabled:     sr.IsEnabled,
				})
			}
		}
	}

	// Also merge default catalog if query param include_catalog=true or if no custom rules exist
	includeCatalog := r.URL.Query().Get("include_catalog") == "true"
	if includeCatalog || len(res) == 0 {
		catalog := rules.DefaultCatalog()
		for _, cr := range catalog {
			res = append(res, dtos.RuleResponse{
				ID:          cr.ID,
				WorkspaceID: wsID,
				Name:        cr.Name,
				PathPattern: cr.PathPattern,
				RegexRule:   cr.RegexRule,
				Severity:    cr.Severity,
				Category:    cr.Category,
				Description: cr.Description,
				Remediation: cr.Remediation,
				Enabled:     true,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (c *RulesController) handleTestRule(w http.ResponseWriter, r *http.Request) {
	var req dtos.TestRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RegexRule == "" {
		http.Error(w, `{"error":"regex_rule is required"}`, http.StatusBadRequest)
		return
	}

	re, err := regexp.Compile(req.RegexRule)
	if err != nil {
		http.Error(w, `{"error":"invalid regex syntax"}`, http.StatusBadRequest)
		return
	}

	lines := strings.Split(req.CodeSnippet, "\n")
	matchedLines := make([]int, 0)
	snippets := make([]string, 0)

	for idx, line := range lines {
		if re.MatchString(line) {
			matchedLines = append(matchedLines, idx+1)
			snippets = append(snippets, strings.TrimSpace(line))
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dtos.TestRuleResponse{
		Matched:       len(matchedLines) > 0,
		MatchLines:    matchedLines,
		MatchSnippets: snippets,
	})
}

func (c *RulesController) handleCreateRule(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	var req dtos.CreateRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || req.RegexRule == "" {
		http.Error(w, `{"error":"name and regex_rule are required"}`, http.StatusBadRequest)
		return
	}

	ruleID := uuid.New()
	rule := models.ReviewRule{
		ID:          ruleID,
		WorkspaceID: wsID,
		Name:        req.Name,
		Description: req.Description,
		Severity:    req.Severity,
		RuleType:    "REGEX",
		RuleContent: req.RegexRule,
		PathPattern: req.PathPattern,
		Remediation: req.Remediation,
		IsEnabled:   true,
	}

	if c.repo != nil {
		if err := c.repo.CreateReviewRule(r.Context(), &rule); err != nil {
			slog.Error("failed creating rule in database", "error", err, "workspace_id", wsID)
			http.Error(w, `{"error":"failed to create review rule"}`, http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(dtos.RuleResponse{
		ID:          ruleID,
		WorkspaceID: wsID,
		Name:        req.Name,
		PathPattern: req.PathPattern,
		RegexRule:   req.RegexRule,
		Severity:    req.Severity,
		Category:    req.Category,
		Description: req.Description,
		Remediation: req.Remediation,
		Enabled:     req.Enabled,
	})
}

func (c *RulesController) handleDeleteRule(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	ruleIDStr := chi.URLParam(r, "ruleID")
	ruleID, err := uuid.Parse(ruleIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid rule ID format"}`, http.StatusBadRequest)
		return
	}

	if c.repo != nil {
		if err := c.repo.DeleteReviewRule(r.Context(), wsID, ruleID); err != nil {
			http.Error(w, `{"error":"failed deleting rule"}`, http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "deleted", "id": ruleID.String()})
}

func (c *RulesController) handleGenerateRule(w http.ResponseWriter, r *http.Request) {
	var req dtos.GenerateRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Prompt) == "" {
		http.Error(w, `{"error":"prompt is required"}`, http.StatusBadRequest)
		return
	}

	if c.llmGateway == nil {
		http.Error(w, `{"error":"LLM gateway is uninitialized or not configured for rule generation"}`, http.StatusServiceUnavailable)
		return
	}

	systemPrompt := fmt.Sprintf(`You are %s (ScanDrix Autonomous Review Agent & Rule Synthesizer).
Your task is to take a developer's natural language policy requirement and synthesize a production-grade static analysis rule definition in strict JSON format.

JSON Schema:
{
  "name": "<concise rule title>",
  "regex_rule": "<valid Go/RE2 regular expression pattern to detect the issue>",
  "path_pattern": "<glob pattern for target files, e.g. **/*.go, **/*.ts, *>",
  "severity": "<CRITICAL | HIGH | MEDIUM | LOW | INFO>",
  "category": "<SECURITY | QUALITY | PERFORMANCE | COMPLIANCE>",
  "description": "<technical explanation of the defect and risk>",
  "remediation": "<concrete steps and code advice to resolve it>",
  "explanation": "<rationale behind this rule's detection pattern>"
}

Output ONLY valid JSON. No markdown fences, no conversational prose.`, drixy.Name)

	userMsg := fmt.Sprintf("Synthesize a Drixy rule for: %s", req.Prompt)
	if req.CodeContext != "" {
		userMsg += fmt.Sprintf("\nTarget Code Context:\n%s", req.CodeContext)
	}
	if req.Language != "" {
		userMsg += fmt.Sprintf("\nLanguage: %s", req.Language)
	}

	aiResp, err := c.llmGateway.GenerateChatResponse(r.Context(), systemPrompt, nil, userMsg)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed generating rule via LLM: %v"}`, err), http.StatusBadGateway)
		return
	}

	// Clean and parse JSON response
	cleaned := strings.TrimSpace(aiResp)
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
	}

	var resp dtos.GenerateRuleResponse
	if err := json.Unmarshal([]byte(cleaned), &resp); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed parsing AI generated rule schema: %v"}`, err), http.StatusInternalServerError)
		return
	}

	if resp.Name == "" {
		http.Error(w, `{"error":"AI generated rule is missing name"}`, http.StatusBadGateway)
		return
	}
	if resp.RegexRule == "" {
		http.Error(w, `{"error":"AI generated rule is missing regex_rule"}`, http.StatusBadGateway)
		return
	}
	if resp.Severity == "" {
		http.Error(w, `{"error":"AI generated rule is missing severity"}`, http.StatusBadGateway)
		return
	}
	if resp.Category == "" {
		http.Error(w, `{"error":"AI generated rule is missing category"}`, http.StatusBadGateway)
		return
	}

	resp.Severity = models.FindingSeverity(strings.ToUpper(string(resp.Severity)))

	// Validate compiled regex strictly — reject if invalid
	if _, err := regexp.Compile(resp.RegexRule); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"AI generated invalid regular expression %q: %v"}`, resp.RegexRule, err), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
