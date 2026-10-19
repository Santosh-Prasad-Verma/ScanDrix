package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/integrations/slack"
	"github.com/scandrix/backend/internal/integrations/teams"
	"github.com/scandrix/backend/pkg/models"
)

// BotActionsController handles inbound interactive button callbacks from Slack Block Kit and MS Teams Adaptive Cards.
type BotActionsController struct {
	repo               *database.Repository
	slackSigningSecret string
	teamsWebhookSecret string
}

// NewBotActionsController instantiates the interactive bot controller.
func NewBotActionsController(
	repo *database.Repository,
	slackSigningSecret string,
	teamsWebhookSecret string,
) *BotActionsController {
	return &BotActionsController{
		repo:               repo,
		slackSigningSecret: slackSigningSecret,
		teamsWebhookSecret: teamsWebhookSecret,
	}
}

// HandleSlackActions processes interactive button clicks (e.g. [Approve PR], [Request Changes]).
func (c *BotActionsController) HandleSlackActions(w http.ResponseWriter, r *http.Request) {
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed reading request body", http.StatusBadRequest)
		return
	}

	// 1. Cryptographic HMAC-SHA256 signature verification
	if err := slack.VerifySlackSignature(c.slackSigningSecret, r.Header, rawBody); err != nil {
		log.Printf("[BotActions] Slack signature verification failed: %v", err)
		http.Error(w, fmt.Sprintf("Unauthorized: %v", err), http.StatusUnauthorized)
		return
	}

	// 2. Parse interaction payload
	payload, err := slack.ParseInteractionPayload(r, rawBody)
	if err != nil {
		log.Printf("[BotActions] Error parsing Slack payload: %v", err)
		http.Error(w, fmt.Sprintf("Bad Request: %v", err), http.StatusBadRequest)
		return
	}

	if len(payload.Actions) == 0 {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "no_action"})
		return
	}

	action := payload.Actions[0]
	var actionVal slack.BlockActionValue
	if action.Value != "" {
		_ = json.Unmarshal([]byte(action.Value), &actionVal)
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	actor := payload.User.Username
	if actor == "" {
		actor = payload.User.Name
	}
	if actor == "" {
		actor = "slack-user"
	}

	var wsUUID uuid.UUID
	if actionVal.WorkspaceID != "" {
		wsUUID, _ = uuid.Parse(actionVal.WorkspaceID)
	}

	switch action.ActionID {
	case "approve_pr":
		eventPayload, _ := json.Marshal(map[string]any{
			"repo":         actionVal.Repo,
			"pr_number":    actionVal.PRNumber,
			"actor":        actor,
			"source":       "slack",
			"verdict":      "APPROVED",
			"workspace_id": actionVal.WorkspaceID,
			"timestamp":    time.Now().UTC().Format(time.RFC3339),
		})
		if c.repo != nil {
			_ = c.repo.InsertOutboxEvent(ctx, &models.OutboxRecord{
				ID:          uuid.New(),
				WorkspaceID: wsUUID,
				EventType:   "pr.review.approved",
				Payload:     eventPayload,
			})
		}

		resp := slack.BuildResolutionResponse(actionVal.Repo, actionVal.PRNumber, "approve_pr", actor)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
		return

	case "request_changes":
		eventPayload, _ := json.Marshal(map[string]any{
			"repo":         actionVal.Repo,
			"pr_number":    actionVal.PRNumber,
			"actor":        actor,
			"source":       "slack",
			"verdict":      "CHANGES_REQUESTED",
			"workspace_id": actionVal.WorkspaceID,
			"timestamp":    time.Now().UTC().Format(time.RFC3339),
		})
		if c.repo != nil {
			_ = c.repo.InsertOutboxEvent(ctx, &models.OutboxRecord{
				ID:          uuid.New(),
				WorkspaceID: wsUUID,
				EventType:   "pr.review.changes_requested",
				Payload:     eventPayload,
			})
		}

		resp := slack.BuildResolutionResponse(actionVal.Repo, actionVal.PRNumber, "request_changes", actor)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
		return

	case "resolve_finding":
		findingKey := actionVal.FindingID
		if findingKey != "" && c.repo != nil {
			_, _ = c.repo.ResolveFindingByTicketKey(ctx, "slack", findingKey, "RESOLVED", "Resolved via Slack Bot action")
		}
		resp := map[string]any{
			"response_type": "ephemeral",
			"text":          fmt.Sprintf("🎯 Finding `%s` was marked RESOLVED by @%s.", findingKey, actor),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// HandleTeamsActions processes Microsoft Teams Adaptive Card button clicks.
func (c *BotActionsController) HandleTeamsActions(w http.ResponseWriter, r *http.Request) {
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed reading request body", http.StatusBadRequest)
		return
	}

	// 1. Validate authorization
	if err := teams.VerifyTeamsAuth(c.teamsWebhookSecret, r.Header.Get("Authorization"), rawBody); err != nil {
		log.Printf("[BotActions] Teams auth verification failed: %v", err)
		http.Error(w, fmt.Sprintf("Unauthorized: %v", err), http.StatusUnauthorized)
		return
	}

	// 2. Parse Teams Action Invoke
	invoke, err := teams.ParseTeamsAction(r, rawBody)
	if err != nil {
		log.Printf("[BotActions] Error parsing Teams payload: %v", err)
		http.Error(w, fmt.Sprintf("Bad Request: %v", err), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	actor := invoke.From.Name
	if actor == "" {
		actor = "teams-user"
	}

	var wsUUID uuid.UUID
	if invoke.Value.WorkspaceID != "" {
		wsUUID, _ = uuid.Parse(invoke.Value.WorkspaceID)
	}

	actionName := invoke.Value.Action
	switch actionName {
	case "approve_pr":
		eventPayload, _ := json.Marshal(map[string]any{
			"repo":         invoke.Value.Repo,
			"pr_number":    invoke.Value.PRNumber,
			"actor":        actor,
			"source":       "teams",
			"verdict":      "APPROVED",
			"workspace_id": invoke.Value.WorkspaceID,
			"timestamp":    time.Now().UTC().Format(time.RFC3339),
		})
		if c.repo != nil {
			_ = c.repo.InsertOutboxEvent(ctx, &models.OutboxRecord{
				ID:          uuid.New(),
				WorkspaceID: wsUUID,
				EventType:   "pr.review.approved",
				Payload:     eventPayload,
			})
		}

		card := teams.BuildResolutionCard(invoke.Value.Repo, invoke.Value.PRNumber, "approve_pr", actor)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(card)
		return

	case "request_changes":
		eventPayload, _ := json.Marshal(map[string]any{
			"repo":         invoke.Value.Repo,
			"pr_number":    invoke.Value.PRNumber,
			"actor":        actor,
			"source":       "teams",
			"verdict":      "CHANGES_REQUESTED",
			"workspace_id": invoke.Value.WorkspaceID,
			"timestamp":    time.Now().UTC().Format(time.RFC3339),
		})
		if c.repo != nil {
			_ = c.repo.InsertOutboxEvent(ctx, &models.OutboxRecord{
				ID:          uuid.New(),
				WorkspaceID: wsUUID,
				EventType:   "pr.review.changes_requested",
				Payload:     eventPayload,
			})
		}

		card := teams.BuildResolutionCard(invoke.Value.Repo, invoke.Value.PRNumber, "request_changes", actor)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(card)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
