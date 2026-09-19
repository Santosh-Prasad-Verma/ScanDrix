package middleware_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/middleware"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/cache/limiter"
	"github.com/scandrix/backend/pkg/models"
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
	if !strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Errorf("expected CSP frame-ancestors 'none', got %s", w.Header().Get("Content-Security-Policy"))
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

func TestRateLimitMiddleware(t *testing.T) {
	tb := limiter.NewTokenBucketLimiter(limiter.RateLimitConfig{
		Capacity:         2,
		RefillRatePerSec: 0.1,
	})
	defer tb.Stop()

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	limiterHandler := middleware.RateLimit(tb)(nextHandler)

	// First 2 requests should succeed
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
		req.RemoteAddr = "203.0.113.195:12345"
		w := httptest.NewRecorder()
		limiterHandler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK on request %d, got %d", i+1, w.Code)
		}
	}

	// Third request from same IP should be 429 Too Many Requests
	reqThrottled := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	reqThrottled.RemoteAddr = "203.0.113.195:12345"
	wThrottled := httptest.NewRecorder()
	limiterHandler.ServeHTTP(wThrottled, reqThrottled)
	if wThrottled.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests, got %d", wThrottled.Code)
	}

	// Probe endpoints (/healthz) should bypass rate limiting
	reqHealth := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	reqHealth.RemoteAddr = "203.0.113.195:12345"
	wHealth := httptest.NewRecorder()
	limiterHandler.ServeHTTP(wHealth, reqHealth)
	if wHealth.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on /healthz bypass, got %d", wHealth.Code)
	}
}

func TestExtractClientIPTrustedProxySecurity(t *testing.T) {
	// Configure only 10.50.0.0/16 as trusted proxy
	middleware.SetTrustedProxies([]string{"10.50.0.0/16"})

	// 1. Untrusted direct client attempting to spoof X-Forwarded-For
	reqUntrusted := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	reqUntrusted.RemoteAddr = "192.168.1.100:12345"
	reqUntrusted.Header.Set("X-Forwarded-For", "8.8.8.8, 1.1.1.1")
	reqUntrusted.Header.Set("X-Real-IP", "8.8.8.8")

	ip := middleware.ExtractClientIP(reqUntrusted)
	if ip != "192.168.1.100" {
		t.Fatalf("expected untrusted IP to ignore spoofed headers, got %s", ip)
	}

	// 2. Trusted proxy forwarding client IP
	reqTrusted := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	reqTrusted.RemoteAddr = "10.50.1.10:12345"
	reqTrusted.Header.Set("X-Forwarded-For", "203.0.113.50, 10.50.1.20")

	ipTrusted := middleware.ExtractClientIP(reqTrusted)
	if ipTrusted != "203.0.113.50" {
		t.Fatalf("expected trusted proxy to extract real client IP 203.0.113.50, got %s", ipTrusted)
	}
}

func TestExtractClientIPPrivateProxyExpansion(t *testing.T) {
	middleware.ResetTrustedProxies()
	defer middleware.ResetTrustedProxies()

	t.Setenv("TRUST_PRIVATE_PROXIES", "true")

	// Direct connecting peer is an internal VPC address (e.g. AWS ALB 10.0.1.5)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.RemoteAddr = "10.0.1.5:4321"
	req.Header.Set("X-Forwarded-For", "198.51.100.25, 10.0.1.20")

	ip := middleware.ExtractClientIP(req)
	if ip != "198.51.100.25" {
		t.Fatalf("expected private proxy expansion to extract client IP 198.51.100.25, got %s", ip)
	}
}

func TestExtractClientIPCloudflareHeader(t *testing.T) {
	middleware.ResetTrustedProxies()
	defer middleware.ResetTrustedProxies()

	// Direct connecting peer is trusted loopback/ingress
	middleware.SetTrustedProxies([]string{"127.0.0.1/32"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.RemoteAddr = "127.0.0.1:8080"
	req.Header.Set("CF-Connecting-IP", "203.0.113.99")
	req.Header.Set("X-Forwarded-For", "198.51.100.1")

	ip := middleware.ExtractClientIP(req)
	if ip != "203.0.113.99" {
		t.Fatalf("expected CF-Connecting-IP priority to return 203.0.113.99, got %s", ip)
	}
}

func TestCSRFProtectionRejectsXRequestedWithBypass(t *testing.T) {
	csrfHandler := middleware.CSRFProtection([]string{"http://localhost:3000"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	// Attempt to bypass CSRF on a state-changing POST from evil origin using X-Requested-With
	reqEvil := httptest.NewRequest(http.MethodPost, "/api/v1/sensitive", nil)
	reqEvil.Header.Set("Origin", "http://evil-attacker.com")
	reqEvil.Header.Set("X-Requested-With", "XMLHttpRequest")
	wEvil := httptest.NewRecorder()

	csrfHandler.ServeHTTP(wEvil, reqEvil)
	if wEvil.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for evil origin even with X-Requested-With, got %d", wEvil.Code)
	}
}

type mockTierResolver struct {
	plans map[uuid.UUID]*models.WorkspacePlanDetails
}

func (m *mockTierResolver) GetWorkspacePlanDetails(ctx context.Context, wsID uuid.UUID) (*models.WorkspacePlanDetails, error) {
	if p, ok := m.plans[wsID]; ok {
		return p, nil
	}
	return &models.WorkspacePlanDetails{PlanTier: "COMMUNITY"}, nil
}

func TestTierRateLimitMiddleware(t *testing.T) {
	limiter := middleware.NewTierAwareRateLimiter()
	defer limiter.Stop()

	communityWs := uuid.New()
	teamWs := uuid.New()

	resolver := &mockTierResolver{
		plans: map[uuid.UUID]*models.WorkspacePlanDetails{
			communityWs: {PlanTier: "COMMUNITY"},
			teamWs:      {PlanTier: "TEAM"},
		},
	}

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	handler := middleware.TierRateLimit(limiter, resolver)(nextHandler)

	// 1. Unauthenticated request without workspace context bypasses tier limiter
	reqNoAuth := httptest.NewRequest(http.MethodGet, "/api/v1/reviews", nil)
	wNoAuth := httptest.NewRecorder()
	handler.ServeHTTP(wNoAuth, reqNoAuth)
	if wNoAuth.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for request without workspace context, got %d", wNoAuth.Code)
	}

	// 2. Healthz bypasses tier limiter
	reqHealth := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	reqHealth = reqHealth.WithContext(auth.WithWorkspaceContext(reqHealth.Context(), communityWs))
	wHealth := httptest.NewRecorder()
	handler.ServeHTTP(wHealth, reqHealth)
	if wHealth.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /healthz bypass, got %d", wHealth.Code)
	}

	// 3. Community Tier allows up to 120 requests
	for i := 0; i < 120; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/reviews", nil)
		req = req.WithContext(auth.WithWorkspaceContext(req.Context(), communityWs))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK on community request %d, got %d", i+1, w.Code)
		}
	}

	// 4. 121st request for Community Tier receives 429 Too Many Requests
	reqCommunityExceeded := httptest.NewRequest(http.MethodGet, "/api/v1/reviews", nil)
	reqCommunityExceeded = reqCommunityExceeded.WithContext(auth.WithWorkspaceContext(reqCommunityExceeded.Context(), communityWs))
	wCommunityExceeded := httptest.NewRecorder()
	handler.ServeHTTP(wCommunityExceeded, reqCommunityExceeded)
	if wCommunityExceeded.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests for community tier on request 121, got %d", wCommunityExceeded.Code)
	}

	var errResp map[string]any
	if err := json.Unmarshal(wCommunityExceeded.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if errResp["tier"] != "COMMUNITY" {
		t.Fatalf("expected tier COMMUNITY in error response, got %v", errResp["tier"])
	}

	// 5. Team Tier with higher capacity (1200) allows 121 requests without throttling
	for i := 0; i < 121; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/reviews", nil)
		req = req.WithContext(auth.WithWorkspaceContext(req.Context(), teamWs))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK on team tier request %d (higher capacity), got %d", i+1, w.Code)
		}
	}
}

