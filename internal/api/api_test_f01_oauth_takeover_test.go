package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/api"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/auth/cliauth"
	"github.com/scandrix/backend/internal/auth/mailer"
	"github.com/scandrix/backend/internal/auth/oauth"
	"github.com/scandrix/backend/internal/review"
	"github.com/scandrix/backend/internal/rules"
)

// buildVulnerableSurfaceRouter builds a router with a nil repo, which is the
// cheapest way to observe route *registration* without needing a database.
// Route shape is identical to production.
func buildVulnerableSurfaceRouter(t *testing.T) http.Handler {
	t.Helper()
	jwtSecret := "test-jwt-secret-key-123456789012"
	evaluator, _ := rules.NewEvaluator(rules.DefaultCatalog())

	return api.BuildRouter(api.RouterConfig{
		Repo:         nil,
		AuthService:  auth.NewAuthenticator(jwtSecret),
		StreamHub:    review.NewStreamHub(),
		Evaluator:    evaluator,
		DeviceFlow:   cliauth.NewDeviceFlowManager(cliauth.NewInMemorySessionStore(), "https://app.scandrix.dev"),
		OAuthService: oauth.NewOAuthService(oauth.ProviderConfig{ClientID: "id", ClientSecret: "secret"}, oauth.ProviderConfig{}),
		Mailer:       mailer.NewNoopSender(),
		AppBaseURL:   "https://app.scandrix.dev",
		JWTSecret:    jwtSecret,
	})
}

// TestPostOAuthRouteIsNotRegistered pins AUDIT_REMEDIATION.md F-01.
//
// POST /auth/oauth accepted a client-supplied email and minted a session for
// whichever account that email resolved to. An unauthenticated caller holding
// only their own GitHub token could therefore obtain a token for any account
// whose email address they knew.
//
// The route must not exist at all. Returning 401/403 would be acceptable; 404
// is what chi produces for an unregistered path, and asserting on "not 2xx and
// not a token response" keeps this test from breaking on the specific code.
func TestPostOAuthRouteIsNotRegistered(t *testing.T) {
	router := buildVulnerableSurfaceRouter(t)

	body := `{"email":"victim@company.com","refreshToken":"attacker-github-token","authProvider":"github"}`

	for _, path := range []string{"/auth/oauth", "/api/v1/auth/oauth"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			// Deliberately no Authorization header: the vulnerability was that
			// none was needed.
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code == http.StatusOK || w.Code == http.StatusCreated {
				t.Fatalf("POST %s returned %d; the unauthenticated OAuth "+
					"token-exchange route must not be registered (AUDIT F-01). body: %s",
					path, w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "accessToken") {
				t.Fatalf("POST %s issued a token: %s", path, w.Body.String())
			}
		})
	}
}

// TestOAuthProviderRedirectFlowStillWorks is the counterweight to
// TestPostOAuthRouteIsNotRegistered. Removing F-01 must not remove the
// legitimate, provider-redirect-based login.
func TestOAuthProviderRedirectFlowStillWorks(t *testing.T) {
	router := buildVulnerableSurfaceRouter(t)

	for _, path := range []string{
		"/auth/oauth/github/authorize",
		"/api/v1/auth/oauth/github/authorize",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("GET %s returned %d, want 200 — the legitimate OAuth "+
				"authorize flow must keep working. body: %s",
				path, w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "authorization_url") {
			t.Fatalf("GET %s did not return an authorization_url: %s", path, w.Body.String())
		}
	}
}
