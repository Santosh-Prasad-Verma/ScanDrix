// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev
// ═══════════════════════════════════════════════════════════════

package controllers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/pkg/models"
)

// PMWebhooksController processes inbound lifecycle events from Jira and Linear,
// automatically synchronizing vulnerability finding resolution states.
type PMWebhooksController struct {
	repo         *database.Repository
	jiraSecret   string
	linearSecret string
}

// NewPMWebhooksController creates a new project management webhook controller.
func NewPMWebhooksController(
	repo *database.Repository,
	jiraSecret, linearSecret string,
) *PMWebhooksController {
	return &PMWebhooksController{
		repo:         repo,
		jiraSecret:   jiraSecret,
		linearSecret: linearSecret,
	}
}

// RegisterRoutes binds Jira and Linear webhook endpoints onto a Chi router.
func (c *PMWebhooksController) RegisterRoutes(r chi.Router) {
	r.Post("/jira", c.HandleJiraWebhook)
	r.Post("/jira/webhook", c.HandleJiraWebhook)
	r.Post("/linear", c.HandleLinearWebhook)
	r.Post("/linear/webhook", c.HandleLinearWebhook)
}

// JiraWebhookPayload models Jira's issue_updated webhook payload.
type JiraWebhookPayload struct {
	WebhookEvent string `json:"webhookEvent"`
	Issue        struct {
		ID     string `json:"id"`
		Key    string `json:"key"`
		Fields struct {
			Summary string `json:"summary"`
			Status  struct {
				Name           string `json:"name"`
				StatusCategory struct {
					ID   int    `json:"id"`
					Key  string `json:"key"` // "done", "new", "indeterminate"
					Name string `json:"name"`
				} `json:"statusCategory"`
			} `json:"status"`
			Resolution *struct {
				ID          string `json:"id"`
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"resolution"`
		} `json:"fields"`
	} `json:"issue"`
}

// LinearWebhookPayload models Linear's issue status change webhook payload.
type LinearWebhookPayload struct {
	Action string `json:"action"` // "create", "update", "remove"
	Type   string `json:"type"`   // "Issue"
	Data   struct {
		ID         string  `json:"id"`
		Identifier string  `json:"identifier"` // e.g. "ENG-42"
		Number     float64 `json:"number"`
		Title      string  `json:"title"`
		State      struct {
			ID   string `json:"id"`
			Name string `json:"name"` // "Done", "Canceled", "In Progress"
			Type string `json:"type"` // "completed", "canceled", "started", "unstarted", "backlog"
		} `json:"state"`
	} `json:"data"`
}

// HandleJiraWebhook processes Jira status change webhooks and marks corresponding findings resolved.
func (c *PMWebhooksController) HandleJiraWebhook(w http.ResponseWriter, r *http.Request) {
	rawBody, err := ReadPayload(w, r)
	if err != nil {
		http.Error(w, "failed to read payload", http.StatusBadRequest)
		return
	}

	// Signature / Secret Verification
	if c.jiraSecret != "" {
		verified := false
		if querySec := r.URL.Query().Get("secret"); querySec != "" && subtle.ConstantTimeCompare([]byte(querySec), []byte(c.jiraSecret)) == 1 {
			verified = true
		} else if headerSec := r.Header.Get("X-Atlassian-Webhook-Identifier"); headerSec != "" && subtle.ConstantTimeCompare([]byte(headerSec), []byte(c.jiraSecret)) == 1 {
			verified = true
		} else if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
			token := strings.TrimPrefix(authHeader, "Bearer ")
			if subtle.ConstantTimeCompare([]byte(token), []byte(c.jiraSecret)) == 1 {
				verified = true
			}
		}

		if !verified {
			slog.Warn("Jira webhook signature or secret mismatch")
			http.Error(w, "unauthorized webhook secret", http.StatusUnauthorized)
			return
		}
	}

	var payload JiraWebhookPayload
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		http.Error(w, "invalid jira webhook json", http.StatusBadRequest)
		return
	}

	issueKey := strings.TrimSpace(payload.Issue.Key)
	if issueKey == "" {
		// Not an issue event or missing issue key, acknowledge OK
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ignored", "reason": "no issue key found"})
		return
	}

	statusCategory := strings.ToLower(strings.TrimSpace(payload.Issue.Fields.Status.StatusCategory.Key))
	statusName := strings.ToLower(strings.TrimSpace(payload.Issue.Fields.Status.Name))
	var resolutionName string
	if payload.Issue.Fields.Resolution != nil {
		resolutionName = strings.ToLower(strings.TrimSpace(payload.Issue.Fields.Resolution.Name))
	}

	var targetStatus string
	var reason string

	// Determine resolution status
	if strings.Contains(resolutionName, "won't fix") || strings.Contains(resolutionName, "wont fix") ||
		strings.Contains(resolutionName, "declined") || strings.Contains(resolutionName, "cannot reproduce") {
		targetStatus = "DISMISSED"
		reason = fmt.Sprintf("Dismissed in Jira (%s)", payload.Issue.Fields.Resolution.Name)
	} else if statusCategory == "done" || strings.Contains(statusName, "done") ||
		strings.Contains(statusName, "closed") || strings.Contains(statusName, "resolved") ||
		strings.Contains(resolutionName, "done") || strings.Contains(resolutionName, "fixed") {
		targetStatus = "RESOLVED"
		reason = fmt.Sprintf("Resolved via Jira issue %s", issueKey)
	} else if statusCategory == "new" || statusCategory == "indeterminate" ||
		strings.Contains(statusName, "open") || strings.Contains(statusName, "progress") || strings.Contains(statusName, "reopened") {
		targetStatus = "OPEN"
		reason = ""
	}

	updatedCount := 0
	if targetStatus != "" && c.repo != nil {
		count, err := c.repo.ResolveFindingByTicketKey(r.Context(), "jira", issueKey, targetStatus, reason)
		if err != nil {
			slog.Error("Failed resolving findings for Jira ticket", "ticket_key", issueKey, "error", err)
			http.Error(w, "database update error", http.StatusInternalServerError)
			return
		}
		updatedCount = count
		slog.Info("Processed Jira issue status sync", "ticket_key", issueKey, "target_status", targetStatus, "findings_updated", count)

		// Record outbox event for downstream pub/sub
		c.recordOutbox(r.Context(), "pm.jira_issue_resolved", map[string]any{
			"platform":         "jira",
			"ticket_key":       issueKey,
			"status":           targetStatus,
			"reason":           reason,
			"findings_updated": updatedCount,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":           "processed",
		"platform":         "jira",
		"ticket_key":       issueKey,
		"target_status":    targetStatus,
		"findings_updated": updatedCount,
	})
}

// HandleLinearWebhook processes Linear status update webhooks and marks corresponding findings resolved.
func (c *PMWebhooksController) HandleLinearWebhook(w http.ResponseWriter, r *http.Request) {
	rawBody, err := ReadPayload(w, r)
	if err != nil {
		http.Error(w, "failed to read payload", http.StatusBadRequest)
		return
	}

	// Cryptographic HMAC-SHA256 signature verification
	if c.linearSecret != "" {
		sigHeader := r.Header.Get("Linear-Signature")
		if sigHeader == "" {
			http.Error(w, "missing Linear-Signature header", http.StatusUnauthorized)
			return
		}

		mac := hmac.New(sha256.New, []byte(c.linearSecret))
		mac.Write(rawBody)
		expectedMAC := hex.EncodeToString(mac.Sum(nil))

		if subtle.ConstantTimeCompare([]byte(sigHeader), []byte(expectedMAC)) != 1 {
			slog.Warn("Linear webhook signature verification failed")
			http.Error(w, "invalid linear webhook signature", http.StatusUnauthorized)
			return
		}
	}

	var payload LinearWebhookPayload
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		http.Error(w, "invalid linear webhook json", http.StatusBadRequest)
		return
	}

	identifier := strings.TrimSpace(payload.Data.Identifier)
	if identifier == "" {
		identifier = strings.TrimSpace(payload.Data.ID)
	}

	if identifier == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ignored", "reason": "no linear identifier found"})
		return
	}

	stateType := strings.ToLower(strings.TrimSpace(payload.Data.State.Type))
	stateName := strings.ToLower(strings.TrimSpace(payload.Data.State.Name))

	var targetStatus string
	var reason string

	if stateType == "completed" || strings.Contains(stateName, "done") || strings.Contains(stateName, "completed") {
		targetStatus = "RESOLVED"
		reason = fmt.Sprintf("Resolved via Linear issue %s", identifier)
	} else if stateType == "canceled" || strings.Contains(stateName, "canceled") || strings.Contains(stateName, "cancelled") || strings.Contains(stateName, "duplicate") {
		targetStatus = "DISMISSED"
		reason = fmt.Sprintf("Canceled in Linear (%s)", payload.Data.State.Name)
	} else if stateType == "started" || stateType == "unstarted" || stateType == "backlog" {
		targetStatus = "OPEN"
		reason = ""
	}

	updatedCount := 0
	if targetStatus != "" && c.repo != nil {
		count, err := c.repo.ResolveFindingByTicketKey(r.Context(), "linear", identifier, targetStatus, reason)
		if err != nil {
			slog.Error("Failed resolving findings for Linear issue", "identifier", identifier, "error", err)
			http.Error(w, "database update error", http.StatusInternalServerError)
			return
		}
		updatedCount = count
		slog.Info("Processed Linear issue status sync", "identifier", identifier, "target_status", targetStatus, "findings_updated", count)

		// Record outbox event for downstream pub/sub
		c.recordOutbox(r.Context(), "pm.linear_issue_resolved", map[string]any{
			"platform":         "linear",
			"ticket_key":       identifier,
			"status":           targetStatus,
			"reason":           reason,
			"findings_updated": updatedCount,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":           "processed",
		"platform":         "linear",
		"ticket_key":       identifier,
		"target_status":    targetStatus,
		"findings_updated": updatedCount,
	})
}

func (c *PMWebhooksController) recordOutbox(ctx context.Context, eventType string, payload any) {
	if c.repo == nil {
		return
	}
	bytes, err := json.Marshal(payload)
	if err != nil {
		return
	}

	outboxRec := &models.OutboxRecord{
		ID:          uuid.New(),
		WorkspaceID: uuid.Nil,
		EventType:   eventType,
		Payload:     bytes,
		Status:      models.OutboxPending,
		CreatedAt:   time.Now().UTC(),
	}
	_ = c.repo.InsertOutboxEvent(ctx, outboxRec)
}
