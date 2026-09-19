// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Controller
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/scandrix/backend/internal/agents/businessrules"
	"github.com/scandrix/backend/internal/agents/conversation"
	"github.com/scandrix/backend/internal/auth"
)

// OrganizationAndTeamDataDto provides tenant context for conversational agents.
type OrganizationAndTeamDataDto struct {
	OrganizationID string `json:"organizationId"`
	TeamID         string `json:"teamId"`
}

// AgentConversationRequestBody models the POST /agent/conversation payload.
type AgentConversationRequestBody struct {
	Prompt                  string                     `json:"prompt"`
	OrganizationAndTeamData OrganizationAndTeamDataDto `json:"organizationAndTeamData"`
	ConversationID          string                     `json:"conversationId,omitempty"`
}

// BusinessRulesValidationRequestBody models the POST /agent/business-rules-validation payload.
type BusinessRulesValidationRequestBody struct {
	TaskContext             string                     `json:"taskContext"`
	PRDiff                  string                     `json:"prDiff"`
	PRBody                  string                     `json:"prBody,omitempty"`
	UserLanguage            string                     `json:"userLanguage,omitempty"`
	OrganizationAndTeamData OrganizationAndTeamDataDto `json:"organizationAndTeamData"`
	ConversationID          string                     `json:"conversationId,omitempty"`
}

// AgentController provides interactive agent conversations and business rules validation.
type AgentController struct {
	conversationProvider *conversation.ConversationAgentProvider
	businessRules        *businessrules.BusinessRulesValidationAgentProvider
}

// NewAgentController constructs the AgentController with providers.
func NewAgentController(
	conversationProvider *conversation.ConversationAgentProvider,
	businessRules *businessrules.BusinessRulesValidationAgentProvider,
) *AgentController {
	return &AgentController{
		conversationProvider: conversationProvider,
		businessRules:        businessRules,
	}
}

// Routes mounts the /agent routes.
func (c *AgentController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Post("/conversation", c.handleConversation)
	r.Post("/business-rules-validation", c.handleBusinessRulesValidation)

	return r
}

// handleConversation executes a multi-turn conversation turn with the agent.
func (c *AgentController) handleConversation(w http.ResponseWriter, r *http.Request) {
	var body AgentConversationRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid JSON request body"}`, http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(body.Prompt) == "" {
		http.Error(w, `{"error":"prompt is required"}`, http.StatusBadRequest)
		return
	}

	orgID := body.OrganizationAndTeamData.OrganizationID
	var userID string

	// Extract workspace/tenant from auth context if present
	if wsID, err := auth.WorkspaceFromContext(r.Context()); err == nil && wsID.String() != "" {
		if orgID == "" {
			orgID = wsID.String()
		}
	}
	if profile, ok := auth.AccountProfileFromContext(r.Context()); ok && profile != nil {
		userID = profile.ID.String()
	}

	if orgID == "" {
		http.Error(w, `{"error":"organization ID missing in user request"}`, http.StatusBadRequest)
		return
	}

	teamID := body.OrganizationAndTeamData.TeamID
	if teamID == "" {
		teamID = "default"
	}

	threadID := createThreadID(orgID, teamID, userID, body.ConversationID)

	req := conversation.ConversationRequest{
		Prompt:         body.Prompt,
		OrganizationID: orgID,
		TeamID:         teamID,
		UserID:         userID,
		ThreadID:       threadID,
	}

	if c.conversationProvider == nil {
		http.Error(w, `{"error":"conversation agent provider not initialized"}`, http.StatusServiceUnavailable)
		return
	}

	resp, err := c.conversationProvider.Execute(r.Context(), req)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed to process conversation: %v"}`, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// handleBusinessRulesValidation evaluates PR diff against user task context and acceptance criteria.
func (c *AgentController) handleBusinessRulesValidation(w http.ResponseWriter, r *http.Request) {
	var body BusinessRulesValidationRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid JSON request body"}`, http.StatusBadRequest)
		return
	}

	orgID := body.OrganizationAndTeamData.OrganizationID
	if wsID, err := auth.WorkspaceFromContext(r.Context()); err == nil && wsID.String() != "" {
		if orgID == "" {
			orgID = wsID.String()
		}
	}

	teamID := body.OrganizationAndTeamData.TeamID
	if teamID == "" {
		teamID = "default"
	}

	threadID := createThreadID(orgID, teamID, "", body.ConversationID)

	bctx := businessrules.BusinessRulesContext{
		TaskContext:    body.TaskContext,
		PRDiff:         body.PRDiff,
		PRBody:         body.PRBody,
		UserLanguage:   body.UserLanguage,
		OrganizationID: orgID,
		TeamID:         teamID,
		ThreadID:       threadID,
	}

	if c.businessRules == nil {
		http.Error(w, `{"error":"business rules validation provider not initialized"}`, http.StatusServiceUnavailable)
		return
	}

	result, err := c.businessRules.Execute(r.Context(), bctx)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed to evaluate business rules: %v"}`, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(result)
}

// createThreadID generates continuity thread identifier format.
// Pattern: cmc:{orgId}:{teamId}[:{userId}][:{conversationId}]
func createThreadID(orgID, teamID, userID, convID string) string {
	parts := []string{"cmc", orgID, teamID}
	if userID != "" {
		parts = append(parts, userID)
	}
	if convID != "" {
		parts = append(parts, convID)
	}
	return strings.Join(parts, ":")
}
