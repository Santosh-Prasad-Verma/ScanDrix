package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/pkg/models"
)

// GitHubRepository defines the data access contract for GitHub app integrations (Clean Architecture).
type GitHubRepository interface {
	GetIntegrationConnection(ctx context.Context, wsID uuid.UUID, provider models.SCMProvider) (*models.IntegrationConnection, error)
	ListTrackedRepositories(ctx context.Context, wsID uuid.UUID) ([]models.TrackedRepository, error)
}

// GitHubController handles GitHub app integration callbacks and organization discovery.
type GitHubController struct {
	repo       GitHubRepository
	httpClient *http.Client
}

// NewGitHubController initializes the GitHub integration controller.
func NewGitHubController(repo GitHubRepository) *GitHubController {
	if isNilInterface(repo) {
		repo = nil
	}
	return &GitHubController{
		repo: repo,
		httpClient: &http.Client{
			Timeout: 8 * time.Second,
		},
	}
}

// Routes mounts the GitHub handshake endpoints.
func (c *GitHubController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/integration", c.handleGetIntegrationStatus)
	r.Get("/organization-name", c.handleGetOrganizationName)

	return r
}

// handleGetIntegrationStatus confirms a GitHub app installation handshake for the web client.
func (c *GitHubController) handleGetIntegrationStatus(w http.ResponseWriter, r *http.Request) {
	wsID, _ := auth.WorkspaceFromContext(r.Context())
	installID := strings.TrimSpace(r.URL.Query().Get("installId"))
	if installID == "" {
		installID = strings.TrimSpace(r.URL.Query().Get("installation_id"))
	}

	accountName := c.resolveActiveGitHubAccount(r.Context(), wsID)

	// When a user has just installed the GitHub App on github.com and returns to /github-integration?installation_id=...,
	// the integration handshake with the current workspace is not yet finalized.
	// Returning "PENDING" allows the web frontend (github-integration.tsx) to call createCodeManagementIntegration,
	// store the installation mapping, and redirect to /settings/git/repositories.
	status := "SUCCESS"
	if installID != "" {
		isLinked := false
		if c.repo != nil && wsID != uuid.Nil {
			if conn, err := c.repo.GetIntegrationConnection(r.Context(), wsID, models.ProviderGitHub); err == nil && conn.IsConnected {
				if repos, err := c.repo.ListTrackedRepositories(r.Context(), wsID); err == nil && len(repos) > 0 {
					isLinked = true
				}
			}
		}
		if !isLinked {
			status = "PENDING"
		}
	}

	respData := map[string]any{
		"status":           status,
		"organizationName": accountName,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":           status,
		"organizationName": accountName,
		"data":             respData,
	})
}

// handleGetOrganizationName returns the GitHub organization or username for the active installation.
func (c *GitHubController) handleGetOrganizationName(w http.ResponseWriter, r *http.Request) {
	wsID, _ := auth.WorkspaceFromContext(r.Context())

	accountName := c.resolveActiveGitHubAccount(r.Context(), wsID)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data":             accountName,
		"organizationName": accountName,
	})
}

// resolveActiveGitHubAccount resolves the authenticated GitHub account name from DB, live token, or environment.
func (c *GitHubController) resolveActiveGitHubAccount(ctx context.Context, wsID uuid.UUID) string {
	// 1. Check workspace-specific integration in database
	if c.repo != nil && wsID != uuid.Nil {
		if conn, err := c.repo.GetIntegrationConnection(ctx, wsID, models.ProviderGitHub); err == nil && conn.AccountName != "" {
			return conn.AccountName
		}
	}

	// 2. Check environment user
	if envUser := os.Getenv("GITHUB_USER"); envUser != "" {
		return envUser
	}

	// 3. Check live GITHUB_TOKEN against api.github.com/user
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
		if err == nil {
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Accept", "application/vnd.github+json")
			req.Header.Set("User-Agent", "ScanDrix-Platform")
			if resp, err := c.httpClient.Do(req); err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					var u struct {
						Login string `json:"login"`
					}
					if json.NewDecoder(resp.Body).Decode(&u) == nil && u.Login != "" {
						return u.Login
					}
				}
			}
		}
	}

	// 4. Clean fallback without hardcoded personal identities
	return "Connected Account"
}
