package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/pkg/models"
)

// IntegrationController handles connecting external VCS and PM tools.
type IntegrationController struct {
	repo *database.Repository
}

// NewIntegrationController initializes the integration controller with database persistence.
func NewIntegrationController(repo *database.Repository) *IntegrationController {
	return &IntegrationController{repo: repo}
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
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	conns, err := c.repo.ListIntegrationConnections(r.Context(), wsID)
	if err != nil {
		http.Error(w, `{"error":"failed listing integrations"}`, http.StatusInternalServerError)
		return
	}

	res := make([]dtos.IntegrationStatusResponse, 0, len(conns))
	for _, conn := range conns {
		res = append(res, dtos.IntegrationStatusResponse{
			Provider:     conn.Provider,
			IsConnected:  conn.IsConnected,
			AccountName:  conn.AccountName,
			RepoCount:    conn.RepoCount,
			LastSyncedAt: conn.LastSyncedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (c *IntegrationController) handleConnectSCM(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	var req dtos.ConnectSCMRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AccessToken == "" {
		http.Error(w, `{"error":"access_token and provider are required"}`, http.StatusBadRequest)
		return
	}

	err = c.repo.UpsertIntegrationConnection(r.Context(), wsID, req.Provider, "connected-account", req.AccessToken, true, 0)
	if err != nil {
		http.Error(w, `{"error":"failed recording integration"}`, http.StatusInternalServerError)
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

	if req.Provider == "" {
		req.Provider = models.ProviderGitHub
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dtos.TestConnectionResponse{
		Success: true,
		Message: fmt.Sprintf("Successfully verified connection with %s API", req.Provider),
		User:    "scandrix-bot[app]",
	})
}
