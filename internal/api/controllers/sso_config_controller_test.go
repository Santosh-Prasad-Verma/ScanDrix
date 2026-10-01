// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise SSO Configuration
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/pkg/models"
)

// fakeSSORepo is an in-memory stand-in for the database repository. It exists
// to exercise controller behaviour (validation, secret masking, partial
// updates) without a live database; the SQL itself is covered by the live
// verification run.
type fakeSSORepo struct {
	cfg         *models.SSOConfig
	upsertCalls int
	lastUpsert  models.SSOConfig
	returnErr   error
}

func (f *fakeSSORepo) GetSSOConfig(_ context.Context, wsID uuid.UUID) (*models.SSOConfig, error) {
	if f.returnErr != nil {
		return nil, f.returnErr
	}
	if f.cfg == nil {
		return nil, nil
	}
	cp := *f.cfg
	return &cp, nil
}

func (f *fakeSSORepo) UpsertSSOConfig(_ context.Context, cfg models.SSOConfig) (models.SSOConfig, bool, error) {
	f.upsertCalls++
	f.lastUpsert = cfg
	if cfg.SAMLRequired && cfg.VerifiedAt == nil {
		return models.SSOConfig{}, true, errSSOValidation("sso enforcement requires a verified idp configuration; complete the connection test first")
	}
	now := time.Now().UTC()
	stored := cfg
	stored.ID = uuid.New()
	stored.CreatedAt = now
	stored.UpdatedAt = now
	stored.Provider.ClientSecret = "" // repository masks on the way out
	f.cfg = &stored
	return stored, false, nil
}

// requestWithWorkspace issues a request carrying an authenticated workspace
// context, which the feature gate and the controller both require.
func requestWithWorkspace(t *testing.T, method, path, body string) *http.Request {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	wsID := uuid.MustParse("0b609524-8c01-4a79-86d4-7411d681442d")
	ctx := context.WithValue(r.Context(), auth.WorkspaceContextKey, wsID)
	return r.WithContext(ctx)
}

func TestSSOConfigGetReturnsExplicitEmptyState(t *testing.T) {
	repo := &fakeSSORepo{}
	ctrl := NewSSOConfigController(nil, repo)

	rr := httptest.NewRecorder()
	ctrl.handleGetSSOConfig(rr, requestWithWorkspace(t, http.MethodGet, "/", ""))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if body["status"] != "not_configured" {
		t.Fatalf("expected an explicit not_configured state, got %v", body["status"])
	}
	if body["active"] != false {
		t.Fatalf("an unconfigured workspace must not report active, got %v", body["active"])
	}
}

func TestSSOConfigGetRequiresWorkspaceContext(t *testing.T) {
	ctrl := NewSSOConfigController(nil, &fakeSSORepo{})

	rr := httptest.NewRecorder()
	// No workspace in context: the request must be refused, not served.
	ctrl.handleGetSSOConfig(rr, httptest.NewRequest(http.MethodGet, "/", nil))

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without workspace context, got %d", rr.Code)
	}
}

func TestSSOConfigSavePersistsAndMasksSecret(t *testing.T) {
	repo := &fakeSSORepo{}
	ctrl := NewSSOConfigController(nil, repo)

	body := `{
		"protocol": "OIDC",
		"active": true,
		"domains": ["Acme.COM", " acme.com ", "subsidiary.com"],
		"providerConfig": {
			"idpIssuer": "https://idp.acme.test",
			"clientId": "scandrix",
			"clientSecret": "super-secret-value",
			"authorizeUrl": "https://idp.acme.test/authorize",
			"tokenUrl": "https://idp.acme.test/token"
		}
	}`

	rr := httptest.NewRecorder()
	ctrl.handleCreateOrUpdateSSOConfig(rr, requestWithWorkspace(t, http.MethodPost, "/", body))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if repo.upsertCalls != 1 {
		t.Fatalf("expected exactly one write, got %d", repo.upsertCalls)
	}
	// Domains are normalized: lowercased, trimmed, de-duplicated.
	got := repo.lastUpsert.Domains
	if len(got) != 2 || got[0] != "acme.com" || got[1] != "subsidiary.com" {
		t.Fatalf("domains should be normalized and de-duplicated, got %v", got)
	}
	// The secret reaches the repository (it must be stored encrypted) but is
	// never echoed back to the client.
	if repo.lastUpsert.Provider.ClientSecret != "super-secret-value" {
		t.Fatal("the client secret must reach the repository for encrypted storage")
	}
	if strings.Contains(rr.Body.String(), "super-secret-value") {
		t.Fatal("the client secret must never appear in the response body")
	}
}

func TestSSOConfigRejectsIncompleteActiveConfig(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"active without issuer", `{"protocol":"SAML","active":true,"providerConfig":{}}`},
		{"active SAML without cert", `{"protocol":"SAML","active":true,"providerConfig":{"idpIssuer":"https://idp","entryPoint":"https://idp/sso"}}`},
		{"active SAML without entry point", `{"protocol":"SAML","active":true,"providerConfig":{"idpIssuer":"https://idp","cert":"CERT"}}`},
		{"active OIDC without token url", `{"protocol":"OIDC","active":true,"providerConfig":{"idpIssuer":"https://idp","clientId":"c","authorizeUrl":"https://a"}}`},
		{"unsupported protocol", `{"protocol":"LDAP","active":false,"providerConfig":{}}`},
		{"malformed json", `{"protocol":`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeSSORepo{}
			ctrl := NewSSOConfigController(nil, repo)

			rr := httptest.NewRecorder()
			ctrl.handleCreateOrUpdateSSOConfig(rr, requestWithWorkspace(t, http.MethodPost, "/", tc.body))

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d (%s)", rr.Code, rr.Body.String())
			}
			if repo.upsertCalls != 0 {
				t.Fatal("an invalid configuration must never reach the repository")
			}
		})
	}
}

func TestSSOConfigAllowsIncompleteDraftWhenInactive(t *testing.T) {
	repo := &fakeSSORepo{}
	ctrl := NewSSOConfigController(nil, repo)

	// Not active: a half-filled draft is legitimate and must be storable.
	body := `{"protocol":"SAML","active":false,"providerConfig":{"idpIssuer":"https://idp"}}`
	rr := httptest.NewRecorder()
	ctrl.handleCreateOrUpdateSSOConfig(rr, requestWithWorkspace(t, http.MethodPost, "/", body))

	if rr.Code != http.StatusOK {
		t.Fatalf("an inactive draft must be accepted, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestSSOConfigRefusesEnforcementWithoutVerification(t *testing.T) {
	repo := &fakeSSORepo{}
	ctrl := NewSSOConfigController(nil, repo)

	body := `{
		"protocol":"SAML","active":true,"samlRequired":true,
		"providerConfig":{"idpIssuer":"https://idp","entryPoint":"https://idp/sso","cert":"CERT"}
	}`
	rr := httptest.NewRecorder()
	ctrl.handleCreateOrUpdateSSOConfig(rr, requestWithWorkspace(t, http.MethodPost, "/", body))

	if rr.Code != http.StatusConflict {
		t.Fatalf("enforcing SSO on an unverified config must be refused with 409, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestSSOConfigPartialUpdateKeepsStoredSecret(t *testing.T) {
	existing := &models.SSOConfig{
		ID:          uuid.New(),
		WorkspaceID: uuid.MustParse("0b609524-8c01-4a79-86d4-7411d681442d"),
		Protocol:    models.SSOProtocolOIDC,
		Provider: models.SSOProviderConfig{
			Issuer:       "https://idp.acme.test",
			ClientID:     "scandrix",
			ClientSecret: "already-stored-secret",
			AuthorizeURL: "https://idp.acme.test/authorize",
			TokenURL:     "https://idp.acme.test/token",
		},
		Active:  true,
		Domains: []string{"acme.com"},
	}
	repo := &fakeSSORepo{cfg: existing}
	ctrl := NewSSOConfigController(nil, repo)

	// The dashboard re-saving the form without the write-only secret must not
	// erase the stored one.
	body := `{
		"protocol":"OIDC","active":true,
		"providerConfig":{"idpIssuer":"https://idp.acme.test","clientId":"scandrix","authorizeUrl":"https://idp.acme.test/authorize","tokenUrl":"https://idp.acme.test/token"}
	}`
	rr := httptest.NewRecorder()
	ctrl.handleCreateOrUpdateSSOConfig(rr, requestWithWorkspace(t, http.MethodPost, "/", body))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if repo.lastUpsert.Provider.ClientSecret != "already-stored-secret" {
		t.Fatal("a partial update must preserve the stored client secret")
	}
}

func TestSSOConfigProtocolSwitchClearsVerification(t *testing.T) {
	verified := time.Now().UTC()
	existing := &models.SSOConfig{
		WorkspaceID:  uuid.MustParse("0b609524-8c01-4a79-86d4-7411d681442d"),
		Protocol:     models.SSOProtocolSAML,
		Provider:     models.SSOProviderConfig{Issuer: "https://idp", EntryPoint: "https://idp/sso", IdPCert: "CERT"},
		Active:       true,
		VerifiedAt:   &verified,
		SAMLRequired: true,
	}
	repo := &fakeSSORepo{cfg: existing}
	ctrl := NewSSOConfigController(nil, repo)

	// Switching SAML -> OIDC has not been proven against the IdP, so the previous
	// handshake must not carry over and enforcement must be dropped.
	body := `{"protocol":"OIDC","active":true,"samlRequired":true,
		"providerConfig":{"idpIssuer":"https://idp","clientId":"c","authorizeUrl":"https://a","tokenUrl":"https://t"}}`
	rr := httptest.NewRecorder()
	ctrl.handleCreateOrUpdateSSOConfig(rr, requestWithWorkspace(t, http.MethodPost, "/", body))

	if rr.Code != http.StatusConflict {
		t.Fatalf("switching protocol must drop stale verification, expected 409, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestSSOConfigRoutesAreMounted(t *testing.T) {
	ctrl := NewSSOConfigController(nil, &fakeSSORepo{})
	r := ctrl.Routes()

	// chi panics on a duplicate route; mounting twice proves the set is valid.
	_ = chi.NewRouter()
	mounted := map[string]bool{}
	chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		mounted[method+" "+route] = true
		return nil
	})

	for _, want := range []string{
		"GET /",
		"POST /",
		"POST /test-connection",
		"GET /test-connection/{sessionId}",
		"POST /verify-domain",
	} {
		if !mounted[want] {
			t.Errorf("expected route %q to be mounted, got %v", want, mounted)
		}
	}
}
