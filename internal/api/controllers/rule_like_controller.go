// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: rule_like_controller.go
// ═══════════════════════════════════════════════════════════════

package controllers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
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

// resolveUserID returns the caller's identity from the verified session only.
//
// The previous implementation fell back to the `x-user-id` request header,
// which is attacker-controlled. On today's mount that fallback is unreachable
// because the route sits behind the authenticated group, but it is exactly the
// identity-from-client-input pattern that AUDIT_REMEDIATION.md F-02 flagged, and
// it would become live the moment this controller was mounted anywhere else.
func (c *RuleLikeController) resolveUserID(r *http.Request) string {
	if profile, ok := auth.AccountProfileFromContext(r.Context()); ok && profile != nil && profile.ID.String() != "" {
		return profile.ID.String()
	}
	return ""
}

// resolveOrganizationID returns the caller's workspace from the verified
// session. It selects the RLS tenant context for drixy_rule_likes
// (AUDIT_REMEDIATION.md F-37), so it must never come from a header or body.
func (c *RuleLikeController) resolveOrganizationID(r *http.Request) string {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil || wsID == uuid.Nil {
		return ""
	}
	return wsID.String()
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
	organizationID := c.resolveOrganizationID(r)
	if userID == "" || organizationID == "" {
		http.Error(w, `{"error":"User not authenticated"}`, http.StatusUnauthorized)
		return
	}

	res, err := c.setRuleLikeUseCase.Execute(r.Context(), organizationID, ruleID, entities.RuleFeedbackType(dto.Feedback), userID)
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
	organizationID := c.resolveOrganizationID(r)
	if userID == "" || organizationID == "" {
		http.Error(w, `{"error":"User not authenticated"}`, http.StatusUnauthorized)
		return
	}

	removed, err := c.removeRuleLikeUseCase.Execute(r.Context(), organizationID, ruleID, userID)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": removed})
}
