package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/auth/oauth"
	"github.com/scandrix/backend/internal/codeanalysis/graph"
	"github.com/scandrix/backend/pkg/models"
)

// CodeManagementRepository defines the data access contract for repository and code tree operations (Clean Architecture).
type CodeManagementRepository interface {
	ListTrackedRepositories(ctx context.Context, wsID uuid.UUID) ([]models.TrackedRepository, error)
	GetIntegrationConnection(ctx context.Context, wsID uuid.UUID, provider models.SCMProvider) (*models.IntegrationConnection, error)
	TrackRepository(ctx context.Context, wsID uuid.UUID, provider models.SCMProvider, externalID, namespacePath, defaultBranch string) (*models.TrackedRepository, error)
	UpdateIntegrationRepoCount(ctx context.Context, wsID uuid.UUID, count int) error
	UpsertIntegrationConnection(ctx context.Context, wsID uuid.UUID, provider models.SCMProvider, accountName, tokenPlain string, isConnected bool, repoCount int) error
	DeleteIntegrationConnection(ctx context.Context, wsID uuid.UUID, provider models.SCMProvider) error
	UntrackAllRepositories(ctx context.Context, wsID uuid.UUID, provider models.SCMProvider) error
	InsertAuditLog(ctx context.Context, wsID uuid.UUID, actorID, actorEmail, ipAddress, action, targetType, targetID string, metadata []byte) error
	GetASTNodesByRepository(ctx context.Context, repoID uuid.UUID) ([]graph.ASTNode, error)
}

// CodeManagementController manages tracked repositories, branches, and code structures.
type CodeManagementController struct {
	repo         CodeManagementRepository
	oauthService *oauth.OAuthService
	httpClient   *http.Client
}

// NewCodeManagementController initializes the repository management controller with repository persistence.
func NewCodeManagementController(repo CodeManagementRepository) *CodeManagementController {
	if isNilInterface(repo) {
		repo = nil
	}
	return &CodeManagementController{
		repo: repo,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// SetOAuthService sets the OAuth service for authorization code exchange.
func (c *CodeManagementController) SetOAuthService(svc *oauth.OAuthService) {
	c.oauthService = svc
}

// WebRepositoryItem matches the frontend Repository type in apps/web/src/lib/services/codeManagement/types.ts
type WebRepositoryItem struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	FullName         string `json:"full_name"`
	DefaultBranch    string `json:"default_branch"`
	HTTPUrl          string `json:"http_url"`
	AvatarURL        string `json:"avatar_url"`
	Language         string `json:"language"`
	OrganizationName string `json:"organizationName"`
	Visibility       string `json:"visibility"`
	WorkspaceID      string `json:"workspaceId,omitempty"`
	Selected         bool   `json:"selected"`
	LastActivityAt   string `json:"lastActivityAt,omitempty"`
}

// WebPullRequestItem matches the frontend PullRequest structure in apps/web/src/lib/services/codeManagement/hooks.ts
type WebPullRequestItem struct {
	ID             string     `json:"id"`
	PullNumber     int        `json:"pull_number"`
	Repository     WebRepoRef `json:"repository"`
	Title          string     `json:"title"`
	URL            string     `json:"url"`
	LastActivityAt string     `json:"lastActivityAt,omitempty"`
}

// WebRepoRef is the nested repository identifier expected in PR payloads
type WebRepoRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Routes mounts repository management routes for both Web dashboard and CLI.
func (c *CodeManagementController) Routes() chi.Router {
	r := chi.NewRouter()

	// Web Dashboard routes (/code-management/*)
	r.Get("/repositories/org", c.handleListOrgRepositories)
	r.Get("/repositories/selected", c.handleListSelectedRepositories)
	r.Post("/repositories", c.handleSaveRepositories)
	r.Post("/auth-integration", c.handleAuthIntegration)
	r.Post("/finish-onboarding", c.handleFinishOnboarding)
	r.Delete("/delete-integration", c.handleDeleteIntegration)
	r.Delete("/delete-integration-and-repositories", c.handleDeleteIntegration)
	r.Get("/get-prs", c.handleGetPullRequests)
	r.Get("/get-prs-repo", c.handleGetPullRequests)

	// CLI & Core API routes
	r.Get("/", c.handleListRepositories)
	r.Post("/track", c.handleTrackRepository)
	r.Get("/{id}/branches", c.handleListBranches)
	r.Get("/{id}/tree", c.handleGetFileTree)

	return r
}

// CLIRepositoriesConfigRoutes mounts CLI repository configuration routes for /cli/config/repositories.
func (c *CodeManagementController) CLIRepositoriesConfigRoutes(paramCtrl *ParametersController) chi.Router {
	r := chi.NewRouter()
	r.Get("/available", c.handleListOrgRepositories)
	r.Get("/selected", c.handleListSelectedRepositories)
	r.Post("/", c.handleSaveRepositories)
	if paramCtrl != nil {
		r.Get("/{id}/settings", paramCtrl.handleGetCodeReviewParameter)
		r.Patch("/{id}/settings", paramCtrl.handleCreateOrUpdateCodeReview)
	}
	return r
}

// handleListOrgRepositories returns all real repositories available to the connected SCM account (GitHub/GitLab).
func (c *CodeManagementController) handleListOrgRepositories(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	platform := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("platform")))

	// 2. Fetch tracked repositories to mark which ones are currently selected
	trackedMap := make(map[string]bool)
	if c.repo != nil {
		if tracked, err := c.repo.ListTrackedRepositories(r.Context(), wsID); err == nil {
			for _, tr := range tracked {
				trackedMap[tr.NamespacePath] = true
				trackedMap[tr.ExternalID] = true
			}
		}
	}

	var repos []WebRepositoryItem

	if platform == "github" {
		token := os.Getenv("GITHUB_TOKEN")
		if c.repo != nil && wsID != uuid.Nil {
			if ghConn, err := c.repo.GetIntegrationConnection(r.Context(), wsID, models.ProviderGitHub); err == nil && ghConn.IsConnected && ghConn.AccessTokenEnc != "" {
				token = ghConn.AccessTokenEnc
			}
		}
		repos = c.fetchGitHubRepositories(r.Context(), token, wsID, trackedMap)
	} else if platform == "gitlab" {
		token := os.Getenv("GITLAB_TOKEN")
		if c.repo != nil && wsID != uuid.Nil {
			if glConn, err := c.repo.GetIntegrationConnection(r.Context(), wsID, models.ProviderGitLab); err == nil && glConn.IsConnected && glConn.AccessTokenEnc != "" {
				token = glConn.AccessTokenEnc
			}
		}
		repos = c.fetchGitLabRepositories(r.Context(), token, wsID, trackedMap)
	} else {
		// No specific platform query (standard web dashboard call) -> aggregate from all connected providers
		ghToken := os.Getenv("GITHUB_TOKEN")
		if c.repo != nil && wsID != uuid.Nil {
			if ghConn, err := c.repo.GetIntegrationConnection(r.Context(), wsID, models.ProviderGitHub); err == nil && ghConn.IsConnected && ghConn.AccessTokenEnc != "" {
				ghToken = ghConn.AccessTokenEnc
			}
		}
		if ghToken != "" {
			ghRepos := c.fetchGitHubRepositories(r.Context(), ghToken, wsID, trackedMap)
			repos = append(repos, ghRepos...)
		}

		glToken := os.Getenv("GITLAB_TOKEN")
		if c.repo != nil && wsID != uuid.Nil {
			if glConn, err := c.repo.GetIntegrationConnection(r.Context(), wsID, models.ProviderGitLab); err == nil && glConn.IsConnected && glConn.AccessTokenEnc != "" {
				glToken = glConn.AccessTokenEnc
			}
		}
		if glToken != "" {
			glRepos := c.fetchGitLabRepositories(r.Context(), glToken, wsID, trackedMap)
			repos = append(repos, glRepos...)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data": repos,
	})
}

// handleListSelectedRepositories returns the repositories currently tracked in the database.
func (c *CodeManagementController) handleListSelectedRepositories(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	var minimalRepos []map[string]any
	if c.repo != nil {
		if tracked, err := c.repo.ListTrackedRepositories(r.Context(), wsID); err == nil {
			for _, tr := range tracked {
				parts := strings.Split(tr.NamespacePath, "/")
				name := tr.NamespacePath
				org := ""
				if len(parts) >= 2 {
					org = parts[0]
					name = parts[1]
				}
				minimalRepos = append(minimalRepos, map[string]any{
					"id":               tr.ExternalID,
					"name":             name,
					"full_name":        tr.NamespacePath,
					"organizationName": org,
					"default_branch":   tr.DefaultBranch,
				})
			}
		}
	}

	if minimalRepos == nil {
		minimalRepos = []map[string]any{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data": minimalRepos,
	})
}

// handleSaveRepositories persists user-selected repositories from the web dashboard.
func (c *CodeManagementController) handleSaveRepositories(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	var req struct {
		Repositories []WebRepositoryItem `json:"repositories"`
		TeamID       string              `json:"teamId"`
		Type         string              `json:"type"` // "replace" | "append"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
		return
	}

	if c.repo == nil {
		http.Error(w, `{"error":"database service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	savedCount := 0
	for _, repo := range req.Repositories {
		if repo.FullName == "" {
			continue
		}
		defBranch := repo.DefaultBranch
		if defBranch == "" {
			defBranch = "main"
		}
		extID := repo.ID
		if extID == "" {
			extID = repo.FullName
		}
		provider := models.ProviderGitHub
		if strings.Contains(repo.HTTPUrl, "gitlab.com") {
			provider = models.ProviderGitLab
		}
		_, err := c.repo.TrackRepository(r.Context(), wsID, provider, extID, repo.FullName, defBranch)
		if err == nil {
			savedCount++
		}
	}

	// Update integration connection repository count
	_ = c.repo.UpdateIntegrationRepoCount(r.Context(), wsID, savedCount)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"statusCode": http.StatusOK,
		"data": map[string]any{
			"success": true,
			"count":   savedCount,
		},
	})
}

// handleAuthIntegration completes the OAuth handshake or App installation handshake.
func (c *CodeManagementController) handleAuthIntegration(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	var req struct {
		Code                    string `json:"code"`
		IntegrationType         string `json:"integrationType"` // "GITHUB" | "GITLAB"
		Token                   string `json:"token"`
		Username                string `json:"username"`
		Email                   string `json:"email"`
		OrgName                 string `json:"orgName"`
		OrganizationAndTeamData struct {
			OrganizationID string `json:"organizationId"`
			TeamID         string `json:"teamId"`
		} `json:"organizationAndTeamData"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
		return
	}

	providerType := strings.ToLower(strings.TrimSpace(req.IntegrationType))
	provider := models.ProviderGitHub
	if strings.Contains(providerType, "gitlab") {
		provider = models.ProviderGitLab
	}

	token := req.Token
	if token == "" && req.Code != "" {
		if strings.HasPrefix(req.Code, "gho_") || strings.HasPrefix(req.Code, "glpat-") {
			token = req.Code
		} else if c.oauthService != nil {
			oauthProv := oauth.ProviderGitHub
			if provider == models.ProviderGitLab {
				oauthProv = oauth.ProviderGitLab
			}
			exchangedToken, err := c.oauthService.ExchangeAccessToken(r.Context(), oauthProv, req.Code)
			if err == nil && exchangedToken != "" {
				token = exchangedToken
			} else if err != nil {
				slog.Warn("Failed to exchange OAuth code for access token in handleAuthIntegration", "provider", provider, "error", err)
			}
		}
	}
	if token == "" {
		if provider == models.ProviderGitHub {
			token = os.Getenv("GITHUB_TOKEN")
		} else {
			token = os.Getenv("GITLAB_TOKEN")
		}
	}

	// Query real user info from provider to avoid any hardcoded accounts
	accountName := req.Username
	repoCount := 0
	if provider == models.ProviderGitHub {
		accountName, repoCount = c.resolveGitHubIdentity(r.Context(), token)
	} else {
		accountName, repoCount = c.resolveGitLabIdentity(r.Context(), token)
	}

	if accountName == "" {
		accountName = req.OrgName
	}
	if accountName == "" {
		accountName = "Connected Account"
	}

	if c.repo != nil {
		_ = c.repo.UpsertIntegrationConnection(r.Context(), wsID, provider, accountName, token, true, repoCount)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"statusCode": http.StatusOK,
		"data": map[string]any{
			"success": true,
			"status":  "SUCCESS",
		},
	})
}

// handleFinishOnboarding concludes the initial workspace setup.
func (c *CodeManagementController) handleFinishOnboarding(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"statusCode": http.StatusOK,
		"data": map[string]any{
			"success": true,
		},
	})
}

// handleDeleteIntegration removes the integration and all associated tracked repositories.
func (c *CodeManagementController) handleDeleteIntegration(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	if c.repo != nil {
		_ = c.repo.DeleteIntegrationConnection(r.Context(), wsID, models.ProviderGitHub)
		_ = c.repo.DeleteIntegrationConnection(r.Context(), wsID, models.ProviderGitLab)
		_ = c.repo.UntrackAllRepositories(r.Context(), wsID, models.ProviderGitHub)
		_ = c.repo.UntrackAllRepositories(r.Context(), wsID, models.ProviderGitLab)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"statusCode": http.StatusOK,
		"data": map[string]any{
			"success": true,
		},
	})
}

// handleGetPullRequests returns PRs for onboarding or repo inspection.
func (c *CodeManagementController) handleGetPullRequests(w http.ResponseWriter, r *http.Request) {
	wsID, _ := auth.WorkspaceFromContext(r.Context())
	numberFilter := strings.TrimSpace(r.URL.Query().Get("number"))
	titleFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("title")))
	repoIDFilter := strings.TrimSpace(r.URL.Query().Get("repositoryId"))
	repoNameFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("repositoryName")))
	if repoNameFilter == "" {
		repoNameFilter = strings.ToLower(strings.TrimSpace(r.URL.Query().Get("repository")))
	}

	prs := make([]WebPullRequestItem, 0)

	// Fetch tracked repos from DB
	var trackedRepos []models.TrackedRepository
	if c.repo != nil && wsID != uuid.Nil {
		if trs, err := c.repo.ListTrackedRepositories(r.Context(), wsID); err == nil {
			trackedRepos = trs
		}
	}

	// Resolve GitHub token
	ghToken := os.Getenv("GITHUB_TOKEN")
	if c.repo != nil && wsID != uuid.Nil {
		if ghConn, err := c.repo.GetIntegrationConnection(r.Context(), wsID, models.ProviderGitHub); err == nil && ghConn.IsConnected && ghConn.AccessTokenEnc != "" {
			ghToken = ghConn.AccessTokenEnc
		}
	}

	// Resolve GitLab token
	glToken := os.Getenv("GITLAB_TOKEN")
	if c.repo != nil && wsID != uuid.Nil {
		if glConn, err := c.repo.GetIntegrationConnection(r.Context(), wsID, models.ProviderGitLab); err == nil && glConn.IsConnected && glConn.AccessTokenEnc != "" {
			glToken = glConn.AccessTokenEnc
		}
	}

	// 1. Fetch from tracked repositories if any exist
	if len(trackedRepos) > 0 {
		for _, tr := range trackedRepos {
			if repoIDFilter != "" && tr.ExternalID != repoIDFilter && tr.ID.String() != repoIDFilter {
				continue
			}
			if repoNameFilter != "" && !strings.Contains(strings.ToLower(tr.NamespacePath), repoNameFilter) {
				continue
			}

			if tr.Provider == models.ProviderGitHub && ghToken != "" {
				ghPrs := c.fetchGitHubPullRequestsForRepo(r.Context(), ghToken, tr.NamespacePath, tr.ExternalID)
				prs = append(prs, ghPrs...)
			} else if tr.Provider == models.ProviderGitLab && glToken != "" {
				glPrs := c.fetchGitLabMergeRequestsForProject(r.Context(), glToken, tr.NamespacePath, tr.ExternalID)
				prs = append(prs, glPrs...)
			}
		}
	} else {
		// If no repos are tracked yet, fetch active PRs from connected user repositories
		if ghToken != "" {
			ghPrs := c.fetchGitHubUserPullRequests(r.Context(), ghToken)
			prs = append(prs, ghPrs...)
		}
		if glToken != "" {
			glPrs := c.fetchGitLabUserMergeRequests(r.Context(), glToken)
			prs = append(prs, glPrs...)
		}
	}

	// Apply query filters
	filtered := make([]WebPullRequestItem, 0, len(prs))
	for _, p := range prs {
		if numberFilter != "" && fmt.Sprintf("%d", p.PullNumber) != numberFilter {
			continue
		}
		if titleFilter != "" && !strings.Contains(strings.ToLower(p.Title), titleFilter) {
			continue
		}
		if repoIDFilter != "" && p.Repository.ID != repoIDFilter {
			continue
		}
		if repoNameFilter != "" && !strings.Contains(strings.ToLower(p.Repository.Name), repoNameFilter) {
			continue
		}
		filtered = append(filtered, p)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data": filtered,
	})
}

func (c *CodeManagementController) fetchGitHubPullRequestsForRepo(ctx context.Context, token, repoFullName, repoID string) []WebPullRequestItem {
	url := fmt.Sprintf("https://api.github.com/repos/%s/pulls?state=all&per_page=20&sort=updated&direction=desc", repoFullName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ScanDrix-Platform")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var rawPulls []struct {
		ID        int64  `json:"id"`
		Number    int    `json:"number"`
		Title     string `json:"title"`
		HTMLURL   string `json:"html_url"`
		UpdatedAt string `json:"updated_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rawPulls); err != nil {
		return nil
	}

	items := make([]WebPullRequestItem, 0, len(rawPulls))
	for _, p := range rawPulls {
		items = append(items, WebPullRequestItem{
			ID:         fmt.Sprintf("%d", p.ID),
			PullNumber: p.Number,
			Repository: WebRepoRef{
				ID:   repoID,
				Name: repoFullName,
			},
			Title:          p.Title,
			URL:            p.HTMLURL,
			LastActivityAt: p.UpdatedAt,
		})
	}
	return items
}

func (c *CodeManagementController) fetchGitHubUserPullRequests(ctx context.Context, token string) []WebPullRequestItem {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user/repos?per_page=5&sort=updated", nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ScanDrix-Platform")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var rawRepos []struct {
		ID       int64  `json:"id"`
		FullName string `json:"full_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rawRepos); err != nil {
		return nil
	}

	var allPRs []WebPullRequestItem
	for _, r := range rawRepos {
		prs := c.fetchGitHubPullRequestsForRepo(ctx, token, r.FullName, fmt.Sprintf("%d", r.ID))
		allPRs = append(allPRs, prs...)
		if len(allPRs) >= 20 {
			break
		}
	}
	return allPRs
}

func (c *CodeManagementController) fetchGitLabMergeRequestsForProject(ctx context.Context, token, projectPath, projectID string) []WebPullRequestItem {
	url := fmt.Sprintf("https://gitlab.com/api/v4/projects/%s/merge_requests?state=all&per_page=20", projectID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("PRIVATE-TOKEN", token)
	req.Header.Set("User-Agent", "ScanDrix-Platform")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var rawMRs []struct {
		ID        int64  `json:"id"`
		IID       int    `json:"iid"`
		Title     string `json:"title"`
		WebURL    string `json:"web_url"`
		UpdatedAt string `json:"updated_at"`
		ProjectID int64  `json:"project_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rawMRs); err != nil {
		return nil
	}

	items := make([]WebPullRequestItem, 0, len(rawMRs))
	for _, m := range rawMRs {
		items = append(items, WebPullRequestItem{
			ID:         fmt.Sprintf("%d", m.ID),
			PullNumber: m.IID,
			Repository: WebRepoRef{
				ID:   projectID,
				Name: projectPath,
			},
			Title:          m.Title,
			URL:            m.WebURL,
			LastActivityAt: m.UpdatedAt,
		})
	}
	return items
}

func (c *CodeManagementController) fetchGitLabUserMergeRequests(ctx context.Context, token string) []WebPullRequestItem {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://gitlab.com/api/v4/merge_requests?scope=all&state=all&per_page=20", nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("PRIVATE-TOKEN", token)
	req.Header.Set("User-Agent", "ScanDrix-Platform")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var rawMRs []struct {
		ID         int64  `json:"id"`
		IID        int    `json:"iid"`
		Title      string `json:"title"`
		WebURL     string `json:"web_url"`
		UpdatedAt  string `json:"updated_at"`
		ProjectID  int64  `json:"project_id"`
		References struct {
			Full string `json:"full"`
		} `json:"references"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rawMRs); err != nil {
		return nil
	}

	items := make([]WebPullRequestItem, 0, len(rawMRs))
	for _, m := range rawMRs {
		name := m.References.Full
		if name == "" {
			name = fmt.Sprintf("project-%d", m.ProjectID)
		}
		items = append(items, WebPullRequestItem{
			ID:         fmt.Sprintf("%d", m.ID),
			PullNumber: m.IID,
			Repository: WebRepoRef{
				ID:   fmt.Sprintf("%d", m.ProjectID),
				Name: name,
			},
			Title:          m.Title,
			URL:            m.WebURL,
			LastActivityAt: m.UpdatedAt,
		})
	}
	return items
}

// ============================================================================
// Real Provider Resolvers & Fetchers (GitHub & GitLab)
// ============================================================================

func (c *CodeManagementController) fetchGitHubRepositories(ctx context.Context, token string, wsID uuid.UUID, tracked map[string]bool) []WebRepositoryItem {
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	if token == "" {
		return []WebRepositoryItem{}
	}

	repos, status := c.executeFetchGitHub(ctx, token, wsID, tracked)
	if status == http.StatusUnauthorized && token != os.Getenv("GITHUB_TOKEN") && os.Getenv("GITHUB_TOKEN") != "" {
		slog.Info("Retrying GitHub repos fetch with environment GITHUB_TOKEN")
		repos, _ = c.executeFetchGitHub(ctx, os.Getenv("GITHUB_TOKEN"), wsID, tracked)
	}
	return repos
}

func (c *CodeManagementController) executeFetchGitHub(ctx context.Context, token string, wsID uuid.UUID, tracked map[string]bool) ([]WebRepositoryItem, int) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user/repos?per_page=100&sort=updated", nil)
	if err != nil {
		return []WebRepositoryItem{}, 0
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ScanDrix-Platform")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		slog.Error("Failed fetching GitHub repositories", "error", err)
		return []WebRepositoryItem{}, 0
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Warn("GitHub repository API non-200", "status", resp.StatusCode)
		return []WebRepositoryItem{}, resp.StatusCode
	}

	var rawRepos []struct {
		ID            int64  `json:"id"`
		Name          string `json:"name"`
		FullName      string `json:"full_name"`
		DefaultBranch string `json:"default_branch"`
		HTMLURL       string `json:"html_url"`
		Language      string `json:"language"`
		Private       bool   `json:"private"`
		UpdatedAt     string `json:"updated_at"`
		Owner         struct {
			Login     string `json:"login"`
			AvatarURL string `json:"avatar_url"`
		} `json:"owner"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rawRepos); err != nil {
		return []WebRepositoryItem{}, resp.StatusCode
	}

	items := make([]WebRepositoryItem, 0, len(rawRepos))
	for _, r := range rawRepos {
		defBranch := r.DefaultBranch
		if defBranch == "" {
			defBranch = "main"
		}
		lang := r.Language
		if lang == "" {
			lang = "Unknown"
		}
		vis := "public"
		if r.Private {
			vis = "private"
		}
		idStr := fmt.Sprintf("%d", r.ID)
		isSelected := tracked[r.FullName] || tracked[idStr]

		items = append(items, WebRepositoryItem{
			ID:               idStr,
			Name:             r.Name,
			FullName:         r.FullName,
			DefaultBranch:    defBranch,
			HTTPUrl:          r.HTMLURL,
			AvatarURL:        r.Owner.AvatarURL,
			Language:         lang,
			OrganizationName: r.Owner.Login,
			Visibility:       vis,
			WorkspaceID:      wsID.String(),
			Selected:         isSelected,
			LastActivityAt:   r.UpdatedAt,
		})
	}
	return items, http.StatusOK
}

func (c *CodeManagementController) fetchGitLabRepositories(ctx context.Context, token string, wsID uuid.UUID, tracked map[string]bool) []WebRepositoryItem {
	if token == "" {
		token = os.Getenv("GITLAB_TOKEN")
	}
	if token == "" {
		return []WebRepositoryItem{}
	}

	repos, status := c.executeFetchGitLab(ctx, token, wsID, tracked)
	if status == http.StatusUnauthorized && token != os.Getenv("GITLAB_TOKEN") && os.Getenv("GITLAB_TOKEN") != "" {
		slog.Info("Retrying GitLab projects fetch with environment GITLAB_TOKEN")
		repos, _ = c.executeFetchGitLab(ctx, os.Getenv("GITLAB_TOKEN"), wsID, tracked)
	}
	return repos
}

func (c *CodeManagementController) executeFetchGitLab(ctx context.Context, token string, wsID uuid.UUID, tracked map[string]bool) ([]WebRepositoryItem, int) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://gitlab.com/api/v4/projects?membership=true&per_page=100", nil)
	if err != nil {
		return []WebRepositoryItem{}, 0
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("PRIVATE-TOKEN", token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		slog.Error("Failed fetching GitLab projects", "error", err)
		return []WebRepositoryItem{}, 0
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return []WebRepositoryItem{}, resp.StatusCode
	}

	var rawProjects []struct {
		ID                int64  `json:"id"`
		Name              string `json:"name"`
		PathWithNamespace string `json:"path_with_namespace"`
		DefaultBranch     string `json:"default_branch"`
		WebURL            string `json:"web_url"`
		AvatarURL         string `json:"avatar_url"`
		Visibility        string `json:"visibility"`
		LastActivityAt    string `json:"last_activity_at"`
		Namespace         struct {
			Name string `json:"name"`
		} `json:"namespace"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rawProjects); err != nil {
		return []WebRepositoryItem{}, resp.StatusCode
	}

	items := make([]WebRepositoryItem, 0, len(rawProjects))
	for _, p := range rawProjects {
		defBranch := p.DefaultBranch
		if defBranch == "" {
			defBranch = "main"
		}
		idStr := fmt.Sprintf("%d", p.ID)
		isSelected := tracked[p.PathWithNamespace] || tracked[idStr]

		items = append(items, WebRepositoryItem{
			ID:               idStr,
			Name:             p.Name,
			FullName:         p.PathWithNamespace,
			DefaultBranch:    defBranch,
			HTTPUrl:          p.WebURL,
			AvatarURL:        p.AvatarURL,
			Language:         "Unknown",
			OrganizationName: p.Namespace.Name,
			Visibility:       p.Visibility,
			WorkspaceID:      wsID.String(),
			Selected:         isSelected,
			LastActivityAt:   p.LastActivityAt,
		})
	}
	return items, http.StatusOK
}

func (c *CodeManagementController) resolveGitHubIdentity(ctx context.Context, token string) (string, int) {
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	if token == "" {
		return "", 0
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ScanDrix-Platform")

	resp, err := c.httpClient.Do(req)
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

func (c *CodeManagementController) resolveGitLabIdentity(ctx context.Context, token string) (string, int) {
	if token == "" {
		token = os.Getenv("GITLAB_TOKEN")
	}
	if token == "" {
		return "", 0
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://gitlab.com/api/v4/user", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("PRIVATE-TOKEN", token)

	resp, err := c.httpClient.Do(req)
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
			if pResp, err := c.httpClient.Do(pReq); err == nil {
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

// ============================================================================
// Core / CLI Repository Endpoints
// ============================================================================

func (c *CodeManagementController) handleListRepositories(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	var repos []models.TrackedRepository
	if c.repo != nil {
		repos, err = c.repo.ListTrackedRepositories(r.Context(), wsID)
		if err != nil {
			http.Error(w, `{"error":"failed listing repositories"}`, http.StatusInternalServerError)
			return
		}
	}

	res := make([]dtos.RepositoryResponse, 0, len(repos))
	for _, repo := range repos {
		res = append(res, dtos.RepositoryResponse{
			ID:            repo.ID,
			WorkspaceID:   repo.WorkspaceID,
			Provider:      repo.Provider,
			ExternalID:    repo.ExternalID,
			NamespacePath: repo.NamespacePath,
			DefaultBranch: repo.DefaultBranch,
			IsActive:      repo.IsActive,
			CreatedAt:     repo.CreatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (c *CodeManagementController) handleTrackRepository(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	profile, ok := auth.AccountProfileFromContext(r.Context())
	if ok && profile != nil && profile.Role == models.RoleViewer {
		http.Error(w, `{"error":"forbidden: viewers cannot track repositories"}`, http.StatusForbidden)
		return
	}

	var req dtos.TrackRepositoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.NamespacePath == "" {
		http.Error(w, `{"error":"namespace_path is required"}`, http.StatusBadRequest)
		return
	}

	if req.DefaultBranch == "" {
		req.DefaultBranch = "main"
	}
	if req.Provider == "" {
		req.Provider = models.ProviderGitHub
	}
	if req.ExternalID == "" {
		req.ExternalID = req.NamespacePath
	}

	if c.repo == nil {
		http.Error(w, `{"error":"database service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	repo, err := c.repo.TrackRepository(r.Context(), wsID, req.Provider, req.ExternalID, req.NamespacePath, req.DefaultBranch)
	if err != nil {
		http.Error(w, `{"error":"failed tracking repository"}`, http.StatusInternalServerError)
		return
	}

	actorID := ""
	actorEmail := ""
	if profile != nil {
		actorID = profile.ID.String()
		actorEmail = profile.Email
	}
	_ = c.repo.InsertAuditLog(r.Context(), wsID, actorID, actorEmail, r.RemoteAddr, "repository.track", "repository", req.NamespacePath, nil)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(dtos.RepositoryResponse{
		ID:            repo.ID,
		WorkspaceID:   wsID,
		Provider:      repo.Provider,
		ExternalID:    repo.ExternalID,
		NamespacePath: repo.NamespacePath,
		DefaultBranch: repo.DefaultBranch,
		IsActive:      repo.IsActive,
		CreatedAt:     repo.CreatedAt,
	})
}

func (c *CodeManagementController) handleListBranches(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	repoIDStr := chi.URLParam(r, "id")
	repoID, err := uuid.Parse(repoIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid repository id"}`, http.StatusBadRequest)
		return
	}

	defaultBranch := "main"
	branches := []string{"main"}
	if c.repo != nil {
		if repos, err := c.repo.ListTrackedRepositories(r.Context(), wsID); err == nil {
			for _, tr := range repos {
				if tr.ID == repoID {
					if tr.DefaultBranch != "" {
						defaultBranch = tr.DefaultBranch
						branches = []string{tr.DefaultBranch}
					}
					break
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dtos.BranchListResponse{
		DefaultBranch: defaultBranch,
		Branches:      branches,
	})
}

func (c *CodeManagementController) handleGetFileTree(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	repoIDStr := chi.URLParam(r, "id")
	repoID, err := uuid.Parse(repoIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid repository id"}`, http.StatusBadRequest)
		return
	}

	if c.repo != nil {
		repos, err := c.repo.ListTrackedRepositories(r.Context(), wsID)
		if err != nil {
			http.Error(w, `{"error":"failed querying repositories"}`, http.StatusInternalServerError)
			return
		}
		found := false
		for _, tr := range repos {
			if tr.ID == repoID {
				found = true
				break
			}
		}
		if !found {
			http.Error(w, `{"error":"forbidden: repository access denied"}`, http.StatusForbidden)
			return
		}
	}

	ref := r.URL.Query().Get("ref")
	if ref == "" {
		ref = "HEAD"
	}

	entries := []dtos.FileTreeEntry{}
	if c.repo != nil {
		if nodes, err := c.repo.GetASTNodesByRepository(r.Context(), repoID); err == nil && len(nodes) > 0 {
			seenPaths := make(map[string]bool)
			for _, node := range nodes {
				if node.FilePath != "" && !seenPaths[node.FilePath] {
					seenPaths[node.FilePath] = true
					entries = append(entries, dtos.FileTreeEntry{
						Path: node.FilePath,
						Type: "blob",
						Size: 0,
					})
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dtos.FileTreeResponse{
		CommitSHA: ref,
		Entries:   entries,
	})
}
