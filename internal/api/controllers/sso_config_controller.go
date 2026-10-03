// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise SSO Configuration REST API Controller
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package controllers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/pkg/models"
)

// SSOConfigRepository is the persistence contract for per-workspace SSO
// configuration. Declared here so the controller depends on behaviour, not on
// the concrete database type.
type SSOConfigRepository interface {
	GetSSOConfig(ctx context.Context, wsID uuid.UUID) (*models.SSOConfig, error)
	UpsertSSOConfig(ctx context.Context, cfg models.SSOConfig) (models.SSOConfig, bool, error)
}

// SSOConfigController manages organization Single Sign-On configuration.
type SSOConfigController struct {
	authCtrl *AuthController
	repo     SSOConfigRepository
}

// NewSSOConfigController constructs the SSO configuration controller.
func NewSSOConfigController(authCtrl *AuthController, repo ...SSOConfigRepository) *SSOConfigController {
	var r SSOConfigRepository
	if len(repo) > 0 && !isNilInterface(repo[0]) {
		r = repo[0]
	}
	return &SSOConfigController{authCtrl: authCtrl, repo: r}
}

// Routes mounts the /sso-config endpoints.
func (c *SSOConfigController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", c.handleGetSSOConfig)
	r.Post("/", c.handleCreateOrUpdateSSOConfig)
	r.Post("/test-connection", c.handleTestConnection)
	r.Get("/test-connection/{sessionId}", c.handleGetTestResult)
	r.Post("/verify-domain", c.handleVerifyDomain)

	return r
}

// ssoConfigPayload is the request body for creating or updating a config.
type ssoConfigPayload struct {
	UUID           string         `json:"uuid,omitempty"`
	Protocol       string         `json:"protocol"`
	ProviderConfig map[string]any `json:"providerConfig"`
	Active         bool           `json:"active"`
	Domains        []string       `json:"domains"`
	SAMLRequired   bool           `json:"samlRequired"`
}

// handleGetSSOConfig returns the workspace's stored SSO configuration.
//
// A workspace with no configuration is not an error: the response is an
// explicit empty state so the dashboard can render the setup form instead of
// treating absence as a failure. The IdP client secret is never included.
func (c *SSOConfigController) handleGetSSOConfig(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		writeSSOError(w, http.StatusUnauthorized, "unauthorized: missing workspace context")
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if c.repo == nil {
		// No persistence configured: report the empty state rather than
		// fabricating a configuration.
		_ = json.NewEncoder(w).Encode(emptySSOConfigResponse(wsID))
		return
	}

	cfg, err := c.repo.GetSSOConfig(r.Context(), wsID)
	if err != nil {
		slog.Error("SSO configuration read failed", "workspace_id", wsID, "error", err)
		writeSSOError(w, http.StatusInternalServerError, "failed to read sso configuration")
		return
	}
	if cfg == nil {
		_ = json.NewEncoder(w).Encode(emptySSOConfigResponse(wsID))
		return
	}

	// Defence in depth: mask the secret even if a future repository forgets to.
	cfg.Provider = maskSSOSecret(cfg.Provider)

	_ = json.NewEncoder(w).Encode(map[string]any{
		"uuid":           cfg.ID,
		"organizationId": cfg.WorkspaceID,
		"protocol":       cfg.Protocol,
		"providerConfig": cfg.Provider,
		"active":         cfg.Active,
		"domains":        cfg.Domains,
		"samlRequired":   cfg.SAMLRequired,
		"verifiedAt":     cfg.VerifiedAt,
		"createdAt":      cfg.CreatedAt,
		"updatedAt":      cfg.UpdatedAt,
	})
}

// handleCreateOrUpdateSSOConfig validates and persists the SSO configuration.
func (c *SSOConfigController) handleCreateOrUpdateSSOConfig(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		writeSSOError(w, http.StatusUnauthorized, "unauthorized: missing workspace context")
		return
	}

	var payload ssoConfigPayload
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10))
	if err := decoder.Decode(&payload); err != nil {
		writeSSOError(w, http.StatusBadRequest, "invalid JSON request body")
		return
	}

	protocol := models.SSOProtocol(strings.ToUpper(strings.TrimSpace(payload.Protocol)))
	if protocol == "" {
		protocol = models.SSOProtocolSAML
	}
	if !protocol.Valid() {
		writeSSOError(w, http.StatusBadRequest, "protocol must be SAML or OIDC")
		return
	}

	provider, err := providerConfigFromPayload(payload.ProviderConfig)
	if err != nil {
		writeSSOError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateProviderConfig(protocol, payload.Active, provider); err != nil {
		writeSSOError(w, http.StatusBadRequest, err.Error())
		return
	}

	if c.repo == nil {
		writeSSOError(w, http.StatusServiceUnavailable,
			"sso configuration storage is not available on this deployment")
		return
	}

	// A partial update must not silently drop an existing field: start from what
	// is stored, then apply what was sent. In particular the OIDC client secret
	// is write-only, so a caller rotating one setting without resending the
	// secret must not erase it.
	existing, err := c.repo.GetSSOConfig(r.Context(), wsID)
	if err != nil {
		writeSSOError(w, http.StatusInternalServerError, "failed to read sso configuration")
		return
	}

	cfg := models.SSOConfig{
		WorkspaceID:  wsID,
		Protocol:     protocol,
		Provider:     provider,
		Active:       payload.Active,
		Domains:      models.NormalizeSSODomains(payload.Domains),
		SAMLRequired: payload.SAMLRequired,
	}
	if existing != nil {
		cfg.VerifiedAt = existing.VerifiedAt
		if provider.ClientSecret == "" {
			cfg.Provider.ClientSecret = existing.Provider.ClientSecret
		}
		// A protocol switch invalidates the previous handshake: the new protocol
		// has not been proven against the IdP. Silently downgrading the requested
		// enforcement here would tell the operator SSO is enforced when it is not,
		// so refuse the write and say what to do instead.
		if existing.Protocol != protocol {
			cfg.VerifiedAt = nil
			if payload.SAMLRequired {
				writeSSOError(w, http.StatusConflict,
					"switching sso protocol resets the verified handshake; save without ssoRequired, run the connection test, then enable enforcement")
				return
			}
		}
	}

	stored, blocked, err := c.repo.UpsertSSOConfig(r.Context(), cfg)
	if err != nil {
		// The client gets a generic message; the operator needs the cause.
		// Logged without the provider config, so no IdP secret reaches the log.
		slog.Error("SSO configuration write failed",
			"workspace_id", wsID,
			"protocol", string(protocol),
			"enforcement_blocked", blocked,
			"error", err)
		if blocked {
			// Refusing enforcement on an unverified config is correct behaviour,
			// not a server fault: tell the caller exactly what to do.
			writeSSOError(w, http.StatusConflict, err.Error())
			return
		}
		writeSSOError(w, http.StatusInternalServerError, "failed to save sso configuration")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"uuid":           stored.ID,
		"organizationId": stored.WorkspaceID,
		"protocol":       stored.Protocol,
		"active":         stored.Active,
		"domains":        stored.Domains,
		"samlRequired":   stored.SAMLRequired,
		"verifiedAt":     stored.VerifiedAt,
		"status":         "saved",
	})
}

func (c *SSOConfigController) handleTestConnection(w http.ResponseWriter, r *http.Request) {
	if c.authCtrl != nil {
		c.authCtrl.HandleStartSSOConnectionTest(w, r)
		return
	}
	writeSSOError(w, http.StatusNotImplemented, "sso connection test is not available on this deployment")
}

func (c *SSOConfigController) handleGetTestResult(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionId")
	if strings.TrimSpace(sessionID) == "" {
		writeSSOError(w, http.StatusBadRequest, "sessionId is required")
		return
	}

	if c.authCtrl != nil {
		c.authCtrl.HandleGetSSOConnectionTestResult(w, r)
		return
	}
	writeSSOError(w, http.StatusNotImplemented, "sso connection test is not available on this deployment")
}

func (c *SSOConfigController) handleVerifyDomain(w http.ResponseWriter, r *http.Request) {
	if c.authCtrl != nil {
		c.authCtrl.HandleVerifyDomainDNS(w, r)
		return
	}
	writeSSOError(w, http.StatusNotImplemented, "domain verification is not available on this deployment")
}

// providerConfigFromPayload maps the loosely typed request body onto the typed
// provider config, so a wrong type for a known key is a 400 rather than a
// silently empty configuration. Unknown keys are ignored, which keeps the
// endpoint forwards-compatible with a newer dashboard.
func providerConfigFromPayload(raw map[string]any) (models.SSOProviderConfig, error) {
	if len(raw) == 0 {
		return models.SSOProviderConfig{}, nil
	}

	encoded, err := json.Marshal(raw)
	if err != nil {
		return models.SSOProviderConfig{}, err
	}

	var provider models.SSOProviderConfig
	if err := json.Unmarshal(encoded, &provider); err != nil {
		return models.SSOProviderConfig{}, err
	}
	return provider, nil
}

// validateProviderConfig enforces the minimum an IdP needs before the config
// may be saved as active. Catching this server-side means a half-filled form
// cannot lock a workspace out of its own login page. An inactive config is
// allowed to be incomplete: it is a draft, not a login path.
func validateProviderConfig(protocol models.SSOProtocol, active bool, p models.SSOProviderConfig) error {
	if !active {
		return nil
	}
	if strings.TrimSpace(p.Issuer) == "" {
		return errSSOValidation("idpIssuer is required to activate sso")
	}

	switch protocol {
	case models.SSOProtocolSAML:
		if strings.TrimSpace(p.EntryPoint) == "" {
			return errSSOValidation("entryPoint is required to activate SAML sso")
		}
		if strings.TrimSpace(p.IdPCert) == "" {
			return errSSOValidation("cert is required to activate SAML sso: assertions cannot be verified without the IdP certificate")
		}
	case models.SSOProtocolOIDC:
		if strings.TrimSpace(p.ClientID) == "" {
			return errSSOValidation("clientId is required to activate OIDC sso")
		}
		if strings.TrimSpace(p.AuthorizeURL) == "" {
			return errSSOValidation("authorizeUrl is required to activate OIDC sso")
		}
		if strings.TrimSpace(p.TokenURL) == "" {
			return errSSOValidation("tokenUrl is required to activate OIDC sso")
		}
	}
	return nil
}

type errSSOValidation string

func (e errSSOValidation) Error() string { return string(e) }

// emptySSOConfigResponse is the explicit "nothing configured yet" shape.
func emptySSOConfigResponse(wsID uuid.UUID) map[string]any {
	return map[string]any{
		"uuid":           nil,
		"organizationId": wsID,
		"protocol":       string(models.SSOProtocolSAML),
		"providerConfig": map[string]any{},
		"active":         false,
		"domains":        []string{},
		"samlRequired":   false,
		"status":         "not_configured",
	}
}

// maskSSOSecret ensures the OIDC client secret never leaves the server.
func maskSSOSecret(p models.SSOProviderConfig) models.SSOProviderConfig {
	p.ClientSecret = ""
	return p
}

func writeSSOError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
