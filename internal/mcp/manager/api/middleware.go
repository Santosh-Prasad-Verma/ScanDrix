// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/scandrix/backend/internal/mcp/manager/models"
)

type contextKey string

const (
	OrgIDContextKey contextKey = "mcp_organization_id"
)

// GetOrgIDFromContext extracts verified organizationId from request context.
func GetOrgIDFromContext(ctx context.Context) string {
	if val, ok := ctx.Value(OrgIDContextKey).(string); ok {
		return val
	}
	return ""
}

// ContextWithOrgID attaches an organizationId to request context.
func ContextWithOrgID(ctx context.Context, orgID string) context.Context {
	return context.WithValue(ctx, OrgIDContextKey, orgID)
}

// AuthGuard validates JWT Bearer tokens and injects verified organizationId.
func AuthGuard(jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeError(w, http.StatusUnauthorized, "No token provided")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				writeError(w, http.StatusUnauthorized, "Invalid authorization header format")
				return
			}

			tokenStr := parts[1]
			token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
				return []byte(jwtSecret), nil
			})

			if err != nil || !token.Valid {
				writeError(w, http.StatusUnauthorized, "Invalid or expired token")
				return
			}

			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok {
				writeError(w, http.StatusUnauthorized, "Invalid token claims")
				return
			}

			orgID, _ := claims["organizationId"].(string)
			if orgID == "" {
				// Fallback to "org_id" or "orgId"
				if alt, ok := claims["org_id"].(string); ok {
					orgID = alt
				} else if alt, ok := claims["orgId"].(string); ok {
					orgID = alt
				}
			}

			if orgID == "" {
				writeError(w, http.StatusForbidden, "Token missing organizationId claim")
				return
			}

			ctx := ContextWithOrgID(r.Context(), orgID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// DocsBasicAuth guards documentation with HTTP Basic Authentication.
func DocsBasicAuth(user, pass string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if user == "" || pass == "" {
				next.ServeHTTP(w, r)
				return
			}

			reqUser, reqPass, ok := r.BasicAuth()
			if !ok || subtle.ConstantTimeCompare([]byte(reqUser), []byte(user)) != 1 ||
				subtle.ConstantTimeCompare([]byte(reqPass), []byte(pass)) != 1 {
				w.Header().Set("WWW-Authenticate", `Basic realm="docs"`)
				writeError(w, http.StatusUnauthorized, "Unauthorized")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func writeError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(models.ErrorResponseDTO{
		StatusCode: statusCode,
		Message:    message,
	})
}
