package clitokens

import (
	"net/http"
	"strings"

	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/pkg/models"
)

// CLITokenMiddleware enforces token authentication on protected endpoints.
type CLITokenMiddleware struct {
	tokenService *TokenService
}

// NewCLITokenMiddleware creates the middleware wrapper.
func NewCLITokenMiddleware(service *TokenService) *CLITokenMiddleware {
	return &CLITokenMiddleware{tokenService: service}
}

// RequireScope returns an HTTP middleware handler verifying the requested token scope.
func (m *CLITokenMiddleware) RequireScope(scope TokenScope) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr := extractToken(r)
			if tokenStr == "" {
				http.Error(w, "missing team api key or authorization bearer token", http.StatusUnauthorized)
				return
			}

			record, err := m.tokenService.ValidateToken(r.Context(), tokenStr, scope)
			if err != nil {
				if err == ErrScopeMissing {
					http.Error(w, "insufficient scope for token: "+string(scope), http.StatusForbidden)
					return
				}
				http.Error(w, "invalid or expired token: "+err.Error(), http.StatusUnauthorized)
				return
			}

			// Construct authenticated profile
			profile := &models.AccountProfile{
				ID:          record.CreatedBy,
				WorkspaceID: record.WorkspaceID,
				Email:       record.Name + "@cli.service",
				DisplayName: record.Name,
				Role:        models.RoleMember,
			}

			// Injected into request context
			ctx := auth.WithWorkspaceContext(r.Context(), record.WorkspaceID)
			ctx = auth.WithAccountContext(ctx, profile)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractToken(r *http.Request) string {
	// 1. Check x-team-key
	if k := r.Header.Get("X-Team-Key"); k != "" {
		return strings.TrimSpace(k)
	}

	// 2. Check x-api-key
	if k := r.Header.Get("X-API-Key"); k != "" {
		return strings.TrimSpace(k)
	}

	// 3. Check Authorization: Bearer <token>
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		return strings.TrimSpace(token)
	}

	return ""
}
