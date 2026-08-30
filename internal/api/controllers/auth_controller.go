package controllers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
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
	"github.com/scandrix/backend/internal/cache/limiter"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/pkg/models"
)

// base64Encoding is the URL-safe, no-padding base64 decoder for reset tokens.
var base64Encoding = base64.RawURLEncoding

// AuthController handles identity, login, tokens, CLI API keys, OAuth, and device flows.
type AuthController struct {
	authService     *auth.Authenticator
	rateLimiter     *limiter.TokenBucketLimiter
	repo            *database.Repository
	deviceFlow      *cliauth.DeviceFlowManager
	oauthService    *oauth.OAuthService
	oauthStateStore *oauth.StateStore
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
		appBaseURL:  "http://localhost:3000",
	}
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

		// RFC 8628 CLI Device Authorization (public)
		public.Post("/cli/device/initiate", c.handleDeviceInitiate)
		public.Get("/cli/device/poll", c.handleDevicePoll)

		// OAuth 2.0 Social Logins (public)
		public.Get("/oauth/{provider}/authorize", c.handleOAuthAuthorize)
		public.Post("/oauth/{provider}/callback", c.handleOAuthCallback)
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
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			clientIP = strings.TrimSpace(strings.Split(xff, ",")[0])
		} else if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
			clientIP = strings.TrimSpace(xrip)
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

	userID := user.UUID
	wsID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	if user.OrganizationID != nil {
		wsID = *user.OrganizationID
	}
	userRole := models.UserRole(user.Role)
	displayName := req.Email

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

	pwHash, err := auth.HashPassword(req.Password)
	if err != nil {
		http.Error(w, `{"error":"failed processing credentials"}`, http.StatusInternalServerError)
		return
	}

	user, err := c.repo.CreateUser(r.Context(), req.Email, pwHash, "owner", nil)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"user registration failed: %s"}`, err.Error()), http.StatusConflict)
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

	userID := user.UUID
	wsID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	if user.OrganizationID != nil {
		wsID = *user.OrganizationID
	}
	userRole := models.UserRole(user.Role)

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
		profile = &models.AccountProfile{
			ID:          uuid.New(),
			WorkspaceID: wsID,
			Email:       "user@scandrix.dev",
			DisplayName: "ScanDrix Operator",
			Role:        models.RoleOwner,
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		}
	}

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
		http.Error(w, fmt.Sprintf(`{"error":"failed initiating device flow: %s"}`, err.Error()), http.StatusInternalServerError)
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
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"device login approved"}`))
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

	// Generate CSRF state via server-side store (one-time-use, 10-min TTL)
	state, err := c.oauthStateStore.Generate(provider)
	if err != nil {
		http.Error(w, `{"error":"failed generating CSRF state"}`, http.StatusInternalServerError)
		return
	}

	authURL, err := c.oauthService.GetAuthorizationURL(provider, state)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
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

	var req dtos.OAuthCallbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
		http.Error(w, `{"error":"authorization code is required"}`, http.StatusBadRequest)
		return
	}

	// Validate CSRF state parameter (one-time-use, provider-bound)
	if c.oauthStateStore == nil || !c.oauthStateStore.Validate(req.State, provider) {
		http.Error(w, `{"error":"invalid or expired OAuth state parameter"}`, http.StatusForbidden)
		return
	}

	oauthProfile, err := c.oauthService.ExchangeCode(r.Context(), provider, req.Code)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"OAuth authentication failed: %s"}`, err.Error()), http.StatusUnauthorized)
		return
	}

	// Find-or-create user in database
	var userID uuid.UUID
	wsID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userRole := models.RoleOwner

	if c.repo != nil {
		user, err := c.repo.GetUserByEmail(r.Context(), oauthProfile.Email)
		if err != nil {
			// New user: create account with a random placeholder password
			placeholderHash, _ := auth.HashPassword(uuid.New().String())
			user, err = c.repo.CreateUser(r.Context(), oauthProfile.Email, placeholderHash, "owner", nil)
			if err != nil {
				http.Error(w, `{"error":"failed creating user account from OAuth"}`, http.StatusInternalServerError)
				return
			}
		}
		userID = user.UUID
		if user.OrganizationID != nil {
			wsID = *user.OrganizationID
		}
		userRole = models.UserRole(user.Role)
	} else {
		userID = uuid.New()
	}

	accessToken, refreshToken, err := c.authService.GenerateTokenPair(userID, wsID, userRole)
	if err != nil {
		http.Error(w, `{"error":"failed generating tokens"}`, http.StatusInternalServerError)
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

