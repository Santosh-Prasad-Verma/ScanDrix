// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - User Log & Code Review Settings Audit Controller
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package controllers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth"
)

// UserLogRepository defines the data access contract for settings audit trails and user activity tracking.
type UserLogRepository interface {
	LogUserStatusChange(ctx context.Context, wsID *uuid.UUID, userID, email, status, reason string) error
	ListSettingsAuditLogs(ctx context.Context, wsID uuid.UUID, teamID, repoID *uuid.UUID, page, perPage int) ([]SettingsAuditLogRecord, int, error)
}

// SettingsAuditLogRecord represents a change to code review settings.
type SettingsAuditLogRecord struct {
	ID             uuid.UUID  `json:"id"`
	WorkspaceID    uuid.UUID  `json:"workspaceId"`
	TeamID         *uuid.UUID `json:"teamId,omitempty"`
	RepositoryID   *uuid.UUID `json:"repositoryId,omitempty"`
	ActorID        string     `json:"actorId"`
	ActorEmail     string     `json:"actorEmail"`
	Action         string     `json:"action"`
	PreviousState  any        `json:"previousState,omitempty"`
	NewState       any        `json:"newState,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
}

// UserLogController manages user status changes and code review setting audit logs.
type UserLogController struct {
	repo AuditRepository
}

// NewUserLogController constructs the user log controller.
func NewUserLogController(repo AuditRepository) *UserLogController {
	if isNilInterface(repo) {
		repo = nil
	}
	return &UserLogController{repo: repo}
}

// Routes mounts the /user-log endpoints for audit logging.
func (c *UserLogController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Post("/status-change", c.handleStatusChange)
	r.Get("/code-review-settings", c.handleListCodeReviewSettingsLogs)

	return r
}

type userStatusChangeRequest struct {
	UserID         string `json:"userId"`
	Email          string `json:"email"`
	Status         string `json:"status"`
	OrganizationID string `json:"organizationId,omitempty"`
	Reason         string `json:"reason,omitempty"`
}

func (c *UserLogController) handleStatusChange(w http.ResponseWriter, r *http.Request) {
	var req userStatusChangeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON request body"}`, http.StatusBadRequest)
		return
	}

	if req.UserID == "" && req.Email == "" {
		http.Error(w, `{"error":"userId or email is required"}`, http.StatusBadRequest)
		return
	}

	slog.Info("User status change recorded",
		"user_id", req.UserID,
		"email", req.Email,
		"status", req.Status,
		"org_id", req.OrganizationID,
		"reason", req.Reason,
	)

	w.WriteHeader(http.StatusNoContent)
}

func (c *UserLogController) handleListCodeReviewSettingsLogs(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace authorization context"}`, http.StatusUnauthorized)
		return
	}

	limit := 50
	if lStr := r.URL.Query().Get("perPage"); lStr != "" {
		if parsed, err := strconv.Atoi(lStr); err == nil && parsed > 0 && parsed <= 200 {
			limit = parsed
		}
	} else if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if parsed, err := strconv.Atoi(lStr); err == nil && parsed > 0 && parsed <= 200 {
			limit = parsed
		}
	}

	if c.repo == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":  []any{},
			"total": 0,
		})
		return
	}

	rawLogs, err := c.repo.ListAuditLogs(r.Context(), wsID, limit)
	if err != nil {
		slog.Error("Failed querying code review settings logs", "workspace_id", wsID, "error", err)
		http.Error(w, `{"error":"failed querying audit logs"}`, http.StatusInternalServerError)
		return
	}

	var results []SettingsAuditLogRecord
	for _, l := range rawLogs {
		results = append(results, SettingsAuditLogRecord{
			ID:          l.ID,
			WorkspaceID: l.WorkspaceID,
			ActorID:     l.ActorID,
			ActorEmail:  l.ActorEmail,
			Action:      l.Action,
			CreatedAt:   l.CreatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data":  results,
		"total": len(results),
	})
}
