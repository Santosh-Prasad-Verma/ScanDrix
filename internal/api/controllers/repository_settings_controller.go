package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/pkg/models"
)

type repositorySettingsStore interface {
	GetRepositoryReviewSettings(context.Context, uuid.UUID, uuid.UUID) (models.RepositoryReviewSettings, error)
	PatchRepositoryReviewSettings(context.Context, uuid.UUID, uuid.UUID, models.RepositoryReviewSettingsPatch) (models.RepositoryReviewSettings, error)
}

type repositoryUntracker interface {
	UntrackRepository(context.Context, uuid.UUID, uuid.UUID) error
}

func (c *ParametersController) handleRepositorySettings(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil || wsID == uuid.Nil {
		http.Error(w, `{"error":"missing workspace context"}`, 401)
		return
	}
	repoID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil || repoID == uuid.Nil {
		http.Error(w, `{"error":"internal repository UUID required"}`, 400)
		return
	}
	if r.Method != http.MethodGet {
		profile, ok := auth.AccountProfileFromContext(r.Context())
		if !ok || profile == nil || (strings.ToUpper(string(profile.Role)) != "OWNER" && strings.ToUpper(string(profile.Role)) != "ADMIN") {
			http.Error(w, `{"error":"repository administration required"}`, 403)
			return
		}
	}
	store, ok := c.repo.(repositorySettingsStore)
	if !ok || isNilInterface(store) {
		http.Error(w, `{"error":"repository settings unavailable"}`, 503)
		return
	}
	if r.Method == http.MethodDelete {
		untracker, ok := c.repo.(repositoryUntracker)
		if !ok || isNilInterface(untracker) {
			http.Error(w, `{"error":"repository tracking unavailable"}`, 503)
			return
		}
		err = untracker.UntrackRepository(r.Context(), wsID, repoID)
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, `{"error":"repository not found"}`, 404)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"repository could not be untracked"}`, 500)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	var settings models.RepositoryReviewSettings
	if r.Method == http.MethodPatch {
		var body struct {
			ConfigValue *models.RepositoryReviewSettingsPatch `json:"configValue"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil || body.ConfigValue == nil {
			http.Error(w, `{"error":"invalid repository settings"}`, 400)
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			http.Error(w, `{"error":"invalid repository settings"}`, 400)
			return
		}
		if err := body.ConfigValue.Validate(); err != nil {
			http.Error(w, `{"error":"invalid repository settings"}`, 400)
			return
		}
		settings, err = store.PatchRepositoryReviewSettings(r.Context(), wsID, repoID, *body.ConfigValue)
	} else {
		settings, err = store.GetRepositoryReviewSettings(r.Context(), wsID, repoID)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, `{"error":"repository not found"}`, 404)
		return
	}
	if err != nil {
		http.Error(w, `{"error":"repository settings could not be loaded or saved"}`, 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"repositoryId": repoID, "configValue": settings, "success": true}})
}
