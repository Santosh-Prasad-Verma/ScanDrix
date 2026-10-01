package controllers

import (
	"context"
	"encoding/json"
	"net/http"
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

// resolveActiveGitHubAccount returns the GitHub account name connected to the
// calling workspace, or "" when the workspace has no GitHub integration.
//
// AUDIT_REMEDIATION.md F-44. This used to fall back, in order, to the
// GITHUB_USER environment variable and then to a live authenticated call to
// api.github.com/user using the platform's GITHUB_TOKEN. Both were reachable
// from an unauthenticated request, so:
//
//   - any anonymous caller could learn which GitHub account the platform is
//     connected as, and
//   - any anonymous caller could make the platform issue an outbound API call
//     with its own token, and
//   - once authentication was required, the same fallbacks would still have
//     disclosed the *operator's* account to every tenant that has not linked
//     GitHub, which is a cross-tenant leak rather than a per-tenant one.
//
// A workspace either has its own integration record or it has nothing to show.
// The previous final fallback returned the literal string "Connected Account",
// which reads as a real account name in the UI while being a fabrication
// (AGENTS.md 2.2), so unknown is now reported as unknown.
func (c *GitHubController) resolveActiveGitHubAccount(ctx context.Context, wsID uuid.UUID) string {
	if c.repo == nil || wsID == uuid.Nil {
		return ""
	}
	conn, err := c.repo.GetIntegrationConnection(ctx, wsID, models.ProviderGitHub)
	if err != nil || conn == nil {
		return ""
	}
	return conn.AccountName
}
