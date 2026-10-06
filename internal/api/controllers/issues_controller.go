package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/issues"
	"github.com/scandrix/backend/pkg/models"
)

// IssuesRepository defines the data contract for issue tracking (Clean Architecture).
type IssuesRepository interface {
	ListTrackedIssues(ctx context.Context, wsID uuid.UUID, status issues.IssueStatus) ([]issues.TrackedIssue, error)
	CountTrackedIssues(ctx context.Context, wsID uuid.UUID, status issues.IssueStatus) (int, error)
	GetTrackedIssue(ctx context.Context, wsID, issueID uuid.UUID) (*issues.TrackedIssue, error)
	UpdateTrackedIssueStatus(ctx context.Context, wsID, issueID uuid.UUID, status issues.IssueStatus) error
}

// IssuesController exposes endpoints for persistent code vulnerability issue tracking.
type IssuesController struct {
	repo IssuesRepository
}

// NewIssuesController initializes the issue lifecycle controller.
func NewIssuesController(repo IssuesRepository) *IssuesController {
	if isNilInterface(repo) {
		repo = nil
	}
	return &IssuesController{repo: repo}
}

// Routes mounts issue tracking endpoints.
func (c *IssuesController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", c.handleListIssues)
	r.Get("/count", c.handleCountIssues)
	r.Get("/{id}", c.handleGetIssue)
	r.Patch("/{id}", c.handleUpdateIssue)

	return r
}

func (c *IssuesController) handleListIssues(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	statusFilter := issues.IssueStatus(r.URL.Query().Get("status"))
	var list []issues.TrackedIssue
	if c.repo != nil {
		var queryErr error
		list, queryErr = c.repo.ListTrackedIssues(r.Context(), wsID, statusFilter)
		if queryErr != nil {
			http.Error(w, `{"error":"failed querying issues"}`, http.StatusInternalServerError)
			return
		}
	}

	if list == nil {
		list = []issues.TrackedIssue{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data":  list,
		"total": len(list),
	})
}

func (c *IssuesController) handleCountIssues(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	if c.repo == nil {
		http.Error(w, `{"error":"database service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	statusFilter := issues.IssueStatus(r.URL.Query().Get("status"))
	count, countErr := c.repo.CountTrackedIssues(r.Context(), wsID, statusFilter)
	if countErr != nil {
		http.Error(w, `{"error":"failed counting issues"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"count": count,
	})
}

func (c *IssuesController) handleGetIssue(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	idStr := chi.URLParam(r, "id")
	issueID, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error":"invalid issue id"}`, http.StatusBadRequest)
		return
	}

	if c.repo == nil {
		http.Error(w, `{"error":"issue not found"}`, http.StatusNotFound)
		return
	}

	issue, err := c.repo.GetTrackedIssue(r.Context(), wsID, issueID)
	if err != nil {
		http.Error(w, `{"error":"issue not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(issue)
}

func (c *IssuesController) handleUpdateIssue(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	// Triaging a finding mutates workspace state, so a read-only viewer must not
	// be able to resolve or reopen issues. Matches the guard on triggerReview.
	profile, ok := auth.AccountProfileFromContext(r.Context())
	if ok && profile != nil && profile.Role == models.RoleViewer {
		http.Error(w, `{"error":"forbidden: viewers cannot update issue status"}`, http.StatusForbidden)
		return
	}

	idStr := chi.URLParam(r, "id")
	issueID, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error":"invalid issue id"}`, http.StatusBadRequest)
		return
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid request json"}`, http.StatusBadRequest)
		return
	}

	// The enum is validated here because tracked_issues.status carries no CHECK
	// constraint. An unvalidated value persisted silently and then broke every
	// analytics filter that groups by status.
	if !issues.ValidIssueStatus(issues.IssueStatus(body.Status)) {
		http.Error(w, `{"error":"invalid issue status"}`, http.StatusBadRequest)
		return
	}

	if c.repo != nil {
		_, err := c.repo.GetTrackedIssue(r.Context(), wsID, issueID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, `{"error":"issue not found"}`, http.StatusNotFound)
				return
			}
			http.Error(w, `{"error":"failed retrieving issue"}`, http.StatusInternalServerError)
			return
		}
		if err := c.repo.UpdateTrackedIssueStatus(r.Context(), wsID, issueID, issues.IssueStatus(body.Status)); err != nil {
			slog.Error("Failed to update tracked issue status", "workspace_id", wsID, "issue_id", issueID, "status", body.Status, "error", err)
			http.Error(w, `{"error":"failed updating issue"}`, http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": "updated",
		"id":     issueID,
	})
}
