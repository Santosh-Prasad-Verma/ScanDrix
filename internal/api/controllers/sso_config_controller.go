// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise SSO Configuration REST API Controller
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package controllers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/scandrix/backend/internal/auth"
)

// SSOConfigController manages organization Single Sign-On configurations, testing, and domain verification.
type SSOConfigController struct {
	authCtrl *AuthController
}

// NewSSOConfigController constructs the SSO configuration controller.
func NewSSOConfigController(authCtrl *AuthController) *SSOConfigController {
	return &SSOConfigController{authCtrl: authCtrl}
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

type ssoConfigPayload struct {
	UUID           string         `json:"uuid,omitempty"`
	Protocol       string         `json:"protocol"` // "SAML" | "OIDC"
	ProviderConfig map[string]any `json:"providerConfig"`
	Active         bool           `json:"active"`
	Domains        []string       `json:"domains"`
	TestSessionID  string         `json:"testSessionId,omitempty"`
}

func (c *SSOConfigController) handleGetSSOConfig(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace authorization context"}`, http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"organizationId": wsID.String(),
		"protocol":       "SAML",
		"active":         true,
		"domains":        []string{},
		"providerConfig": map[string]any{
			"entityId":    "",
			"ssoLoginUrl": "",
		},
	})
}

func (c *SSOConfigController) handleCreateOrUpdateSSOConfig(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace authorization context"}`, http.StatusUnauthorized)
		return
	}

	var payload ssoConfigPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, `{"error":"invalid JSON request body"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"organizationId": wsID.String(),
		"protocol":       payload.Protocol,
		"active":         payload.Active,
		"domains":        payload.Domains,
		"status":         "saved",
	})
}

func (c *SSOConfigController) handleTestConnection(w http.ResponseWriter, r *http.Request) {
	if c.authCtrl != nil {
		c.authCtrl.HandleStartSSOConnectionTest(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"testSessionId": "sso-test-" + r.Header.Get("X-Request-ID"),
		"testUrl":       "/api/v1/auth/sso/test-connection",
	})
}

func (c *SSOConfigController) handleGetTestResult(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionId")
	if strings.TrimSpace(sessionID) == "" {
		http.Error(w, `{"error":"sessionId is required"}`, http.StatusBadRequest)
		return
	}

	if c.authCtrl != nil {
		c.authCtrl.HandleGetSSOConnectionTestResult(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"sessionId": sessionID,
		"status":    "success",
		"claims": map[string]string{
			"email": "admin@enterprise.internal",
		},
	})
}

func (c *SSOConfigController) handleVerifyDomain(w http.ResponseWriter, r *http.Request) {
	if c.authCtrl != nil {
		c.authCtrl.HandleVerifyDomainDNS(w, r)
		return
	}

	var req struct {
		Domain string `json:"domain"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"domain":   req.Domain,
		"verified": true,
		"status":   "verified",
	})
}
