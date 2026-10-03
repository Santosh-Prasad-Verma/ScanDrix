// ═══════════════════════════════════════════════════════════════════════════
// ScanDrix AI - SCIM provisioning token administration
// ═══════════════════════════════════════════════════════════════════════════

package controllers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/enterprise/scim"
)

// SCIMTokenController issues and revokes the per-workspace SCIM provisioning
// token that identifies a tenant to the SCIM endpoint.
//
// It is separate from the SCIM service's own routes because those authenticate
// with the provisioning token itself and carry no session. This surface is
// session-authenticated, workspace-scoped, and mounted behind the same
// entitlement gate as the rest of the SSO/SCIM settings.
type SCIMTokenController struct {
	svc *scim.SCIMService
}

// NewSCIMTokenController constructs the controller. A nil service disables the
// routes rather than serving fabricated state.
func NewSCIMTokenController(svc *scim.SCIMService) *SCIMTokenController {
	return &SCIMTokenController{svc: svc}
}

// Routes mounts the /scim-config endpoints.
func (c *SCIMTokenController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/token", c.handleGetTokenStatus)
	r.Post("/token", c.handleIssueToken)
	r.Delete("/token", c.handleRevokeToken)

	return r
}

func (c *SCIMTokenController) unavailable(w http.ResponseWriter) bool {
	if c.svc == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": "SCIM provisioning is not configured on this deployment",
		})
		return true
	}
	return false
}

// handleGetTokenStatus reports whether provisioning is enabled for the caller's
// workspace. It never returns the token itself: only the hash is stored, so
// there is nothing to return.
func (c *SCIMTokenController) handleGetTokenStatus(w http.ResponseWriter, r *http.Request) {
	if c.unavailable(w) {
		return
	}
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		writeSCIMTokenError(w, http.StatusUnauthorized, "unauthorized: missing workspace context")
		return
	}

	enabled, err := c.svc.TokenState(r.Context(), wsID)
	if err != nil {
		writeSCIMTokenError(w, http.StatusInternalServerError, "failed reading the SCIM token state")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"enabled":   enabled,
		"workspace": wsID.String(),
		// Stated plainly so the UI does not offer a "reveal" affordance for a
		// value that cannot be revealed.
		"token_recoverable": false,
	})
}

// handleIssueToken mints a token and returns the plaintext exactly once. An
// existing token is replaced, which is the rotation path.
func (c *SCIMTokenController) handleIssueToken(w http.ResponseWriter, r *http.Request) {
	if c.unavailable(w) {
		return
	}
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		writeSCIMTokenError(w, http.StatusUnauthorized, "unauthorized: missing workspace context")
		return
	}

	token, prefix, err := c.svc.IssueToken(r.Context(), wsID)
	if err != nil {
		writeSCIMTokenError(w, http.StatusInternalServerError, "failed issuing a SCIM provisioning token")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"token":  token,
		"prefix": prefix,
		// Repeated here so a client that stores the response knows the value is
		// not retrievable later.
		"token_recoverable": false,
	})
}

// handleRevokeToken disables SCIM provisioning for the caller's workspace.
func (c *SCIMTokenController) handleRevokeToken(w http.ResponseWriter, r *http.Request) {
	if c.unavailable(w) {
		return
	}
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		writeSCIMTokenError(w, http.StatusUnauthorized, "unauthorized: missing workspace context")
		return
	}

	if err := c.svc.RevokeToken(r.Context(), wsID); err != nil {
		if errors.Is(err, scim.ErrSCIMWorkspaceRequired) {
			writeSCIMTokenError(w, http.StatusBadRequest, "workspace is required")
			return
		}
		writeSCIMTokenError(w, http.StatusInternalServerError, "failed revoking the SCIM provisioning token")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"enabled": false})
}

func writeSCIMTokenError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": message})
}
