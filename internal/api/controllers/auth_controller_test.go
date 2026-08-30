package controllers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/cache/limiter"
	"github.com/scandrix/backend/pkg/models"
)

func TestAuthRateLimiterThrottling(t *testing.T) {
	authService := auth.NewAuthenticator("test-secret-key-1234567890123456")
	ctrl := controllers.NewAuthController(authService, nil)

	// Configure a tight rate limiter for testing: Capacity = 3, Refill = 0.01/sec
	testLimiter := limiter.NewTokenBucketLimiter(limiter.RateLimitConfig{
		Capacity:          3,
		RefillRatePerSec:  0.01,
		ExpirationTimeout: 1 * time.Minute,
	})
	ctrl.SetRateLimiter(testLimiter)
	router := ctrl.Routes()

	loginBody := `{"email":"admin@example.com","password":"secretpassword"}`

	// 1. Send 3 valid requests -> Should all pass through without 429
	for i := 1; i <= 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBufferString(loginBody))
		req.RemoteAddr = "192.168.1.100:54321"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code == http.StatusTooManyRequests {
			t.Fatalf("request %d should have been allowed, got 429 Too Many Requests", i)
		}
	}

	// 2. Fourth request from the same IP -> Must be blocked with 429 Too Many Requests
	reqBlocked := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBufferString(loginBody))
	reqBlocked.RemoteAddr = "192.168.1.100:54321"
	wBlocked := httptest.NewRecorder()
	router.ServeHTTP(wBlocked, reqBlocked)

	if wBlocked.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 4th request to be blocked with 429 Too Many Requests, got %d", wBlocked.Code)
	}

	retryAfter := wBlocked.Header().Get("Retry-After")
	if retryAfter == "" {
		t.Error("expected Retry-After header to be set on 429 response")
	}

	// 3. Request from a different IP -> Should still be allowed (IP isolation)
	reqOtherIP := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBufferString(loginBody))
	reqOtherIP.RemoteAddr = "192.168.1.200:54321"
	wOther := httptest.NewRecorder()
	router.ServeHTTP(wOther, reqOtherIP)

	if wOther.Code == http.StatusTooManyRequests {
		t.Errorf("request from different IP should be allowed, got 429")
	}
}

func TestAuthFailClosedWhenRepoNil(t *testing.T) {
	authService := auth.NewAuthenticator("test-jwt-secret-key-123456789012")
	ctrl := controllers.NewAuthController(authService, nil)
	router := ctrl.Routes()

	// 1. /login must fail with 503 Service Unavailable when repo is nil
	loginPayload := `{"email":"engineer@company.com","password":"StrongPassword123!"}`
	reqLogin := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBufferString(loginPayload))
	wLogin := httptest.NewRecorder()
	router.ServeHTTP(wLogin, reqLogin)

	if wLogin.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 Service Unavailable from /login when repo is nil, got %d: %s", wLogin.Code, wLogin.Body.String())
	}

	// 2. /register must fail with 503 Service Unavailable when repo is nil
	regPayload := `{"email":"engineer@company.com","password":"StrongPassword123!","display_name":"Engineer"}`
	reqReg := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBufferString(regPayload))
	wReg := httptest.NewRecorder()
	router.ServeHTTP(wReg, reqReg)

	if wReg.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 Service Unavailable from /register when repo is nil, got %d: %s", wReg.Code, wReg.Body.String())
	}

	// 3. /refresh must fail with 503 Service Unavailable when repo is nil
	refreshPayload := `{"refresh_token":"some_refresh_token"}`
	reqRefresh := httptest.NewRequest(http.MethodPost, "/refresh", bytes.NewBufferString(refreshPayload))
	wRefresh := httptest.NewRecorder()
	router.ServeHTTP(wRefresh, reqRefresh)

	if wRefresh.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 Service Unavailable from /refresh when repo is nil, got %d: %s", wRefresh.Code, wRefresh.Body.String())
	}
}

func TestAuthProtectedEndpoints(t *testing.T) {
	authService := auth.NewAuthenticator("test-jwt-secret-key-123456789012")
	ctrl := controllers.NewAuthController(authService, nil)
	router := ctrl.Routes()

	userID := uuid.New()
	wsID := uuid.New()
	accessToken, refreshToken, err := authService.GenerateTokenPair(userID, wsID, models.RoleOwner)
	if err != nil {
		t.Fatalf("failed generating token pair: %v", err)
	}

	// 1. Call protected /me endpoint with valid access token -> Expect 200 OK
	reqMe := httptest.NewRequest(http.MethodGet, "/me", nil)
	reqMe.Header.Set("Authorization", "Bearer "+accessToken)
	wMe := httptest.NewRecorder()
	router.ServeHTTP(wMe, reqMe)

	if wMe.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from /me with valid JWT, got %d: %s", wMe.Code, wMe.Body.String())
	}

	// 2. Call /me with tampered token -> Expect 401 Unauthorized
	reqTampered := httptest.NewRequest(http.MethodGet, "/me", nil)
	reqTampered.Header.Set("Authorization", "Bearer "+accessToken+"invalid")
	wTampered := httptest.NewRecorder()
	router.ServeHTTP(wTampered, reqTampered)

	if wTampered.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for tampered token, got %d", wTampered.Code)
	}

	// 3. Call /logout with refresh token
	logoutPayload, _ := json.Marshal(dtos.LogoutRequest{
		RefreshToken: refreshToken,
	})
	reqLogout := httptest.NewRequest(http.MethodPost, "/logout", bytes.NewReader(logoutPayload))
	wLogout := httptest.NewRecorder()
	router.ServeHTTP(wLogout, reqLogout)

	if wLogout.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from /logout, got %d", wLogout.Code)
	}
}
