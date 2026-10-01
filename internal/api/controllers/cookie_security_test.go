package controllers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// TestCookieSecureIsDefaultOn pins AUDIT_REMEDIATION.md F-19.
//
// isSecureRequest used to fail open: no TLS, no `X-Forwarded-Proto: https`, and
// no environment variable literally named "production" all meant "not secure",
// so staging and misconfigured deployments issued session cookies that browsers
// would happily send back over plaintext HTTP. It also let a client set the
// decision with a header.
//
// Secure is now the default, and the only way to turn it off is an explicit
// COOKIE_SECURE=false that is itself refused in production.
func TestCookieSecureIsDefaultOn(t *testing.T) {
	envKeys := []string{"ENVIRONMENT", "APP_ENV", "SCANDRIX_ENV", "GO_ENV", "COOKIE_SECURE"}

	clear := func(t *testing.T) {
		t.Helper()
		for _, k := range envKeys {
			t.Setenv(k, "")
			if err := os.Unsetenv(k); err != nil {
				t.Fatalf("unset %s: %v", k, err)
			}
		}
	}

	t.Run("default is secure", func(t *testing.T) {
		clear(t)
		if !isSecureRequest(httptest.NewRequest(http.MethodGet, "/", nil)) {
			t.Fatal("cookies must carry Secure by default")
		}
	})

	t.Run("nil request is secure", func(t *testing.T) {
		clear(t)
		if !isSecureRequest(nil) {
			t.Fatal("a nil request must not downgrade cookie security")
		}
	})

	t.Run("no environment name implies insecure", func(t *testing.T) {
		clear(t)
		t.Setenv("ENVIRONMENT", "staging")
		t.Setenv("APP_ENV", "development")
		if !isSecureRequest(httptest.NewRequest(http.MethodGet, "/", nil)) {
			t.Fatal("a non-production environment name must not disable Secure")
		}
	})

	t.Run("client header cannot decide", func(t *testing.T) {
		clear(t)
		// An attacker controlling this header must not be able to force the
		// insecure path, and a client cannot grant itself Secure either --
		// the decision is server-side configuration only.
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Forwarded-Proto", "http")
		if !isSecureRequest(req) {
			t.Fatal("X-Forwarded-Proto must not influence cookie security")
		}
	})

	t.Run("explicit opt-out works outside production", func(t *testing.T) {
		clear(t)
		t.Setenv("COOKIE_SECURE", "false")
		if isSecureRequest(httptest.NewRequest(http.MethodGet, "/", nil)) {
			t.Fatal("COOKIE_SECURE=false should relax Secure outside production")
		}
	})

	t.Run("production cannot opt out", func(t *testing.T) {
		for _, key := range []string{"ENVIRONMENT", "APP_ENV", "SCANDRIX_ENV", "GO_ENV"} {
			t.Run(key, func(t *testing.T) {
				clear(t)
				t.Setenv("COOKIE_SECURE", "false")
				t.Setenv(key, "production")
				if !isSecureRequest(httptest.NewRequest(http.MethodGet, "/", nil)) {
					t.Fatalf("production must not downgrade cookie security via COOKIE_SECURE=false (%s)", key)
				}
			})
		}
	})
}

// TestSetAuthCookieFlags locks the attributes actually written on the wire.
func TestSetAuthCookieFlags(t *testing.T) {
	for _, k := range []string{"ENVIRONMENT", "APP_ENV", "SCANDRIX_ENV", "GO_ENV", "COOKIE_SECURE"} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	setAuthCookie(w, req, "scandrix_token", "value", 900)

	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}
	c := cookies[0]

	if !c.Secure {
		t.Error("Secure must be set by default")
	}
	if !c.HttpOnly {
		t.Error("HttpOnly must be set so the token is not readable by script")
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite = %v, want Lax", c.SameSite)
	}
	if c.Path != "/" {
		t.Errorf("Path = %q, want /", c.Path)
	}
	// The __Host- prefix is only valid alongside Secure, Path=/ and no Domain.
	if c.Name == "__Host-scandrix_token" {
		if c.Domain != "" {
			t.Error("a __Host- cookie must not set Domain")
		}
	}
}
