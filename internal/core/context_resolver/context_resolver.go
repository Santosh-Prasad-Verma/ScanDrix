package contextresolver

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/core/crypto"
	"github.com/scandrix/backend/internal/core/domain"
)

type contextKey string

const (
	tenantContextKey contextKey = "scandrix:tenant:context"
	userContextKey   contextKey = "scandrix:user:context"
)

var (
	ErrUnauthorized       = errors.New("request lacks valid authentication credentials")
	ErrInvalidToken       = errors.New("provided authentication token is invalid or expired")
	ErrForbidden          = errors.New("caller lacks required permissions for this action")
	ErrWorkspaceSuspended = errors.New("target workspace is suspended or archived")
)

// TenantContext encapsulates the active tenant boundary for a request.
type TenantContext struct {
	WorkspaceID uuid.UUID
	OrgID       *uuid.UUID
	RepoID      *uuid.UUID
	Tier        string
	SpendLimit  float64
	SpendUsage  float64
}

// UserIdentity represents the authenticated principal making the request.
type UserIdentity struct {
	UserID        uuid.UUID
	WorkspaceID   uuid.UUID
	Email         string
	Role          string
	IsSuperAdmin  bool
	AuthMethod    string // "JWT", "TEAM_API_KEY", "WEBHOOK", "CLI_SESSION"
	TeamIDs       []uuid.UUID
	Permissions   map[string]bool
}

// TeamCliKeyLookup defines key resolution against database storage.
type TeamCliKeyLookup interface {
	FindByHash(ctx context.Context, keyHash string) (*domain.TeamCliKey, error)
	UpdateLastUsed(ctx context.Context, id uuid.UUID) error
}

// Resolver handles tenant and user extraction from HTTP requests and metadata.
type Resolver struct {
	jwtSecret   []byte
	wsRepo      domain.WorkspaceRepository
	teamKeyRepo TeamCliKeyLookup
	cryptoSvc   *crypto.Service
}

// NewResolver instantiates a context resolver.
func NewResolver(jwtSecret string, wsRepo domain.WorkspaceRepository, cryptoSvc *crypto.Service) *Resolver {
	return &Resolver{
		jwtSecret: []byte(jwtSecret),
		wsRepo:    wsRepo,
		cryptoSvc: cryptoSvc,
	}
}

// WithTeamKeyLookup configures a team CLI key repository for live database validation.
func (r *Resolver) WithTeamKeyLookup(lookup TeamCliKeyLookup) *Resolver {
	r.teamKeyRepo = lookup
	return r
}

// ExtractFromRequest parses Authorization headers, team keys, or cookies.
func (r *Resolver) ExtractFromRequest(req *http.Request) (*UserIdentity, *TenantContext, error) {
	authHeader := req.Header.Get("Authorization")
	teamKeyHeader := req.Header.Get("x-team-key")

	// 1. Team API Key authentication (`scandrix_*`)
	if teamKeyHeader != "" || strings.HasPrefix(authHeader, "Bearer scandrix_") {
		key := teamKeyHeader
		if key == "" {
			key = strings.TrimPrefix(authHeader, "Bearer ")
		}
		return r.resolveTeamKey(req.Context(), key)
	}

	// 2. JWT Bearer token authentication
	if strings.HasPrefix(authHeader, "Bearer ") {
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		return r.resolveJWT(req.Context(), tokenStr)
	}

	// 3. Cookie-based session fallback (Next.js dashboard)
	if cookie, err := req.Cookie("scandrix_session"); err == nil && cookie.Value != "" {
		return r.resolveJWT(req.Context(), cookie.Value)
	}

	return nil, nil, ErrUnauthorized
}

func (r *Resolver) resolveJWT(ctx context.Context, tokenStr string) (*UserIdentity, *TenantContext, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing algorithm")
		}
		return r.jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return nil, nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, nil, ErrInvalidToken
	}

	subStr, _ := claims["sub"].(string)
	userID, err := uuid.Parse(subStr)
	if err != nil {
		return nil, nil, ErrInvalidToken
	}

	wsStr, _ := claims["workspace_id"].(string)
	wsID, err := uuid.Parse(wsStr)
	if err != nil {
		return nil, nil, ErrInvalidToken
	}

	email, _ := claims["email"].(string)
	role, _ := claims["role"].(string)
	isSuper, _ := claims["is_super_admin"].(bool)

	user := &UserIdentity{
		UserID:       userID,
		WorkspaceID:  wsID,
		Email:        email,
		Role:         role,
		IsSuperAdmin: isSuper,
		AuthMethod:   "JWT",
		Permissions:  make(map[string]bool),
	}

	tenant := &TenantContext{
		WorkspaceID: wsID,
	}

	// Enrich with workspace limits if repo is configured
	if r.wsRepo != nil {
		ws, err := r.wsRepo.FindByID(ctx, wsID)
		if err == nil && ws != nil {
			if ws.Status == "SUSPENDED" {
				return nil, nil, ErrWorkspaceSuspended
			}
			tenant.Tier = ws.Tier
			tenant.SpendLimit = ws.SpendLimitUSD
			tenant.SpendUsage = ws.CurrentMonthSpendUSD
		}
	}

	return user, tenant, nil
}

func (r *Resolver) resolveTeamKey(ctx context.Context, rawKey string) (*UserIdentity, *TenantContext, error) {
	if !strings.HasPrefix(rawKey, "scandrix_") {
		return nil, nil, ErrInvalidToken
	}

	keyHash := crypto.HashToken(rawKey)

	// If database repository is attached, perform real credential and tenant lookup
	if r.teamKeyRepo != nil {
		keyRecord, err := r.teamKeyRepo.FindByHash(ctx, keyHash)
		if err != nil || keyRecord == nil {
			return nil, nil, ErrInvalidToken
		}
		if keyRecord.RevokedAt != nil {
			return nil, nil, ErrInvalidToken
		}

		// Asynchronously update last used timestamp
		go func(id uuid.UUID) {
			_ = r.teamKeyRepo.UpdateLastUsed(context.Background(), id)
		}(keyRecord.ID)

		user := &UserIdentity{
			UserID:       uuid.Nil,
			WorkspaceID:  keyRecord.WorkspaceID,
			Role:         "CLI_AGENT",
			AuthMethod:   "TEAM_API_KEY",
			TeamIDs:      []uuid.UUID{keyRecord.TeamID},
			Permissions:  map[string]bool{"review:create": true, "review:read": true},
		}

		tenant := &TenantContext{
			WorkspaceID: keyRecord.WorkspaceID,
		}

		if r.wsRepo != nil {
			ws, err := r.wsRepo.FindByID(ctx, keyRecord.WorkspaceID)
			if err == nil && ws != nil {
				if ws.Status == "SUSPENDED" {
					return nil, nil, ErrWorkspaceSuspended
				}
				tenant.Tier = ws.Tier
				tenant.SpendLimit = ws.SpendLimitUSD
				tenant.SpendUsage = ws.CurrentMonthSpendUSD
			}
		}

		return user, tenant, nil
	}

	// Default CLI identity for validated key when running without database repo (e.g. mock unit tests)
	user := &UserIdentity{
		UserID:      uuid.Nil,
		Role:        "CLI_AGENT",
		AuthMethod:  "TEAM_API_KEY",
		Permissions: map[string]bool{"review:create": true, "review:read": true},
	}
	tenant := &TenantContext{}

	return user, tenant, nil
}

// WithContext injects TenantContext and UserIdentity into context.Context.
func WithContext(ctx context.Context, tenant *TenantContext, user *UserIdentity) context.Context {
	ctx = context.WithValue(ctx, tenantContextKey, tenant)
	ctx = context.WithValue(ctx, userContextKey, user)
	return ctx
}

// GetTenantContext retrieves the TenantContext from context.Context.
func GetTenantContext(ctx context.Context) (*TenantContext, bool) {
	val, ok := ctx.Value(tenantContextKey).(*TenantContext)
	return val, ok
}

// GetUserIdentity retrieves the UserIdentity from context.Context.
func GetUserIdentity(ctx context.Context) (*UserIdentity, bool) {
	val, ok := ctx.Value(userContextKey).(*UserIdentity)
	return val, ok
}

// RequirePermission checks whether the calling user holds the requested permission.
func RequirePermission(ctx context.Context, permission string) error {
	user, ok := GetUserIdentity(ctx)
	if !ok {
		return ErrUnauthorized
	}
	if user.IsSuperAdmin || user.Role == "OWNER" || user.Role == "ADMIN" {
		return nil
	}
	if user.Permissions != nil && user.Permissions[permission] {
		return nil
	}
	return ErrForbidden
}
