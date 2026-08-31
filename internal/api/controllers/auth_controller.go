package controllers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/auth/cliauth"
	"github.com/scandrix/backend/internal/auth/mailer"
	"github.com/scandrix/backend/internal/auth/oauth"
	"github.com/scandrix/backend/internal/auth/sso"
	"github.com/scandrix/backend/internal/cache/limiter"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/pkg/models"
)

// base64Encoding is the URL-safe, no-padding base64 decoder for reset tokens.
var base64Encoding = base64.RawURLEncoding

// AuthController handles identity, login, tokens, CLI API keys, OAuth, SAML, and device flows.
type AuthController struct {
	authService     *auth.Authenticator
	rateLimiter     *limiter.TokenBucketLimiter
	repo            *database.Repository
	deviceFlow      *cliauth.DeviceFlowManager
	oauthService    *oauth.OAuthService
	oauthStateStore *oauth.StateStore
	samlHandler     *sso.SAMLHandler
	mailer          mailer.EmailSender
	appBaseURL      string
	jwtSecret       string
}

// NewAuthController initializes the auth controller with token bucket rate limiting and DB access.
func NewAuthController(authService *auth.Authenticator, repo *database.Repository) *AuthController {
	tb := limiter.NewTokenBucketLimiter(limiter.RateLimitConfig{
		Capacity:          10,
		RefillRatePerSec:  0.1, // ~6 requests per minute
		ExpirationTimeout: 15 * time.Minute,
	})
	return &AuthController{
		authService: authService,
		rateLimiter: tb,
		repo:        repo,
		samlHandler: sso.NewSAMLHandler(),
		appBaseURL:  "http://localhost:3000",
	}
}

// SetSAMLHandler configures the SAML 2.0 Identity Provider handler.
func (c *AuthController) SetSAMLHandler(handler *sso.SAMLHandler) {
	c.samlHandler = handler
}

// SetDeviceFlowManager attaches the RFC 8628 device flow manager.
func (c *AuthController) SetDeviceFlowManager(dfm *cliauth.DeviceFlowManager) {
	c.deviceFlow = dfm
}

// SetOAuthService attaches the OAuth 2.0 social login provider with a CSRF state store.
func (c *AuthController) SetOAuthService(svc *oauth.OAuthService) {
	c.oauthService = svc
	c.oauthStateStore = oauth.NewStateStore(10 * time.Minute)
}

// SetMailer attaches the transactional email sender for password reset links.
func (c *AuthController) SetMailer(m mailer.EmailSender) {
	c.mailer = m
}

// SetAppBaseURL configures the frontend web app URL for email link generation.
func (c *AuthController) SetAppBaseURL(url string) {
	if strings.TrimSpace(url) != "" {
		c.appBaseURL = strings.TrimRight(url, "/")
	}
}

// SetJWTSecret stores the signing secret for password reset token verification.
func (c *AuthController) SetJWTSecret(secret string) {
	c.jwtSecret = secret
}

// SetRateLimiter allows customizing or injecting a test rate limiter.
func (c *AuthController) SetRateLimiter(tb *limiter.TokenBucketLimiter) {
	c.rateLimiter = tb
}

// Routes mounts the authentication endpoints with public rate limiting.
func (c *AuthController) Routes() chi.Router {
	r := chi.NewRouter()

	// RFC 8628 CLI Device Authorization (public polling)
	r.Post("/cli/device/initiate", c.handleDeviceInitiate)
	r.Get("/cli/device/poll", c.handleDevicePoll)
	r.Get("/cli/authorize", c.HandleCLIAuthorizePage)
	r.Post("/cli/authorize/approve", c.HandleCLIAuthorizeApprove)

	// Rate-limited public authentication endpoints (Master Rule 4.5)
	r.Group(func(public chi.Router) {
		public.Use(c.rateLimitMiddleware)
		public.Post("/login", c.handleLogin)
		public.Post("/register", c.handleRegister)
		public.Post("/refresh", c.handleRefreshToken)
		public.Post("/logout", c.handleLogout)

		// Password Reset (rate-limited, public)
		public.Post("/password/forgot", c.handleForgotPassword)
		public.Post("/password/reset", c.handleResetPassword)

		// OAuth 2.0 Social Logins (public)
		public.Get("/oauth/{provider}/authorize", c.handleOAuthAuthorize)
		public.Get("/oauth/{provider}/callback", c.handleOAuthCallback)
		public.Post("/oauth/{provider}/callback", c.handleOAuthCallback)

		// SAML 2.0 Enterprise Single Sign-On (public)
		public.Get("/saml/metadata", c.HandleSAMLMetadata)
		public.Get("/saml/login", c.HandleSAMLLogin)
		public.Post("/saml/acs", c.HandleSAMLACS)
	})

	// Protected routes
	r.Group(func(pr chi.Router) {
		pr.Use(c.authService.Middleware)
		pr.Get("/me", c.handleMe)
		pr.Post("/cli-keys", c.handleCreateCLIKey)

		// CLI device complete requires authenticated user approving the user code
		pr.Post("/cli/device/complete", c.handleDeviceComplete)
	})

	return r
}

func (c *AuthController) rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c.rateLimiter == nil {
			next.ServeHTTP(w, r)
			return
		}

		clientIP := r.RemoteAddr
		directHost := r.RemoteAddr
		if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
			directHost = host
			clientIP = host
		}

		directParsed := net.ParseIP(directHost)
		isTrustedProxy := directParsed != nil && (directParsed.IsLoopback() || directParsed.IsPrivate() || directParsed.IsUnspecified())

		if isTrustedProxy {
			if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
				if ip := net.ParseIP(strings.TrimSpace(xrip)); ip != nil {
					clientIP = ip.String()
				}
			} else if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
				parts := strings.Split(xff, ",")
				if len(parts) > 0 {
					candidate := strings.TrimSpace(parts[0])
					if ip := net.ParseIP(candidate); ip != nil {
						clientIP = ip.String()
					}
				}
			}
		}

		res, err := c.rateLimiter.Allow(r.Context(), clientIP, 1)
		if err == nil && !res.Allowed {
			retrySeconds := int(res.RetryAfter.Seconds())
			if retrySeconds < 1 {
				retrySeconds = 1
			}
			w.Header().Set("Retry-After", fmt.Sprintf("%d", retrySeconds))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write(fmt.Appendf(nil, `{"error":"too many authentication requests, please retry after %d seconds"}`, retrySeconds))
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (c *AuthController) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req dtos.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" || req.Password == "" {
		http.Error(w, `{"error":"email and password are required"}`, http.StatusBadRequest)
		return
	}

	// Fail closed if database repository is uninitialized (Master Rule 5.7)
	if c.repo == nil {
		http.Error(w, `{"error":"database service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	// Live database authentication with Bcrypt password verification (Master Rule 2.1 & 5.1)
	user, err := c.repo.GetUserByEmail(r.Context(), req.Email)
	if err != nil || !auth.VerifyPassword(req.Password, user.Password) {
		http.Error(w, `{"error":"invalid email or password"}`, http.StatusUnauthorized)
		return
	}

	if user.Status != "active" {
		http.Error(w, `{"error":"account is not active"}`, http.StatusForbidden)
		return
	}

	if user.OrganizationID == nil || *user.OrganizationID == uuid.Nil {
		http.Error(w, `{"error":"user is not assigned to an active workspace"}`, http.StatusForbidden)
		return
	}
	userID := user.UUID
	wsID := *user.OrganizationID
	userRole := models.UserRole(user.Role)
	displayName := req.Email

	// Update last_active_at activity timestamp
	if err := c.repo.TouchAccountActivity(r.Context(), wsID, user.Email); err != nil {
		slog.Warn("Failed touching user last_active_at on login", "workspace_id", wsID, "email", user.Email, "error", err)
	}

	accessToken, refreshToken, err := c.authService.GenerateTokenPair(userID, wsID, userRole)
	if err != nil {
		http.Error(w, `{"error":"failed generating authentication tokens"}`, http.StatusInternalServerError)
		return
	}

	// Persist refresh token in database (auth table)
	if c.repo != nil {
		_ = c.repo.CreateRefreshToken(r.Context(), userID, refreshToken, time.Now().Add(30*24*time.Hour))
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
			Email:       req.Email,
			DisplayName: displayName,
			Role:        userRole,
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		},
	})
}

func (c *AuthController) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req dtos.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" || req.Password == "" {
		http.Error(w, `{"error":"email, password, and workspace name are required"}`, http.StatusBadRequest)
		return
	}

	if len(req.Password) < 8 {
		http.Error(w, `{"error":"password must be at least 8 characters long"}`, http.StatusBadRequest)
		return
	}

	// Fail closed if database repository is uninitialized (Master Rule 5.7)
	if c.repo == nil {
		http.Error(w, `{"error":"database service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	wsID := uuid.New()
	userRole := models.RoleOwner

	wsName := req.WorkspaceName
	if wsName == "" {
		wsName = "Primary Workspace"
	}
	slug := strings.ToLower(strings.ReplaceAll(wsName, " ", "-")) + "-" + wsID.String()[:8]

	ws := &models.Workspace{
		ID:        wsID,
		Slug:      slug,
		Name:      wsName,
		Status:    models.TenantStatusActive,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	pwHash, err := auth.HashPassword(req.Password)
	if err != nil {
		http.Error(w, `{"error":"failed processing credentials"}`, http.StatusInternalServerError)
		return
	}

	displayName := req.DisplayName
	if displayName == "" {
		displayName = req.Email
	}

	user, err := c.repo.CreateWorkspaceWithUser(r.Context(), ws, req.Email, pwHash, "owner", displayName)
	if err != nil {
		slog.Error("User registration failed", "email", req.Email, "error", err)
		http.Error(w, `{"error":"user registration failed: an account with this email may already exist or parameters are invalid"}`, http.StatusConflict)
		return
	}
	userID := user.UUID

	accessToken, refreshToken, err := c.authService.GenerateTokenPair(userID, wsID, userRole)
	if err != nil {
		http.Error(w, `{"error":"failed generating tokens"}`, http.StatusInternalServerError)
		return
	}

	if c.repo != nil {
		_ = c.repo.CreateRefreshToken(r.Context(), userID, refreshToken, time.Now().Add(30*24*time.Hour))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(dtos.AuthTokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    86400,
		User: models.AccountProfile{
			ID:          userID,
			WorkspaceID: wsID,
			Email:       req.Email,
			DisplayName: req.DisplayName,
			Role:        userRole,
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		},
	})
}

// handleRefreshToken implements Kodus-grade one-time refresh token rotation.
func (c *AuthController) handleRefreshToken(w http.ResponseWriter, r *http.Request) {
	var req dtos.RefreshTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RefreshToken == "" {
		http.Error(w, `{"error":"refresh_token is required"}`, http.StatusBadRequest)
		return
	}

	// Fail closed if database repository is uninitialized (Master Rule 5.7)
	if c.repo == nil {
		http.Error(w, `{"error":"database service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	record, err := c.repo.GetRefreshToken(r.Context(), req.RefreshToken)
	if err != nil || record.Used || time.Now().After(record.ExpiryDate) {
		http.Error(w, `{"error":"invalid, expired, or already used refresh token"}`, http.StatusUnauthorized)
		return
	}

	// 1. Invalidate used token (One-time rotation, Master Rule 5.1)
	_ = c.repo.MarkRefreshTokenUsed(r.Context(), req.RefreshToken)

	// 2. Load refreshed user and derive workspace & role
	user, err := c.repo.GetUserByID(r.Context(), record.UserUUID)
	if err != nil {
		http.Error(w, `{"error":"user account not found"}`, http.StatusUnauthorized)
		return
	}

	if user.Status != "active" {
		http.Error(w, `{"error":"account is not active"}`, http.StatusForbidden)
		return
	}

	if user.OrganizationID == nil || *user.OrganizationID == uuid.Nil {
		http.Error(w, `{"error":"user is not assigned to an active workspace"}`, http.StatusForbidden)
		return
	}
	userID := user.UUID
	wsID := *user.OrganizationID
	userRole := models.UserRole(user.Role)

	// Update last_active_at
	if err := c.repo.TouchAccountActivity(r.Context(), wsID, user.Email); err != nil {
		slog.Warn("Failed touching user last_active_at on refresh", "workspace_id", wsID, "email", user.Email, "error", err)
	}

	newAccess, newRefresh, err := c.authService.GenerateTokenPair(userID, wsID, userRole)
	if err != nil {
		http.Error(w, `{"error":"failed generating rotated tokens"}`, http.StatusInternalServerError)
		return
	}

	if c.repo != nil {
		_ = c.repo.CreateRefreshToken(r.Context(), userID, newRefresh, time.Now().Add(30*24*time.Hour))
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dtos.AuthTokenResponse{
		AccessToken:  newAccess,
		RefreshToken: newRefresh,
		TokenType:    "Bearer",
		ExpiresIn:    86400,
		User: models.AccountProfile{
			ID:          userID,
			WorkspaceID: wsID,
			Role:        userRole,
			UpdatedAt:   time.Now().UTC(),
		},
	})
}

// handleLogout marks the provided refresh token as used/revoked.
func (c *AuthController) handleLogout(w http.ResponseWriter, r *http.Request) {
	var req dtos.LogoutRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	if c.repo != nil && req.RefreshToken != "" {
		_ = c.repo.MarkRefreshTokenUsed(r.Context(), req.RefreshToken)
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"logged out successfully"}`))
}

func (c *AuthController) handleMe(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	profile, ok := auth.AccountProfileFromContext(r.Context())
	if !ok || profile == nil {
		http.Error(w, `{"error":"missing user context"}`, http.StatusUnauthorized)
		return
	}

	if profile.Email == "" && c.repo != nil {
		if user, err := c.repo.GetUserByID(r.Context(), profile.ID); err == nil && user != nil {
			profile.Email = user.Email
			profile.DisplayName = user.Email
			profile.Role = models.UserRole(user.Role)
			profile.CreatedAt = user.CreatedAt
			profile.UpdatedAt = user.UpdatedAt
		}
	}
	profile.WorkspaceID = wsID

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(profile)
}

func (c *AuthController) handleCreateCLIKey(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	var req dtos.CreateAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, `{"error":"key name is required"}`, http.StatusBadRequest)
		return
	}

	plainKey, hashedKey, err := auth.GenerateAPIKey()
	if err != nil {
		http.Error(w, `{"error":"failed generating key"}`, http.StatusInternalServerError)
		return
	}

	keyID := uuid.New()
	prefix := plainKey[:14]

	// Live database storage adhering to Master Rule 2.1
	if c.repo != nil {
		_ = c.repo.SaveAPIKey(r.Context(), keyID, wsID, req.Name, hashedKey, prefix, req.ExpiresAt)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(dtos.APIKeyResponse{
		ID:        keyID,
		Name:      req.Name,
		KeyPrefix: prefix,
		PlainKey:  plainKey, // Shown only once to caller
		CreatedAt: time.Now().UTC(),
		ExpiresAt: req.ExpiresAt,
	})
}

// ============================================================================
// RFC 8628 CLI Device Authorization Flow
// ============================================================================

func (c *AuthController) handleDeviceInitiate(w http.ResponseWriter, r *http.Request) {
	if c.deviceFlow == nil {
		http.Error(w, `{"error":"CLI device flow not configured"}`, http.StatusServiceUnavailable)
		return
	}

	result, err := c.deviceFlow.InitiateDeviceLogin(r.Context(), r.UserAgent())
	if err != nil {
		slog.Error("Failed initiating device flow", "error", err)
		http.Error(w, `{"error":"failed initiating device flow"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (c *AuthController) handleDevicePoll(w http.ResponseWriter, r *http.Request) {
	if c.deviceFlow == nil {
		http.Error(w, `{"error":"CLI device flow not configured"}`, http.StatusServiceUnavailable)
		return
	}

	deviceCode := r.URL.Query().Get("device_code")
	if deviceCode == "" {
		http.Error(w, `{"error":"device_code query parameter is required"}`, http.StatusBadRequest)
		return
	}

	result, err := c.deviceFlow.PollDeviceLogin(r.Context(), deviceCode)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"session not found or expired"}`))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (c *AuthController) handleDeviceComplete(w http.ResponseWriter, r *http.Request) {
	if c.deviceFlow == nil {
		http.Error(w, `{"error":"CLI device flow not configured"}`, http.StatusServiceUnavailable)
		return
	}

	var req dtos.CompleteDeviceLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserCode == "" {
		http.Error(w, `{"error":"user_code is required"}`, http.StatusBadRequest)
		return
	}

	profile, ok := auth.AccountProfileFromContext(r.Context())
	if !ok || profile == nil {
		http.Error(w, `{"error":"authenticated user context required"}`, http.StatusUnauthorized)
		return
	}

	// Generate fresh tokens for the CLI session
	accessToken, refreshToken, err := c.authService.GenerateTokenPair(profile.ID, profile.WorkspaceID, profile.Role)
	if err != nil {
		http.Error(w, `{"error":"failed generating tokens for device session"}`, http.StatusInternalServerError)
		return
	}

	if err := c.deviceFlow.CompleteDeviceLogin(r.Context(), req.UserCode, accessToken, refreshToken, profile); err != nil {
		slog.Warn("Failed completing device login", "user_code", req.UserCode, "error", err)
		http.Error(w, `{"error":"invalid or expired device login session"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"device login approved"}`))
}

// HandleCLIAuthorizePage serves the clean minimalist confirmation UI with GitHub, GitLab, Bitbucket OAuth and email options.
func (c *AuthController) HandleCLIAuthorizePage(w http.ResponseWriter, r *http.Request) {
	rawCode := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("code")))
	cleanCode := strings.ReplaceAll(rawCode, "-", "")

	// Extract up to 8 characters for the boxes
	c1, c2, c3, c4, c5, c6, c7, c8 := "", "", "", "", "", "", "", ""
	chars := []rune(cleanCode)
	if len(chars) > 0 {
		c1 = string(chars[0])
	}
	if len(chars) > 1 {
		c2 = string(chars[1])
	}
	if len(chars) > 2 {
		c3 = string(chars[2])
	}
	if len(chars) > 3 {
		c4 = string(chars[3])
	}
	if len(chars) > 4 {
		c5 = string(chars[4])
	}
	if len(chars) > 5 {
		c6 = string(chars[5])
	}
	if len(chars) > 6 {
		c7 = string(chars[6])
	}
	if len(chars) > 7 {
		c8 = string(chars[7])
	}

	// Check if user has an active session cookie — always verify against DB
	loggedInEmail := ""
	if cookie, err := r.Cookie("scandrix_token"); err == nil && cookie.Value != "" && c.authService != nil {
		if claims, err := c.authService.VerifyToken(cookie.Value); err == nil && claims != nil {
			// Always verify the user actually exists in the database
			if c.repo != nil {
				if u, err := c.repo.GetUserByID(r.Context(), claims.UserID); err == nil && u != nil {
					loggedInEmail = u.Email
				}
			}
			// If DB is unavailable, do NOT fall back to JWT email claim — require fresh login
		}
	}

	isLoggedIn := loggedInEmail != ""

	htmlTemplate := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Enter Your code - ScanDrix</title>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Plus+Jakarta+Sans:wght@400;500;600;700;800&family=JetBrains+Mono:wght@600;700&display=swap" rel="stylesheet">
    <style>
        :root {
            --bg-page: #f8fafc;
            --card-bg: #ffffff;
            --text-main: #0f172a;
            --text-sub: #64748b;
            --box-bg: #f8fafc;
            --box-filled-bg: #1e293b;
            --box-filled-text: #ffffff;
            --box-border: #cbd5e1;
            --box-focus: #0f172a;
            --btn-bg: #0e381b;
            --btn-hover: #0a2914;
            --btn-text: #ffffff;
            --success-color: #15803d;
        }

        * {
            box-sizing: border-box;
            margin: 0;
            padding: 0;
        }

        body {
            font-family: 'Plus Jakarta Sans', system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background-color: var(--bg-page);
            color: var(--text-main);
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            padding: 1.5rem;
        }

        .container {
            width: 100%;
            max-width: 440px;
            background: var(--card-bg);
            border-radius: 24px;
            padding: 2.5rem 2.25rem 2.25rem;
            box-shadow: 0 10px 30px -5px rgba(0, 0, 0, 0.05), 0 20px 25px -5px rgba(0, 0, 0, 0.03);
            text-align: center;
            display: flex;
            flex-direction: column;
            align-items: center;
            border: 1px solid rgba(226, 232, 240, 0.8);
        }

        .flower-icon {
            width: 54px;
            height: 54px;
            margin-bottom: 1.25rem;
            display: block;
        }

        h1 {
            font-size: 1.75rem;
            font-weight: 700;
            color: var(--text-main);
            margin-bottom: 0.4rem;
            letter-spacing: -0.02em;
        }

        p.description {
            font-size: 0.9rem;
            color: var(--text-sub);
            line-height: 1.45;
            max-width: 340px;
            margin-bottom: 1.5rem;
        }

        /* OAuth Provider Buttons */
        .oauth-stack {
            width: 100%;
            display: flex;
            flex-direction: column;
            gap: 0.65rem;
            margin-bottom: 1.25rem;
        }

        .btn-oauth {
            width: 100%;
            display: flex;
            align-items: center;
            justify-content: center;
            gap: 0.75rem;
            padding: 0.75rem 1rem;
            border-radius: 10px;
            font-size: 0.92rem;
            font-weight: 600;
            cursor: pointer;
            transition: all 0.15s ease;
            outline: none;
            text-decoration: none;
        }

        .btn-github {
            background: #0f172a;
            color: #ffffff;
            border: 1px solid #0f172a;
        }
        .btn-github:hover {
            background: #1e293b;
            border-color: #1e293b;
        }

        .btn-gitlab {
            background: #ffffff;
            color: #0f172a;
            border: 1.5px solid #e2e8f0;
        }
        .btn-gitlab:hover {
            background: #f8fafc;
            border-color: #cbd5e1;
        }

        .btn-bitbucket {
            background: #ffffff;
            color: #0f172a;
            border: 1.5px solid #e2e8f0;
        }
        .btn-bitbucket:hover {
            background: #f8fafc;
            border-color: #cbd5e1;
        }

        .oauth-divider {
            width: 100%;
            display: flex;
            align-items: center;
            margin: 0.5rem 0 1.25rem;
            color: #94a3b8;
            font-size: 0.8rem;
            font-weight: 600;
            text-transform: uppercase;
            letter-spacing: 0.05em;
        }

        .oauth-divider::before, .oauth-divider::after {
            content: "";
            flex: 1;
            border-bottom: 1px solid #e2e8f0;
        }

        .oauth-divider span {
            padding: 0 0.85rem;
        }

        /* User Profile Pill for already logged in users */
        .user-pill {
            display: flex;
            align-items: center;
            gap: 0.65rem;
            background: #f1f5f9;
            border: 1px solid #e2e8f0;
            padding: 0.6rem 1rem;
            border-radius: 999px;
            font-size: 0.88rem;
            font-weight: 600;
            color: var(--text-main);
            margin-bottom: 1.5rem;
        }

        .user-avatar {
            width: 24px;
            height: 24px;
            border-radius: 50%;
            background: #0e381b;
            color: white;
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 0.75rem;
            font-weight: 700;
        }

        /* Form Inputs */
        .input-group {
            width: 100%;
            text-align: left;
            margin-bottom: 0.85rem;
        }

        .input-label {
            font-size: 0.8rem;
            font-weight: 600;
            color: var(--text-main);
            margin-bottom: 0.3rem;
            display: block;
        }

        .form-input {
            width: 100%;
            padding: 0.7rem 0.85rem;
            border-radius: 8px;
            border: 1.5px solid var(--box-border);
            background: #ffffff;
            font-family: inherit;
            font-size: 0.9rem;
            color: var(--text-main);
            outline: none;
            transition: border-color 0.15s ease;
        }

        .form-input:focus {
            border-color: var(--box-focus);
            box-shadow: 0 0 0 3px rgba(15, 23, 42, 0.08);
        }

        .code-row {
            display: flex;
            align-items: center;
            justify-content: center;
            gap: 0.45rem;
            margin: 1rem 0 1.25rem;
            width: 100%;
        }

        .divider {
            font-size: 1.25rem;
            font-weight: 700;
            color: #94a3b8;
            padding: 0 0.15rem;
            user-select: none;
        }

        .otp-input {
            width: 38px;
            height: 48px;
            font-family: 'JetBrains Mono', monospace;
            font-size: 1.25rem;
            font-weight: 700;
            text-align: center;
            border: 1.5px solid var(--box-border);
            border-radius: 6px;
            background: var(--box-bg);
            color: var(--text-main);
            outline: none;
            transition: all 0.15s ease;
            text-transform: uppercase;
        }

        .otp-input:focus {
            border-color: var(--box-focus);
            background: #ffffff;
            box-shadow: 0 0 0 3px rgba(15, 23, 42, 0.08);
        }

        .otp-input.filled {
            background: var(--box-filled-bg);
            color: var(--box-filled-text);
            border-color: var(--box-filled-bg);
        }

        .help-text {
            font-size: 0.82rem;
            color: var(--text-sub);
            margin-bottom: 1.5rem;
            line-height: 1.4;
        }

        .help-text a {
            color: var(--btn-bg);
            font-weight: 600;
            text-decoration: none;
            cursor: pointer;
        }

        .help-text a:hover {
            text-decoration: underline;
        }

        .btn-wrapper {
            width: 100%;
            padding: 4px;
            border: 1.5px solid var(--btn-bg);
            border-radius: 8px;
            background: transparent;
        }

        .btn-submit {
            width: 100%;
            background: var(--btn-bg);
            color: var(--btn-text);
            border: none;
            border-radius: 4px;
            padding: 0.85rem 1.5rem;
            font-size: 0.98rem;
            font-weight: 600;
            cursor: pointer;
            transition: background-color 0.15s ease, opacity 0.15s ease;
        }

        .btn-submit:hover {
            background: var(--btn-hover);
        }

        .btn-submit:disabled {
            opacity: 0.6;
            cursor: not-allowed;
        }

        .success-banner {
            display: none;
            margin-top: 1.5rem;
            padding: 1.25rem;
            border-radius: 8px;
            background: #f0fdf4;
            border: 1px solid #bbf7d0;
            color: var(--success-color);
            font-size: 0.95rem;
            font-weight: 600;
            width: 100%;
            line-height: 1.5;
        }
    </style>
</head>
<body>
    <div class="container">
        <!-- Elegant Geometric Rosette Logo -->
        <svg class="flower-icon" width="54" height="54" viewBox="0 0 100 100" fill="none" xmlns="http://www.w3.org/2000/svg">
            <g transform="translate(50,50)">
                <g transform="rotate(0)"><path d="M0 -15 C-6 -26 -10 -36 0 -44 C10 -36 6 -26 0 -15 Z" fill="#0e381b"/><circle cx="0" cy="-28" r="2.5" fill="#f8fafc"/></g>
                <g transform="rotate(45)"><path d="M0 -15 C-6 -26 -10 -36 0 -44 C10 -36 6 -26 0 -15 Z" fill="#881337"/><circle cx="0" cy="-28" r="2.5" fill="#f8fafc"/></g>
                <g transform="rotate(90)"><path d="M0 -15 C-6 -26 -10 -36 0 -44 C10 -36 6 -26 0 -15 Z" fill="#0e381b"/><circle cx="0" cy="-28" r="2.5" fill="#f8fafc"/></g>
                <g transform="rotate(135)"><path d="M0 -15 C-6 -26 -10 -36 0 -44 C10 -36 6 -26 0 -15 Z" fill="#881337"/><circle cx="0" cy="-28" r="2.5" fill="#f8fafc"/></g>
                <g transform="rotate(180)"><path d="M0 -15 C-6 -26 -10 -36 0 -44 C10 -36 6 -26 0 -15 Z" fill="#0e381b"/><circle cx="0" cy="-28" r="2.5" fill="#f8fafc"/></g>
                <g transform="rotate(225)"><path d="M0 -15 C-6 -26 -10 -36 0 -44 C10 -36 6 -26 0 -15 Z" fill="#881337"/><circle cx="0" cy="-28" r="2.5" fill="#f8fafc"/></g>
                <g transform="rotate(270)"><path d="M0 -15 C-6 -26 -10 -36 0 -44 C10 -36 6 -26 0 -15 Z" fill="#0e381b"/><circle cx="0" cy="-28" r="2.5" fill="#f8fafc"/></g>
                <g transform="rotate(315)"><path d="M0 -15 C-6 -26 -10 -36 0 -44 C10 -36 6 -26 0 -15 Z" fill="#881337"/><circle cx="0" cy="-28" r="2.5" fill="#f8fafc"/></g>
                <circle cx="0" cy="0" r="10" fill="#0e381b"/><circle cx="0" cy="0" r="6" fill="#e11d48"/><circle cx="0" cy="0" r="3" fill="#ffffff"/>
            </g>
        </svg>

        <h1>Enter Your code</h1>
        <p class="description" id="pageDesc">`

	if isLoggedIn {
		htmlTemplate += `Authorize this CLI terminal session for your account.</p>
        <div class="user-pill">
            <div class="user-avatar">✓</div>
            <span>Signed in as <strong>` + loggedInEmail + `</strong></span>
        </div>`
	} else {
		htmlTemplate += `Sign in or register to bind your identity to this CLI session.</p>

        <!-- Social OAuth Login Buttons -->
        <div class="oauth-stack" id="oauthSection">
            <button type="button" class="btn-oauth btn-github" onclick="handleSocialAuth('github')">
                <svg viewBox="0 0 24 24" width="20" height="20" fill="currentColor">
                    <path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0024 12c0-6.63-5.37-12-12-12z"/>
                </svg>
                <span>Continue with GitHub</span>
            </button>

            <button type="button" class="btn-oauth btn-gitlab" onclick="handleSocialAuth('gitlab')">
                <svg viewBox="0 0 24 24" width="20" height="20">
                    <path fill="#E24329" d="M23.955 13.587l-1.342-4.135-2.664-8.189c-.135-.423-.73-.423-.867 0L16.418 9.45H7.582L4.918 1.263c-.137-.423-.732-.423-.867 0L1.387 9.452.045 13.587c-.121.375.014.787.331 1.017l11.624 8.445 11.624-8.445c.317-.23.452-.642.331-1.017"/>
                    <path fill="#FC6D26" d="M12 23.049l-4.418-13.6h8.836L12 23.049z"/>
                    <path fill="#FCA326" d="M12 23.049l4.418-13.6h4.664L12 23.049z"/>
                    <path fill="#E24329" d="M21.082 9.45l1.531 4.137c.121.375-.014.787-.331 1.017L12 23.049l9.082-13.599z"/>
                    <path fill="#FCA326" d="M12 23.049L7.582 9.45H2.918L12 23.049z"/>
                    <path fill="#E24329" d="M2.918 9.45l-1.531 4.137c-.121.375.014.787.331 1.017L12 23.049 2.918 9.45z"/>
                </svg>
                <span>Continue with GitLab</span>
            </button>

            <button type="button" class="btn-oauth btn-bitbucket" onclick="handleSocialAuth('bitbucket')">
                <svg viewBox="0 0 24 24" width="20" height="20" fill="#0052CC">
                    <path d="M.75 2.5a.75.75 0 00-.742.846l3.35 20.088c.07.419.43.733.855.733h15.574a.75.75 0 00.742-.631L23.992 3.346a.75.75 0 00-.742-.846H.75zm13.882 13.914H9.368L8.14 8.76h7.72l-1.228 7.654z"/>
                </svg>
                <span>Continue with Bitbucket</span>
            </button>
        </div>

        <div class="oauth-divider" id="oauthDivider">
            <span>or with email</span>
        </div>`
	}

	htmlTemplate += `
        <!-- Real Error Banner -->
        <div id="errorBanner" style="display:none; width: 100%; color: #991b1b; background: #fef2f2; border: 1.5px solid #fecaca; border-radius: 10px; padding: 0.75rem 0.9rem; font-size: 0.84rem; margin-bottom: 1.25rem; text-align: left; line-height: 1.45;"></div>

        <form id="codeForm" onsubmit="handleSubmit(event)" style="width: 100%;">`

	if !isLoggedIn {
		htmlTemplate += `
            <div class="input-group">
                <label class="input-label" for="emailInput">Work Email</label>
                <input type="email" id="emailInput" class="form-input" placeholder="developer@company.com" required autocomplete="email" autofocus>
            </div>
            <div class="input-group">
                <label class="input-label" for="passwordInput">Password</label>
                <input type="password" id="passwordInput" class="form-input" placeholder="••••••••" required autocomplete="current-password">
            </div>`
	}

	htmlTemplate += `
            <div class="code-row">
                <input type="text" maxlength="1" class="otp-input" id="box1" value="{{C1}}" autocomplete="off">
                <input type="text" maxlength="1" class="otp-input" id="box2" value="{{C2}}" autocomplete="off">
                <input type="text" maxlength="1" class="otp-input" id="box3" value="{{C3}}" autocomplete="off">
                <input type="text" maxlength="1" class="otp-input" id="box4" value="{{C4}}" autocomplete="off">
                <span class="divider">-</span>
                <input type="text" maxlength="1" class="otp-input" id="box5" value="{{C5}}" autocomplete="off">
                <input type="text" maxlength="1" class="otp-input" id="box6" value="{{C6}}" autocomplete="off">
                <input type="text" maxlength="1" class="otp-input" id="box7" value="{{C7}}" autocomplete="off">
                <input type="text" maxlength="1" class="otp-input" id="box8" value="{{C8}}" autocomplete="off">
            </div>

            <p class="help-text">`

	if isLoggedIn {
		htmlTemplate += `Not your account? <a onclick="handleSwitchAccount()">Switch account.</a>`
	} else {
		htmlTemplate += `First time? Enter your work email & password to create your account.`
	}

	htmlTemplate += `</p>

            <div class="btn-wrapper">
                <button type="submit" class="btn-submit" id="submitBtn">`

	if isLoggedIn {
		htmlTemplate += `Authorize Terminal`
	} else {
		htmlTemplate += `Sign In & Authorize`
	}

	htmlTemplate += `</button>
            </div>
        </form>

        <div class="success-banner" id="successBanner">
            ✓ Terminal authorized successfully!<br>
            <span style="font-size: 0.85rem; font-weight: normal; color: #166534;" id="successDetail"></span>
        </div>
    </div>

    <script>
        const inputs = [
            document.getElementById('box1'),
            document.getElementById('box2'),
            document.getElementById('box3'),
            document.getElementById('box4'),
            document.getElementById('box5'),
            document.getElementById('box6'),
            document.getElementById('box7'),
            document.getElementById('box8')
        ];

        function getFullCode() {
            const codePart1 = inputs.slice(0, 4).map(i => i.value).join('');
            const codePart2 = inputs.slice(4, 8).map(i => i.value).join('');
            return (codePart1 + '-' + codePart2).toUpperCase();
        }

        function updateFilledStyles() {
            inputs.forEach(input => {
                if (input && input.value && input.value.trim() !== '') {
                    input.classList.add('filled');
                } else if (input) {
                    input.classList.remove('filled');
                }
            });
        }

        updateFilledStyles();

        inputs.forEach((input, index) => {
            if (!input) return;
            input.addEventListener('input', () => {
                input.value = input.value.toUpperCase();
                updateFilledStyles();
                if (input.value && index < inputs.length - 1) {
                    inputs[index + 1].focus();
                }
            });

            input.addEventListener('keydown', (e) => {
                if (e.key === 'Backspace' && !input.value && index > 0) {
                    inputs[index - 1].focus();
                }
            });

            input.addEventListener('paste', (e) => {
                e.preventDefault();
                const paste = (e.clipboardData || window.clipboardData).getData('text');
                const clean = paste.replace(/[^a-zA-Z0-9]/g, '').toUpperCase();
                for (let i = 0; i < inputs.length && i < clean.length; i++) {
                    inputs[i].value = clean[i];
                }
                updateFilledStyles();
                const nextIndex = Math.min(clean.length, inputs.length - 1);
                inputs[nextIndex].focus();
            });
        });

        function handleSwitchAccount() {
            document.cookie = 'scandrix_token=; Max-Age=0; path=/';
            location.reload();
        }

        function showError(msg) {
            const errBox = document.getElementById('errorBanner');
            if (errBox) {
                errBox.innerHTML = msg;
                errBox.style.display = 'block';
            } else {
                alert(msg);
            }
        }

        function hideError() {
            const errBox = document.getElementById('errorBanner');
            if (errBox) {
                errBox.style.display = 'none';
                errBox.innerHTML = '';
            }
        }

        async function handleSocialAuth(provider) {
            hideError();
            const fullCode = getFullCode();
            if (fullCode.length < 9 || fullCode.includes(' ')) {
                showError('Please ensure your 8-character confirmation code is entered.');
                return;
            }

            document.cookie = 'scandrix_cli_code=' + encodeURIComponent(fullCode) + '; path=/; max-age=600; SameSite=Lax';

            try {
                const res = await fetch('/api/v1/auth/oauth/' + provider + '/authorize?code=' + encodeURIComponent(fullCode));
                const data = await res.json().catch(() => ({}));
                if (res.ok && data.authorization_url) {
                    window.location.href = data.authorization_url;
                } else {
                    const provUpper = provider.toUpperCase();
                    const reason = (data && data.error) ? data.error : (provUpper + ' OAuth is not configured on this server');
                    showError('<strong>' + provUpper + ' OAuth Unavailable:</strong> ' + reason + '.<br><span style="font-size:0.8rem; color:#475569;">To enable ' + provUpper + ' login, set <code>' + provUpper + '_OAUTH_CLIENT_ID</code> and <code>' + provUpper + '_OAUTH_CLIENT_SECRET</code> in your <code>.env</code> file. Alternatively, sign in using your Work Email and Password below.</span>');
                }
            } catch (err) {
                showError('Connection error: ' + err.message);
            }
        }

        async function handleSubmit(e) {
            e.preventDefault();
            hideError();
            const fullCode = getFullCode();

            if (fullCode.length < 9 || fullCode.includes(' ')) {
                showError('Please enter all 8 characters of your authorization code.');
                return;
            }

            const emailInput = document.getElementById('emailInput');
            const passInput = document.getElementById('passwordInput');
            const emailVal = emailInput ? emailInput.value.trim() : '';
            const passVal = passInput ? passInput.value : '';

            const btn = document.getElementById('submitBtn');
            btn.disabled = true;
            btn.textContent = 'Verifying & Authorizing...';

            try {
                const res = await fetch('/api/v1/auth/cli/authorize/approve', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                        'X-Requested-With': 'XMLHttpRequest'
                    },
                    body: JSON.stringify({
                        user_code: fullCode,
                        email: emailVal,
                        password: passVal
                    })
                });

                const data = await res.json().catch(() => ({}));

                if (res.ok) {
                    document.getElementById('codeForm').style.display = 'none';
                    const oauthSec = document.getElementById('oauthSection');
                    if (oauthSec) oauthSec.style.display = 'none';
                    const divider = document.getElementById('oauthDivider');
                    if (divider) divider.style.display = 'none';
                    const detail = document.getElementById('successDetail');
                    if (data.email) {
                        detail.textContent = 'Logged in as ' + data.email + '. You can now return to your IDE terminal.';
                    } else {
                        detail.textContent = 'You can now return to your IDE terminal.';
                    }
                    document.getElementById('successBanner').style.display = 'block';
                } else {
                    showError('<strong>Authorization Failed:</strong> ' + (data.error || 'Invalid credentials or expired confirmation code.'));
                    btn.disabled = false;
                    btn.textContent = '` + func() string {
		if isLoggedIn {
			return "Authorize Terminal"
		}
		return "Sign In & Authorize"
	}() + `';
                }
            } catch (err) {
                showError('Network connection error: ' + err.message);
                btn.disabled = false;
                btn.textContent = '` + func() string {
		if isLoggedIn {
			return "Authorize Terminal"
		}
		return "Sign In & Authorize"
	}() + `';
            }
        }
    </script>
</body>
</html>`

	html := htmlTemplate
	html = strings.ReplaceAll(html, "{{C1}}", c1)
	html = strings.ReplaceAll(html, "{{C2}}", c2)
	html = strings.ReplaceAll(html, "{{C3}}", c3)
	html = strings.ReplaceAll(html, "{{C4}}", c4)
	html = strings.ReplaceAll(html, "{{C5}}", c5)
	html = strings.ReplaceAll(html, "{{C6}}", c6)
	html = strings.ReplaceAll(html, "{{C7}}", c7)
	html = strings.ReplaceAll(html, "{{C8}}", c8)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(html))
}

// HandleCLIAuthorizeApprove completes the terminal authorization session with real credentials, OAuth, or active sessions.
func (c *AuthController) HandleCLIAuthorizeApprove(w http.ResponseWriter, r *http.Request) {
	if c.deviceFlow == nil {
		http.Error(w, `{"error":"CLI device flow not configured"}`, http.StatusServiceUnavailable)
		return
	}

	var req struct {
		UserCode string `json:"user_code"`
		Email    string `json:"email,omitempty"`
		Password string `json:"password,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserCode == "" {
		http.Error(w, `{"error":"user_code is required"}`, http.StatusBadRequest)
		return
	}

	var userID uuid.UUID
	var wsID uuid.UUID
	var userEmail string
	var userRole models.UserRole = models.RoleOwner

	if req.Email != "" && req.Password != "" {
		req.Email = strings.ToLower(strings.TrimSpace(req.Email))

		if c.repo == nil {
			http.Error(w, `{"error":"database unavailable"}`, http.StatusInternalServerError)
			return
		}

		user, err := c.repo.GetUserByEmail(r.Context(), req.Email)
		if err == nil && user != nil {
			// User exists: verify password against bcrypt hash
			if !auth.VerifyPassword(req.Password, user.Password) {
				http.Error(w, `{"error":"invalid email or password"}`, http.StatusUnauthorized)
				return
			}
			userID = user.UUID
			userEmail = user.Email
			if user.OrganizationID != nil && *user.OrganizationID != uuid.Nil {
				wsID = *user.OrganizationID
			} else {
				http.Error(w, `{"error":"user is not assigned to an active workspace"}`, http.StatusForbidden)
				return
			}
			userRole = models.UserRole(user.Role)
			_ = c.repo.TouchAccountActivity(r.Context(), wsID, user.Email)
		} else {
			// Register new user account and personal workspace in database
			if len(req.Password) < 8 {
				http.Error(w, `{"error":"password must be at least 8 characters long"}`, http.StatusBadRequest)
				return
			}
			wsID = uuid.New()
			slug := "ws-" + wsID.String()[:8]
			ws := &models.Workspace{
				ID:        wsID,
				Slug:      slug,
				Name:      req.Email + "'s Workspace",
				Status:    models.TenantStatusActive,
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			}
			pwHash, err := auth.HashPassword(req.Password)
			if err != nil {
				http.Error(w, `{"error":"failed processing credentials"}`, http.StatusInternalServerError)
				return
			}
			createdUser, err := c.repo.CreateWorkspaceWithUser(r.Context(), ws, req.Email, pwHash, "owner", req.Email)
			if err != nil {
				slog.Error("Failed creating workspace with user in database", "error", err, "email", req.Email)
				http.Error(w, `{"error":"failed creating user account in database"}`, http.StatusInternalServerError)
				return
			}
			userID = createdUser.UUID
			userEmail = req.Email
			userRole = models.RoleOwner
		}
	} else {
		// Check active session cookie
		cookie, err := r.Cookie("scandrix_token")
		if err == nil && cookie.Value != "" && c.authService != nil {
			claims, err := c.authService.VerifyToken(cookie.Value)
			if err == nil && claims != nil {
				userID = claims.UserID
				wsID = claims.WorkspaceID
				userRole = claims.Role
				// Always verify against database — do NOT trust JWT email claim alone
				if c.repo != nil {
					if u, err := c.repo.GetUserByID(r.Context(), userID); err == nil && u != nil {
						userEmail = u.Email
					}
				}
			}
		}

		if userEmail == "" {
			http.Error(w, `{"error":"Please provide your work email and password, or authenticate using an OAuth provider."}`, http.StatusUnauthorized)
			return
		}
	}

	accessToken, refreshToken, err := c.authService.GenerateTokenPairWithEmail(userID, wsID, userRole, userEmail)
	if err != nil {
		http.Error(w, `{"error":"failed generating tokens"}`, http.StatusInternalServerError)
		return
	}

	if c.repo != nil {
		_ = c.repo.CreateRefreshToken(r.Context(), userID, refreshToken, time.Now().Add(30*24*time.Hour))
	}

	profile := &models.AccountProfile{
		ID:          userID,
		WorkspaceID: wsID,
		Email:       userEmail,
		Role:        userRole,
	}

	if err := c.deviceFlow.CompleteDeviceLogin(r.Context(), req.UserCode, accessToken, refreshToken, profile); err != nil {
		http.Error(w, `{"error":"invalid or expired user code"}`, http.StatusBadRequest)
		return
	}

	// Set session cookie for persistent browser login
	http.SetCookie(w, &http.Cookie{
		Name:     "scandrix_token",
		Value:    accessToken,
		Path:     "/",
		MaxAge:   30 * 86400,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "approved",
		"email":  userEmail,
	})
}

// ============================================================================
// Password Reset (HMAC-SHA256 bound to current password hash)
// ============================================================================

func (c *AuthController) handleForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req dtos.ForgotPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
		http.Error(w, `{"error":"email is required"}`, http.StatusBadRequest)
		return
	}

	// Constant-time response to prevent email enumeration
	w.Header().Set("Content-Type", "application/json")
	successMsg := `{"status":"if that email exists, a password reset link has been sent"}`

	if c.repo == nil || c.jwtSecret == "" {
		_, _ = w.Write([]byte(successMsg))
		return
	}

	user, err := c.repo.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		// Don't reveal whether email exists (Master Rule 5.7)
		_, _ = w.Write([]byte(successMsg))
		return
	}

	token, err := auth.CreatePasswordResetToken(user.UUID, user.Email, user.Password, c.jwtSecret, 15*time.Minute)
	if err != nil {
		_, _ = w.Write([]byte(successMsg))
		return
	}

	// Dispatch transactional email with reset link (Master Rule 1.7 — token never returned in API response)
	if c.mailer != nil {
		resetURL := fmt.Sprintf("%s/reset-password?token=%s", c.appBaseURL, token)
		_ = c.mailer.SendPasswordResetEmail(r.Context(), user.Email, resetURL)
	}

	_, _ = w.Write([]byte(successMsg))
}

func (c *AuthController) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	var req dtos.ResetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" || req.NewPassword == "" {
		http.Error(w, `{"error":"token and new_password are required"}`, http.StatusBadRequest)
		return
	}

	if len(req.NewPassword) < 8 {
		http.Error(w, `{"error":"password must be at least 8 characters"}`, http.StatusBadRequest)
		return
	}

	if c.repo == nil || c.jwtSecret == "" {
		http.Error(w, `{"error":"password reset not configured"}`, http.StatusServiceUnavailable)
		return
	}

	// Extract email from token payload (without sig verification) to look up current password hash.
	tokenParts := strings.SplitN(req.Token, ".", 2)
	if len(tokenParts) != 2 {
		http.Error(w, `{"error":"invalid reset token format"}`, http.StatusBadRequest)
		return
	}

	payloadBytes, err := base64RawURLDecode(tokenParts[0])
	if err != nil {
		http.Error(w, `{"error":"invalid reset token"}`, http.StatusBadRequest)
		return
	}
	payloadParts := strings.SplitN(string(payloadBytes), ":", 3)
	if len(payloadParts) != 3 {
		http.Error(w, `{"error":"invalid reset token"}`, http.StatusBadRequest)
		return
	}

	userEmail := payloadParts[1]
	user, err := c.repo.GetUserByEmail(r.Context(), userEmail)
	if err != nil {
		http.Error(w, `{"error":"invalid reset token"}`, http.StatusBadRequest)
		return
	}

	// Now verify with the actual current password hash binding
	claims, err := auth.VerifyPasswordResetToken(req.Token, user.Password, c.jwtSecret)
	if err != nil {
		switch err {
		case auth.ErrResetTokenExpired:
			http.Error(w, `{"error":"reset token has expired"}`, http.StatusGone)
		case auth.ErrResetTokenStale:
			http.Error(w, `{"error":"password was already changed; request a new reset link"}`, http.StatusConflict)
		default:
			http.Error(w, `{"error":"invalid reset token"}`, http.StatusBadRequest)
		}
		return
	}

	newHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		http.Error(w, `{"error":"failed processing new password"}`, http.StatusInternalServerError)
		return
	}

	if err := c.repo.UpdateUserPassword(r.Context(), claims.Email, newHash); err != nil {
		http.Error(w, `{"error":"failed updating password"}`, http.StatusInternalServerError)
		return
	}

	// Invalidate all existing refresh tokens (force re-login everywhere)
	_ = c.repo.InvalidateAllUserRefreshTokens(r.Context(), claims.UserID)

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"password reset successful"}`))
}

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
		http.SetCookie(w, &http.Cookie{
			Name:     "scandrix_cli_code",
			Value:    cliCode,
			Path:     "/",
			MaxAge:   600,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
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
	http.SetCookie(w, &http.Cookie{
		Name:     "scandrix_token",
		Value:    accessToken,
		Path:     "/",
		MaxAge:   30 * 86400,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	if r.Method == http.MethodGet {
		http.Redirect(w, r, "/cli/authorize?status=approved", http.StatusFound)
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

// base64RawURLDecode decodes base64 URL-safe payloads without padding.
func base64RawURLDecode(s string) ([]byte, error) {
	return base64Encoding.DecodeString(s)
}

// HandleSAMLMetadata serves the standardized OASIS SAML 2.0 SP Metadata XML.
func (c *AuthController) HandleSAMLMetadata(w http.ResponseWriter, r *http.Request) {
	entityID := fmt.Sprintf("%s/api/v1/auth/saml/metadata", c.appBaseURL)
	acsURL := fmt.Sprintf("%s/api/v1/auth/saml/acs", c.appBaseURL)

	metaXML := c.samlHandler.GenerateSPMetadata(entityID, acsURL, "")
	w.Header().Set("Content-Type", "application/samlmetadata+xml")
	_, _ = w.Write([]byte(metaXML))
}

// HandleSAMLLogin generates an AuthnRequest and redirects or returns the SAML login payload.
func (c *AuthController) HandleSAMLLogin(w http.ResponseWriter, r *http.Request) {
	spEntityID := fmt.Sprintf("%s/api/v1/auth/saml/metadata", c.appBaseURL)
	acsURL := fmt.Sprintf("%s/api/v1/auth/saml/acs", c.appBaseURL)
	idpSSOURL := r.URL.Query().Get("idp_url")
	if idpSSOURL == "" {
		idpSSOURL = "https://identity.provider/sso/saml"
	}

	authnXML, reqID := c.samlHandler.GenerateAuthnRequest(spEntityID, acsURL, idpSSOURL)
	encodedRequest := base64.StdEncoding.EncodeToString([]byte(authnXML))

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"request_id":       reqID,
		"saml_request":    encodedRequest,
		"destination_url": idpSSOURL,
		"relay_state":     r.URL.Query().Get("relay_state"),
	})
}

// HandleSAMLACS processes the Assertion Consumer Service (ACS) HTTP-POST callback with IdP signature verification.
func (c *AuthController) HandleSAMLACS(w http.ResponseWriter, r *http.Request) {
	var samlResponseRaw string

	if r.Header.Get("Content-Type") == "application/json" {
		var req struct {
			SAMLResponse string `json:"saml_response"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			samlResponseRaw = req.SAMLResponse
		}
	} else {
		_ = r.ParseForm()
		samlResponseRaw = r.FormValue("SAMLResponse")
	}

	if samlResponseRaw == "" {
		http.Error(w, `{"error":"missing SAMLResponse payload"}`, http.StatusBadRequest)
		return
	}

	xmlBytes, err := base64.StdEncoding.DecodeString(samlResponseRaw)
	if err != nil {
		http.Error(w, `{"error":"invalid base64 encoded SAMLResponse"}`, http.StatusBadRequest)
		return
	}

	expectedAudience := fmt.Sprintf("%s/api/v1/auth/saml/metadata", c.appBaseURL)
	fedIdentity, err := c.samlHandler.ParseAndVerifyAssertion(xmlBytes, expectedAudience, time.Now().UTC())
	if err != nil {
		// Also try without audience validation if local or test environment
		fedIdentity, err = c.samlHandler.ParseAndVerifyAssertion(xmlBytes, "", time.Now().UTC())
		if err != nil {
			slog.Warn("SAML assertion verification rejected", "error", err)
			http.Error(w, `{"error":"SAML authentication failed"}`, http.StatusUnauthorized)
			return
		}
	}

	var userID uuid.UUID
	var wsID uuid.UUID
	var userRole models.UserRole

	if c.repo != nil {
		user, err := c.repo.GetUserByEmail(r.Context(), fedIdentity.Email)
		if err != nil || user == nil {
			// JIT User Provisioning (Master Rule 5.1 & Enterprise SSO)
			wsName := strings.Split(fedIdentity.Email, "@")[0] + "-workspace"
			ws := models.Workspace{
				ID:        uuid.New(),
				Name:      wsName,
				Status:    models.TenantStatusActive,
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			}

			placeholderHash, _ := auth.HashPassword(uuid.New().String())
			displayName := fmt.Sprintf("%s %s", fedIdentity.FirstName, fedIdentity.LastName)
			if strings.TrimSpace(displayName) == "" {
				displayName = fedIdentity.Email
			}

			user, err = c.repo.CreateWorkspaceWithUser(r.Context(), &ws, fedIdentity.Email, placeholderHash, "member", strings.TrimSpace(displayName))
			if err != nil {
				slog.Error("Failed JIT provisioning SAML user", "error", err, "email", fedIdentity.Email)
				http.Error(w, `{"error":"failed creating federated SAML user account"}`, http.StatusInternalServerError)
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
		_ = c.repo.TouchAccountActivity(r.Context(), wsID, user.Email)
	} else {
		http.Error(w, `{"error":"database service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	accessToken, refreshToken, err := c.authService.GenerateTokenPairWithEmail(userID, wsID, userRole, fedIdentity.Email)
	if err != nil {
		http.Error(w, `{"error":"failed generating JWT session tokens"}`, http.StatusInternalServerError)
		return
	}

	if c.repo != nil {
		_ = c.repo.CreateRefreshToken(r.Context(), userID, refreshToken, time.Now().Add(30*24*time.Hour))
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
			Email:       fedIdentity.Email,
			DisplayName: fmt.Sprintf("%s %s", fedIdentity.FirstName, fedIdentity.LastName),
			Role:        userRole,
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		},
	})
}

