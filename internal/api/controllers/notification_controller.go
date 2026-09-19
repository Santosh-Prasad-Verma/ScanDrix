package controllers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/pkg/models"
)

// NotificationRepository defines the data access contract for notification channels (Clean Architecture).
type NotificationRepository interface {
	ListNotificationChannels(ctx context.Context, wsID uuid.UUID) ([]models.NotificationChannel, error)
	CreateNotificationChannel(ctx context.Context, wsID uuid.UUID, chType, target string, severity models.FindingSeverity) (*models.NotificationChannel, error)
}

// NotificationController manages channel alert destinations and notification policies.
type NotificationController struct {
	repo NotificationRepository
}

// NewNotificationController initializes the notification controller with repository dependency.
func NewNotificationController(repo NotificationRepository) *NotificationController {
	if isNilInterface(repo) {
		repo = nil
	}
	return &NotificationController{repo: repo}
}

// Routes mounts notification endpoints.
func (c *NotificationController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/channels", c.handleListChannels)
	r.Post("/channels", c.handleCreateChannel)

	return r
}

func (c *NotificationController) handleListChannels(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	var channels []models.NotificationChannel
	if c.repo != nil {
		var err error
		channels, err = c.repo.ListNotificationChannels(r.Context(), wsID)
		if err != nil {
			http.Error(w, `{"error":"failed listing notification channels"}`, http.StatusInternalServerError)
			return
		}
	}

	res := make([]dtos.NotificationChannelDTO, 0, len(channels))
	for _, ch := range channels {
		res = append(res, dtos.NotificationChannelDTO{
			ID:       ch.ID,
			Type:     ch.Type,
			Target:   ch.Target,
			Severity: ch.Severity,
			Enabled:  ch.Enabled,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (c *NotificationController) handleCreateChannel(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	if c.repo == nil {
		http.Error(w, `{"error":"database service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	var req dtos.NotificationChannelDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Target == "" {
		http.Error(w, `{"error":"target url/email is required"}`, http.StatusBadRequest)
		return
	}

	if req.Severity == "" {
		req.Severity = models.SeverityHigh
	}
	if req.Type == "" {
		req.Type = "WEBHOOK"
	}

	ch, err := c.repo.CreateNotificationChannel(r.Context(), wsID, req.Type, req.Target, req.Severity)
	if err != nil {
		http.Error(w, `{"error":"failed creating notification channel"}`, http.StatusInternalServerError)
		return
	}

	req.ID = ch.ID
	req.Enabled = ch.Enabled

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(req)
}
