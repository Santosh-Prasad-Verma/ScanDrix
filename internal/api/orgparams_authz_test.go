package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/auth/cliauth"
	"github.com/scandrix/backend/internal/auth/mailer"
	"github.com/scandrix/backend/internal/auth/oauth"
	"github.com/scandrix/backend/internal/enterprise/license"
	"github.com/scandrix/backend/internal/review"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/pkg/models"
)

// Workspace-scoped provider configuration and spend limits are high-value
// targets: repointing BYOK credentials sends every subsequent review's source
// code to an endpoint of the caller's choosing, and lifting a spend cap
// removes a budget control.
//
// Both mounts previously sat on the authenticated group with no policy guard
// at all, so any valid session — including viewer and member — could reach
// them. These tests lock the intended behaviour in place so a later refactor
// cannot quietly reopen the hole.
func newAuthzTestRouter(t *testing.T, secret string) http.Handler {
	t.Helper()

	evaluator, err := rules.NewEvaluator(rules.DefaultCatalog())
	if err != nil {
		t.Fatalf("build evaluator: %v", err)
	}

	return api.BuildRouter(api.RouterConfig{
		Repo:         nil,
		AuthService:  auth.NewAuthenticator(secret),
		StreamHub:    review.NewStreamHub(),
		Evaluator:    evaluator,
		SCIMService:  nil,
		DeviceFlow:   cliauth.NewDeviceFlowManager(cliauth.NewInMemorySessionStore(), "https://app.scandrix.dev"),
		OAuthService: oauth.NewOAuthService(oauth.ProviderConfig{}, oauth.ProviderConfig{}),
		Mailer:       mailer.NewNoopSender(),
		AppBaseURL:   "https://app.scandrix.dev",
		JWTSecret:    secret,
	})
}

func authzRequest(t *testing.T, router http.Handler, secret string, role models.UserRole, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()

	token, err := auth.NewAuthenticator(secret).GenerateToken(uuid.New(), uuid.New(), role)
	if err != nil {
		t.Fatalf("generate token for %s: %v", role, err)
	}

	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// TestOrganizationParametersWritesRequireWorkspaceUpdate is the core
// regression guard: a non-privileged role must not be able to rewrite the
// workspace's LLM credentials, no matter how well-formed the request is.
func TestOrganizationParametersWritesRequireWorkspaceUpdate(t *testing.T) {
	const secret = "authz-test-secret-key-abcdefghijklmnop"
	router := newAuthzTestRouter(t, secret)

	privilegedWrites := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/api/v1/organization-parameters/create-or-update", `{"key":"byok_config","configValue":{}}`},
		{http.MethodDelete, "/api/v1/organization-parameters/delete-byok-config", ""},
		{http.MethodPost, "/api/v1/organization-parameters/test-byok", `{}`},
		{http.MethodPost, "/api/v1/organization-parameters/model-overrides/clear", `{}`},
		{http.MethodPost, "/api/v1/organization-parameters/cockpit-metrics-visibility", `{}`},
		{http.MethodPost, "/api/v1/organization-parameters/auto-license/allowed-users", `{}`},
	}

	for _, role := range []models.UserRole{models.RoleViewer, models.RoleMember} {
		for _, tc := range privilegedWrites {
			rec := authzRequest(t, router, secret, role, tc.method, tc.path, tc.body)
			if rec.Code != http.StatusForbidden {
				t.Errorf("%s %s: expected 403, got %d (body %q)",
					role, tc.method+" "+tc.path, rec.Code, rec.Body.String())
			}
		}
	}
}

// TestSpendLimitWritesRequireBillingAuthority covers the spend-cap half of the
// hole. Reads stay available to read-authority roles; only writes are blocked.
func TestSpendLimitWritesRequireBillingAuthority(t *testing.T) {
	const secret = "authz-test-secret-key-abcdefghijklmnop"
	router := newAuthzTestRouter(t, secret)

	for _, role := range []models.UserRole{models.RoleViewer, models.RoleMember} {
		rec := authzRequest(t, router, secret, role, http.MethodPost, "/api/v1/spend-limit/", `{"limitUsd":100000}`)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s POST /api/v1/spend-limit/: expected 403, got %d (body %q)",
				role, rec.Code, rec.Body.String())
		}
	}
}

// TestWorkspaceConfigurationReadsRemainAvailable guards against over-correcting
// the fix into a lockout: the roles that hold workspace read authority must
// still be able to see which model the workspace uses.
func TestWorkspaceConfigurationReadsRemainAvailable(t *testing.T) {
	const secret = "authz-test-secret-key-abcdefghijklmnop"
	router := newAuthzTestRouter(t, secret)

	// Workspace read authority. Spend-limit reads are deliberately NOT listed
	// here: the role matrix grants billing read to admin and owner only, so
	// viewer and member are correctly refused. That separation is asserted in
	// TestSpendLimitReadsRequireBillingAuthority.
	workspaceReadPaths := []string{
		"/api/v1/organization-parameters/llm-config/status",
		"/api/v1/organization-parameters/cockpit-metrics-visibility",
	}

	for _, role := range []models.UserRole{models.RoleViewer, models.RoleMember, models.RoleAdmin, models.RoleOwner} {
		for _, path := range workspaceReadPaths {
			rec := authzRequest(t, router, secret, role, http.MethodGet, path, "")
			// This router is built with a nil repository, so a correctly
			// authorised read fails downstream with 400/404/500. The property
			// under test is that authorization did NOT reject it with 401/403.
			if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
				t.Errorf("%s GET %s: expected authorization to pass, got %d (body %q)",
					role, path, rec.Code, rec.Body.String())
			}
		}
	}
}

// TestSpendLimitReadsRequireBillingAuthority pins the intended read boundary.
// Spend figures are budget information and the role matrix grants billing read
// to admin and owner only, so viewer and member must not see them.
func TestSpendLimitReadsRequireBillingAuthority(t *testing.T) {
	const secret = "authz-test-secret-key-abcdefghijklmnop"
	router := newAuthzTestRouter(t, secret)

	readPaths := []string{"/api/v1/spend-limit/status", "/api/v1/spend-limit/"}

	for _, role := range []models.UserRole{models.RoleViewer, models.RoleMember} {
		for _, path := range readPaths {
			rec := authzRequest(t, router, secret, role, http.MethodGet, path, "")
			if rec.Code != http.StatusForbidden {
				t.Errorf("%s GET %s: expected 403, got %d (body %q)", role, path, rec.Code, rec.Body.String())
			}
		}
	}

	// Admin holds billing read: authorization must pass and the request should
	// fail downstream instead (this router has no repository configured).
	for _, path := range readPaths {
		rec := authzRequest(t, router, secret, models.RoleAdmin, http.MethodGet, path, "")
		if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
			t.Errorf("ADMIN GET %s: expected authorization to pass, got %d (body %q)", path, rec.Code, rec.Body.String())
		}
	}
}

// TestUnauthorizedRequestsStillRejected confirms the new guards did not turn
// unauthenticated access into something other than a 401.
func TestUnauthorizedRequestsStillRejected(t *testing.T) {
	const secret = "authz-test-secret-key-abcdefghijklmnop"
	router := newAuthzTestRouter(t, secret)

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1/organization-parameters/create-or-update"},
		{http.MethodGet, "/api/v1/spend-limit/status"},
		{http.MethodGet, "/api/v1/organization-parameters/llm-config/status"},
	}

	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader("{}"))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without a session: expected 401, got %d", tc.method, tc.path, rec.Code)
		}
	}
}

var _ = license.FeatureSSOSAML
