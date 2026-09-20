package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/pkg/models"
)

// PullRequestMessagesRepository defines the data contract for custom PR review messages.
type PullRequestMessagesRepository interface {
	GetPullRequestMessages(ctx context.Context, wsID uuid.UUID, repoID *uuid.UUID, directoryID string) (*models.PullRequestMessageSettings, error)
	SavePullRequestMessages(ctx context.Context, wsID uuid.UUID, settings *models.PullRequestMessageSettings) error
}

// PullRequestMessagesController manages custom automated review comment messaging.
type PullRequestMessagesController struct {
	repo PullRequestMessagesRepository
}

// NewPullRequestMessagesController creates the controller.
func NewPullRequestMessagesController(repo PullRequestMessagesRepository) *PullRequestMessagesController {
	if isNilInterface(repo) {
		repo = nil
	}
	return &PullRequestMessagesController{repo: repo}
}

// Routes mounts the pull-request-messages endpoints.
func (c *PullRequestMessagesController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", c.handleGetMessages)
	r.Post("/", c.handleSaveMessages)

	return r
}

func (c *PullRequestMessagesController) handleGetMessages(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	var repoID *uuid.UUID
	repoIDStr := r.URL.Query().Get("repositoryId")
	if repoIDStr != "" && strings.ToLower(repoIDStr) != "global" {
		if rid, err := uuid.Parse(repoIDStr); err == nil {
			repoID = &rid
		}
	}
	directoryID := r.URL.Query().Get("directoryId")

	if c.repo == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(models.PullRequestMessageSettings{
			WorkspaceID:  wsID,
			RepositoryID: repoID,
			ConfigLevel:  "global",
			StartReviewMessage: models.MessageContentWithStatus{
				Content: "ScanDrix is analyzing your pull request changes...",
				Status:  models.PRMessageStatusActive,
			},
			HideComments:         false,
			SuggestionCopyPrompt: true,
		})
		return
	}

	settings, err := c.repo.GetPullRequestMessages(r.Context(), wsID, repoID, directoryID)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed fetching messages: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(settings)
}

func (c *PullRequestMessagesController) handleSaveMessages(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	var req struct {
		UUID               *uuid.UUID                        `json:"uuid,omitempty"`
		TeamID             *uuid.UUID                        `json:"teamId,omitempty"`
		RepositoryID       any                               `json:"repositoryId"` // can be string UUID, "global", or nil
		DirectoryID        string                            `json:"directoryId,omitempty"`
		ConfigLevel        string                            `json:"configLevel"`
		StartReviewMessage models.MessageContentWithStatus   `json:"startReviewMessage"`
		EndReviewMessage   *models.MessageContentWithStatus  `json:"endReviewMessage,omitempty"`
		ErrorReviewMessage *models.MessageContentWithStatus  `json:"errorReviewMessage,omitempty"`
		GlobalSettings     *struct {
			HideComments         bool `json:"hideComments"`
			SuggestionCopyPrompt bool `json:"suggestionCopyPrompt"`
		} `json:"globalSettings,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request json"}`, http.StatusBadRequest)
		return
	}

	var repoUUID *uuid.UUID
	if req.RepositoryID != nil {
		if s, ok := req.RepositoryID.(string); ok && s != "" && strings.ToLower(s) != "global" {
			if parsed, err := uuid.Parse(s); err == nil {
				repoUUID = &parsed
			}
		}
	}

	settings := models.PullRequestMessageSettings{
		UUID:               req.UUID,
		WorkspaceID:        wsID,
		TeamID:             req.TeamID,
		RepositoryID:       repoUUID,
		DirectoryID:        req.DirectoryID,
		ConfigLevel:        req.ConfigLevel,
		StartReviewMessage: req.StartReviewMessage,
		EndReviewMessage:   req.EndReviewMessage,
		ErrorReviewMessage: req.ErrorReviewMessage,
	}

	if req.GlobalSettings != nil {
		settings.HideComments = req.GlobalSettings.HideComments
		settings.SuggestionCopyPrompt = req.GlobalSettings.SuggestionCopyPrompt
	}

	if c.repo != nil {
		if err := c.repo.SavePullRequestMessages(r.Context(), wsID, &settings); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"failed saving message settings: %s"}`, err.Error()), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(settings)
}
