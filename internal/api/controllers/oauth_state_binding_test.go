package controllers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/scandrix/backend/internal/auth/oauth"
)

// The OAuth state store proves a token is unused and unexpired. It does not
// prove the callback came from the browser that started the flow, so an
// attacker could complete their own authorization and then walk a victim into
// the callback URL with the attacker's code and state - logging the victim into
// the attacker's identity. The state must therefore be bound to the browser.
//
// AUDIT_REMEDIATION.md F-16.
func TestOAuthStateIsBoundToRequestingBrowser(t *testing.T) {
	store := oauth.NewStateStore(0)

	state, err := store.Generate(oauth.ProviderGitHub)
	if err != nil {
		t.Fatalf("generate state: %v", err)
	}
	binding := store.Bind(state)

	t.Run("cookie matching the state is accepted", func(t *testing.T) {
		if !store.VerifyBinding(state, binding) {
			t.Fatal("expected the binding cookie to verify")
		}
	})

	t.Run("missing cookie is rejected", func(t *testing.T) {
		if store.VerifyBinding(state, "") {
			t.Fatal("expected a missing cookie to be rejected")
		}
	})

	t.Run("cookie from a different flow is rejected", func(t *testing.T) {
		other, err := store.Generate(oauth.ProviderGitHub)
		if err != nil {
			t.Fatalf("generate second state: %v", err)
		}
		if store.VerifyBinding(state, store.Bind(other)) {
			t.Fatal("expected another flow's cookie to be rejected")
		}
	})

	t.Run("binding is not the state token itself", func(t *testing.T) {
		if binding == state {
			t.Fatal("binding must not be the raw state, or a leaked cookie could be replayed as a state")
		}
	})

	t.Run("shared bind secret works across stores", func(t *testing.T) {
		secret := []byte("a-shared-cluster-secret-value-32b")
		a := oauth.NewStateStore(0).WithBindSecret(secret)
		b := oauth.NewStateStore(0).WithBindSecret(secret)
		s, err := a.Generate(oauth.ProviderGitHub)
		if err != nil {
			t.Fatalf("generate: %v", err)
		}
		if !b.VerifyBinding(s, a.Bind(s)) {
			t.Fatal("a cluster peer must be able to verify the binding")
		}
	})
}

// getAuthCookie must find the cookie in the form setAuthCookie actually writes.
// In production setAuthCookie renames to __Host-, so reading the bare name
// silently missed the cookie.
func TestGetAuthCookieFindsPrefixedCookie(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		plain := getAuthCookie(r, "scandrix_oauth_state")
		http.SetCookie(w, &http.Cookie{Name: "scandrix_oauth_state", Value: "plain-value", Path: "/", MaxAge: 600})
		http.SetCookie(w, &http.Cookie{Name: "__Host-scandrix_oauth_state", Value: "prefixed-value", Path: "/", MaxAge: 600})
		_, _ = w.Write([]byte(plain))
	})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	// The request had no cookies, so the first read is empty...
	if rec.Body.String() != "" {
		t.Fatalf("expected no cookie on a bare request, got %q", rec.Body.String())
	}

	// ...and a request carrying the __Host- form resolves correctly.
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "__Host-scandrix_oauth_state", Value: "prefixed-value"})
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req)
	if got := rec2.Body.String(); got != "prefixed-value" {
		t.Fatalf("expected the __Host- cookie to be found, got %q", got)
	}
}

// Handler-level enforcement: a callback carrying a genuinely valid, unused
// state token but no matching browser cookie must be refused. This is the
// login-CSRF case, and it is the assertion that matters - the store tests above
// only prove the helper works, not that the route calls it.
func TestOAuthCallbackRefusesUnboundState(t *testing.T) {
	store := oauth.NewStateStore(0)
	// A service is needed to get past the "OAuth not configured" guard, but the
	// binding check runs before ExchangeCode, so no network call is made.
	ctrl := &AuthController{
		oauthStateStore: store,
		oauthService:    oauth.NewOAuthService(oauth.ProviderConfig{}, oauth.ProviderConfig{}),
	}

	router := chi.NewRouter()
	router.Get("/oauth/{provider}/callback", ctrl.handleOAuthCallback)

	state, err := store.Generate(oauth.ProviderGitHub)
	if err != nil {
		t.Fatalf("generate state: %v", err)
	}

	t.Run("valid state without a browser cookie is refused", func(t *testing.T) {
		// Sanity: the state really is valid, so the refusal is the binding and
		// not the state store.
		if !store.Validate(state, oauth.ProviderGitHub) {
			t.Fatal("precondition: state should be valid")
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(
			http.MethodGet, "/oauth/github/callback?code=attacker-code&state="+state, nil))

		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 for an unbound state, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("cookie bound to a different state is refused", func(t *testing.T) {
		other, err := store.Generate(oauth.ProviderGitHub)
		if err != nil {
			t.Fatalf("generate second state: %v", err)
		}
		req := httptest.NewRequest(http.MethodGet, "/oauth/github/callback?code=attacker-code&state="+state, nil)
		req.AddCookie(&http.Cookie{Name: "scandrix_oauth_state", Value: store.Bind(other)})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 for a mismatched binding, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}
