package rbac

import (
	"fmt"
	"net/http"

	"github.com/scandrix/backend/internal/auth"
)

// RequirePolicy creates a Chi HTTP middleware enforcing Kodus CASL Action + Resource authorization.
func RequirePolicy(engine *PolicyEngine, action Action, resource Resource) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			profile, ok := auth.AccountProfileFromContext(r.Context())
			if !ok || profile == nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"unauthorized: missing authenticated user context"}`))
				return
			}

			if !engine.Can(profile.Role, action, resource) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				errMsg := fmt.Sprintf(`{"error":"forbidden: role '%s' lacks '%s' permission on '%s'"}`, profile.Role, action, resource)
				_, _ = w.Write([]byte(errMsg))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequirePermission creates an HTTP middleware enforcing legacy discrete permissions.
func RequirePermission(engine *PolicyEngine, perm Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			profile, ok := auth.AccountProfileFromContext(r.Context())
			if !ok || profile == nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"unauthorized: missing authenticated user context"}`))
				return
			}

			if err := engine.CheckPermission(profile.Role, perm); err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				errMsg := fmt.Sprintf(`{"error":"forbidden: %s"}`, err.Error())
				_, _ = w.Write([]byte(errMsg))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
