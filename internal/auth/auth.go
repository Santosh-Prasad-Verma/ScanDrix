package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// ═══════════════════════════════════════════════════════════════
// 1. CONTEXT KEYS & CLAIMS MODEL (Tenancy context & JWT claims)
// ═══════════════════════════════════════════════════════════════

type ContextKey string

const (
	WorkspaceContextKey ContextKey = "scandrix.workspace_id"
	AccountContextKey   ContextKey = "scandrix.account_profile"
)

var (
	ErrInvalidToken = errors.New("invalid token format or signature")
	ErrTokenExpired = errors.New("token has expired")
)

// CLITokenVerifier validates CLI API keys against storage.
type CLITokenVerifier func(ctx context.Context, plaintext string) (workspaceID uuid.UUID, profile *models.AccountProfile, err error)

// TokenClaims represents decoded claims from a verified JWT.
type TokenClaims struct {
	UserID      uuid.UUID       `json:"sub"`
	WorkspaceID uuid.UUID       `json:"ws"`
	Role        models.UserRole `json:"role"`
	Email       string          `json:"email,omitempty"`
	IssuedAt    int64           `json:"iat"`
	ExpiresAt   int64           `json:"exp"`
	NotBefore   int64           `json:"nbf,omitempty"`
}

// WithAccountContext stores the account profile in request context.
func WithAccountContext(ctx context.Context, profile *models.AccountProfile) context.Context {
	return context.WithValue(ctx, AccountContextKey, profile)
}

// AccountProfileFromContext extracts the account profile from request context.
func AccountProfileFromContext(ctx context.Context) (*models.AccountProfile, bool) {
	val, ok := ctx.Value(AccountContextKey).(*models.AccountProfile)
	return val, ok
}

// CallerEmail returns the authenticated caller's email address.
//
// Use this to scope a lookup to the calling principal. Anything that picks a
// tenant on a caller's behalf must be scoped this way: an unscoped
// "list everything" query hands one user another tenant's workspace.
func CallerEmail(ctx context.Context) (string, error) {
	profile, ok := AccountProfileFromContext(ctx)
	if !ok || profile == nil {
		return "", errors.New("no authenticated caller in context")
	}
	email := strings.TrimSpace(profile.Email)
	if email == "" {
		return "", errors.New("authenticated caller has no email")
	}
	return email, nil
}

const (
	// DefaultAccessTokenTTL defines the recommended 15-minute lifetime for JWT access tokens (OWASP ASVS V3.2.1).
	DefaultAccessTokenTTL = 15 * time.Minute
)

// ═══════════════════════════════════════════════════════════════
// 2. AUTHENTICATOR INITIALIZATION & CONFIGURATION (JWT lifetime & verifiers)
// ═══════════════════════════════════════════════════════════════

// Authenticator verifies API tokens, CLI keys, and JWT sessions.
type Authenticator struct {
	jwtSecret         []byte
	cliVerifier       CLITokenVerifier
	revocationChecker RevocationChecker
	identityValidator IdentityValidator
	accessTokenTTL    time.Duration
}

// RevocationChecker verifies whether an access token has been revoked before expiration.
// Returns true if the token is revoked, false if valid.
type RevocationChecker func(ctx context.Context, userID uuid.UUID, issuedAt int64) bool

// IdentityValidator verifies and refreshes identity context against durable store,
// preventing stale privilege or deactivation bypass (AUDIT-ACCEPTANCE F02).
type IdentityValidator func(ctx context.Context, userID, workspaceID uuid.UUID) (*models.AccountProfile, error)

// NewAuthenticator initializes the auth service with default 15-minute access token TTL.
// If an empty secret is provided, it generates a cryptographically secure 32-byte ephemeral key
// rather than permitting insecure empty HMAC signing keys (Master Rule 1.1, 1.6 & 5.1).
func NewAuthenticator(jwtSecret string) *Authenticator {
	secretBytes := []byte(jwtSecret)
	if len(secretBytes) == 0 {
		ephemeral := make([]byte, 32)
		if _, err := rand.Read(ephemeral); err != nil {
			panic(fmt.Sprintf("auth: failed to generate secure ephemeral secret: %v", err))
		}
		secretBytes = ephemeral
		slog.Warn("Authenticator initialized with empty secret: generated secure ephemeral key (tokens will be invalidated upon restart). Configure JWT_SECRET in environment.")
	}
	return &Authenticator{
		jwtSecret:      secretBytes,
		accessTokenTTL: DefaultAccessTokenTTL,
	}
}

// SetAccessTokenTTL configures the access token lifetime.
func (a *Authenticator) SetAccessTokenTTL(ttl time.Duration) {
	if ttl > 0 {
		a.accessTokenTTL = ttl
	}
}

// AccessTokenTTL returns the configured access token lifetime.
func (a *Authenticator) AccessTokenTTL() time.Duration {
	if a == nil || a.accessTokenTTL <= 0 {
		return DefaultAccessTokenTTL
	}
	return a.accessTokenTTL
}

// AccessTokenExpiresIn returns the expiration in seconds (e.g. 900 for 15m).
func (a *Authenticator) AccessTokenExpiresIn() int64 {
	return int64(a.AccessTokenTTL().Seconds())
}

// SetCLIVerifier injects CLI token lifecycle management.
func (a *Authenticator) SetCLIVerifier(verifier CLITokenVerifier) {
	a.cliVerifier = verifier
}

// SetRevocationChecker injects a revocation verification hook.
func (a *Authenticator) SetRevocationChecker(checker RevocationChecker) {
	a.revocationChecker = checker
}

// SetIdentityValidator injects a validator to recheck persisted user/workspace status and role.
func (a *Authenticator) SetIdentityValidator(validator IdentityValidator) {
	a.identityValidator = validator
}

// ═══════════════════════════════════════════════════════════════
// 3. MULTI-TENANCY CONTEXT HELPERS (Tenant isolation & context propagation)
// ═══════════════════════════════════════════════════════════════

// WithWorkspaceContext stores the active workspace ID into the request context.
func WithWorkspaceContext(ctx context.Context, workspaceID uuid.UUID) context.Context {
	return context.WithValue(ctx, WorkspaceContextKey, workspaceID)
}

// WorkspaceFromContext retrieves the tenant ID, strictly enforcing tenancy isolation (Master Rule 4.3).
func WorkspaceFromContext(ctx context.Context) (uuid.UUID, error) {
	val := ctx.Value(WorkspaceContextKey)
	if val == nil {
		return uuid.Nil, errors.New("unauthenticated: no workspace context present")
	}
	id, ok := val.(uuid.UUID)
	if !ok || id == uuid.Nil {
		return uuid.Nil, errors.New("invalid workspace context")
	}
	return id, nil
}

// ═══════════════════════════════════════════════════════════════
// 4. CRYPTOGRAPHIC TOKEN VERIFICATION (HS256 validation & replay checks)
// ═══════════════════════════════════════════════════════════════

// VerifyToken decodes and validates an HMAC-SHA256 JWT, verifying cryptographic signature, header algorithm pinning, and expiration.
func (a *Authenticator) VerifyToken(tokenString string) (*TokenClaims, error) {
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidToken
	}

	headerB64 := parts[0]
	payloadB64 := parts[1]
	providedSigB64 := parts[2]

	// 1. Decode and pin JWT header algorithm to prevent algorithm confusion attacks (Master Rule 5.1)
	headerBytes, err := base64.RawURLEncoding.DecodeString(headerB64)
	if err != nil {
		return nil, ErrInvalidToken
	}
	var header struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, ErrInvalidToken
	}
	if header.Alg != "HS256" {
		return nil, fmt.Errorf("%w: unsupported signing algorithm '%s'", ErrInvalidToken, header.Alg)
	}

	// 2. Recompute HMAC-SHA256 signature using server secret
	mac := hmac.New(sha256.New, a.jwtSecret)
	mac.Write([]byte(headerB64 + "." + payloadB64))
	expectedSig := mac.Sum(nil)
	expectedSigB64 := base64.RawURLEncoding.EncodeToString(expectedSig)

	// 3. Constant-time signature comparison to eliminate side-channel timing attacks (Master Rule 5.1)
	if subtle.ConstantTimeCompare([]byte(providedSigB64), []byte(expectedSigB64)) != 1 {
		return nil, ErrInvalidToken
	}

	// 4. Decode claims payload
	payloadBytes, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return nil, ErrInvalidToken
	}

	var claims TokenClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, ErrInvalidToken
	}

	// 5. Enforce claims validation (Master Rule 5.1 & Token Security Audits)
	now := time.Now().Unix()

	// Expiration claim is strictly mandatory
	if claims.ExpiresAt <= 0 || now >= claims.ExpiresAt {
		return nil, ErrTokenExpired
	}

	// IssuedAt claim is strictly mandatory and cannot be in future
	if claims.IssuedAt <= 0 || claims.IssuedAt > now {
		return nil, ErrInvalidToken
	}

	// NotBefore claim if present cannot be in the future
	if claims.NotBefore > 0 && claims.NotBefore > now {
		return nil, ErrInvalidToken
	}

	// Subject (user ID) cannot be nil
	if claims.UserID == uuid.Nil {
		return nil, ErrInvalidToken
	}

	// Workspace ID cannot be nil
	if claims.WorkspaceID == uuid.Nil {
		return nil, ErrInvalidToken
	}

	// Role must be an authorized enum value
	switch strings.ToUpper(string(claims.Role)) {
	case string(models.RoleOwner), string(models.RoleAdmin), string(models.RoleMember), string(models.RoleViewer):
		// valid
	default:
		return nil, ErrInvalidToken
	}

	return &claims, nil
}

// ═══════════════════════════════════════════════════════════════
// 5. AUTHENTICATION MIDDLEWARE & EXTRACTION (Bearer, CLI keys & query tokens)
// ═══════════════════════════════════════════════════════════════

// Middleware creates an HTTP handler that enforces valid token or API key credentials.
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractAuthToken(r)

		if token == "" {
			http.Error(w, `{"error":"unauthorized: missing authorization header"}`, http.StatusUnauthorized)
			return
		}

		// Handle CLI team tokens (scandrix_ prefix)
		if strings.HasPrefix(token, "scandrix_") {
			if a.cliVerifier != nil {
				wsID, profile, err := a.cliVerifier(r.Context(), token)
				if err != nil {
					slog.Error("CLI token verification failed", "error", err)
					http.Error(w, `{"error":"unauthorized: invalid CLI token"}`, http.StatusUnauthorized)
					return
				}
				ctx := WithWorkspaceContext(r.Context(), wsID)
				if profile != nil {
					ctx = WithAccountContext(ctx, profile)
				}
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// CLI verifier not configured — reject the token rather than
			// silently granting access. Fail loudly so operators notice.
			http.Error(w, `{"error":"unauthorized: CLI token authentication is not configured"}`, http.StatusUnauthorized)
			return
		}

		// Cryptographic JWT signature and expiration verification (Master Rule 5.1)
		claims, err := a.VerifyToken(token)
		if err != nil {
			slog.Debug("JWT verification failed", "error", err)
			http.Error(w, `{"error":"unauthorized: invalid or expired token"}`, http.StatusUnauthorized)
			return
		}

		// Verify against revocation store (RFC 6749 / ASVS V3.5.3)
		if a.revocationChecker != nil && a.revocationChecker(r.Context(), claims.UserID, claims.IssuedAt) {
			slog.Warn("Rejected revoked token", "user_id", claims.UserID)
			http.Error(w, `{"error":"unauthorized: session has been revoked"}`, http.StatusUnauthorized)
			return
		}

		profile := &models.AccountProfile{
			ID:          claims.UserID,
			WorkspaceID: claims.WorkspaceID,
			Email:       claims.Email,
			Role:        claims.Role,
		}
		if a.identityValidator != nil {
			liveProfile, err := a.identityValidator(r.Context(), claims.UserID, claims.WorkspaceID)
			if err != nil {
				slog.Warn("Rejected inactive or modified identity", "user_id", claims.UserID, "error", err)
				http.Error(w, `{"error":"unauthorized: account or workspace is inactive or suspended"}`, http.StatusUnauthorized)
				return
			}
			if liveProfile != nil {
				profile = liveProfile
			}
		}

		ctx := WithWorkspaceContext(r.Context(), profile.WorkspaceID)
		ctx = WithAccountContext(ctx, profile)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func extractAuthToken(r *http.Request) string {
	// 1. Authorization: Bearer <token>
	if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	} else if strings.HasPrefix(authHeader, "Bearer") {
		return strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer"))
	}

	// 2. X-Team-Key header (ScanDrix CLI standard)
	if k := r.Header.Get("X-Team-Key"); k != "" {
		return strings.TrimSpace(k)
	}

	// 3. X-API-Key header
	if k := r.Header.Get("X-API-Key"); k != "" {
		return strings.TrimSpace(k)
	}

	// 4. X-Workspace-Key header
	if k := r.Header.Get("X-Workspace-Key"); k != "" {
		return strings.TrimSpace(k)
	}

	// 5. Query parameter token (e.g. for SSE streams or WebSockets)
	if q := r.URL.Query().Get("token"); q != "" {
		return strings.TrimSpace(q)
	}

	return ""
}

// HashAPIKey generates a SHA-256 hash of an API key for safe database storage.
func HashAPIKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// ConstantTimeCompare prevents timing attacks when checking tokens.
func ConstantTimeCompare(provided, expected string) bool {
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

// ═══════════════════════════════════════════════════════════════
// 6. ROLE ACCESS CONTROL & POLICY GUARDS (Least privilege enforcement)
// ═══════════════════════════════════════════════════════════════

// roleLevel returns numeric rank for hierarchical RBAC comparison.
func roleLevel(r models.UserRole) int {
	switch strings.ToUpper(strings.TrimSpace(string(r))) {
	case string(models.RoleOwner):
		return 4
	case string(models.RoleAdmin):
		return 3
	case string(models.RoleMember):
		return 2
	case string(models.RoleViewer):
		return 1
	default:
		return 0
	}
}

// RoleGuard ensures the authenticated user possesses at least the required authorization level.
func RoleGuard(requiredRole models.UserRole, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		profile, ok := r.Context().Value(AccountContextKey).(*models.AccountProfile)
		if !ok || profile == nil {
			// Always deny when no profile is present — a workspace-only
			// context without a profile has no role to check against.
			http.Error(w, `{"error":"forbidden: insufficient access"}`, http.StatusForbidden)
			return
		}

		if roleLevel(profile.Role) < roleLevel(requiredRole) {
			http.Error(w, `{"error":"forbidden: role does not satisfy policy"}`, http.StatusForbidden)
			return
		}

		next(w, r)
	}
}

// ═══════════════════════════════════════════════════════════════
// 7. SECURE TOKEN ISSUANCE & KEY GENERATION (Access/refresh tokens & API keys)
// ═══════════════════════════════════════════════════════════════

// GenerateToken issues a signed access token containing tenant and user claims (expires in 24h).
func (a *Authenticator) GenerateToken(userID, wsID uuid.UUID, role models.UserRole) (string, error) {
	return a.GenerateTokenWithEmail(userID, wsID, role, "")
}

// GenerateTokenWithEmail issues a signed access token embedding the user's verified email.
func (a *Authenticator) GenerateTokenWithEmail(userID, wsID uuid.UUID, role models.UserRole, email string) (string, error) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	ttl := a.AccessTokenTTL()
	now := time.Now().UTC()

	claims := TokenClaims{
		UserID:      userID,
		WorkspaceID: wsID,
		Role:        role,
		Email:       email,
		IssuedAt:    now.Unix(),
		ExpiresAt:   now.Add(ttl).Unix(),
	}

	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("failed marshaling jwt claims: %w", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)

	mac := hmac.New(sha256.New, a.jwtSecret)
	mac.Write([]byte(header + "." + payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return header + "." + payload + "." + sig, nil
}

// GenerateTokenPair issues a short-lived access token along with a cryptographically secure random refresh token.
func (a *Authenticator) GenerateTokenPair(userID, wsID uuid.UUID, role models.UserRole) (accessToken, refreshToken string, err error) {
	return a.GenerateTokenPairWithEmail(userID, wsID, role, "")
}

// GenerateTokenPairWithEmail issues an access token with email claim and a refresh token.
func (a *Authenticator) GenerateTokenPairWithEmail(userID, wsID uuid.UUID, role models.UserRole, email string) (accessToken, refreshToken string, err error) {
	accessToken, err = a.GenerateTokenWithEmail(userID, wsID, role, email)
	if err != nil {
		return "", "", err
	}

	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", "", fmt.Errorf("failed generating refresh token: %w", err)
	}
	refreshToken = hex.EncodeToString(randomBytes)

	return accessToken, refreshToken, nil
}

// GenerateAPIKey creates a cryptographically secure random CLI API key with the scandrix_ prefix.
func GenerateAPIKey() (plainKey, hashedKey string, err error) {
	bytes := make([]byte, 24)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", err
	}
	plainKey = "scandrix_" + hex.EncodeToString(bytes)
	hashedKey = HashAPIKey(plainKey)
	return plainKey, hashedKey, nil
}
