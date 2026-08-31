package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/api/middleware"
)

func TestSecurityHeadersMiddleware(t *testing.T) {
	handler := middleware.SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("expected X-Content-Type-Options: nosniff, got %s", w.Header().Get("X-Content-Type-Options"))
	}
	if w.Header().Get("X-Frame-Options") != "DENY" {
		t.Errorf("expected X-Frame-Options: DENY, got %s", w.Header().Get("X-Frame-Options"))
	}
	if w.Header().Get("X-XSS-Protection") != "1; mode=block" {
		t.Errorf("expected X-XSS-Protection: 1; mode=block, got %s", w.Header().Get("X-XSS-Protection"))
	}
	if w.Header().Get("Strict-Transport-Security") == "" {
		t.Errorf("expected Strict-Transport-Security header to be present")
	}
	if !strings.HasPrefix(w.Header().Get("Content-Security-Policy"), "default-src 'self'") {
		t.Errorf("expected CSP header, got %s", w.Header().Get("Content-Security-Policy"))
	}
}

func TestCORSMiddlewarePreflightAndOrigin(t *testing.T) {
	corsHandler := middleware.CORS(middleware.DefaultCORSConfig())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// 1. Preflight OPTIONS request from allowed origin
	reqOptions := httptest.NewRequest(http.MethodOptions, "/api/v1/resource", nil)
	reqOptions.Header.Set("Origin", "http://localhost:3000")
	wOptions := httptest.NewRecorder()
	corsHandler.ServeHTTP(wOptions, reqOptions)

	if wOptions.Code != http.StatusNoContent {
		t.Fatalf("expected 204 No Content for OPTIONS preflight, got %d", wOptions.Code)
	}
	if wOptions.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Fatalf("expected allow origin header for localhost:3000, got %s", wOptions.Header().Get("Access-Control-Allow-Origin"))
	}
	if wOptions.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("expected allow credentials: true")
	}

	// 2. Disallowed Origin
	reqDisallowed := httptest.NewRequest(http.MethodGet, "/api/v1/resource", nil)
	reqDisallowed.Header.Set("Origin", "http://evil-attacker.com")
	wDisallowed := httptest.NewRecorder()
	corsHandler.ServeHTTP(wDisallowed, reqDisallowed)

	if wDisallowed.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("expected empty allow origin header for untrusted origin, got %s", wDisallowed.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestCSRFProtectionMiddleware(t *testing.T) {
	csrfHandler := middleware.CSRFProtection([]string{"http://localhost:3000"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	// 1. Safe GET request is allowed
	reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/resource", nil)
	wGet := httptest.NewRecorder()
	csrfHandler.ServeHTTP(wGet, reqGet)
	if wGet.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for GET, got %d", wGet.Code)
	}

	// 2. Cross-site browser POST without authorization is blocked
	reqCrossSite := httptest.NewRequest(http.MethodPost, "/api/v1/resource", nil)
	reqCrossSite.Header.Set("Sec-Fetch-Site", "cross-site")
	wCrossSite := httptest.NewRecorder()
	csrfHandler.ServeHTTP(wCrossSite, reqCrossSite)
	if wCrossSite.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for cross-site POST, got %d", wCrossSite.Code)
	}

	// 3. POST with Authorization Bearer header is allowed (API / CLI client)
	reqAuth := httptest.NewRequest(http.MethodPost, "/api/v1/resource", nil)
	reqAuth.Header.Set("Authorization", "Bearer eyJhbGciOi...")
	wAuth := httptest.NewRecorder()
	csrfHandler.ServeHTTP(wAuth, reqAuth)
	if wAuth.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for authorized POST, got %d", wAuth.Code)
	}

	// 4. POST from allowed origin is permitted
	reqAllowed := httptest.NewRequest(http.MethodPost, "/api/v1/resource", nil)
	reqAllowed.Header.Set("Origin", "http://localhost:3000")
	wAllowed := httptest.NewRecorder()
	csrfHandler.ServeHTTP(wAllowed, reqAllowed)
	if wAllowed.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for allowed origin POST, got %d", wAllowed.Code)
	}

	// 5. POST from disallowed origin is blocked
	reqEvil := httptest.NewRequest(http.MethodPost, "/api/v1/resource", nil)
	reqEvil.Header.Set("Origin", "http://evil-attacker.com")
	wEvil := httptest.NewRecorder()
	csrfHandler.ServeHTTP(wEvil, reqEvil)
	if wEvil.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for evil origin POST, got %d", wEvil.Code)
	}
}

