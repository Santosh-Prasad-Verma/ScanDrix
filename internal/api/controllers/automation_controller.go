package controllers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/automation"
	"github.com/scandrix/backend/internal/database"
)

// AutomationController manages team workflow rules.
type AutomationController struct {
	repo *database.Repository
}

// NewAutomationController initializes the workflow automation controller.
func NewAutomationController(repo *database.Repository) *AutomationController {
	return &AutomationController{repo: repo}
}

// Routes mounts workflow automation endpoints.
func (c *AutomationController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", c.handleListRules)
	r.Post("/", c.handleCreateRule)
	r.Delete("/{id}", c.handleDeleteRule)

	return r
}

func (c *AutomationController) handleListRules(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	rules, err := c.repo.ListAutomationRules(r.Context(), wsID)
	if err != nil {
		http.Error(w, `{"error":"failed querying automations"}`, http.StatusInternalServerError)
		return
	}

	if rules == nil {
		rules = []automation.AutomationRule{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data":  rules,
		"total": len(rules),
	})
}

func (c *AutomationController) handleCreateRule(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	var rule automation.AutomationRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
		return
	}

	rule.WorkspaceID = wsID
	if err := c.repo.CreateAutomationRule(r.Context(), &rule); err != nil {
		http.Error(w, `{"error":"failed creating automation rule"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(rule)
}

func (c *AutomationController) handleDeleteRule(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	idStr := chi.URLParam(r, "id")
	ruleID, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error":"invalid rule id"}`, http.StatusBadRequest)
		return
	}

	if err := c.repo.DeleteAutomationRule(r.Context(), wsID, ruleID); err != nil {
		http.Error(w, `{"error":"failed deleting automation rule"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": "deleted",
		"id":     ruleID,
	})
}
