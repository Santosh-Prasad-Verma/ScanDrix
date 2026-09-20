// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: rule_like_controller.go
// ═══════════════════════════════════════════════════════════════

package controllers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/rules/drixy/application/usecases"
	"github.com/scandrix/backend/internal/rules/drixy/domain/entities"
	"github.com/scandrix/backend/internal/rules/drixy/dtos"
)

// RuleLikeController manages user votes and feedback on rules.
type RuleLikeController struct {
	setRuleLikeUseCase    *usecases.SetRuleLikeUseCase
	removeRuleLikeUseCase *usecases.RemoveRuleLikeUseCase
}

// NewRuleLikeController constructs the feedback controller.
func NewRuleLikeController(
	setRuleLikeUseCase *usecases.SetRuleLikeUseCase,
	removeRuleLikeUseCase *usecases.RemoveRuleLikeUseCase,
) *RuleLikeController {
	return &RuleLikeController{
		setRuleLikeUseCase:    setRuleLikeUseCase,
		removeRuleLikeUseCase: removeRuleLikeUseCase,
	}
}

// Routes mounts rule-like endpoints.
func (c *RuleLikeController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Post("/{ruleId}/feedback", c.handleSetFeedback)
	r.Delete("/{ruleId}/feedback", c.handleRemoveFeedback)

	return r
}

func (c *RuleLikeController) resolveUserID(r *http.Request) string {
	if profile, ok := auth.AccountProfileFromContext(r.Context()); ok && profile != nil && profile.ID.String() != "" {
		return profile.ID.String()
	}
	if hUser := r.Header.Get("x-user-id"); hUser != "" {
		return hUser
	}
	return ""
}

func (c *RuleLikeController) handleSetFeedback(w http.ResponseWriter, r *http.Request) {
	ruleID := chi.URLParam(r, "ruleId")
	if ruleID == "" {
		http.Error(w, `{"error":"ruleId is required"}`, http.StatusBadRequest)
		return
	}

	var dto dtos.SetRuleFeedbackDto
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		http.Error(w, `{"error":"Invalid request payload"}`, http.StatusBadRequest)
		return
	}

	userID := c.resolveUserID(r)
	if userID == "" {
		http.Error(w, `{"error":"User not authenticated"}`, http.StatusUnauthorized)
		return
	}

	res, err := c.setRuleLikeUseCase.Execute(r.Context(), ruleID, entities.RuleFeedbackType(dto.Feedback), userID)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(res.ToObject())
}

func (c *RuleLikeController) handleRemoveFeedback(w http.ResponseWriter, r *http.Request) {
	ruleID := chi.URLParam(r, "ruleId")
	if ruleID == "" {
		http.Error(w, `{"error":"ruleId is required"}`, http.StatusBadRequest)
		return
	}

	userID := c.resolveUserID(r)
	if userID == "" {
		http.Error(w, `{"error":"User not authenticated"}`, http.StatusUnauthorized)
		return
	}

	removed, err := c.removeRuleLikeUseCase.Execute(r.Context(), ruleID, userID)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": removed})
}
