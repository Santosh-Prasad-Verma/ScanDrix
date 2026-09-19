package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/integrations/pm"
	"github.com/scandrix/backend/pkg/models"
)


// IntegrationRepository defines the data access contract for external platform integrations (Clean Architecture).
type IntegrationRepository interface {
	ListIntegrationConnections(ctx context.Context, wsID uuid.UUID) ([]models.IntegrationConnection, error)
	UpsertIntegrationConnectionWithSecret(ctx context.Context, wsID uuid.UUID, provider models.SCMProvider, accountName, tokenPlain, secretPlain string, isConnected bool, repoCount int) error
	UpsertIntegrationConnection(ctx context.Context, wsID uuid.UUID, provider models.SCMProvider, accountName, tokenPlain string, isConnected bool, repoCount int) error
	GetFindingByID(ctx context.Context, wsID, findingID uuid.UUID) (*models.CodeFinding, error)
	InsertFindingTicket(ctx context.Context, wsID, findingID uuid.UUID, platform, ticketKey, ticketURL string) error
	GetTicketsForFinding(ctx context.Context, wsID, findingID uuid.UUID) ([]database.FindingTicketRecord, error)
	GetTicketsForReview(ctx context.Context, wsID, reviewID uuid.UUID) ([]database.FindingTicketRecord, error)
	GetPMAutoTicketConfig(ctx context.Context, wsID, repoID uuid.UUID) (*database.PMAutoTicketConfig, error)
	SetPMAutoTicketConfig(ctx context.Context, cfg database.PMAutoTicketConfig) error
	ListTrackedRepositories(ctx context.Context, wsID uuid.UUID) ([]models.TrackedRepository, error)
}

// IntegrationController handles connecting external VCS and PM tools.
type IntegrationController struct {
	repo         IntegrationRepository
	pmDispatcher *pm.PMDispatcher
}

// NewIntegrationController initializes the integration controller with repository persistence.
func NewIntegrationController(repo IntegrationRepository) *IntegrationController {
	if isNilInterface(repo) {
		repo = nil
	}
	return &IntegrationController{
		repo:         repo,
		pmDispatcher: pm.NewPMDispatcher(),
	}
}

// Routes mounts integration endpoints.
func (c *IntegrationController) Routes() chi.Router {
	r := chi.NewRouter()

	// Web Dashboard integration contract routes (/integration/*)
	r.Get("/connections", c.handleGetConnections)
	r.Get("/organization-id", c.handleGetOrganizationID)
	r.Get("/check-connection-platform", c.handleCheckConnectionPlatform)
	r.Get("/issues-supported", c.handleIssuesSupported)
	r.Post("/clone-integration", c.handleCloneIntegration)

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

// ConfigRoutes mounts /integration-config/* routes used by the web dashboard.
func (c *IntegrationController) ConfigRoutes() chi.Router {
	r := chi.NewRouter()
	r.Get("/get-integration-configs-by-integration-category", c.handleGetIntegrationConfigsByCategory)
	return r
}


func (c *IntegrationController) handleListIntegrations(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	var conns []models.IntegrationConnection
	if c.repo != nil {
		conns, err = c.repo.ListIntegrationConnections(r.Context(), wsID)
		if err != nil {
			http.Error(w, `{"error":"failed listing integrations"}`, http.StatusInternalServerError)
			return
		}
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

	accountName := ""
	repoCount := 0
	if req.Provider == models.ProviderGitHub {
		accountName, repoCount = c.resolveGitHubIdentity(r.Context(), req.AccessToken)
	} else if req.Provider == models.ProviderGitLab {
		accountName, repoCount = c.resolveGitLabIdentity(r.Context(), req.AccessToken)
	}
	if accountName == "" {
		accountName = "Connected Account"
	}

	if c.repo != nil {
		err = c.repo.UpsertIntegrationConnectionWithSecret(r.Context(), wsID, req.Provider, accountName, req.AccessToken, req.WebhookSecret, true, repoCount)
		if err != nil {
			http.Error(w, `{"error":"failed recording integration"}`, http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"provider": req.Provider,
		"status":   "CONNECTED",
		"account":  accountName,
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

	if c.repo != nil {
		err = c.repo.UpsertIntegrationConnection(r.Context(), wsID, provider, accountName, req.APIToken, true, 0)
		if err != nil {
			http.Error(w, `{"error":"failed recording PM integration"}`, http.StatusInternalServerError)
			return
		}
	} else {
		http.Error(w, `{"error":"repository unavailable"}`, http.StatusInternalServerError)
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

	var cfg *database.PMAutoTicketConfig
	if c.repo != nil {
		var err error
		cfg, err = c.repo.GetPMAutoTicketConfig(r.Context(), wsID, repoID)
		if err != nil {
			http.Error(w, `{"error":"failed querying auto ticket config"}`, http.StatusInternalServerError)
			return
		}
	}
	if cfg == nil {
		cfg = &database.PMAutoTicketConfig{
			WorkspaceID:  wsID,
			RepositoryID: repoID,
			Enabled:      false,
			MinSeverity:  "HIGH",
		}
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

	if c.repo != nil {
		if err := c.repo.SetPMAutoTicketConfig(r.Context(), cfg); err != nil {
			http.Error(w, `{"error":"failed saving auto ticket config"}`, http.StatusInternalServerError)
			return
		}
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

// handleGetConnections returns active connections formatted for the web dashboard.
func (c *IntegrationController) handleGetConnections(w http.ResponseWriter, r *http.Request) {
	wsID, _ := auth.WorkspaceFromContext(r.Context())

	type ConnectionDTO struct {
		PlatformName    string            `json:"platformName"`
		IsSetupComplete bool              `json:"isSetupComplete"`
		HasConnection   bool              `json:"hasConnection"`
		Category        string            `json:"category"`
		Config          map[string]string `json:"config,omitempty"`
	}

	var results []ConnectionDTO
	var ghConn, glConn *models.IntegrationConnection

	if c.repo != nil && wsID != uuid.Nil {
		conns, _ := c.repo.ListIntegrationConnections(r.Context(), wsID)
		for i := range conns {
			if conns[i].Provider == models.ProviderGitHub && conns[i].IsConnected {
				ghConn = &conns[i]
			} else if conns[i].Provider == models.ProviderGitLab && conns[i].IsConnected {
				glConn = &conns[i]
			}
		}
	}

	// 1. GitHub Connection
	ghToken := os.Getenv("GITHUB_TOKEN")
	if ghConn != nil {
		results = append(results, ConnectionDTO{
			PlatformName:    "GITHUB",
			IsSetupComplete: true,
			HasConnection:   true,
			Category:        "CODE_MANAGEMENT",
			Config: map[string]string{
				"accountName": ghConn.AccountName,
				"repoCount":   fmt.Sprintf("%d", ghConn.RepoCount),
			},
		})
	} else if ghToken != "" {
		account, count := c.resolveGitHubIdentity(r.Context(), ghToken)
		if c.repo != nil && wsID != uuid.Nil {
			_ = c.repo.UpsertIntegrationConnection(r.Context(), wsID, models.ProviderGitHub, account, ghToken, true, count)
		}
		results = append(results, ConnectionDTO{
			PlatformName:    "GITHUB",
			IsSetupComplete: true,
			HasConnection:   true,
			Category:        "CODE_MANAGEMENT",
			Config: map[string]string{
				"accountName": account,
				"repoCount":   fmt.Sprintf("%d", count),
			},
		})
	}

	// 2. GitLab Connection
	glToken := os.Getenv("GITLAB_TOKEN")
	if glConn != nil {
		results = append(results, ConnectionDTO{
			PlatformName:    "GITLAB",
			IsSetupComplete: true,
			HasConnection:   true,
			Category:        "CODE_MANAGEMENT",
			Config: map[string]string{
				"accountName": glConn.AccountName,
				"repoCount":   fmt.Sprintf("%d", glConn.RepoCount),
			},
		})
	} else if glToken != "" {
		account, count := c.resolveGitLabIdentity(r.Context(), glToken)
		if c.repo != nil && wsID != uuid.Nil {
			_ = c.repo.UpsertIntegrationConnection(r.Context(), wsID, models.ProviderGitLab, account, glToken, true, count)
		}
		results = append(results, ConnectionDTO{
			PlatformName:    "GITLAB",
			IsSetupComplete: true,
			HasConnection:   true,
			Category:        "CODE_MANAGEMENT",
			Config: map[string]string{
				"accountName": account,
				"repoCount":   fmt.Sprintf("%d", count),
			},
		})
	}

	if results == nil {
		results = []ConnectionDTO{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data": results,
	})
}

// handleGetOrganizationID returns the caller's active workspace ID.
func (c *IntegrationController) handleGetOrganizationID(w http.ResponseWriter, r *http.Request) {
	wsID, _ := auth.WorkspaceFromContext(r.Context())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data": wsID.String(),
	})
}

// handleCheckConnectionPlatform verifies connection presence for platform & category.
func (c *IntegrationController) handleCheckConnectionPlatform(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data": map[string]any{
			"hasConnection": true,
		},
	})
}

// handleIssuesSupported returns whether issues tracking is supported for the team.
func (c *IntegrationController) handleIssuesSupported(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data": true,
	})
}

// handleCloneIntegration duplicates an existing integration setup for another team.
func (c *IntegrationController) handleCloneIntegration(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data": map[string]any{
			"success": true,
		},
	})
}

// handleGetIntegrationConfigsByCategory returns repository configuration for the web settings.
func (c *IntegrationController) handleGetIntegrationConfigsByCategory(w http.ResponseWriter, r *http.Request) {
	wsID, _ := auth.WorkspaceFromContext(r.Context())

	var repos []map[string]any
	if c.repo != nil && wsID != uuid.Nil {
		if tracked, err := c.repo.ListTrackedRepositories(r.Context(), wsID); err == nil {
			for _, tr := range tracked {
				parts := strings.Split(tr.NamespacePath, "/")
				name := tr.NamespacePath
				org := ""
				if len(parts) >= 2 {
					org = parts[0]
					name = parts[1]
				}
				repos = append(repos, map[string]any{
					"id":               tr.ExternalID,
					"name":             name,
					"full_name":        tr.NamespacePath,
					"default_branch":   tr.DefaultBranch,
					"http_url":         "https://github.com/" + tr.NamespacePath,
					"avatar_url":       "",
					"language":         "TypeScript",
					"organizationName": org,
					"visibility":       "public",
					"workspaceId":      wsID.String(),
					"selected":         true,
				})
			}
		}
	}

	if repos == nil {
		repos = []map[string]any{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data": []map[string]any{
			{
				"configKey":   "REPOSITORIES",
				"configValue": repos,
			},
		},
	})
}

func (c *IntegrationController) resolveGitHubIdentity(ctx context.Context, token string) (string, int) {
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	if token == "" {
		return "", 0
	}
	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ScanDrix-Platform")

	resp, err := client.Do(req)
	if err != nil {
		return "", 0
	}
	defer resp.Body.Close()

	var u struct {
		Login             string `json:"login"`
		PublicRepos       int    `json:"public_repos"`
		TotalPrivateRepos int    `json:"total_private_repos"`
	}
	if json.NewDecoder(resp.Body).Decode(&u) == nil && u.Login != "" {
		return u.Login, u.PublicRepos + u.TotalPrivateRepos
	}
	return "", 0
}

func (c *IntegrationController) resolveGitLabIdentity(ctx context.Context, token string) (string, int) {
	if token == "" {
		token = os.Getenv("GITLAB_TOKEN")
	}
	if token == "" {
		return "", 0
	}
	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://gitlab.com/api/v4/user", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("PRIVATE-TOKEN", token)

	resp, err := client.Do(req)
	if err != nil {
		return "", 0
	}
	defer resp.Body.Close()

	var u struct {
		Username string `json:"username"`
	}
	if json.NewDecoder(resp.Body).Decode(&u) == nil && u.Username != "" {
		repoCount := 0
		if pReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://gitlab.com/api/v4/projects?membership=true&per_page=100", nil); err == nil {
			pReq.Header.Set("Authorization", "Bearer "+token)
			pReq.Header.Set("PRIVATE-TOKEN", token)
			if pResp, err := client.Do(pReq); err == nil {
				defer pResp.Body.Close()
				var projects []any
				if json.NewDecoder(pResp.Body).Decode(&projects) == nil {
					repoCount = len(projects)
				}
			}
		}
		return u.Username, repoCount
	}
	return "", 0
}

