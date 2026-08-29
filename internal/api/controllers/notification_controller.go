package controllers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/pkg/models"
)

// NotificationController manages channel alert destinations and notification policies.
type NotificationController struct{}

// NewNotificationController initializes the notification controller.
func NewNotificationController() *NotificationController {
	return &NotificationController{}
}

// Routes mounts notification endpoints.
func (c *NotificationController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/channels", c.handleListChannels)
	r.Post("/channels", c.handleCreateChannel)

	return r
}

func (c *NotificationController) handleListChannels(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode([]dtos.NotificationChannelDTO{
		{
			ID:       uuid.MustParse("00000000-0000-0000-0004-000000000001"),
			Type:     "SLACK",
			Target:   "https://hooks.slack.com/services/T00/B00/XXXX",
			Severity: models.SeverityHigh,
			Enabled:  true,
		},
		{
			ID:       uuid.MustParse("00000000-0000-0000-0004-000000000002"),
			Type:     "TEAMS",
			Target:   "https://outlook.office.com/webhook/XXXX",
			Severity: models.SeverityCritical,
			Enabled:  true,
		},
	})
}

func (c *NotificationController) handleCreateChannel(w http.ResponseWriter, r *http.Request) {
	var req dtos.NotificationChannelDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Target == "" {
		http.Error(w, `{"error":"target url/email is required"}`, http.StatusBadRequest)
		return
	}

	req.ID = uuid.New()
	req.Enabled = true

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(req)
}
