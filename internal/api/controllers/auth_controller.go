package controllers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/auth/cliauth"
	"github.com/scandrix/backend/internal/auth/clitokens"
	"github.com/scandrix/backend/internal/auth/mailer"
	"github.com/scandrix/backend/internal/auth/oauth"
	"github.com/scandrix/backend/internal/auth/sso"
	"github.com/scandrix/backend/internal/cache"
	"github.com/scandrix/backend/internal/cache/limiter"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/telemetry"
	"github.com/scandrix/backend/pkg/models"
	scandrixMiddleware "github.com/scandrix/backend/internal/api/middleware"
)

// base64Encoding is the URL-safe, no-padding base64 decoder for reset tokens.
var base64Encoding = base64.RawURLEncoding

// AuthRepository defines the data access contract for user authentication, sessions, and tokens (Clean Architecture).
type AuthRepository interface {
	GetUserByEmail(ctx context.Context, email string) (*database.UserRecord, error)
	UpdateUserPassword(ctx context.Context, email, passwordHash string) error
	TouchAccountActivity(ctx context.Context, wsID uuid.UUID, email string) error
	CreateRefreshToken(ctx context.Context, userUUID uuid.UUID, token string, expiryDate time.Time) error
	CreateWorkspaceWithUser(ctx context.Context, ws *models.Workspace, userEmail, passwordHash, role, name string) (*database.UserRecord, error)
	UpdateUserStatus(ctx context.Context, userUUID uuid.UUID, status string) error
	GetRefreshToken(ctx context.Context, token string) (*database.RefreshTokenRecord, error)
	InvalidateAllUserRefreshTokens(ctx context.Context, userUUID uuid.UUID) error
	MarkRefreshTokenUsed(ctx context.Context, token string) error
	GetUserByID(ctx context.Context, userUUID uuid.UUID) (*database.UserRecord, error)
	SaveAPIKey(ctx context.Context, id, workspaceID uuid.UUID, name, keyHash, prefix string, expiresAt *time.Time) error
	UpsertIntegrationConnection(ctx context.Context, wsID uuid.UUID, provider models.SCMProvider, accountName, tokenPlain string, isConnected bool, repoCount int) error
	VerifyCLIToken(ctx context.Context, rawKey string) (workspaceID uuid.UUID, profile *models.AccountProfile, err error)
	GetWorkspaceByID(ctx context.Context, id uuid.UUID) (*models.Workspace, error)
	GetTeamByID(ctx context.Context, id uuid.UUID) (*models.Team, error)
	ListTeams(ctx context.Context, wsID uuid.UUID) ([]models.Team, error)
	GetOrganizationByEmailDomain(ctx context.Context, domain string) (*uuid.UUID, error)
	ListAPIKeys(ctx context.Context, workspaceID uuid.UUID) ([]*models.TeamCLIKey, error)
	RevokeAPIKey(ctx context.Context, workspaceID, keyID uuid.UUID) error
}

// AuthController handles identity, login, tokens, CLI API keys, OAuth, SAML, and device flows.
type AuthController struct {
	authService         *auth.Authenticator
	rateLimiter         limiter.RateLimiter
	registerRateLimiter limiter.RateLimiter
	accountLimiter      limiter.RateLimiter
	cacheClient         *cache.Client
	failedLock          sync.Mutex
	failedLogins        map[string]*failedLoginEntry
	repo                AuthRepository
	deviceFlow          *cliauth.DeviceFlowManager
	oauthService        *oauth.OAuthService
	oauthStateStore     *oauth.StateStore
	samlHandler         *sso.SAMLHandler
	mailer              mailer.EmailSender
	appBaseURL          string
	jwtSecret           string
	cliTokenService     *clitokens.TokenService
	loopbackMgr         *cliauth.LoopbackManager
	helpdeskSvc         *auth.HelpdeskTokenService
	deviceQuota         *auth.DeviceManager
	domainVerifier      *sso.DomainVerifierService
	testWorkbench       *sso.SSOTestSessionWorkbench

	// Registration anti-abuse & security controls
	requireEmailVerification bool
	blockedEmailDomains      []string
	turnstileSecretKey       string
}

// NewAuthController initializes the auth controller with token bucket rate limiting and DB access.
func NewAuthController(authService *auth.Authenticator, repo AuthRepository) *AuthController {
	if isNilInterface(repo) {
		repo = nil
	}
	tb := limiter.NewTokenBucketLimiter(limiter.RateLimitConfig{
		Capacity:          10,
		RefillRatePerSec:  0.1, // ~6 requests per minute
		ExpirationTimeout: 15 * time.Minute,
	})
	regLimiter := limiter.NewTokenBucketLimiter(limiter.RateLimitConfig{
		Capacity:          3,      // Max burst of 3 account signups per IP
		RefillRatePerSec:  0.001,  // ~3-4 signups per hour per IP
		ExpirationTimeout: 2 * time.Hour,
	})
	acctLimiter := limiter.NewTokenBucketLimiter(limiter.RateLimitConfig{
		Capacity:          15,   // Max burst of 15 attempts against same account
		RefillRatePerSec:  0.25, // Refill 1 every 4 seconds
		ExpirationTimeout: 15 * time.Minute,
	})
	saml := sso.NewSAMLHandler()
	return &AuthController{
		authService:         authService,
		rateLimiter:         tb,
		registerRateLimiter: regLimiter,
		accountLimiter:      acctLimiter,
		failedLogins:        make(map[string]*failedLoginEntry),
		repo:                repo,
		samlHandler:         saml,
		domainVerifier:      sso.NewDomainVerifierService(nil, false),
		testWorkbench:       sso.NewSSOTestSessionWorkbench(saml, nil),
		appBaseURL:          "http://localhost:3000",
		loopbackMgr:         cliauth.NewLoopbackManager("http://localhost:3000"),
		deviceQuota:         auth.NewDeviceManager(nil, 10),
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

// SetLoopbackManager attaches the RFC 8252 loopback authorization manager.
func (c *AuthController) SetLoopbackManager(mgr *cliauth.LoopbackManager) {
	c.loopbackMgr = mgr
}

// SetHelpdeskTokenService attaches the RS256 single sign-on token minter for helpdesk iframe.
func (c *AuthController) SetHelpdeskTokenService(svc *auth.HelpdeskTokenService) {
	c.helpdeskSvc = svc
}

// SetDeviceManager attaches the hardware tracking and device quota manager.
func (c *AuthController) SetDeviceManager(dm *auth.DeviceManager) {
	c.deviceQuota = dm
}

// SetOAuthService attaches the OAuth 2.0 social login provider with a CSRF state store.
func (c *AuthController) SetOAuthService(svc *oauth.OAuthService) {
	c.oauthService = svc
	if c.oauthStateStore == nil {
		c.oauthStateStore = oauth.NewStateStore(10 * time.Minute)
	}
}

// SetOAuthStateStore attaches a custom or distributed Redis-backed OAuth state store.
func (c *AuthController) SetOAuthStateStore(store *oauth.StateStore) {
	c.oauthStateStore = store
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
func (c *AuthController) SetRateLimiter(tb limiter.RateLimiter) {
	c.rateLimiter = tb
}

// SetRegisterRateLimiter allows customizing or injecting a test registration rate limiter.
func (c *AuthController) SetRegisterRateLimiter(tb limiter.RateLimiter) {
	c.registerRateLimiter = tb
}

// SetAccountLimiter allows customizing or injecting a test account-targeted rate limiter.
func (c *AuthController) SetAccountLimiter(tb limiter.RateLimiter) {
	c.accountLimiter = tb
}

// SetCacheClient attaches the Redis cache client and activates distributed rate limiting across API pods.
func (c *AuthController) SetCacheClient(client *cache.Client) {
	c.cacheClient = client
	if client != nil && client.RawClient() != nil {
		c.rateLimiter = limiter.NewRedisTokenBucketLimiter(client.RawClient(), limiter.RateLimitConfig{
			Capacity:          10,
			RefillRatePerSec:  0.1,
			ExpirationTimeout: 15 * time.Minute,
		})
		c.registerRateLimiter = limiter.NewRedisTokenBucketLimiter(client.RawClient(), limiter.RateLimitConfig{
			Capacity:          3,
			RefillRatePerSec:  0.001,
			ExpirationTimeout: 2 * time.Hour,
		})
		c.accountLimiter = limiter.NewRedisTokenBucketLimiter(client.RawClient(), limiter.RateLimitConfig{
			Capacity:          15,
			RefillRatePerSec:  0.25,
			ExpirationTimeout: 15 * time.Minute,
		})
	}
}

// SetRequireEmailVerification toggles email confirmation requirement before issuing access tokens.
func (c *AuthController) SetRequireEmailVerification(require bool) {
	c.requireEmailVerification = require
}

// SetBlockedEmailDomains sets custom blacklisted email domains for registration.
func (c *AuthController) SetBlockedEmailDomains(domains []string) {
	c.blockedEmailDomains = domains
}

// SetTurnstileSecretKey configures Cloudflare Turnstile secret key for bot verification.
func (c *AuthController) SetTurnstileSecretKey(key string) {
	c.turnstileSecretKey = strings.TrimSpace(key)
}

// SetCLITokenService attaches the token service for managed team CLI keys.
func (c *AuthController) SetCLITokenService(svc *clitokens.TokenService) {
	c.cliTokenService = svc
}

// SSOTestWorkbench returns the ephemeral SSO diagnostic test workbench.
func (c *AuthController) SSOTestWorkbench() *sso.SSOTestSessionWorkbench {
	return c.testWorkbench
}

// DomainVerifier returns the enterprise domain verifier service.
func (c *AuthController) DomainVerifier() *sso.DomainVerifierService {
	return c.domainVerifier
}

// Routes mounts the authentication endpoints with public rate limiting.
func (c *AuthController) Routes() chi.Router {
	r := chi.NewRouter()

	// CLI key validation & health check
	r.Get("/cli/validate-key", c.HandleValidateCLIKey)
	r.Post("/cli/validate-key", c.HandleValidateCLIKey)

	// CLI auth session info for confirmation UI
	r.Get("/cli/auth/login-info", c.HandleCLILoginInfo)
	r.Get("/cli/login-info", c.HandleCLILoginInfo)

	// SSO check
	r.Get("/sso/check", c.HandleSSOCheck)
	r.Get("/auth/sso/check", c.HandleSSOCheck)
	r.Get("/sso/login/{organizationId}", c.HandleSAMLLogin)
	r.Post("/sso/saml/callback/{organizationId}", c.HandleSAMLACS)

	// SSO Domain Verification & Connection Test Workbench
	r.Post("/sso/domains/verify-dns", c.HandleVerifyDomainDNS)
	r.Post("/sso/domains/confirm-token", c.HandleConfirmDomainToken)
	r.Get("/sso/test-connection/result", c.HandleGetSSOConnectionTestResult)
	r.Post("/sso/test-connection/callback", c.HandleSSOConnectionTestCallback)

	// RFC 8628 CLI Device Authorization (public polling)
	r.Post("/cli/device/initiate", c.handleDeviceInitiate)
	r.Get("/cli/device/poll", c.handleDevicePoll)
	r.Get("/cli/authorize", c.HandleCLIAuthorizePage)
	r.Post("/cli/authorize/approve", c.HandleCLIAuthorizeApprove)

	// RFC 8252 CLI Loopback Authentication (public polling)
	r.Post("/cli/loopback/initiate", c.handleLoopbackInitiate)
	r.Get("/cli/loopback/poll", c.handleLoopbackPoll)
	r.Post("/cli/auth/login-init", c.handleLoopbackInitiate)
	r.Post("/cli/auth/device-init", c.HandleCLIDeviceInit)
	r.Get("/cli/auth/login-poll", c.HandleCLILoginPoll)

	// Rate-limited public authentication endpoints (Master Rule 4.5)
	r.Group(func(public chi.Router) {
		public.Use(c.rateLimitMiddleware)
		public.Post("/login", c.handleLogin)
		public.Post("/refresh", c.handleRefreshToken)
		public.Post("/logout", c.handleLogout)

		// Registration with dedicated stricter rate limiter (Master Rule 4.5)
		public.Group(func(reg chi.Router) {
			reg.Use(c.registerRateLimitMiddleware)
			reg.Post("/register", c.handleRegister)
			reg.Post("/signup", c.handleRegister)
		})

		// Email Confirmation & Resend
		public.Post("/confirm-email", c.handleConfirmEmail)
		public.Post("/resend-email", c.handleResendEmail)

		// Password Reset (rate-limited, public)
		public.Post("/password/forgot", c.handleForgotPassword)
		public.Post("/password/reset", c.handleResetPassword)
		public.Post("/forgot-password", c.handleForgotPassword)
		public.Post("/reset-password", c.handleResetPassword)

		// OAuth 2.0 Social Logins (public)
		public.Get("/oauth/{provider}/authorize", c.handleOAuthAuthorize)
		public.Get("/oauth/{provider}/callback", c.handleOAuthCallback)
		public.Post("/oauth/{provider}/callback", c.handleOAuthCallback)
		public.Post("/oauth", c.handlePostOAuth)

		// SAML 2.0 Enterprise Single Sign-On (public)
		public.Get("/saml/metadata", c.HandleSAMLMetadata)
		public.Get("/saml/login", c.HandleSAMLLogin)
		public.Post("/saml/acs", c.HandleSAMLACS)
	})

	// Protected routes
	r.Group(func(pr chi.Router) {
		pr.Use(c.authService.Middleware)
		pr.Get("/me", c.handleMe)
		pr.Get("/cli-keys", c.handleListCLIKeys)
		pr.Post("/cli-keys", c.handleCreateCLIKey)
		pr.Delete("/cli-keys/{keyId}", c.handleRevokeCLIKey)

		// CLI device complete requires authenticated user approving the user code
		pr.Post("/cli/device/complete", c.handleDeviceComplete)
		pr.Post("/cli/auth/login-complete", c.HandleCLILoginComplete)

		// Helpdesk RS256 SSO token generation (protected, supports GET & POST)
		pr.Get("/helpdesk-token", c.handleGenerateHelpdeskToken)
		pr.Post("/helpdesk-token", c.handleGenerateHelpdeskToken)
		pr.Post("/api/v1/auth/helpdesk-token", c.handleGenerateHelpdeskToken)

		// SSO domain & connection test administration
		pr.Post("/sso/domains/request-verification", c.HandleRequestDomainVerification)
		pr.Get("/sso/domains/status", c.HandleGetDomainStatus)
		pr.Post("/sso/test-connection/start", c.HandleStartSSOConnectionTest)
	})

	return r
}

func (c *AuthController) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req dtos.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" || req.Password == "" {
		http.Error(w, `{"error":"email and password are required"}`, http.StatusBadRequest)
		return
	}

	// 1. Dual-Axis Axis 2 Check: Account Lockout (Master Rule 4.5, ASVS V2.2.1)
	if locked, retryAfter := c.IsAccountLocked(r.Context(), req.Email); locked {
		retrySec := int(retryAfter.Seconds())
		if retrySec < 1 {
			retrySec = 1
		}
		w.Header().Set("Retry-After", fmt.Sprintf("%d", retrySec))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write(fmt.Appendf(nil, `{"error":"account temporarily locked due to multiple failed login attempts, please retry after %d seconds"}`, retrySec))
		return
	}

	// 2. Dual-Axis Account-Targeted Rapid Attempt Limiter (prevents distributed proxy blasting against single email)
	if c.accountLimiter != nil {
		res, err := c.accountLimiter.Allow(r.Context(), "account:"+hashEmailKey(req.Email), 1)
		if err == nil && !res.Allowed {
			retrySec := int(res.RetryAfter.Seconds())
			if retrySec < 1 {
				retrySec = 1
			}
			w.Header().Set("Retry-After", fmt.Sprintf("%d", retrySec))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write(fmt.Appendf(nil, `{"error":"too many login attempts for this account, please retry after %d seconds"}`, retrySec))
			return
		}
	}

	// Fail closed if database repository is uninitialized (Master Rule 5.7)
	if c.repo == nil {
		http.Error(w, `{"error":"database service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	// Live database authentication with Argon2id/Bcrypt password verification (Master Rule 2.1 & 5.1)
	user, err := c.repo.GetUserByEmail(r.Context(), req.Email)
	validPassword := false
	if err == nil && user != nil {
		validPassword = auth.VerifyPassword(req.Password, user.Password)
	} else {
		// Anti-enumeration: execute authentic dummy verification with identical CPU/memory cost
		// to eliminate timing side-channels for account enumeration (CWE-208 / ASVS V2.2)
		_ = auth.DummyVerify(req.Password)
	}

	if err != nil || user == nil || !validPassword {
		clientIP := scandrixMiddleware.ExtractClientIP(r)
		emailHash := hashEmailKey(req.Email)
		locked, retryAfter := c.RecordFailedLogin(r.Context(), req.Email)
		if locked {
			slog.Warn("auth.login.failed",
				"event", "auth.login.failed",
				"client_ip", clientIP,
				"email_hash", emailHash,
				"reason", "account_locked",
			)
			telemetry.GetMetrics().AuthAttemptsTotal.WithLabelValues("failure", "account_locked").Inc()
			retrySec := int(retryAfter.Seconds())
			w.Header().Set("Retry-After", fmt.Sprintf("%d", retrySec))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write(fmt.Appendf(nil, `{"error":"account temporarily locked due to multiple failed login attempts, please retry after %d seconds"}`, retrySec))
			return
		}
		slog.Warn("auth.login.failed",
			"event", "auth.login.failed",
			"client_ip", clientIP,
			"email_hash", emailHash,
			"reason", "invalid_credentials",
		)
		telemetry.GetMetrics().AuthAttemptsTotal.WithLabelValues("failure", "invalid_credentials").Inc()
		http.Error(w, `{"error":"invalid email or password"}`, http.StatusUnauthorized)
		return
	}

	// Successful password verification: clear failed login counter immediately
	c.ClearFailedLogins(r.Context(), req.Email)

	// Transparent on-login work factor upgrade if stored hash was computed with an older cost factor
	if auth.NeedsRehash(user.Password) {
		if updatedHash, err := auth.HashPassword(req.Password); err == nil {
			_ = c.repo.UpdateUserPassword(r.Context(), user.Email, updatedHash)
		}
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

	accessToken, refreshToken, err := c.authService.GenerateTokenPairWithEmail(userID, wsID, userRole, req.Email)
	if err != nil {
		http.Error(w, `{"error":"failed generating authentication tokens"}`, http.StatusInternalServerError)
		return
	}

	clientIP := scandrixMiddleware.ExtractClientIP(r)
	emailHash := hashEmailKey(req.Email)
	slog.Info("auth.login.success",
		"event", "auth.login.success",
		"client_ip", clientIP,
		"email_hash", emailHash,
		"user_id", userID,
		"workspace_id", wsID,
	)
	telemetry.GetMetrics().AuthAttemptsTotal.WithLabelValues("success", "credentials").Inc()

	// Persist refresh token in database (auth table)
	if c.repo != nil {
		_ = c.repo.CreateRefreshToken(r.Context(), userID, refreshToken, time.Now().Add(30*24*time.Hour))
	}

	// Set hardened session cookie for browser logins
	setAuthCookie(w, r, "scandrix_token", accessToken, 30*86400)

	expiresIn := int64(900)
	if c.authService != nil {
		expiresIn = c.authService.AccessTokenExpiresIn()
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"statusCode":   http.StatusOK,
		"accessToken":  accessToken,
		"refreshToken": refreshToken,
		"tokenType":    "Bearer",
		"expiresIn":    expiresIn,
		"user": models.AccountProfile{
			ID:          userID,
			WorkspaceID: wsID,
			Email:       req.Email,
			DisplayName: displayName,
			Role:        userRole,
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		},
		"data": map[string]any{
			"accessToken":  accessToken,
			"refreshToken": refreshToken,
			"tokenType":    "Bearer",
			"expiresIn":    expiresIn,
			"user": map[string]any{
				"id":             userID,
				"organizationId": wsID,
				"email":          req.Email,
				"name":           displayName,
				"role":           userRole,
			},
		},
	})
}

func (c *AuthController) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req dtos.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" || req.Password == "" {
		http.Error(w, `{"error":"email, password, and workspace name are required"}`, http.StatusBadRequest)
		return
	}

	// 1. Anti-bot honeypot check (Master Rule 4.5): Automated scripts filling invisible fields are dropped
	if strings.TrimSpace(req.WebsiteURL) != "" {
		http.Error(w, `{"error":"registration request could not be processed"}`, http.StatusBadRequest)
		return
	}

	if len(req.Password) < 8 {
		http.Error(w, `{"error":"password must be at least 8 characters long"}`, http.StatusBadRequest)
		return
	}
	if len(req.Password) > auth.MaxBcryptPasswordLength {
		http.Error(w, `{"error":"password exceeds maximum allowed length of 72 bytes"}`, http.StatusBadRequest)
		return
	}

	// 2. Email syntax & disposable email domain check: Reject throwaway/temporary inboxes and sanitize canonical address
	canonicalEmail, err := auth.ValidateRegistrationEmail(req.Email, c.blockedEmailDomains)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}
	req.Email = canonicalEmail

	// 3. Optional Cloudflare Turnstile bot verification
	if c.turnstileSecretKey != "" {
		clientIP := scandrixMiddleware.ExtractClientIP(r)
		if err := verifyTurnstileToken(r.Context(), c.turnstileSecretKey, req.TurnstileToken, clientIP); err != nil {
			http.Error(w, `{"error":"bot verification challenge failed"}`, http.StatusBadRequest)
			return
		}
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

	// 4. Email Verification Gating: When enabled, user status starts pending and no JWTs are issued until email is clicked
	if c.requireEmailVerification {
		_ = c.repo.UpdateUserStatus(r.Context(), userID, "pending_verification")

		if c.jwtSecret != "" {
			token, err := auth.CreateEmailConfirmationToken(userID, req.Email, c.jwtSecret, 24*time.Hour)
			if err == nil && c.mailer != nil {
				confirmURL := fmt.Sprintf("%s/confirm-email?token=%s", c.appBaseURL, token)
				_ = c.mailer.SendEmailConfirmation(r.Context(), req.Email, confirmURL)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"statusCode":                http.StatusCreated,
			"message":                   "Account created successfully. Please check your email to activate your account.",
			"requiresEmailVerification": true,
			"user": map[string]any{
				"id":             userID,
				"organizationId": wsID,
				"email":          req.Email,
				"name":           displayName,
				"role":           userRole,
				"status":         "pending_verification",
			},
		})
		return
	}

	accessToken, refreshToken, err := c.authService.GenerateTokenPairWithEmail(userID, wsID, userRole, req.Email)
	if err != nil {
		http.Error(w, `{"error":"failed generating tokens"}`, http.StatusInternalServerError)
		return
	}

	if c.repo != nil {
		_ = c.repo.CreateRefreshToken(r.Context(), userID, refreshToken, time.Now().Add(30*24*time.Hour))
	}

	expiresIn := int64(900)
	if c.authService != nil {
		expiresIn = c.authService.AccessTokenExpiresIn()
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"statusCode":   http.StatusCreated,
		"accessToken":  accessToken,
		"refreshToken": refreshToken,
		"tokenType":    "Bearer",
		"expiresIn":    expiresIn,
		"user": models.AccountProfile{
			ID:          userID,
			WorkspaceID: wsID,
			Email:       req.Email,
			DisplayName: req.DisplayName,
			Role:        userRole,
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		},
		"data": map[string]any{
			"statusCode":   http.StatusCreated,
			"accessToken":  accessToken,
			"refreshToken": refreshToken,
			"tokenType":    "Bearer",
			"expiresIn":    expiresIn,
			"user": map[string]any{
				"id":             userID,
				"organizationId": wsID,
				"email":          req.Email,
				"name":           req.DisplayName,
				"role":           userRole,
			},
		},
	})
}

// handleRefreshToken implements secure one-time refresh token rotation.
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
	if err != nil {
		http.Error(w, `{"error":"invalid or expired refresh token"}`, http.StatusUnauthorized)
		return
	}

	// Token Theft Detection: Replay of an already-used refresh token indicates compromised credentials (RFC 6819, Master Rule 5.1).
	// Immediately revoke the entire refresh token family for this user to contain the breach.
	if record.Used {
		_ = c.repo.InvalidateAllUserRefreshTokens(r.Context(), record.UserUUID)
		slog.Warn("Security alert: refresh token reuse detected; all active sessions revoked for user",
			"user_id", record.UserUUID,
		)
		http.Error(w, `{"error":"security violation: token reuse detected; all active sessions revoked"}`, http.StatusUnauthorized)
		return
	}

	if time.Now().After(record.ExpiryDate) {
		http.Error(w, `{"error":"refresh token has expired"}`, http.StatusUnauthorized)
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

	expiresIn := int64(900)
	if c.authService != nil {
		expiresIn = c.authService.AccessTokenExpiresIn()
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"statusCode":   http.StatusOK,
		"accessToken":  newAccess,
		"refreshToken": newRefresh,
		"tokenType":    "Bearer",
		"expiresIn":    expiresIn,
		"user": models.AccountProfile{
			ID:          userID,
			WorkspaceID: wsID,
			Role:        userRole,
			UpdatedAt:   time.Now().UTC(),
		},
		"data": map[string]any{
			"accessToken":  newAccess,
			"refreshToken": newRefresh,
			"tokenType":    "Bearer",
			"expiresIn":    expiresIn,
			"user": map[string]any{
				"id":             userID,
				"organizationId": wsID,
				"role":           userRole,
			},
		},
	})
}

// handleLogout marks the provided refresh token as used/revoked and expires session cookies.
func (c *AuthController) handleLogout(w http.ResponseWriter, r *http.Request) {
	var req dtos.LogoutRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	if c.repo != nil && req.RefreshToken != "" {
		_ = c.repo.MarkRefreshTokenUsed(r.Context(), req.RefreshToken)
	}

	setAuthCookie(w, r, "scandrix_token", "", -1)
	setAuthCookie(w, r, "scandrix_cli_code", "", -1)

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

	if c.jwtSecret == "" {
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

	var userEmail string
	var jsonPayload struct {
		UserID    uuid.UUID `json:"uid"`
		Email     string    `json:"email"`
		ExpiresAt int64     `json:"exp"`
	}
	if err := json.Unmarshal(payloadBytes, &jsonPayload); err == nil && jsonPayload.Email != "" {
		userEmail = jsonPayload.Email
	} else if payloadParts := strings.SplitN(string(payloadBytes), ":", 3); len(payloadParts) == 3 {
		userEmail = payloadParts[1]
	} else {
		http.Error(w, `{"error":"invalid reset token"}`, http.StatusBadRequest)
		return
	}

	if c.repo == nil {
		http.Error(w, `{"error":"database service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

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

	if len(req.NewPassword) < 8 {
		http.Error(w, `{"error":"password must be at least 8 characters long"}`, http.StatusBadRequest)
		return
	}
	if len(req.NewPassword) > auth.MaxBcryptPasswordLength {
		http.Error(w, `{"error":"password exceeds maximum allowed length of 72 bytes"}`, http.StatusBadRequest)
		return
	}

	newHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		if errors.Is(err, auth.ErrPasswordTooLong) {
			http.Error(w, `{"error":"password exceeds maximum allowed length of 72 bytes"}`, http.StatusBadRequest)
			return
		}
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

// HandleCheckEmail checks if an email address is already registered in the system.
func (c *AuthController) HandleCheckEmail(w http.ResponseWriter, r *http.Request) {
	email := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("email")))
	exists := false
	if c.repo != nil && email != "" {
		if _, err := c.repo.GetUserByEmail(r.Context(), email); err == nil {
			exists = true
		}
	}
	w.Header().Set("Content-Type", "application/json")
	if exists {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"statusCode": http.StatusConflict,
			"message":    "An user with this e-mail already exists.",
			"code":       "DUPLICATE_USER_EMAIL",
			"error":      "Duplicate Record",
			"error_key":  "DUPLICATE_USER_EMAIL",
			"data":       true,
		})
		return
	}
	_ = json.NewEncoder(w).Encode(false)
}

// handleConfirmEmail verifies an email confirmation token and marks the account active.
func (c *AuthController) handleConfirmEmail(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
		http.Error(w, `{"error":"verification token is required"}`, http.StatusBadRequest)
		return
	}

	secret := c.jwtSecret
	if secret == "" {
		http.Error(w, `{"error":"email verification service unavailable: security key not configured"}`, http.StatusServiceUnavailable)
		return
	}

	claims, err := auth.VerifyEmailConfirmationToken(req.Token, secret)
	if err != nil {
		http.Error(w, `{"error":"invalid or expired verification token"}`, http.StatusUnauthorized)
		return
	}

	if c.repo != nil {
		user, err := c.repo.GetUserByEmail(r.Context(), claims.Email)
		if err != nil || user == nil {
			http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
			return
		}
		if user.Status == "active" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "Email already confirmed"})
			return
		}
		_ = c.repo.UpdateUserStatus(r.Context(), user.UUID, "active")
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Email confirmed successfully"})
}

// handleResendEmail generates and sends a new verification token for the requested email.
func (c *AuthController) handleResendEmail(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
		http.Error(w, `{"error":"email is required"}`, http.StatusBadRequest)
		return
	}

	email := strings.TrimSpace(strings.ToLower(req.Email))

	if c.repo != nil {
		user, err := c.repo.GetUserByEmail(r.Context(), email)
		if err != nil || user == nil {
			// Fail closed with constant message to prevent account enumeration
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "Verification email sent"})
			return
		}

		secret := c.jwtSecret
		if secret == "" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "Verification email sent"})
			return
		}

		token, err := auth.CreateEmailConfirmationToken(user.UUID, user.Email, secret, 24*time.Hour)
		if err == nil && c.mailer != nil {
			confirmURL := fmt.Sprintf("%s/confirm-email?token=%s", c.appBaseURL, token)
			_ = c.mailer.SendEmailConfirmation(r.Context(), user.Email, confirmURL)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Verification email sent"})
}

