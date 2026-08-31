package controllers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/enterprise/audit"
)

// AuditController handles compliance audit log queries and SIEM streaming exports.
type AuditController struct {
	repo     *database.Repository
	streamer *audit.SIEMAuditStreamer
}

// NewAuditController initializes the audit log controller.
func NewAuditController(repo *database.Repository) *AuditController {
	return &AuditController{
		repo:     repo,
		streamer: audit.NewSIEMAuditStreamer(),
	}
}

// Routes mounts the audit log endpoints.
func (c *AuditController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", c.handleListAuditLogs)
	r.Get("/export", c.handleExportSIEM)

	return r
}

func (c *AuditController) handleListAuditLogs(w http.ResponseWriter, r *http.Request) {
	wsIDStr := chi.URLParam(r, "workspaceId")
	wsID, err := uuid.Parse(wsIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid workspace id"}`, http.StatusBadRequest)
		return
	}

	limit := 100
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if parsed, err := strconv.Atoi(lStr); err == nil && parsed > 0 && parsed <= 500 {
			limit = parsed
		}
	}

	if c.repo == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]any{})
		return
	}

	logs, err := c.repo.ListAuditLogs(r.Context(), wsID, limit)
	if err != nil {
		slog.Error("Failed querying audit logs", "workspace_id", wsID, "error", err)
		http.Error(w, `{"error":"failed querying audit logs"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(logs)
}

func (c *AuditController) handleExportSIEM(w http.ResponseWriter, r *http.Request) {
	wsIDStr := chi.URLParam(r, "workspaceId")
	wsID, err := uuid.Parse(wsIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid workspace id"}`, http.StatusBadRequest)
		return
	}

	format := r.URL.Query().Get("format") // "cef", "syslog", "json"
	if format == "" {
		format = "cef"
	}

	limit := 500
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if parsed, err := strconv.Atoi(lStr); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}

	if c.repo == nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	logs, err := c.repo.ListAuditLogs(r.Context(), wsID, limit)
	if err != nil {
		slog.Error("Failed querying audit logs for export", "workspace_id", wsID, "error", err)
		http.Error(w, `{"error":"failed querying audit logs for export"}`, http.StatusInternalServerError)
		return
	}

	if format == "json" {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"audit-export-%s.json\"", wsID.String()))
		_ = json.NewEncoder(w).Encode(logs)
		return
	}

	// Stream CEF:0 or RFC-5424 text
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"audit-export-%s.cef\"", wsID.String()))

	for _, l := range logs {
		event := audit.EnterpriseAuditEvent{
			EventID:      l.ID,
			WorkspaceID:  l.WorkspaceID,
			ActorID:      l.ActorID,
			ActorEmail:   l.ActorEmail,
			ClientIP:     l.IPAddress,
			Action:       audit.AuditAction(l.Action),
			ResourceType: l.TargetType,
			ResourceID:   l.TargetID,
			Timestamp:    l.CreatedAt,
		}
		if event.Timestamp.IsZero() {
			event.Timestamp = time.Now().UTC()
		}

		if format == "syslog" {
			_, _ = fmt.Fprintln(w, audit.FormatRFC5424(event))
		} else {
			_, _ = fmt.Fprintln(w, audit.FormatCEF(event))
		}
	}
}
