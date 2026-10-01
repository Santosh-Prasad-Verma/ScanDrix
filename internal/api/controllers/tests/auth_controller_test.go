package controllers_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/auth/cliauth"
	"github.com/scandrix/backend/internal/auth/clitokens"
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

	loginBody := `{"email":"admin@example.com","password":"Zq7-Kv4-Mn9-Tb2-Xc6-Rp8"}`

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
	loginPayload := `{"email":"engineer@company.com","password":"Zq7-Kv4-Mn9-Tb2-Xc6-Rp8"}`
	reqLogin := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBufferString(loginPayload))
	wLogin := httptest.NewRecorder()
	router.ServeHTTP(wLogin, reqLogin)

	if wLogin.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 Service Unavailable from /login when repo is nil, got %d: %s", wLogin.Code, wLogin.Body.String())
	}

	// 2. /register must fail with 503 Service Unavailable when repo is nil
	regPayload := `{"email":"engineer@company.com","password":"Zq7-Kv4-Mn9-Tb2-Xc6-Rp8","display_name":"Engineer"}`
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

	// 3. Call /logout with refresh token.
	//
	// This controller was built with a nil repository, and logout cannot end a
	// session without one: the access token has to be revoked server-side, and
	// with no store the only honest answer is "the session could not be ended".
	// It previously returned 200 here, which told the caller they were logged
	// out while their bearer token kept working (AUDIT_REMEDIATION.md F-18).
	// Cookies are still cleared, which is why the cookie-flag test above can
	// keep exercising this endpoint; see logout_revocation_test.go for the
	// successful path with a real repository.
	logoutPayload, _ := json.Marshal(dtos.LogoutRequest{
		RefreshToken: refreshToken,
	})
	reqLogout := httptest.NewRequest(http.MethodPost, "/logout", bytes.NewReader(logoutPayload))
	wLogout := httptest.NewRecorder()
	router.ServeHTTP(wLogout, reqLogout)

	if wLogout.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 from /logout when no repository is available, got %d: %s",
			wLogout.Code, wLogout.Body.String())
	}
	if len(wLogout.Result().Cookies()) == 0 {
		t.Error("logout must still clear the session cookies even when it cannot revoke")
	}
}

func TestCLILoopbackFlowEndpoints(t *testing.T) {
	authService := auth.NewAuthenticator("test-jwt-secret-key-123456789012")
	ctrl := controllers.NewAuthController(authService, nil)
	loopbackMgr := cliauth.NewLoopbackManager("http://localhost:3000")
	ctrl.SetLoopbackManager(loopbackMgr)
	router := ctrl.Routes()

	// 1. Initiate loopback login
	initPayload := `{"port": 8085}`
	reqInit := httptest.NewRequest(http.MethodPost, "/cli/loopback/initiate", bytes.NewBufferString(initPayload))
	wInit := httptest.NewRecorder()
	router.ServeHTTP(wInit, reqInit)

	if wInit.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from /cli/loopback/initiate, got %d: %s", wInit.Code, wInit.Body.String())
	}

	var initRes struct {
		State           string `json:"state"`
		VerificationURI string `json:"verification_uri"`
	}
	if err := json.Unmarshal(wInit.Body.Bytes(), &initRes); err != nil || initRes.State == "" {
		t.Fatalf("failed parsing initiate response: %v", err)
	}

	// 2. Poll loopback while still pending
	reqPoll := httptest.NewRequest(http.MethodGet, "/cli/loopback/poll?state="+initRes.State, nil)
	wPoll := httptest.NewRecorder()
	router.ServeHTTP(wPoll, reqPoll)

	if wPoll.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from /cli/loopback/poll, got %d", wPoll.Code)
	}
	var pollRes struct {
		Status string `json:"status"`
	}
	_ = json.Unmarshal(wPoll.Body.Bytes(), &pollRes)
	if pollRes.Status != "pending" {
		t.Fatalf("expected pending status, got %s", pollRes.Status)
	}

	// 3. Approve loopback authorization via CompleteLoopback
	user := &models.AccountProfile{
		ID:          uuid.New(),
		WorkspaceID: uuid.New(),
		Email:       "engineer@example.com",
		Role:        models.RoleOwner,
	}
	err := loopbackMgr.CompleteLoopback(reqPoll.Context(), initRes.State, "mock-access-token", "mock-refresh-token", user)
	if err != nil {
		t.Fatalf("failed completing loopback: %v", err)
	}

	// 4. Poll again -> should be completed with tokens
	wPoll2 := httptest.NewRecorder()
	reqPoll2 := httptest.NewRequest(http.MethodGet, "/cli/loopback/poll?state="+initRes.State, nil)
	router.ServeHTTP(wPoll2, reqPoll2)

	if wPoll2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on poll after approval, got %d", wPoll2.Code)
	}
	var pollRes2 struct {
		Status      string `json:"status"`
		AccessToken string `json:"access_token"`
		UserEmail   string `json:"user_email"`
	}
	_ = json.Unmarshal(wPoll2.Body.Bytes(), &pollRes2)
	if pollRes2.Status != "completed" || pollRes2.AccessToken != "mock-access-token" || pollRes2.UserEmail != "engineer@example.com" {
		t.Fatalf("unexpected completed poll response: %+v", pollRes2)
	}
}

func TestHelpdeskSSOTokenEndpoint(t *testing.T) {
	authService := auth.NewAuthenticator("test-jwt-secret-key-123456789012")
	ctrl := controllers.NewAuthController(authService, nil)

	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed generating test RSA key: %v", err)
	}
	helpdeskSvc := auth.NewHelpdeskTokenServiceWithKey(privKey, "scandrix-api", "scandrix-helpdesk")
	ctrl.SetHelpdeskTokenService(helpdeskSvc)
	router := ctrl.Routes()

	userID := uuid.New()
	wsID := uuid.New()
	accessToken, _, _ := authService.GenerateTokenPairWithEmail(userID, wsID, models.RoleOwner, "support-user@company.com")

	// 1. Call protected /helpdesk-token with Bearer token
	req := httptest.NewRequest(http.MethodPost, "/helpdesk-token", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from /helpdesk-token, got %d: %s", w.Code, w.Body.String())
	}

	var res struct {
		Token     string `json:"token"`
		TokenType string `json:"token_type"`
		ExpiresIn int    `json:"expires_in"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil || res.Token == "" {
		t.Fatalf("failed unmarshaling helpdesk token: %v", err)
	}
	if res.ExpiresIn != 300 || res.TokenType != "Bearer" {
		t.Errorf("expected 300s TTL and Bearer type, got expires_in=%d token_type=%s", res.ExpiresIn, res.TokenType)
	}

	// 2. Cryptographically verify the minted RS256 token against the public key
	claims, err := helpdeskSvc.VerifyHelpdeskToken(res.Token)
	if err != nil {
		t.Fatalf("failed verifying minted helpdesk token: %v", err)
	}
	if claims.Email != "support-user@company.com" || claims.Sub != userID.String() {
		t.Errorf("claims mismatch: email=%s, sub=%v", claims.Email, claims.Sub)
	}
}

func TestDeviceQuotaHardwareLimitEnforcement(t *testing.T) {
	authService := auth.NewAuthenticator("test-jwt-secret-key-123456789012")
	ctrl := controllers.NewAuthController(authService, nil)
	dfm := cliauth.NewDeviceFlowManager(nil, "http://localhost:3000")
	ctrl.SetDeviceFlowManager(dfm)

	// Set hardware limit to 2 devices
	quotaMgr := auth.NewDeviceManager(nil, 2)
	ctrl.SetDeviceManager(quotaMgr)
	router := ctrl.Routes()

	wsID := uuid.New()
	userID := uuid.New()
	token, _, _ := authService.GenerateTokenPairWithEmail(userID, wsID, models.RoleOwner, "dev@company.com")

	// 1. First device -> succeeds
	dev1, err := quotaMgr.ValidateOrRegisterDevice(httptest.NewRequest(http.MethodPost, "/", nil).Context(), wsID, "hw-mac-01", "ScanDrix-CLI")
	if err != nil || dev1 == nil {
		t.Fatalf("device 1 should succeed, got %v", err)
	}

	// 2. Second device -> succeeds
	dev2, err := quotaMgr.ValidateOrRegisterDevice(httptest.NewRequest(http.MethodPost, "/", nil).Context(), wsID, "hw-mac-02", "ScanDrix-CLI")
	if err != nil || dev2 == nil {
		t.Fatalf("device 2 should succeed, got %v", err)
	}

	// 3. Third device -> exceeds quota limit (DEVICE_LIMIT_REACHED)
	_, err = quotaMgr.ValidateOrRegisterDevice(httptest.NewRequest(http.MethodPost, "/", nil).Context(), wsID, "hw-mac-03", "ScanDrix-CLI")
	if err != auth.ErrDeviceLimitReached {
		t.Fatalf("expected ErrDeviceLimitReached, got %v", err)
	}

	// 4. Test via HTTP endpoint with X-Device-Id header on /cli/device/complete
	initRes, _ := dfm.InitiateDeviceLogin(httptest.NewRequest(http.MethodPost, "/", nil).Context(), "ScanDrix-CLI")

	completeBody := fmt.Sprintf(`{"user_code":"%s"}`, initRes.UserCode)
	reqComplete := httptest.NewRequest(http.MethodPost, "/cli/device/complete", bytes.NewBufferString(completeBody))
	reqComplete.Header.Set("Authorization", "Bearer "+token)
	reqComplete.Header.Set("X-Device-Id", "hw-mac-03")
	wComplete := httptest.NewRecorder()
	router.ServeHTTP(wComplete, reqComplete)

	if wComplete.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized with DEVICE_LIMIT_REACHED, got %d: %s", wComplete.Code, wComplete.Body.String())
	}
	var errResp map[string]any
	_ = json.Unmarshal(wComplete.Body.Bytes(), &errResp)
	if errResp["code"] != "DEVICE_LIMIT_REACHED" {
		t.Fatalf("expected code DEVICE_LIMIT_REACHED, got %v", errResp["code"])
	}
}

func TestValidateCLIKeyEndpoint(t *testing.T) {
	authService := auth.NewAuthenticator("test-jwt-secret-key-123456789012")
	ctrl := controllers.NewAuthController(authService, nil)

	// Set up TokenService
	tokenSvc := clitokens.NewTokenService(nil)
	ctrl.SetCLITokenService(tokenSvc)

	// Set up DeviceQuota
	deviceQuota := auth.NewDeviceManager(nil, 2)
	ctrl.SetDeviceManager(deviceQuota)

	router := ctrl.Routes()

	wsID := uuid.New()
	mintResp, err := tokenSvc.MintToken(httptest.NewRequest(http.MethodPost, "/", nil).Context(), clitokens.MintTokenRequest{
		WorkspaceID: wsID,
		Name:        "CI Key",
		Scopes:      []clitokens.TokenScope{clitokens.ScopeReviewRead, clitokens.ScopeReviewWrite},
		TTL:         1 * time.Hour,
	})
	if err != nil {
		t.Fatalf("failed minting token: %v", err)
	}

	t.Run("valid key via X-Team-Key header and device registration", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/cli/validate-key", nil)
		req.Header.Set("x-team-key", mintResp.Token)
		req.Header.Set("x-scandrix-device-id", "macbook-pro-m3")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
		}

		// Device token header must be returned
		devToken := w.Header().Get("x-scandrix-device-token")
		if devToken == "" {
			t.Errorf("expected x-scandrix-device-token header to be present")
		}

		var resp map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if resp["valid"] != true {
			t.Errorf("expected valid: true, got %v", resp["valid"])
		}
		if resp["organizationId"] != wsID.String() {
			t.Errorf("expected organizationId %s, got %v", wsID.String(), resp["organizationId"])
		}
	})

	t.Run("valid key via Authorization Bearer header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/cli/validate-key", nil)
		req.Header.Set("Authorization", "Bearer "+mintResp.Token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if resp["valid"] != true {
			t.Errorf("expected valid: true, got %v", resp["valid"])
		}
	})

	t.Run("valid key with JWT Bearer token", func(t *testing.T) {
		userID := uuid.New()
		jwtToken, _, _ := authService.GenerateTokenPairWithEmail(userID, wsID, models.RoleMember, "dev@scandrix.io")

		req := httptest.NewRequest(http.MethodGet, "/cli/validate-key", nil)
		req.Header.Set("Authorization", "Bearer "+jwtToken)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for valid JWT, got %d: %s", w.Code, w.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if resp["valid"] != true {
			t.Errorf("expected valid: true, got %v", resp["valid"])
		}
		if resp["email"] != "dev@scandrix.io" {
			t.Errorf("expected email dev@scandrix.io, got %v", resp["email"])
		}
	})

	t.Run("device limit reached triggers DEVICE_LIMIT_REACHED", func(t *testing.T) {
		// Register 2nd device -> ok
		req2 := httptest.NewRequest(http.MethodGet, "/cli/validate-key", nil)
		req2.Header.Set("x-team-key", mintResp.Token)
		req2.Header.Set("x-scandrix-device-id", "linux-laptop-02")
		w2 := httptest.NewRecorder()
		router.ServeHTTP(w2, req2)
		if w2.Code != http.StatusOK {
			t.Fatalf("expected device 2 to succeed, got %d", w2.Code)
		}

		// 3rd device -> exceeds limit of 2
		req3 := httptest.NewRequest(http.MethodGet, "/cli/validate-key", nil)
		req3.Header.Set("x-team-key", mintResp.Token)
		req3.Header.Set("x-scandrix-device-id", "unauthorized-pc-03")
		w3 := httptest.NewRecorder()
		router.ServeHTTP(w3, req3)

		if w3.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized, got %d", w3.Code)
		}
		var errResp map[string]any
		_ = json.Unmarshal(w3.Body.Bytes(), &errResp)
		if errResp["code"] != "DEVICE_LIMIT_REACHED" {
			t.Errorf("expected code DEVICE_LIMIT_REACHED, got %v", errResp["code"])
		}
		if errResp["valid"] != false {
			t.Errorf("expected valid: false, got %v", errResp["valid"])
		}
	})

	t.Run("invalid team key fails", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/cli/validate-key", nil)
		req.Header.Set("x-team-key", "scandrix_invalid_token_12345")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized for invalid key, got %d", w.Code)
		}
		var resp map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if resp["valid"] != false {
			t.Errorf("expected valid: false, got %v", resp["valid"])
		}
	})
}

func TestCLILoginInfoEndpoint(t *testing.T) {
	authService := auth.NewAuthenticator("test-jwt-secret-key-123456789012")
	ctrl := controllers.NewAuthController(authService, nil)

	dfm := cliauth.NewDeviceFlowManager(nil, "http://localhost:3000")
	ctrl.SetDeviceFlowManager(dfm)

	loopbackMgr := cliauth.NewLoopbackManager("http://localhost:3000")
	ctrl.SetLoopbackManager(loopbackMgr)

	router := ctrl.Routes()

	// 1. Initiate loopback session
	initLoopback, err := loopbackMgr.InitLoopback(httptest.NewRequest(http.MethodPost, "/", nil).Context(), 8085, "ScanDrix-CLI")
	if err != nil {
		t.Fatalf("failed init loopback: %v", err)
	}

	// Query login-info by state
	reqState := httptest.NewRequest(http.MethodGet, "/cli/auth/login-info?state="+initLoopback.State, nil)
	wState := httptest.NewRecorder()
	router.ServeHTTP(wState, reqState)

	if wState.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", wState.Code, wState.Body.String())
	}
	var stateResp map[string]any
	_ = json.Unmarshal(wState.Body.Bytes(), &stateResp)
	if stateResp["found"] != true {
		t.Errorf("expected found: true, got %v", stateResp["found"])
	}
	if stateResp["mode"] != "loopback" {
		t.Errorf("expected mode: loopback, got %v", stateResp["mode"])
	}

	// 2. Initiate device flow session
	initDevice, err := dfm.InitiateDeviceLogin(httptest.NewRequest(http.MethodPost, "/", nil).Context(), "ScanDrix-CLI")
	if err != nil {
		t.Fatalf("failed init device flow: %v", err)
	}

	// Query login-info by device_code.
	//
	// This used to query by user_code, which is short and human-typable, so an
	// unauthenticated caller could enumerate codes and confirm which existed
	// (AUDIT_REMEDIATION.md F-31). device_code is the high-entropy secret the CLI
	// already holds and is what login-poll requires.
	reqCode := httptest.NewRequest(http.MethodGet, "/cli/auth/login-info?device_code="+initDevice.DeviceCode, nil)
	wCode := httptest.NewRecorder()
	router.ServeHTTP(wCode, reqCode)

	if wCode.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", wCode.Code, wCode.Body.String())
	}
	var codeResp map[string]any
	_ = json.Unmarshal(wCode.Body.Bytes(), &codeResp)
	if codeResp["found"] != true {
		t.Errorf("expected found: true, got %v", codeResp["found"])
	}
	if codeResp["mode"] != "device" {
		t.Errorf("expected mode: device, got %v", codeResp["mode"])
	}
	// The browser user agent was a fingerprinting aid the CLI does not need.
	if _, leaked := codeResp["userAgent"]; leaked {
		t.Error("login-info must not return the session user agent")
	}

	// F-31 regression: a user code on its own must reveal nothing at all.
	reqUserCode := httptest.NewRequest(http.MethodGet, "/cli/auth/login-info?user_code="+initDevice.UserCode, nil)
	wUserCode := httptest.NewRecorder()
	router.ServeHTTP(wUserCode, reqUserCode)
	if wUserCode.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when only a user_code is supplied, got %d: %s",
			wUserCode.Code, wUserCode.Body.String())
	}
	var userCodeResp map[string]any
	_ = json.Unmarshal(wUserCode.Body.Bytes(), &userCodeResp)
	if userCodeResp["found"] == true {
		t.Error("a user code must not be sufficient to query login-info")
	}

	// No credential at all is likewise rejected rather than answered.
	reqNone := httptest.NewRequest(http.MethodGet, "/cli/auth/login-info", nil)
	wNone := httptest.NewRecorder()
	router.ServeHTTP(wNone, reqNone)
	if wNone.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 with no credential, got %d: %s", wNone.Code, wNone.Body.String())
	}

	// 3. Query with nonexistent state -> found: false
	reqNotFound := httptest.NewRequest(http.MethodGet, "/cli/auth/login-info?state=nonexistent-state-12345", nil)
	wNotFound := httptest.NewRecorder()
	router.ServeHTTP(wNotFound, reqNotFound)

	if wNotFound.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", wNotFound.Code)
	}
	var notFoundResp map[string]any
	_ = json.Unmarshal(wNotFound.Body.Bytes(), &notFoundResp)
	if notFoundResp["found"] != false {
		t.Errorf("expected found: false, got %v", notFoundResp["found"])
	}
}

func TestSSOCheckEndpoint(t *testing.T) {
	authService := auth.NewAuthenticator("test-jwt-secret-key-123456789012")
	ctrl := controllers.NewAuthController(authService, nil)
	router := ctrl.Routes()

	t.Run("empty domain returns active false", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/sso/check", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", w.Code)
		}
		var resp map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if resp["active"] != false {
			t.Errorf("expected active: false, got %v", resp["active"])
		}
	})

	t.Run("domain check query parameter", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/sso/check?domain=acme-corp.com", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", w.Code)
		}
	})
}

func TestEmailConfirmationEndpoint(t *testing.T) {
	authService := auth.NewAuthenticator("test-jwt-secret-key-123456789012")
	ctrl := controllers.NewAuthController(authService, nil)
	ctrl.SetJWTSecret("test-jwt-secret-key-123456789012")
	router := ctrl.Routes()

	userID := uuid.New()
	token, err := auth.CreateEmailConfirmationToken(userID, "dev@scandrix.io", "test-jwt-secret-key-123456789012", 15*time.Minute)
	if err != nil {
		t.Fatalf("failed creating token: %v", err)
	}

	t.Run("valid token confirms email", func(t *testing.T) {
		body := fmt.Sprintf(`{"token":"%s"}`, token)
		req := httptest.NewRequest(http.MethodPost, "/confirm-email", bytes.NewBufferString(body))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
		}
		var resp map[string]string
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if resp["message"] != "Email confirmed successfully" {
			t.Errorf("expected 'Email confirmed successfully', got %s", resp["message"])
		}
	})

	t.Run("invalid token fails confirmation", func(t *testing.T) {
		body := `{"token":"invalid-tampered-token"}`
		req := httptest.NewRequest(http.MethodPost, "/confirm-email", bytes.NewBufferString(body))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized, got %d", w.Code)
		}
	})

	t.Run("resend email confirmation", func(t *testing.T) {
		body := `{"email":"dev@scandrix.io"}`
		req := httptest.NewRequest(http.MethodPost, "/resend-email", bytes.NewBufferString(body))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", w.Code)
		}
		var resp map[string]string
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if resp["message"] != "Verification email sent" {
			t.Errorf("expected 'Verification email sent', got %s", resp["message"])
		}
	})
}

func TestCLIDeviceInitAndUnifiedPoll(t *testing.T) {
	authService := auth.NewAuthenticator("test-jwt-secret-key-123456789012")
	deviceFlow := cliauth.NewDeviceFlowManager(cliauth.NewInMemorySessionStore(), "http://localhost:3000")
	loopbackMgr := cliauth.NewLoopbackManager("http://localhost:3000")

	ctrl := controllers.NewAuthController(authService, nil)
	ctrl.SetDeviceFlowManager(deviceFlow)
	ctrl.SetLoopbackManager(loopbackMgr)

	r := chi.NewRouter()
	r.Post("/cli/auth/device-init", ctrl.HandleCLIDeviceInit)
	r.Post("/cli/auth/login-init", ctrl.HandleCLILoginInit)
	r.Get("/cli/auth/login-poll", ctrl.HandleCLILoginPoll)

	t.Run("device-init returns wire-compatible payload with camelCase aliases", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/cli/auth/device-init", nil)
		req.Header.Set("User-Agent", "ScanDrix-CLI/v1.0")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &resp)

		if resp["deviceCode"] == nil || resp["device_code"] == nil {
			t.Errorf("expected deviceCode and device_code, got %v", resp)
		}
		if resp["userCode"] == nil || resp["user_code"] == nil {
			t.Errorf("expected userCode and user_code, got %v", resp)
		}
		if resp["verificationUri"] == nil {
			t.Errorf("expected verificationUri, got %v", resp)
		}

		// Poll pending device code
		pollReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/cli/auth/login-poll?deviceCode=%v", resp["deviceCode"]), nil)
		pollW := httptest.NewRecorder()
		r.ServeHTTP(pollW, pollReq)

		if pollW.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for poll, got %d", pollW.Code)
		}
		var pollResp map[string]any
		_ = json.Unmarshal(pollW.Body.Bytes(), &pollResp)
		if pollResp["status"] != "pending" {
			t.Errorf("expected status: pending, got %v", pollResp["status"])
		}
	})

	t.Run("loopback login-init and login-poll flow", func(t *testing.T) {
		body := `{"port":45678}`
		initReq := httptest.NewRequest(http.MethodPost, "/cli/auth/login-init", bytes.NewBufferString(body))
		initW := httptest.NewRecorder()
		r.ServeHTTP(initW, initReq)

		if initW.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", initW.Code)
		}
		var initResp map[string]any
		_ = json.Unmarshal(initW.Body.Bytes(), &initResp)

		state, ok := initResp["state"].(string)
		if !ok || state == "" {
			t.Fatalf("expected non-empty state")
		}

		// Poll pending loopback
		pollReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/cli/auth/login-poll?state=%s", state), nil)
		pollW := httptest.NewRecorder()
		r.ServeHTTP(pollW, pollReq)

		if pollW.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", pollW.Code)
		}
		var pollResp map[string]any
		_ = json.Unmarshal(pollW.Body.Bytes(), &pollResp)
		if pollResp["status"] != "pending" {
			t.Errorf("expected pending, got %v", pollResp["status"])
		}
	})
}

func TestCLILoginComplete(t *testing.T) {
	authService := auth.NewAuthenticator("test-jwt-secret-key-123456789012")
	loopbackMgr := cliauth.NewLoopbackManager("http://localhost:3000")
	deviceFlow := cliauth.NewDeviceFlowManager(cliauth.NewInMemorySessionStore(), "http://localhost:3000")

	ctrl := controllers.NewAuthController(authService, nil)
	ctrl.SetLoopbackManager(loopbackMgr)
	ctrl.SetDeviceFlowManager(deviceFlow)

	r := chi.NewRouter()
	r.Use(authService.Middleware)
	r.Post("/cli/auth/login-complete", ctrl.HandleCLILoginComplete)

	wsID := uuid.New()
	userID := uuid.New()
	jwtToken, _, _ := authService.GenerateTokenPairWithEmail(userID, wsID, models.RoleOwner, "admin@scandrix.io")

	t.Run("complete loopback session", func(t *testing.T) {
		initRes, err := loopbackMgr.InitLoopback(context.Background(), 54321, "ScanDrix-CLI")
		if err != nil {
			t.Fatalf("init loopback failed: %v", err)
		}

		body := fmt.Sprintf(`{"state":"%s"}`, initRes.State)
		req := httptest.NewRequest(http.MethodPost, "/cli/auth/login-complete", bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer "+jwtToken)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if resp["mode"] != "loopback" {
			t.Errorf("expected mode: loopback, got %v", resp["mode"])
		}
		if resp["state"] != initRes.State {
			t.Errorf("expected state %s, got %v", initRes.State, resp["state"])
		}

		// Verify tokens deliverable via PollLoopback
		pollRes, err := loopbackMgr.PollLoopback(context.Background(), initRes.State)
		if err != nil {
			t.Fatalf("poll loopback failed: %v", err)
		}
		if pollRes.Status != cliauth.StatusCompleted || pollRes.AccessToken == "" {
			t.Errorf("expected completed with access token, got %+v", pollRes)
		}
	})

	t.Run("complete device session", func(t *testing.T) {
		initRes, err := deviceFlow.InitiateDeviceLogin(context.Background(), "ScanDrix-CLI")
		if err != nil {
			t.Fatalf("init device login failed: %v", err)
		}

		body := fmt.Sprintf(`{"userCode":"%s"}`, initRes.UserCode)
		req := httptest.NewRequest(http.MethodPost, "/cli/auth/login-complete", bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer "+jwtToken)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if resp["mode"] != "device" {
			t.Errorf("expected mode: device, got %v", resp["mode"])
		}

		// Verify poll delivered tokens
		pollRes, err := deviceFlow.PollDeviceLogin(context.Background(), initRes.DeviceCode)
		if err != nil {
			t.Fatalf("poll device failed: %v", err)
		}
		if pollRes.Status != cliauth.StatusCompleted || pollRes.AccessToken == "" {
			t.Errorf("expected completed with access token, got %+v", pollRes)
		}
	})
}

func TestPermissionsAndOrganizationParity(t *testing.T) {
	authService := auth.NewAuthenticator("test-jwt-secret-key-123456789012")
	permCtrl := controllers.NewPermissionsController()
	orgCtrl := controllers.NewOrganizationController(nil)

	r := chi.NewRouter()
	r.Use(authService.Middleware)
	r.Mount("/permissions", permCtrl.Routes())
	r.Mount("/organization", orgCtrl.Routes())

	wsID := uuid.New()
	userID := uuid.New()
	jwtToken, _, _ := authService.GenerateTokenPairWithEmail(userID, wsID, models.RoleOwner, "owner@scandrix.io")

	t.Run("permissions can-access check", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/permissions/can-access?action=read&resource=review", nil)
		req.Header.Set("Authorization", "Bearer "+jwtToken)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", w.Code)
		}
		var resp map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if resp["allowed"] != true {
			t.Errorf("expected allowed: true for owner, got %v", resp["allowed"])
		}
	})

	t.Run("organization release-track", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/organization/release-track", nil)
		req.Header.Set("Authorization", "Bearer "+jwtToken)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", w.Code)
		}
		var resp map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if resp["releaseTrack"] != "beta" {
			t.Errorf("expected beta release track, got %v", resp["releaseTrack"])
		}
	})

	t.Run("per-user repository assignment and access restriction", func(t *testing.T) {
		devUser := uuid.New()
		repo1 := uuid.New()
		repo2 := uuid.New()
		devToken, _, _ := authService.GenerateTokenPairWithEmail(devUser, wsID, models.RoleMember, "dev@scandrix.io")

		// 1. Assign repo1 to devUser
		assignBody := bytes.NewBufferString(fmt.Sprintf(`{"repo_ids":["%s"]}`, repo1.String()))
		assignReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/permissions/users/%s/repositories", devUser.String()), assignBody)
		assignReq.Header.Set("Authorization", "Bearer "+jwtToken) // Admin assigns
		assignReq.Header.Set("Content-Type", "application/json")
		assignW := httptest.NewRecorder()
		r.ServeHTTP(assignW, assignReq)

		if assignW.Code != http.StatusOK {
			t.Fatalf("assign repositories expected 200, got %d: %s", assignW.Code, assignW.Body.String())
		}

		// 2. Query assigned repos
		getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/permissions/users/%s/repositories", devUser.String()), nil)
		getReq.Header.Set("Authorization", "Bearer "+jwtToken)
		getW := httptest.NewRecorder()
		r.ServeHTTP(getW, getReq)

		if getW.Code != http.StatusOK {
			t.Fatalf("get assigned repositories expected 200, got %d", getW.Code)
		}

		// 3. Check access: devUser can access repo1
		canReq1 := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/permissions/can-access?action=read&resource=repository&repository_id=%s", repo1.String()), nil)
		canReq1.Header.Set("Authorization", "Bearer "+devToken)
		canW1 := httptest.NewRecorder()
		r.ServeHTTP(canW1, canReq1)
		var canResp1 map[string]any
		_ = json.Unmarshal(canW1.Body.Bytes(), &canResp1)
		if canResp1["allowed"] != true {
			t.Errorf("expected allowed: true for assigned repo1, got %v", canResp1["allowed"])
		}

		// 4. Check access: devUser CANNOT access repo2
		canReq2 := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/permissions/can-access?action=read&resource=repository&repository_id=%s", repo2.String()), nil)
		canReq2.Header.Set("Authorization", "Bearer "+devToken)
		canW2 := httptest.NewRecorder()
		r.ServeHTTP(canW2, canReq2)
		var canResp2 map[string]any
		_ = json.Unmarshal(canW2.Body.Bytes(), &canResp2)
		if canResp2["allowed"] != false {
			t.Errorf("expected allowed: false for unassigned repo2, got %v", canResp2["allowed"])
		}

		// 5. Revoke repo1
		revokeReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/permissions/users/%s/repositories/%s", devUser.String(), repo1.String()), nil)
		revokeReq.Header.Set("Authorization", "Bearer "+jwtToken)
		revokeW := httptest.NewRecorder()
		r.ServeHTTP(revokeW, revokeReq)

		if revokeW.Code != http.StatusOK {
			t.Fatalf("revoke repo expected 200, got %d", revokeW.Code)
		}
	})

	t.Run("legacy assign-repos and assigned-repos wire parity", func(t *testing.T) {
		devUser := uuid.New()
		repoX := uuid.New()

		// Legacy assign
		legacyBody := bytes.NewBufferString(fmt.Sprintf(`{"userId":"%s","repoIds":["%s"]}`, devUser.String(), repoX.String()))
		legacyReq := httptest.NewRequest(http.MethodPost, "/permissions/assign-repos", legacyBody)
		legacyReq.Header.Set("Authorization", "Bearer "+jwtToken)
		legacyReq.Header.Set("Content-Type", "application/json")
		legacyW := httptest.NewRecorder()
		r.ServeHTTP(legacyW, legacyReq)

		if legacyW.Code != http.StatusOK {
			t.Fatalf("legacy assign-repos expected 200, got %d: %s", legacyW.Code, legacyW.Body.String())
		}

		// Legacy get
		legacyGetReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/permissions/assigned-repos?userId=%s", devUser.String()), nil)
		legacyGetReq.Header.Set("Authorization", "Bearer "+jwtToken)
		legacyGetW := httptest.NewRecorder()
		r.ServeHTTP(legacyGetW, legacyGetReq)

		if legacyGetW.Code != http.StatusOK {
			t.Fatalf("legacy assigned-repos expected 200, got %d", legacyGetW.Code)
		}
	})
}

func TestEmailConfirmationFailsClosedWhenSecretEmpty(t *testing.T) {
	authService := auth.NewAuthenticator("test-secret-1234567890123456")
	ctrl := controllers.NewAuthController(authService, nil)
	// Explicitly configure empty JWT secret to test fail-closed behavior (no fallback secret allowed)
	ctrl.SetJWTSecret("")
	router := ctrl.Routes()

	body := `{"token":"crafted.email.confirmation.token"}`
	req := httptest.NewRequest(http.MethodPost, "/confirm-email", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Must return 503 Service Unavailable, not accept fallback "scandrix-default-jwt-secret"
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 Service Unavailable when secret is empty, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHeaderSpoofingRateLimitIsolation(t *testing.T) {
	authService := auth.NewAuthenticator("test-secret-1234567890123456")
	ctrl := controllers.NewAuthController(authService, nil)

	testLimiter := limiter.NewTokenBucketLimiter(limiter.RateLimitConfig{
		Capacity:          2,
		RefillRatePerSec:  0.01,
		ExpirationTimeout: 1 * time.Minute,
	})
	ctrl.SetRateLimiter(testLimiter)
	router := ctrl.Routes()

	loginBody := `{"email":"admin@example.com","password":"Zq7-Kv4-Mn9-Tb2-Xc6-Rp8"}`

	// 1. Send 2 requests from client IP 198.51.100.50
	for i := 1; i <= 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBufferString(loginBody))
		req.RemoteAddr = "198.51.100.50:54321"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code == http.StatusTooManyRequests {
			t.Fatalf("request %d should have passed, got 429", i)
		}
	}

	// 2. Third request from 198.51.100.50 attempting to spoof X-Real-IP and X-Forwarded-For
	reqSpoofed := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBufferString(loginBody))
	reqSpoofed.RemoteAddr = "198.51.100.50:54321"
	reqSpoofed.Header.Set("X-Real-IP", "203.0.113.199")
	reqSpoofed.Header.Set("X-Forwarded-For", "203.0.113.199")
	wSpoofed := httptest.NewRecorder()
	router.ServeHTTP(wSpoofed, reqSpoofed)

	// Must be blocked because untrusted socket connection cannot spoof IP headers
	if wSpoofed.Code != http.StatusTooManyRequests {
		t.Fatalf("expected request with spoofed IP header from untrusted peer to be blocked with 429, got %d", wSpoofed.Code)
	}
}

func TestStandardPasswordVerification(t *testing.T) {
	pw := "CorrectHorseBatteryStaple123!"
	hashed, err := auth.HashPassword(pw)
	if err != nil {
		t.Fatalf("failed hashing password: %v", err)
	}

	if !auth.VerifyPassword(pw, hashed) {
		t.Fatal("expected valid password to verify successfully")
	}

	if auth.VerifyPassword("WrongPassword!", hashed) {
		t.Fatal("expected wrong password to return false")
	}

	if auth.VerifyPassword(pw, "") {
		t.Fatal("expected empty hash to return false")
	}
}

func TestRegisterDisposableEmailRejected(t *testing.T) {
	authService := auth.NewAuthenticator("test-secret-key-1234567890123456")
	ctrl := controllers.NewAuthController(authService, nil)
	router := ctrl.Routes()

	disposableEmails := []string{
		"bot@mailinator.com",
		"spam@tempmail.com",
		"attacker@10minutemail.com",
		"sybil@guerrillamail.com",
		"throwaway@sharklasers.com",
	}

	for idx, email := range disposableEmails {
		t.Run(email, func(t *testing.T) {
			payload := fmt.Sprintf(`{"email":"%s","password":"Zq7-Kv4-Mn9-Tb2-Xc6-Rp8","workspace_name":"Spam Org"}`, email)
			req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBufferString(payload))
			req.RemoteAddr = fmt.Sprintf("192.168.1.%d:12345", idx+50)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 Bad Request for disposable email %s, got %d", email, w.Code)
			}
			if !bytes.Contains(w.Body.Bytes(), []byte("disposable or temporary email")) {
				t.Fatalf("expected disposable email error message, got: %s", w.Body.String())
			}
		})
	}
}

func TestRegisterHoneypotTriggered(t *testing.T) {
	authService := auth.NewAuthenticator("test-secret-key-1234567890123456")
	ctrl := controllers.NewAuthController(authService, nil)
	router := ctrl.Routes()

	// Honeypot website_url is filled by an automated scraper/bot
	botPayload := `{"email":"legit@gmail.com","password":"Zq7-Kv4-Mn9-Tb2-Xc6-Rp8","workspace_name":"My Workspace","website_url":"https://spam-link.com"}`
	req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBufferString(botPayload))
	req.RemoteAddr = "192.168.1.51:12345"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request when honeypot field is filled, got %d", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("registration request could not be processed")) {
		t.Fatalf("expected honeypot rejection error message, got: %s", w.Body.String())
	}
}

func TestRegisterRateLimiterThrottling(t *testing.T) {
	authService := auth.NewAuthenticator("test-secret-key-1234567890123456")
	ctrl := controllers.NewAuthController(authService, nil)

	// Inject tight register rate limiter: Capacity = 2
	testLimiter := limiter.NewTokenBucketLimiter(limiter.RateLimitConfig{
		Capacity:          2,
		RefillRatePerSec:  0.0001,
		ExpirationTimeout: 1 * time.Minute,
	})
	ctrl.SetRegisterRateLimiter(testLimiter)
	router := ctrl.Routes()

	registerBody := `{"email":"newuser@gmail.com","password":"Zq7-Kv4-Mn9-Tb2-Xc6-Rp8","workspace_name":"Test Workspace"}`

	// First 2 requests pass rate limit
	for i := 1; i <= 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBufferString(registerBody))
		req.RemoteAddr = "203.0.113.88:45678"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code == http.StatusTooManyRequests {
			t.Fatalf("request %d should not have been rate limited, got 429", i)
		}
	}

	// Third request from same IP is throttled
	reqBlocked := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBufferString(registerBody))
	reqBlocked.RemoteAddr = "203.0.113.88:45678"
	wBlocked := httptest.NewRecorder()
	router.ServeHTTP(wBlocked, reqBlocked)

	if wBlocked.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 3rd registration request to be blocked with 429 Too Many Requests, got %d", wBlocked.Code)
	}
	if !bytes.Contains(wBlocked.Body.Bytes(), []byte("too many registration attempts")) {
		t.Fatalf("expected rate limit error message, got: %s", wBlocked.Body.String())
	}
}

func TestRegisterCustomBlockedDomains(t *testing.T) {
	authService := auth.NewAuthenticator("test-secret-key-1234567890123456")
	ctrl := controllers.NewAuthController(authService, nil)
	ctrl.SetBlockedEmailDomains([]string{"scamdomain.com", "phishing.org"})
	router := ctrl.Routes()

	blockedPayload := `{"email":"attacker@scamdomain.com","password":"Zq7-Kv4-Mn9-Tb2-Xc6-Rp8","workspace_name":"Scam Team"}`
	req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBufferString(blockedPayload))
	req.RemoteAddr = "192.168.1.52:12345"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for custom blocked domain, got %d", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("disposable or temporary email")) {
		t.Fatalf("expected disposable domain error message, got: %s", w.Body.String())
	}
}

func TestDualAxisAccountLockout(t *testing.T) {
	ctx := context.Background()
	authService := auth.NewAuthenticator("test-secret-key-1234567890123456")
	ctrl := controllers.NewAuthController(authService, nil)
	router := ctrl.Routes()

	targetEmail := "victim.user@enterprise.com"

	// 1. Initial state: account is not locked
	locked, _ := ctrl.IsAccountLocked(ctx, targetEmail)
	if locked {
		t.Fatal("account should not be locked initially")
	}

	// 2. Record 4 failed logins: account should still not be locked
	for i := 1; i <= 4; i++ {
		isLocked, _ := ctrl.RecordFailedLogin(ctx, targetEmail)
		if isLocked {
			t.Fatalf("attempt %d should not have triggered lockout yet", i)
		}
	}

	// 3. 5th failed login triggers account lockout (OWASP ASVS V2.2.1)
	isLocked, retryAfter := ctrl.RecordFailedLogin(ctx, targetEmail)
	if !isLocked {
		t.Fatal("5th failed attempt must trigger account lockout")
	}
	if retryAfter <= 0 {
		t.Fatal("lockout must have positive retryAfter duration")
	}

	// Verify IsAccountLocked reports true
	nowLocked, retrySec := ctrl.IsAccountLocked(ctx, targetEmail)
	if !nowLocked || retrySec <= 0 {
		t.Fatal("IsAccountLocked should report true after 5 failures")
	}

	// 4. HTTP /login request for the locked account from a COMPLETELY NEW IP must be blocked with 429
	loginBody := fmt.Sprintf(`{"email":"%s","password":"SomePassword123!"}`, targetEmail)
	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBufferString(loginBody))
	req.RemoteAddr = "198.51.100.99:34567" // brand new IP
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected locked account to be blocked with 429 Too Many Requests, got %d", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("account temporarily locked")) {
		t.Fatalf("expected account locked error message, got: %s", w.Body.String())
	}
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header to be set on 429 response")
	}

	// 5. A different account is unaffected
	otherEmail := "other.user@enterprise.com"
	otherLocked, _ := ctrl.IsAccountLocked(ctx, otherEmail)
	if otherLocked {
		t.Fatal("unrelated account should not be locked")
	}

	// 6. ClearFailedLogins (e.g. after successful auth) clears lockout
	ctrl.ClearFailedLogins(ctx, targetEmail)
	stillLocked, _ := ctrl.IsAccountLocked(ctx, targetEmail)
	if stillLocked {
		t.Fatal("lockout should be removed after ClearFailedLogins")
	}
}

func TestDualAxisAccountTargetedLimiter(t *testing.T) {
	authService := auth.NewAuthenticator("test-secret-key-1234567890123456")
	ctrl := controllers.NewAuthController(authService, nil)

	// Configure account limiter with capacity 2 (burst 2)
	accountLimiter := limiter.NewTokenBucketLimiter(limiter.RateLimitConfig{
		Capacity:          2,
		RefillRatePerSec:  0.0001,
		ExpirationTimeout: 1 * time.Minute,
	})
	ctrl.SetAccountLimiter(accountLimiter)
	router := ctrl.Routes()

	targetPayload := `{"email":"targeted.executive@corp.com","password":"PasswordGuess1"}`

	// Request 1 from Proxy IP 1 -> Passes account limiter (fails with 503 because repo is nil)
	req1 := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBufferString(targetPayload))
	req1.RemoteAddr = "101.0.0.1:1111"
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)
	if w1.Code == http.StatusTooManyRequests {
		t.Fatal("1st request should not have been rate limited")
	}

	// Request 2 from Proxy IP 2 -> Passes account limiter
	req2 := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBufferString(targetPayload))
	req2.RemoteAddr = "102.0.0.2:2222"
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code == http.StatusTooManyRequests {
		t.Fatal("2nd request should not have been rate limited")
	}

	// Request 3 from Proxy IP 3 -> BLOCKED with 429 Too Many Requests because the account bucket is exhausted!
	req3 := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBufferString(targetPayload))
	req3.RemoteAddr = "103.0.0.3:3333"
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, req3)

	if w3.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 3rd request from rotating proxy to be blocked with 429 Too Many Requests, got %d", w3.Code)
	}
	if !bytes.Contains(w3.Body.Bytes(), []byte("too many login attempts for this account")) {
		t.Fatalf("expected account rate limit message, got: %s", w3.Body.String())
	}
}

func TestPasswordResetTokenRoundTripParsing(t *testing.T) {
	jwtSecret := "jwt-reset-test-secret-32-chars-long!"
	authService := auth.NewAuthenticator(jwtSecret)
	ctrl := controllers.NewAuthController(authService, nil)
	ctrl.SetJWTSecret(jwtSecret)
	router := ctrl.Routes()

	userID := uuid.New()
	email := "developer@scandrix.dev"
	currentPasswordHash := "$2a$10$abcdefghijklmnopqrstuvwxyz123456"

	token, err := auth.CreatePasswordResetToken(userID, email, currentPasswordHash, jwtSecret, 15*time.Minute)
	if err != nil {
		t.Fatalf("failed creating password reset token: %v", err)
	}

	// 1. Sending valid reset token without DB initialized returns 503 (repo nil), confirming payload parsing succeeded
	resetBody := fmt.Sprintf(`{"token":"%s","new_password":"NewZq7-Kv4-Mn9-Tb2-Xc6-Rp8"}`, token)
	req := httptest.NewRequest(http.MethodPost, "/reset-password", bytes.NewBufferString(resetBody))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Since repo is nil in this unit test, it reaches "password reset not configured" (503 Service Unavailable)
	// ONLY if token format and JSON payload are valid.
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 Service Unavailable when repo is nil, got %d: %s", w.Code, w.Body.String())
	}

	// 2. Sending corrupted token fails with 400 Bad Request
	corruptedBody := `{"token":"invalid.token","new_password":"NewZq7-Kv4-Mn9-Tb2-Xc6-Rp8"}`
	reqCorrupt := httptest.NewRequest(http.MethodPost, "/reset-password", bytes.NewBufferString(corruptedBody))
	wCorrupt := httptest.NewRecorder()
	router.ServeHTTP(wCorrupt, reqCorrupt)

	if wCorrupt.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for corrupted token, got %d", wCorrupt.Code)
	}
}

func TestSecureCookieFlagsEvaluation(t *testing.T) {
	authService := auth.NewAuthenticator("test-secret-1234567890123456")
	ctrl := controllers.NewAuthController(authService, nil)
	router := ctrl.Routes()

	// 1. Secure is now applied by DEFAULT, including to a plain-HTTP request.
	//
	// AUDIT_REMEDIATION.md F-19: this assertion used to require Secure=false
	// for a non-TLS development request, which is the fail-open behaviour the
	// finding describes -- browsers then send the session cookie over
	// plaintext HTTP. The corrected contract is that the attribute is on unless
	// an operator explicitly opts out.
	for _, k := range []string{"ENVIRONMENT", "APP_ENV", "SCANDRIX_ENV", "GO_ENV", "COOKIE_SECURE"} {
		t.Setenv(k, "")
		if err := os.Unsetenv(k); err != nil {
			t.Fatalf("unset %s: %v", k, err)
		}
	}
	reqDev := httptest.NewRequest(http.MethodPost, "/logout", bytes.NewBufferString(`{}`))
	wDev := httptest.NewRecorder()
	router.ServeHTTP(wDev, reqDev)

	cookiesDev := wDev.Result().Cookies()
	for _, c := range cookiesDev {
		if c.Name == "scandrix_token" {
			if !c.Secure {
				t.Errorf("expected Secure=true by default on a non-TLS request, got false")
			}
			if !c.HttpOnly {
				t.Errorf("expected HttpOnly=true, got false")
			}
		}
	}

	// 2. Secure stays on, and a client-supplied header cannot change it.
	//    X-Forwarded-Proto is no longer an input to this decision.
	reqTLS := httptest.NewRequest(http.MethodPost, "/logout", bytes.NewBufferString(`{}`))
	reqTLS.Header.Set("X-Forwarded-Proto", "https")
	wTLS := httptest.NewRecorder()
	router.ServeHTTP(wTLS, reqTLS)

	cookiesTLS := wTLS.Result().Cookies()
	foundTLS := false
	for _, c := range cookiesTLS {
		if c.Name == "scandrix_token" {
			foundTLS = true
			if !c.Secure {
				t.Errorf("expected Secure=true, got false")
			}
		}
	}
	if !foundTLS {
		t.Error("expected scandrix_token cookie in logout response")
	}

	// 3. In production environment, cookie prefix __Host- is used for secure cookies
	t.Setenv("ENVIRONMENT", "production")
	reqProd := httptest.NewRequest(http.MethodPost, "/logout", bytes.NewBufferString(`{}`))
	reqProd.Header.Set("X-Forwarded-Proto", "https")
	wProd := httptest.NewRecorder()
	router.ServeHTTP(wProd, reqProd)

	cookiesProd := wProd.Result().Cookies()
	foundProd := false
	for _, c := range cookiesProd {
		if c.Name == "__Host-scandrix_token" {
			foundProd = true
			if !c.Secure {
				t.Errorf("expected Secure=true in production, got false")
			}
			if !c.HttpOnly {
				t.Errorf("expected HttpOnly=true in production, got false")
			}
		}
	}
	if !foundProd {
		t.Error("expected __Host-scandrix_token cookie in production logout response")
	}
}

func TestSSODomainVerificationAndTestConnectionEndpoints(t *testing.T) {
	jwtSecret := "test-secret-key-12345678901234567890123456789012"
	authenticator := auth.NewAuthenticator(jwtSecret)
	ctrl := controllers.NewAuthController(authenticator, nil)
	router := ctrl.Routes()

	wsID := uuid.New()
	userUUID := uuid.New()
	token, _ := authenticator.GenerateToken(userUUID, wsID, models.RoleAdmin)

	// 1. Request Domain Verification
	reqBody := bytes.NewBufferString(`{"domain":"acme.corp","contact_email":"admin@acme.corp"}`)
	req := httptest.NewRequest(http.MethodPost, "/sso/domains/request-verification", reqBody)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("RequestDomainVerification expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var verifResp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&verifResp); err != nil {
		t.Fatalf("Failed parsing verification response: %v", err)
	}
	if verifResp["domain"] != "acme.corp" {
		t.Fatalf("Expected domain acme.corp, got %v", verifResp["domain"])
	}

	// 2. Get Domain Status
	reqStatus := httptest.NewRequest(http.MethodGet, "/sso/domains/status?domain=acme.corp", nil)
	reqStatus.Header.Set("Authorization", "Bearer "+token)
	wStatus := httptest.NewRecorder()
	router.ServeHTTP(wStatus, reqStatus)

	if wStatus.Code != http.StatusOK {
		t.Fatalf("GetDomainStatus expected 200, got %d: %s", wStatus.Code, wStatus.Body.String())
	}

	// 3. Start SSO Connection Test
	startBody := bytes.NewBufferString(`{"provider_type":"SAML2","sso_url":"https://idp.acme.corp/sso","domains":["acme.corp"]}`)
	reqStart := httptest.NewRequest(http.MethodPost, "/sso/test-connection/start", startBody)
	reqStart.Header.Set("Authorization", "Bearer "+token)
	reqStart.Header.Set("Content-Type", "application/json")
	wStart := httptest.NewRecorder()
	router.ServeHTTP(wStart, reqStart)

	if wStart.Code != http.StatusOK {
		t.Fatalf("StartSSOConnectionTest expected 200, got %d: %s", wStart.Code, wStart.Body.String())
	}

	var startResp map[string]any
	if err := json.NewDecoder(wStart.Body).Decode(&startResp); err != nil {
		t.Fatalf("Failed parsing start response: %v", err)
	}
	sessionID, ok := startResp["session_id"].(string)
	if !ok || sessionID == "" {
		t.Fatalf("Expected valid session_id, got %v", startResp["session_id"])
	}

	// 4. Poll SSO Connection Test Result (should be PENDING)
	reqResult := httptest.NewRequest(http.MethodGet, "/sso/test-connection/result?session_id="+sessionID, nil)
	wResult := httptest.NewRecorder()
	router.ServeHTTP(wResult, reqResult)

	if wResult.Code != http.StatusOK {
		t.Fatalf("GetSSOConnectionTestResult expected 200, got %d: %s", wResult.Code, wResult.Body.String())
	}
	var resResp map[string]any
	if err := json.NewDecoder(wResult.Body).Decode(&resResp); err != nil {
		t.Fatalf("Failed parsing test result: %v", err)
	}
	if resResp["status"] != "PENDING" {
		t.Fatalf("Expected status PENDING, got %v", resResp["status"])
	}

	// 5. Test Callback with missing SAML response
	cbBody := bytes.NewBufferString(`{"session_id":"` + sessionID + `","saml_response":""}`)
	reqCb := httptest.NewRequest(http.MethodPost, "/sso/test-connection/callback", cbBody)
	reqCb.Header.Set("Content-Type", "application/json")
	wCb := httptest.NewRecorder()
	router.ServeHTTP(wCb, reqCb)

	if wCb.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400 on empty SAML response, got %d", wCb.Code)
	}
}
