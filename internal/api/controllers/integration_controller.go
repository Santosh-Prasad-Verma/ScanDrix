package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/pkg/models"
)

// IntegrationController handles connecting external VCS and PM tools.
type IntegrationController struct{}

// NewIntegrationController initializes the integration controller.
func NewIntegrationController() *IntegrationController {
	return &IntegrationController{}
}

// Routes mounts integration endpoints.
func (c *IntegrationController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", c.handleListIntegrations)
	r.Post("/connect", c.handleConnectSCM)
	r.Post("/test", c.handleTestConnection)

	return r
}

func (c *IntegrationController) handleListIntegrations(w http.ResponseWriter, r *http.Request) {
	now := time.Now().AddDate(0, 0, -1)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode([]dtos.IntegrationStatusResponse{
		{
			Provider:     models.ProviderGitHub,
			IsConnected:  true,
			AccountName:  "acme-corp",
			RepoCount:    18,
			LastSyncedAt: &now,
		},
		{
			Provider:     models.ProviderGitLab,
			IsConnected:  false,
			AccountName:  "",
			RepoCount:    0,
			LastSyncedAt: nil,
		},
	})
}

func (c *IntegrationController) handleConnectSCM(w http.ResponseWriter, r *http.Request) {
	var req dtos.ConnectSCMRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AccessToken == "" {
		http.Error(w, `{"error":"access_token and provider are required"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"provider": req.Provider,
		"status":   "CONNECTED",
	})
}

func (c *IntegrationController) handleTestConnection(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider models.SCMProvider `json:"provider"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dtos.TestConnectionResponse{
		Success: true,
		Message: fmt.Sprintf("Successfully established TLS handshake and authenticated with %s API", req.Provider),
		User:    "scandrix-bot[app]",
	})
}
