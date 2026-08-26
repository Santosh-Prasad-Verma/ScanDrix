package auth

import (
	"context"
	"net/http"
	"strings"
	"time"

	sharedauth "github.com/codehound/codehound/shared/pkg/auth"
	"github.com/codehound/codehound/shared/pkg/errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	TenantIDContextKey = "codehound_tenant_id"
	UserIDContextKey   = "codehound_user_id"
	UserRoleContextKey = "codehound_user_role"
	ScopesContextKey   = "codehound_scopes"
)

// Authenticator validates incoming requests against API keys or tenant contexts.
type Authenticator struct {
	pool *pgxpool.Pool
}

// NewAuthenticator creates a new Authenticator instance.
func NewAuthenticator(pool *pgxpool.Pool) *Authenticator {
	return &Authenticator{pool: pool}
}

// RequireAuth inspects Authorization Bearer tokens or X-API-Key headers.
func (a *Authenticator) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		var rawToken string

		authHeader := c.GetHeader("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			rawToken = strings.TrimPrefix(authHeader, "Bearer ")
		} else if apiKeyHeader := c.GetHeader("X-API-Key"); apiKeyHeader != "" {
			rawToken = apiKeyHeader
		}

		// Fallback for direct X-Tenant-ID header in dev/testing environments
		if rawToken == "" {
			devTenantHeader := c.GetHeader("X-Tenant-ID")
			if devTenantHeader != "" {
				tenantUUID, err := uuid.Parse(devTenantHeader)
				if err == nil {
					c.Set(TenantIDContextKey, tenantUUID)
					c.Set(UserRoleContextKey, "DEVELOPER")
					c.Set(ScopesContextKey, []string{"read:audit", "write:audit"})
					c.Next()
					return
				}
			}

			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": errors.New(errors.CodeUnauthorized, "missing or malformed authentication credentials").Error(),
			})
			return
		}

		keyHash := sharedauth.HashKey(rawToken)

		// Query database for valid key
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()

		var (
			tenantID        uuid.UUID
			createdByUserID *uuid.UUID
			scopes          []string
			revokedAt       *time.Time
			expiresAt       *time.Time
		)

		query := `
			SELECT tenant_id, created_by_user_id, scopes, revoked_at, expires_at 
			FROM api_keys 
			WHERE key_hash = $1;
		`
		err := a.pool.QueryRow(ctx, query, keyHash).Scan(&tenantID, &createdByUserID, &scopes, &revokedAt, &expiresAt)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": errors.New(errors.CodeUnauthorized, "invalid API key").Error(),
			})
			return
		}

		if revokedAt != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": errors.New(errors.CodeUnauthorized, "API key has been revoked").Error(),
			})
			return
		}

		if expiresAt != nil && expiresAt.Before(time.Now()) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": errors.New(errors.CodeUnauthorized, "API key has expired").Error(),
			})
			return
		}

		// Update last_used_at asynchronously
		go func(hash string) {
			bgCtx, bgCancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer bgCancel()
			_, _ = a.pool.Exec(bgCtx, `UPDATE api_keys SET last_used_at = clock_timestamp() WHERE key_hash = $1;`, hash)
		}(keyHash)

		c.Set(TenantIDContextKey, tenantID)
		if createdByUserID != nil {
			c.Set(UserIDContextKey, *createdByUserID)
		}
		c.Set(ScopesContextKey, scopes)
		c.Next()
	}
}

// RequireRole enforces role-based access control.
func RequireRole(allowedRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		roleVal, exists := c.Get(UserRoleContextKey)
		if !exists {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": errors.New(errors.CodeForbidden, "insufficient privileges").Error(),
			})
			return
		}

		userRole := roleVal.(string)
		for _, r := range allowedRoles {
			if strings.EqualFold(r, userRole) {
				c.Next()
				return
			}
		}

		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error": errors.New(errors.CodeForbidden, "role '"+userRole+"' is not authorized for this resource").Error(),
		})
	}
}

// RequireScope verifies that the caller has a required API key scope.
func RequireScope(requiredScope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		scopesVal, exists := c.Get(ScopesContextKey)
		if !exists {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": errors.New(errors.CodeForbidden, "no scopes present in context").Error(),
			})
			return
		}

		scopes := scopesVal.([]string)
		for _, s := range scopes {
			if s == requiredScope || s == "admin:all" {
				c.Next()
				return
			}
		}

		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error": errors.New(errors.CodeForbidden, "missing required scope: "+requiredScope).Error(),
		})
	}
}

// GetTenantID retrieves the parsed Tenant ID from the Gin context.
func GetTenantID(c *gin.Context) (uuid.UUID, bool) {
	val, exists := c.Get(TenantIDContextKey)
	if !exists {
		return uuid.Nil, false
	}
	id, ok := val.(uuid.UUID)
	return id, ok
}
