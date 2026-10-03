package controllers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/auth"
)

// F-28/F-29/F-32: account lockout is the brute-force control. It used to fall
// back to a per-process map whenever the shared store was absent or unreachable,
// so with N replicas the effective allowance was N times the intended one and an
// attacker only had to retry until they landed on a replica with a low counter.
//
// These tests need no Redis: the branch they cover is "no shared store
// configured", which is exactly the state a production deployment should never be
// in and therefore the one worth pinning.
func TestAccountLockoutRequiresSharedStoreOutsideDevelopment(t *testing.T) {
	ctrl := NewAuthController(auth.NewAuthenticator("test-jwt-secret-key-123456789012"), nil)
	// No cache client is attached.
	ctx := context.Background()

	for _, env := range []string{"production", "staging"} {
		t.Run(env+" denies", func(t *testing.T) {
			t.Setenv("APP_ENV", env)
			locked, ttl := ctrl.IsAccountLocked(ctx, "victim@scandrix.internal")
			if !locked {
				t.Fatalf("%s with no shared lockout store must not report the account as unlocked", env)
			}
			if ttl <= 0 {
				t.Error("a denial must carry a retry-after duration")
			}
		})

		t.Run(env+" locks rather than discarding a failed attempt", func(t *testing.T) {
			t.Setenv("APP_ENV", env)
			locked, _ := ctrl.RecordFailedLogin(ctx, "victim@scandrix.internal")
			if !locked {
				t.Fatalf("%s: a failed attempt that cannot be counted anywhere shared must not be discarded", env)
			}
		})
	}

	// Single-process environments keep the local counter, which is correct there.
	t.Run("development keeps the local counter", func(t *testing.T) {
		t.Setenv("APP_ENV", "development")
		if locked, _ := ctrl.IsAccountLocked(ctx, "victim@scandrix.internal"); locked {
			t.Error("development may serve lockout state locally")
		}
	})
}

// The login route must not succeed while the brute-force control is unavailable.
func TestLoginDeniedWithoutRateLimitStoreInProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	ctrl := NewAuthController(auth.NewAuthenticator("test-jwt-secret-key-123456789012"), nil)

	rec := performLogin(t, ctrl, "nobody@scandrix.internal")
	if rec.Code == 200 {
		t.Fatalf("login must not succeed while rate limiting is unavailable, got 200: %s", rec.Body.String())
	}
	if rec.Code != 429 && rec.Code != 503 {
		t.Errorf("expected 429 or 503 while the limiter is unavailable, got %d: %s",
			rec.Code, rec.Body.String())
	}
}

// performLogin drives the login route with a minimal valid-shaped body.
func performLogin(t *testing.T, ctrl *AuthController, email string) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"email":"` + email + `","password":"Zq7-Kv4-Mn9-Tb2-Xc6-Rp8"}`
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	ctrl.Routes().ServeHTTP(rec, req)
	return rec
}
