package controllers

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/rules"
)

// RulesController manages security rules and real-time rule testing.
type RulesController struct {
	evaluator *rules.Evaluator
}

// NewRulesController initializes the rules controller.
func NewRulesController(evaluator *rules.Evaluator) *RulesController {
	return &RulesController{evaluator: evaluator}
}

// Routes mounts rules endpoints.
func (c *RulesController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/catalog", c.handleGetCatalog)
	r.Post("/test", c.handleTestRule)
	r.Post("/", c.handleCreateRule)

	return r
}

func (c *RulesController) handleGetCatalog(w http.ResponseWriter, r *http.Request) {
	catalog := rules.DefaultCatalog()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(catalog)
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
