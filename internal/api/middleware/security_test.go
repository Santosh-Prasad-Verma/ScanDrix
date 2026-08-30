package middleware_test

import (
	"net/http"
	"net/http/httptest"
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
	if w.Header().Get("Content-Security-Policy") != "default-src 'self'" {
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
