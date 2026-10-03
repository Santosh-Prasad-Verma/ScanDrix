package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/enterprise/license"
)

// EntitlementSource resolves the entitlement for a request. Implemented by
// *license.Resolver; declared here so the gate does not depend on the concrete
// resolver type.
type EntitlementSource interface {
	Resolve(ctx context.Context, wsID uuid.UUID) *license.Entitlement
}

// RequireFeature returns a middleware that denies the request with 403 unless
// the caller's workspace is entitled to the given feature.
//
// The check fails closed: a missing workspace context, a nil resolver or an
// unresolvable entitlement all deny. Authorization is enforced per request on
// the backend, so hiding a control in the dashboard is never the gate.
func RequireFeature(source EntitlementSource, flag license.FeatureFlag) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			wsID, err := auth.WorkspaceFromContext(r.Context())
			if err != nil {
				writeFeatureDenied(w, http.StatusUnauthorized,
					"unauthorized: missing workspace context")
				return
			}

			if source == nil {
				writeFeatureDenied(w, http.StatusForbidden,
					"forbidden: entitlement resolution is unavailable")
				return
			}

			ent := source.Resolve(r.Context(), wsID)
			if ent == nil || !ent.Allows(flag) {
				writeFeatureDenied(w, http.StatusForbidden, featureDeniedMessage(flag, ent))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func featureDeniedMessage(flag license.FeatureFlag, ent *license.Entitlement) string {
	if ent == nil {
		return "forbidden: entitlement resolution is unavailable"
	}
	if !ent.Valid {
		return fmt.Sprintf("forbidden: %s requires an active license", flag)
	}
	return fmt.Sprintf("forbidden: %s is not included in the %s plan", flag, ent.Tier)
}

func writeFeatureDenied(w http.ResponseWriter, code int, message string) {
	body, err := json.Marshal(map[string]string{"error": message})
	if err != nil {
		body = []byte(`{"error":"forbidden"}`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(body)
}
