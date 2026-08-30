package controllers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/cache/limiter"
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

func TestAuthTokenRotationAndLogout(t *testing.T) {
	authService := auth.NewAuthenticator("test-jwt-secret-key-123456789012")
	ctrl := controllers.NewAuthController(authService, nil)
	router := ctrl.Routes()

	// 1. Register a new user -> Receive access token + refresh token
	regPayload := `{"email":"engineer@company.com","password":"StrongPassword123!","display_name":"Engineer"}`
	reqReg := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBufferString(regPayload))
	wReg := httptest.NewRecorder()
	router.ServeHTTP(wReg, reqReg)

	if wReg.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", wReg.Code, wReg.Body.String())
	}

	var authResp dtos.AuthTokenResponse
	if err := json.NewDecoder(wReg.Body).Decode(&authResp); err != nil {
		t.Fatalf("failed decoding auth response: %v", err)
	}

	if authResp.AccessToken == "" || authResp.RefreshToken == "" {
		t.Fatal("expected non-empty access and refresh tokens")
	}

	// 2. Call /refresh with the refresh token -> Expect new rotated tokens
	refreshPayload, _ := json.Marshal(dtos.RefreshTokenRequest{
		RefreshToken: authResp.RefreshToken,
	})
	reqRefresh := httptest.NewRequest(http.MethodPost, "/refresh", bytes.NewReader(refreshPayload))
	wRefresh := httptest.NewRecorder()
	router.ServeHTTP(wRefresh, reqRefresh)

	if wRefresh.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from /refresh, got %d: %s", wRefresh.Code, wRefresh.Body.String())
	}

	var rotatedResp dtos.AuthTokenResponse
	if err := json.NewDecoder(wRefresh.Body).Decode(&rotatedResp); err != nil {
		t.Fatalf("failed decoding rotated response: %v", err)
	}

	if rotatedResp.AccessToken == "" || rotatedResp.RefreshToken == "" {
		t.Fatal("expected new rotated tokens from /refresh")
	}

	// 3. Call protected /me endpoint with rotated access token -> Expect 200 OK
	reqMe := httptest.NewRequest(http.MethodGet, "/me", nil)
	reqMe.Header.Set("Authorization", "Bearer "+rotatedResp.AccessToken)
	wMe := httptest.NewRecorder()
	router.ServeHTTP(wMe, reqMe)

	if wMe.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from /me with valid JWT, got %d: %s", wMe.Code, wMe.Body.String())
	}

	// 4. Call /me with tampered token -> Expect 401 Unauthorized
	reqTampered := httptest.NewRequest(http.MethodGet, "/me", nil)
	reqTampered.Header.Set("Authorization", "Bearer "+rotatedResp.AccessToken+"invalid")
	wTampered := httptest.NewRecorder()
	router.ServeHTTP(wTampered, reqTampered)

	if wTampered.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for tampered token, got %d", wTampered.Code)
	}

	// 5. Call /logout with refresh token
	logoutPayload, _ := json.Marshal(dtos.LogoutRequest{
		RefreshToken: rotatedResp.RefreshToken,
	})
	reqLogout := httptest.NewRequest(http.MethodPost, "/logout", bytes.NewReader(logoutPayload))
	wLogout := httptest.NewRecorder()
	router.ServeHTTP(wLogout, reqLogout)

	if wLogout.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from /logout, got %d", wLogout.Code)
	}
}
