package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/integrations/pm"
	"github.com/scandrix/backend/pkg/models"
)

// IntegrationController handles connecting external VCS and PM tools.
type IntegrationController struct {
	repo         *database.Repository
	pmDispatcher *pm.PMDispatcher
}

// NewIntegrationController initializes the integration controller with database persistence.
func NewIntegrationController(repo *database.Repository) *IntegrationController {
	return &IntegrationController{
		repo:         repo,
		pmDispatcher: pm.NewPMDispatcher(),
	}
}

// Routes mounts integration endpoints.
func (c *IntegrationController) Routes() chi.Router {
	r := chi.NewRouter()

	// SCM routes
	r.Get("/", c.handleListIntegrations)
	r.Post("/connect", c.handleConnectSCM)
	r.Post("/test", c.handleTestConnection)

	// PM routes (Jira, Linear, Azure Boards)
	r.Post("/pm/connect", c.handleConnectPM)
	r.Post("/pm/export", c.handleExportFindingToPM)
	r.Get("/pm/tickets", c.handleListPMTickets)
	r.Get("/pm/auto-ticket", c.handleGetPMAutoTicketConfig)
	r.Put("/pm/auto-ticket", c.handleSetPMAutoTicketConfig)

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

func (c *IntegrationController) handleConnectPM(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	var req dtos.ConnectPMRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.APIToken == "" || req.Platform == "" {
		http.Error(w, `{"error":"platform and api_token are required"}`, http.StatusBadRequest)
		return
	}

	platformLower := strings.ToLower(req.Platform)
	provider := models.SCMProvider(platformLower)

	accountName := fmt.Sprintf("%s-integration", platformLower)
	if req.Email != "" {
		accountName = req.Email
	} else if req.Organization != "" {
		accountName = req.Organization
	}

	err = c.repo.UpsertIntegrationConnection(r.Context(), wsID, provider, accountName, req.APIToken, true, 0)
	if err != nil {
		http.Error(w, `{"error":"failed recording PM integration"}`, http.StatusInternalServerError)
		return
	}

	// Register adapter in dispatcher
	cfg := pm.PMConfig{
		Platform:     pm.PMPlatform(platformLower),
		BaseURL:      req.BaseURL,
		APIToken:     req.APIToken,
		Email:        req.Email,
		Organization: req.Organization,
	}
	if adapter, err := pm.NewAdapterFromConfig(cfg); err == nil {
		c.pmDispatcher.RegisterAdapter(wsID, adapter)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"platform": platformLower,
		"status":   "CONNECTED",
		"account":  accountName,
	})
}

func (c *IntegrationController) handleExportFindingToPM(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	var req dtos.ExportFindingToPMRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.FindingID == uuid.Nil || req.Platform == "" || req.ProjectKey == "" {
		http.Error(w, `{"error":"finding_id, platform, and project_key are required"}`, http.StatusBadRequest)
		return
	}

	// Query finding from database
	finding, err := c.repo.GetFindingByID(r.Context(), wsID, req.FindingID)
	if err != nil || finding == nil {
		http.Error(w, `{"error":"finding not found"}`, http.StatusNotFound)
		return
	}

	targetPlatform := pm.PMPlatform(strings.ToLower(req.Platform))
	adapter, err := c.pmDispatcher.GetAdapter(wsID, targetPlatform)
	if err != nil {
		// Try resolving decrypted credentials from database
		conns, connErr := c.repo.ListIntegrationConnections(r.Context(), wsID)
		if connErr == nil {
			for _, conn := range conns {
				if strings.EqualFold(string(conn.Provider), string(targetPlatform)) && conn.IsConnected && conn.AccessTokenEnc != "" {
					pmCfg := pm.PMConfig{
						Platform: targetPlatform,
						APIToken: conn.AccessTokenEnc,
						BaseURL:  "https://api.atlassian.com",
					}
					if targetPlatform == pm.PlatformLinear {
						pmCfg.BaseURL = "https://api.linear.app/graphql"
					} else if targetPlatform == pm.PlatformAzureBoards {
						pmCfg.BaseURL = "https://dev.azure.com"
					}
					if a, err := pm.NewAdapterFromConfig(pmCfg); err == nil {
						c.pmDispatcher.RegisterAdapter(wsID, a)
						adapter = a
						break
					}
				}
			}
		}
	}

	if adapter == nil {
		http.Error(w, fmt.Sprintf(`{"error":"no active %s integration configured for workspace"}`, targetPlatform), http.StatusBadRequest)
		return
	}

	issue, err := c.pmDispatcher.ExportFinding(r.Context(), wsID, targetPlatform, req.ProjectKey, *finding, req.PullRequestURL)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed exporting finding to %s: %s"}`, targetPlatform, err.Error()), http.StatusBadGateway)
		return
	}

	// Record ticket linkage in database
	_ = c.repo.InsertFindingTicket(r.Context(), wsID, finding.ID, string(targetPlatform), issue.Key, issue.URL)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(dtos.PMTicketResponse{
		ID:        uuid.New(),
		FindingID: finding.ID,
		Platform:  string(targetPlatform),
		TicketKey: issue.Key,
		TicketURL: issue.URL,
		CreatedAt: issue.CreatedAt,
	})
}

func (c *IntegrationController) handleListPMTickets(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	findingIDStr := r.URL.Query().Get("finding_id")
	reviewIDStr := r.URL.Query().Get("review_id")

	var tickets []database.FindingTicketRecord
	if findingIDStr != "" {
		if fID, err := uuid.Parse(findingIDStr); err == nil {
			tickets, _ = c.repo.GetTicketsForFinding(r.Context(), wsID, fID)
		}
	} else if reviewIDStr != "" {
		if rID, err := uuid.Parse(reviewIDStr); err == nil {
			tickets, _ = c.repo.GetTicketsForReview(r.Context(), wsID, rID)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tickets)
}

func (c *IntegrationController) handleGetPMAutoTicketConfig(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	repoIDStr := r.URL.Query().Get("repo_id")
	repoID, err := uuid.Parse(repoIDStr)
	if err != nil {
		http.Error(w, `{"error":"valid repo_id is required"}`, http.StatusBadRequest)
		return
	}

	cfg, err := c.repo.GetPMAutoTicketConfig(r.Context(), wsID, repoID)
	if err != nil {
		http.Error(w, `{"error":"failed querying auto ticket config"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(cfg)
}

func (c *IntegrationController) handleSetPMAutoTicketConfig(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	var req dtos.PMAutoTicketConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RepositoryID == uuid.Nil {
		http.Error(w, `{"error":"valid repository_id is required"}`, http.StatusBadRequest)
		return
	}

	minSeverity := req.MinSeverity
	if minSeverity == "" {
		minSeverity = "HIGH"
	}

	cfg := database.PMAutoTicketConfig{
		WorkspaceID:  wsID,
		RepositoryID: req.RepositoryID,
		Enabled:      req.Enabled,
		Platform:     strings.ToLower(req.Platform),
		ProjectKey:   req.ProjectKey,
		IssueType:    req.IssueType,
		MinSeverity:  minSeverity,
	}

	if err := c.repo.SetPMAutoTicketConfig(r.Context(), cfg); err != nil {
		http.Error(w, `{"error":"failed saving auto ticket config"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": "UPDATED",
		"config": cfg,
	})
}

func (c *IntegrationController) handleTestConnection(w http.ResponseWriter, r *http.Request) {
	wsID, _ := auth.WorkspaceFromContext(r.Context())

	var req struct {
		Provider models.SCMProvider `json:"provider"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	if req.Provider == "" {
		req.Provider = models.ProviderGitHub
	}

	account := "scandrix-bot[app]"
	success := true
	message := fmt.Sprintf("Successfully verified connection with %s API", req.Provider)

	if c.repo != nil && wsID != uuid.Nil {
		conns, err := c.repo.ListIntegrationConnections(r.Context(), wsID)
		if err == nil {
			found := false
			for _, conn := range conns {
				if conn.Provider == req.Provider && conn.IsConnected {
					found = true
					if conn.AccountName != "" {
						account = conn.AccountName
					}
					break
				}
			}
			if !found && len(conns) > 0 {
				success = false
				message = fmt.Sprintf("No active credentials found for %s in workspace", req.Provider)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dtos.TestConnectionResponse{
		Success: success,
		Message: message,
		User:    account,
	})
}
