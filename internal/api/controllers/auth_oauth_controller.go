package controllers

import (
	"encoding/json"
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
	// State + PKCE verifier + ID-token nonce are minted together and held
	// server-side. The verifier never reaches the browser: only its S256
	// challenge does. AUDIT_REMEDIATION.md.
	state, codeVerifier, nonce, err := c.oauthStateStore.GeneratePKCE(provider)
	if err != nil {
		http.Error(w, `{"error":"failed generating CSRF state"}`, http.StatusInternalServerError)
		return
	}

	// Bind the state to this browser (AUDIT_REMEDIATION.md F-16). The state
	// store proves the token is unused and unexpired; this cookie proves the
	// callback arrives in the same browser that started the flow. Lax (not
	// Strict) is required: the provider returns here with a top-level GET.
	setAuthCookie(w, r, "scandrix_oauth_state", c.oauthStateStore.Bind(state), 600)

	authURL, err := c.oauthService.GetAuthorizationURLWithPKCE(provider, state, oauth.S256Challenge(codeVerifier), nonce)
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

	// The state token must be presented by the same browser that requested it.
	// Without this an attacker can complete their own OAuth flow and then send
	// a victim straight to the callback with the attacker's code and state.
	// AUDIT_REMEDIATION.md F-16.
	if c.oauthStateStore == nil || !c.oauthStateStore.VerifyBinding(state, getAuthCookie(r, "scandrix_oauth_state")) {
		http.Error(w, `{"error":"OAuth state does not match this browser session"}`, http.StatusForbidden)
		return
	}

	// Consume the state and recover the PKCE verifier bound to it. Consume is
	// single-use, so this is also the CSRF check.
	if c.oauthStateStore == nil {
		http.Error(w, `{"error":"invalid or expired OAuth state parameter"}`, http.StatusForbidden)
		return
	}
	codeVerifier, nonce, ok := c.oauthStateStore.Consume(state, provider)
	if !ok {
		http.Error(w, `{"error":"invalid or expired OAuth state parameter"}`, http.StatusForbidden)
		return
	}

	oauthProfile, err := c.oauthService.ExchangeCodeWithPKCE(r.Context(), provider, code, codeVerifier)
	if err != nil {
		slog.Warn("OAuth code exchange failed", "provider", provider, "error", err)
		http.Error(w, `{"error":"OAuth authentication failed"}`, http.StatusUnauthorized)
		return
	}
	_ = nonce
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
	if cliCode := getAuthCookie(r, "scandrix_cli_code"); cliCode != "" && c.deviceFlow != nil {
		profile := &models.AccountProfile{
			ID:          userID,
			WorkspaceID: wsID,
			Email:       oauthProfile.Email,
			Role:        userRole,
		}
		_ = c.deviceFlow.CompleteDeviceLogin(r.Context(), cliCode, accessToken, refreshToken, profile)
	}

	// Set session cookie
	setAuthCookie(w, r, "scandrix_token", accessToken, int(auth.DefaultAccessTokenTTL.Seconds()))

	if r.Method == http.MethodGet {
		if getAuthCookie(r, "scandrix_cli_code") != "" {
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
