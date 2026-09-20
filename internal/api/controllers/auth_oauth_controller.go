package controllers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/auth/oauth"
	"github.com/scandrix/backend/pkg/models"
)

// ============================================================================
// OAuth 2.0 Social Logins (GitHub & GitLab)
// ============================================================================

func (c *AuthController) handleOAuthAuthorize(w http.ResponseWriter, r *http.Request) {
	if c.oauthService == nil {
		http.Error(w, `{"error":"OAuth not configured"}`, http.StatusServiceUnavailable)
		return
	}

	providerStr := chi.URLParam(r, "provider")
	provider := oauth.OAuthProvider(providerStr)

	// Save CLI code in cookie if passed in query
	if cliCode := r.URL.Query().Get("code"); cliCode != "" {
		setAuthCookie(w, r, "scandrix_cli_code", cliCode, 600)
	}

	// Generate CSRF state via server-side store (one-time-use, 10-min TTL)
	state, err := c.oauthStateStore.Generate(provider)
	if err != nil {
		http.Error(w, `{"error":"failed generating CSRF state"}`, http.StatusInternalServerError)
		return
	}

	authURL, err := c.oauthService.GetAuthorizationURL(provider, state)
	if err != nil {
		slog.Warn("Failed generating OAuth authorization URL", "provider", provider, "error", err)
		http.Error(w, `{"error":"failed generating OAuth authorization URL"}`, http.StatusBadRequest)
		return
	}

	// If browser directly navigated, redirect directly to provider
	if strings.Contains(r.Header.Get("Accept"), "text/html") || r.URL.Query().Get("redirect") == "true" {
		http.Redirect(w, r, authURL, http.StatusFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"authorization_url": authURL,
		"state":             state,
	})
}

func (c *AuthController) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if c.oauthService == nil {
		http.Error(w, `{"error":"OAuth not configured"}`, http.StatusServiceUnavailable)
		return
	}

	providerStr := chi.URLParam(r, "provider")
	provider := oauth.OAuthProvider(providerStr)

	var code, state string
	if r.Method == http.MethodGet {
		code = r.URL.Query().Get("code")
		state = r.URL.Query().Get("state")
	} else {
		var req dtos.OAuthCallbackRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			code = req.Code
			state = req.State
		}
	}

	if code == "" {
		http.Error(w, `{"error":"authorization code is required"}`, http.StatusBadRequest)
		return
	}

	// Validate CSRF state parameter (one-time-use, provider-bound)
	if c.oauthStateStore == nil || !c.oauthStateStore.Validate(state, provider) {
		http.Error(w, `{"error":"invalid or expired OAuth state parameter"}`, http.StatusForbidden)
		return
	}

	oauthProfile, err := c.oauthService.ExchangeCode(r.Context(), provider, code)
	if err != nil {
		slog.Warn("OAuth code exchange failed", "provider", provider, "error", err)
		http.Error(w, `{"error":"OAuth authentication failed"}`, http.StatusUnauthorized)
		return
	}

	// Find-or-create user in database
	var userID uuid.UUID
	var wsID uuid.UUID
	var userRole models.UserRole

	if c.repo != nil {
		user, err := c.repo.GetUserByEmail(r.Context(), oauthProfile.Email)
		if err != nil {
			// New OAuth user: atomically create personal workspace and user account
			newWsID := uuid.New()
			wsName := oauthProfile.DisplayName + "'s Workspace"
			if oauthProfile.DisplayName == "" {
				wsName = "Personal Workspace"
			}
			slug := strings.ToLower(strings.ReplaceAll(oauthProfile.Username, " ", "-")) + "-" + newWsID.String()[:8]
			ws := &models.Workspace{
				ID:        newWsID,
				Slug:      slug,
				Name:      wsName,
				Status:    models.TenantStatusActive,
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			}

			placeholderHash, _ := auth.HashPassword(uuid.New().String())
			displayName := oauthProfile.DisplayName
			if displayName == "" {
				displayName = oauthProfile.Username
			}
			user, err = c.repo.CreateWorkspaceWithUser(r.Context(), ws, oauthProfile.Email, placeholderHash, "owner", displayName)
			if err != nil {
				slog.Error("Failed creating user account from OAuth", "error", err, "email", oauthProfile.Email)
				http.Error(w, `{"error":"failed creating user account from OAuth"}`, http.StatusInternalServerError)
				return
			}
		}
		userID = user.UUID
		if user.OrganizationID != nil && *user.OrganizationID != uuid.Nil {
			wsID = *user.OrganizationID
		} else {
			http.Error(w, `{"error":"user is not assigned to an active workspace"}`, http.StatusForbidden)
			return
		}
		userRole = models.UserRole(user.Role)
		if err := c.repo.TouchAccountActivity(r.Context(), wsID, user.Email); err != nil {
			slog.Warn("Failed touching user last_active_at on OAuth", "workspace_id", wsID, "email", user.Email, "error", err)
		}
	} else {
		http.Error(w, `{"error":"internal: database not available for OAuth flow"}`, http.StatusInternalServerError)
		return
	}

	accessToken, refreshToken, err := c.authService.GenerateTokenPairWithEmail(userID, wsID, userRole, oauthProfile.Email)
	if err != nil {
		http.Error(w, `{"error":"failed generating tokens"}`, http.StatusInternalServerError)
		return
	}

	if c.repo != nil {
		_ = c.repo.CreateRefreshToken(r.Context(), userID, refreshToken, time.Now().Add(30*24*time.Hour))
		if oauthProfile.AccessToken != "" {
			accountName := oauthProfile.Username
			if accountName == "" {
				accountName = oauthProfile.DisplayName
			}
			providerModel := models.ProviderGitHub
			if provider == oauth.ProviderGitLab {
				providerModel = models.ProviderGitLab
			}
			_ = c.repo.UpsertIntegrationConnection(r.Context(), wsID, providerModel, accountName, oauthProfile.AccessToken, true, 0)
		}
	}

	// Complete CLI device login if session cookie exists
	if cliCookie, err := r.Cookie("scandrix_cli_code"); err == nil && cliCookie.Value != "" && c.deviceFlow != nil {
		profile := &models.AccountProfile{
			ID:          userID,
			WorkspaceID: wsID,
			Email:       oauthProfile.Email,
			Role:        userRole,
		}
		_ = c.deviceFlow.CompleteDeviceLogin(r.Context(), cliCookie.Value, accessToken, refreshToken, profile)
	}

	// Set session cookie
	setAuthCookie(w, r, "scandrix_token", accessToken, 30*86400)

	if r.Method == http.MethodGet {
		if cliCookie, err := r.Cookie("scandrix_cli_code"); err == nil && cliCookie.Value != "" {
			http.Redirect(w, r, "/cli/authorize?status=approved", http.StatusFound)
			return
		}
		redirectURL := c.appBaseURL
		if redirectURL == "" {
			redirectURL = "/"
		}
		http.Redirect(w, r, redirectURL, http.StatusFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dtos.AuthTokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    86400,
		User: models.AccountProfile{
			ID:          userID,
			WorkspaceID: wsID,
			Email:       oauthProfile.Email,
			DisplayName: oauthProfile.DisplayName,
			Role:        userRole,
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		},
	})
}

// PostOAuthRequest defines the payload sent by web frontend NextAuth OAuth login or direct API callers.
type PostOAuthRequest struct {
	Name         string `json:"name"`
	Email        string `json:"email"`
	RefreshToken string `json:"refreshToken"`
	Token        string `json:"token"`
	AccessToken  string `json:"accessToken"`
	AuthProvider string `json:"authProvider"`
	Provider     string `json:"provider"`
}

// handlePostOAuth authenticates or registers users via OAuth from the web frontend (NextAuth).
func (c *AuthController) handlePostOAuth(w http.ResponseWriter, r *http.Request) {
	var req PostOAuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
		return
	}

	if c.repo == nil {
		http.Error(w, `{"error":"database service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.Name = strings.TrimSpace(req.Name)
	req.AuthProvider = strings.TrimSpace(strings.ToLower(req.AuthProvider))
	req.RefreshToken = strings.TrimSpace(req.RefreshToken)
	if req.RefreshToken == "" {
		if strings.TrimSpace(req.Token) != "" {
			req.RefreshToken = strings.TrimSpace(req.Token)
		} else if strings.TrimSpace(req.AccessToken) != "" {
			req.RefreshToken = strings.TrimSpace(req.AccessToken)
		}
	}
	if req.AuthProvider == "" && strings.TrimSpace(req.Provider) != "" {
		req.AuthProvider = strings.TrimSpace(strings.ToLower(req.Provider))
	}

	var accountName string
	var repoCount int
	provider := models.SCMProvider(req.AuthProvider)
	if provider == "" {
		if strings.HasPrefix(req.RefreshToken, "glpat-") {
			provider = models.ProviderGitLab
		} else {
			provider = models.ProviderGitHub
		}
	}

	// If provider token is passed, resolve email and identity from GitHub or GitLab if missing
	if req.RefreshToken != "" {
		if provider == models.ProviderGitHub {
			ghReq, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://api.github.com/user", nil)
			ghReq.Header.Set("Authorization", "Bearer "+req.RefreshToken)
			ghReq.Header.Set("Accept", "application/vnd.github.v3+json")
			if ghResp, err := http.DefaultClient.Do(ghReq); err == nil {
				defer ghResp.Body.Close()
				if ghResp.StatusCode == http.StatusOK {
					var ghUser struct {
						ID                int64  `json:"id"`
						Login             string `json:"login"`
						Name              string `json:"name"`
						Email             string `json:"email"`
						PublicRepos       int    `json:"public_repos"`
						TotalPrivateRepos int    `json:"total_private_repos"`
					}
					if json.NewDecoder(ghResp.Body).Decode(&ghUser) == nil && ghUser.Login != "" {
						accountName = ghUser.Login
						repoCount = ghUser.PublicRepos + ghUser.TotalPrivateRepos
						if req.Name == "" {
							if ghUser.Name != "" {
								req.Name = ghUser.Name
							} else {
								req.Name = ghUser.Login
							}
						}
						if req.Email == "" && ghUser.Email != "" {
							req.Email = strings.TrimSpace(strings.ToLower(ghUser.Email))
						}
					}
				}
			}

			// If email is still empty (e.g. private on GitHub profile), query /user/emails
			if req.Email == "" {
				emReq, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://api.github.com/user/emails", nil)
				emReq.Header.Set("Authorization", "Bearer "+req.RefreshToken)
				emReq.Header.Set("Accept", "application/vnd.github.v3+json")
				if emResp, err := http.DefaultClient.Do(emReq); err == nil {
					defer emResp.Body.Close()
					if emResp.StatusCode == http.StatusOK {
						var emails []struct {
							Email    string `json:"email"`
							Primary  bool   `json:"primary"`
							Verified bool   `json:"verified"`
						}
						if json.NewDecoder(emResp.Body).Decode(&emails) == nil {
							for _, e := range emails {
								if e.Primary && e.Email != "" {
									req.Email = strings.TrimSpace(strings.ToLower(e.Email))
									break
								}
							}
							if req.Email == "" && len(emails) > 0 && emails[0].Email != "" {
								req.Email = strings.TrimSpace(strings.ToLower(emails[0].Email))
							}
						}
					}
				}
			}

			// If still empty, fall back to GitHub noreply format
			if req.Email == "" && accountName != "" {
				req.Email = fmt.Sprintf("%s@users.noreply.github.com", accountName)
			}
		} else if provider == models.ProviderGitLab {
			glReq, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://gitlab.com/api/v4/user", nil)
			glReq.Header.Set("Authorization", "Bearer "+req.RefreshToken)
			glReq.Header.Set("PRIVATE-TOKEN", req.RefreshToken)
			if glResp, err := http.DefaultClient.Do(glReq); err == nil {
				defer glResp.Body.Close()
				if glResp.StatusCode == http.StatusOK {
					var glUser struct {
						ID       int64  `json:"id"`
						Username string `json:"username"`
						Name     string `json:"name"`
						Email    string `json:"email"`
					}
					if json.NewDecoder(glResp.Body).Decode(&glUser) == nil && glUser.Username != "" {
						accountName = glUser.Username
						if req.Name == "" {
							if glUser.Name != "" {
								req.Name = glUser.Name
							} else {
								req.Name = glUser.Username
							}
						}
						if req.Email == "" && glUser.Email != "" {
							req.Email = strings.TrimSpace(strings.ToLower(glUser.Email))
						}
					}
				}
			}

			if req.Email == "" {
				emReq, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://gitlab.com/api/v4/user/emails", nil)
				emReq.Header.Set("Authorization", "Bearer "+req.RefreshToken)
				emReq.Header.Set("PRIVATE-TOKEN", req.RefreshToken)
				if emResp, err := http.DefaultClient.Do(emReq); err == nil {
					defer emResp.Body.Close()
					if emResp.StatusCode == http.StatusOK {
						var emails []struct {
							Email string `json:"email"`
						}
						if json.NewDecoder(emResp.Body).Decode(&emails) == nil && len(emails) > 0 && emails[0].Email != "" {
							req.Email = strings.TrimSpace(strings.ToLower(emails[0].Email))
						}
					}
				}
			}

			if req.Email == "" && accountName != "" {
				req.Email = fmt.Sprintf("%s@users.noreply.gitlab.com", accountName)
			}

			glProjReq, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://gitlab.com/api/v4/projects?membership=true&per_page=100", nil)
			glProjReq.Header.Set("Authorization", "Bearer "+req.RefreshToken)
			glProjReq.Header.Set("PRIVATE-TOKEN", req.RefreshToken)
			if glProjResp, err := http.DefaultClient.Do(glProjReq); err == nil {
				defer glProjResp.Body.Close()
				if glProjResp.StatusCode == http.StatusOK {
					var projects []any
					if json.NewDecoder(glProjResp.Body).Decode(&projects) == nil {
						repoCount = len(projects)
					}
				}
			}
		}
	}

	if req.Email == "" {
		http.Error(w, `{"error":"valid email is required"}`, http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		req.Name = strings.Split(req.Email, "@")[0]
	}

	user, err := c.repo.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		newWsID := uuid.New()
		wsName := req.Name + "'s Workspace"
		slug := strings.ToLower(strings.ReplaceAll(req.Name, " ", "-")) + "-" + newWsID.String()[:8]
		ws := &models.Workspace{
			ID:        newWsID,
			Slug:      slug,
			Name:      wsName,
			Status:    models.TenantStatusActive,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		placeholderHash, _ := auth.HashPassword(uuid.New().String())
		user, err = c.repo.CreateWorkspaceWithUser(r.Context(), ws, req.Email, placeholderHash, "owner", req.Name)
		if err != nil {
			slog.Error("Failed creating user account from OAuth", "error", err, "email", req.Email)
			http.Error(w, `{"error":"failed creating user account from OAuth"}`, http.StatusInternalServerError)
			return
		}
	}

	userID := user.UUID
	if user.OrganizationID == nil || *user.OrganizationID == uuid.Nil {
		http.Error(w, `{"error":"user is not assigned to an active workspace"}`, http.StatusForbidden)
		return
	}
	wsID := *user.OrganizationID
	userRole := models.UserRole(user.Role)

	// If provider token is passed, save/upsert the integration connection
	if req.RefreshToken != "" {
		if accountName == "" {
			accountName = req.Name
		}
		_ = c.repo.UpsertIntegrationConnection(r.Context(), wsID, provider, accountName, req.RefreshToken, true, repoCount)
	}

	_ = c.repo.TouchAccountActivity(r.Context(), wsID, user.Email)

	accessToken, refreshToken, err := c.authService.GenerateTokenPairWithEmail(userID, wsID, userRole, req.Email)
	if err != nil {
		http.Error(w, `{"error":"failed generating tokens"}`, http.StatusInternalServerError)
		return
	}

	_ = c.repo.CreateRefreshToken(r.Context(), userID, refreshToken, time.Now().Add(30*24*time.Hour))

	// Complete CLI device login if session cookie exists
	if cliCookie, err := r.Cookie("scandrix_cli_code"); err == nil && cliCookie.Value != "" && c.deviceFlow != nil {
		profile := &models.AccountProfile{
			ID:          userID,
			WorkspaceID: wsID,
			Email:       req.Email,
			Role:        userRole,
		}
		_ = c.deviceFlow.CompleteDeviceLogin(r.Context(), cliCookie.Value, accessToken, refreshToken, profile)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"statusCode":   http.StatusOK,
		"accessToken":  accessToken,
		"refreshToken": refreshToken,
		"tokenType":    "Bearer",
		"expiresIn":    86400,
		"user": map[string]any{
			"id":             userID,
			"organizationId": wsID,
			"email":          req.Email,
			"name":           req.Name,
			"role":           userRole,
		},
		"data": map[string]any{
			"accessToken":  accessToken,
			"refreshToken": refreshToken,
			"tokenType":    "Bearer",
			"expiresIn":    86400,
			"user": map[string]any{
				"id":             userID,
				"organizationId": wsID,
				"email":          req.Email,
				"name":           req.Name,
				"role":           userRole,
			},
		},
	})
}

// HandleCheckEmail checks if an email address is already registered in the system.
