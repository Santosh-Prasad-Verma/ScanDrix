package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/auth/cliauth"
	"github.com/scandrix/backend/internal/auth/mailer"
	"github.com/scandrix/backend/internal/auth/oauth"
	"github.com/scandrix/backend/internal/review"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/pkg/models"
)

func TestAPIRouterEndToEnd(t *testing.T) {
	jwtSecret := "test-jwt-secret-key-123456789012"
	authService := auth.NewAuthenticator(jwtSecret)
	streamHub := review.NewStreamHub()
	evaluator, _ := rules.NewEvaluator(rules.DefaultCatalog())
	inMemStore := cliauth.NewInMemorySessionStore()
	deviceFlow := cliauth.NewDeviceFlowManager(inMemStore, "https://app.scandrix.dev")
	oauthService := oauth.NewOAuthService(
		oauth.ProviderConfig{ClientID: "mock-gh-client-id", ClientSecret: "mock-gh-secret"},
		oauth.ProviderConfig{ClientID: "mock-gl-client-id", ClientSecret: "mock-gl-secret"},
	)
	mockMailer := mailer.NewNoopSender()

	router := api.BuildRouter(api.RouterConfig{
		Repo:         nil, // not needed for pure HTTP contract tests
		AuthService:  authService,
		Orchestrator: nil,
		StreamHub:    streamHub,
		Evaluator:    evaluator,
		SCIMService:  nil,
		DeviceFlow:   deviceFlow,
		OAuthService: oauthService,
		Mailer:       mockMailer,
		AppBaseURL:   "https://app.scandrix.dev",
		JWTSecret:    jwtSecret,
	})

	// 1. Test Public Healthz
	reqHealth := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	wHealth := httptest.NewRecorder()
	router.ServeHTTP(wHealth, reqHealth)
	if wHealth.Code != http.StatusOK {
		t.Fatalf("expected 200 for /healthz, got %d", wHealth.Code)
	}

	// 2. Test User Registration (Repo is nil -> Expect 503 Service Unavailable fail-closed)
	regBody, _ := json.Marshal(dtos.RegisterRequest{
		Email:         "lead@techcorp.com",
		Password:      "SecurePassword123!",
		DisplayName:   "Tech Lead",
		WorkspaceName: "TechCorp Global",
	})
	reqReg := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(regBody))
	wReg := httptest.NewRecorder()
	router.ServeHTTP(wReg, reqReg)
	if wReg.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 Service Unavailable for register with nil repo, got %d: %s", wReg.Code, wReg.Body.String())
	}

	// Generate valid test JWT token for protected API contract testing
	testUserID := uuid.New()
	testWsID := uuid.New()
	accessToken, _, err := authService.GenerateTokenPair(testUserID, testWsID, models.RoleOwner)
	if err != nil {
		t.Fatalf("failed generating test JWT: %v", err)
	}

	// 3. Test Rules Catalog (Protected)
	reqCat := httptest.NewRequest(http.MethodGet, "/api/v1/rules/catalog", nil)
	reqCat.Header.Set("Authorization", "Bearer "+accessToken)
	wCat := httptest.NewRecorder()
	router.ServeHTTP(wCat, reqCat)
	if wCat.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/v1/rules/catalog, got %d", wCat.Code)
	}

	// 4. Test Rule Regex Real-Time Matcher
	testRuleBody, _ := json.Marshal(dtos.TestRuleRequest{
		RegexRule:   `fmt\.Sprintf\(["'].*SELECT`,
		CodeSnippet: `query := fmt.Sprintf("SELECT * FROM users")`,
		FilePath:    "query.go",
	})
	reqTestRule := httptest.NewRequest(http.MethodPost, "/api/v1/rules/test", bytes.NewReader(testRuleBody))
	reqTestRule.Header.Set("Authorization", "Bearer "+accessToken)
	wTestRule := httptest.NewRecorder()
	router.ServeHTTP(wTestRule, reqTestRule)
	if wTestRule.Code != http.StatusOK {
		t.Fatalf("expected 200 for rule test, got %d", wTestRule.Code)
	}

	var testRuleResp dtos.TestRuleResponse
	_ = json.NewDecoder(wTestRule.Body).Decode(&testRuleResp)
	if !testRuleResp.Matched || len(testRuleResp.MatchLines) != 1 || testRuleResp.MatchLines[0] != 1 {
		t.Fatalf("expected regex match on line 1, got %+v", testRuleResp)
	}

	// 5. Test Cockpit Metrics
	reqCockpit := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/cockpit", nil)
	reqCockpit.Header.Set("Authorization", "Bearer "+accessToken)
	wCockpit := httptest.NewRecorder()
	router.ServeHTTP(wCockpit, reqCockpit)
	if wCockpit.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/v1/workspaces/cockpit, got %d", wCockpit.Code)
	}

	var cockpit dtos.CockpitMetricsResponse
	_ = json.NewDecoder(wCockpit.Body).Decode(&cockpit)
	if cockpit.TotalReviews < 0 || cockpit.PassRatePercentage < 0 {
		t.Fatalf("unexpected cockpit metrics: %+v", cockpit)
	}

	// 6. Test Token Usage & Quotas
	reqUsage := httptest.NewRequest(http.MethodGet, "/api/v1/usage", nil)
	reqUsage.Header.Set("Authorization", "Bearer "+accessToken)
	wUsage := httptest.NewRecorder()
	router.ServeHTTP(wUsage, reqUsage)
	if wUsage.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/v1/usage, got %d", wUsage.Code)
	}

	var usage dtos.TokenUsageResponse
	_ = json.NewDecoder(wUsage.Body).Decode(&usage)
	if usage.TotalTokens < 0 || usage.EstimatedCostUSD < 0 {
		t.Fatalf("unexpected usage response: %+v", usage)
	}


	// 7. Test Teams API
	teamBody, _ := json.Marshal(dtos.CreateTeamRequest{
		Name:        "Security Architecture",
		Description: "Owns security controls and compliance checks",
	})
	reqTeam := httptest.NewRequest(http.MethodPost, "/api/v1/teams", bytes.NewReader(teamBody))
	reqTeam.Header.Set("Authorization", "Bearer "+accessToken)
	wTeam := httptest.NewRecorder()
	router.ServeHTTP(wTeam, reqTeam)
	if wTeam.Code != http.StatusCreated {
		t.Fatalf("expected 201 for /api/v1/teams, got %d", wTeam.Code)
	}

	// 8. Test Repository Tracking API
	repoBody, _ := json.Marshal(dtos.TrackRepositoryRequest{
		NamespacePath: "acme/auth-service",
		DefaultBranch: "main",
	})
	reqRepo := httptest.NewRequest(http.MethodPost, "/api/v1/repos/track", bytes.NewReader(repoBody))
	reqRepo.Header.Set("Authorization", "Bearer "+accessToken)
	wRepo := httptest.NewRecorder()
	router.ServeHTTP(wRepo, reqRepo)
	if wRepo.Code != http.StatusCreated {
		t.Fatalf("expected 201 for /api/v1/repos/track, got %d", wRepo.Code)
	}

	// 9. Test Review Parameters API
	reqParam := httptest.NewRequest(http.MethodGet, "/api/v1/parameters/review", nil)
	reqParam.Header.Set("Authorization", "Bearer "+accessToken)
	wParam := httptest.NewRecorder()
	router.ServeHTTP(wParam, reqParam)
	if wParam.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/v1/parameters/review, got %d", wParam.Code)
	}

	// 10. Test Integrations Test Connection API
	testIntegBody, _ := json.Marshal(map[string]string{"provider": "GITHUB"})
	reqInteg := httptest.NewRequest(http.MethodPost, "/api/v1/integrations/test", bytes.NewReader(testIntegBody))
	reqInteg.Header.Set("Authorization", "Bearer "+accessToken)
	wInteg := httptest.NewRecorder()
	router.ServeHTTP(wInteg, reqInteg)
	if wInteg.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/v1/integrations/test, got %d", wInteg.Code)
	}

	// 11. Test Permissions API
	reqPerm := httptest.NewRequest(http.MethodGet, "/api/v1/permissions/me", nil)
	reqPerm.Header.Set("Authorization", "Bearer "+accessToken)
	wPerm := httptest.NewRecorder()
	router.ServeHTTP(wPerm, reqPerm)
	if wPerm.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/v1/permissions/me, got %d", wPerm.Code)
	}

	// 12. Test Enterprise License API
	reqLic := httptest.NewRequest(http.MethodGet, "/api/v1/license", nil)
	reqLic.Header.Set("Authorization", "Bearer "+accessToken)
	wLic := httptest.NewRecorder()
	router.ServeHTTP(wLic, reqLic)
	if wLic.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/v1/license, got %d", wLic.Code)
	}

	// 13. Test Webhook Delivery Health & Latency API
	reqWebhookHealth := httptest.NewRequest(http.MethodGet, "/api/v1/health/webhooks", nil)
	reqWebhookHealth.Header.Set("Authorization", "Bearer "+accessToken)
	wWebhookHealth := httptest.NewRecorder()
	router.ServeHTTP(wWebhookHealth, reqWebhookHealth)
	if wWebhookHealth.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/v1/health/webhooks, got %d", wWebhookHealth.Code)
	}

	// 14. Test Notification Routing Channels API
	reqNotif := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/channels", nil)
	reqNotif.Header.Set("Authorization", "Bearer "+accessToken)
	wNotif := httptest.NewRecorder()
	router.ServeHTTP(wNotif, reqNotif)
	if wNotif.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/v1/notifications/channels, got %d", wNotif.Code)
	}

	// 15. Test AI Finding Feedback API
	dummyFindingID := uuid.New()
	feedbackBody, _ := json.Marshal(dtos.FindingFeedbackRequest{
		Sentiment: "HELPFUL",
		Comments:  "Caught an actual SQL injection risk in our query builder",
	})
	reqFeedback := httptest.NewRequest(http.MethodPost, "/api/v1/findings/"+dummyFindingID.String()+"/feedback", bytes.NewReader(feedbackBody))
	reqFeedback.Header.Set("Authorization", "Bearer "+accessToken)
	wFeedback := httptest.NewRecorder()
	router.ServeHTTP(wFeedback, reqFeedback)
	if wFeedback.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/v1/findings/{id}/feedback, got %d", wFeedback.Code)
	}

	// 16. Verify Unauthenticated Requests Fail
	reqUnauthorized := httptest.NewRequest(http.MethodGet, "/api/v1/rules/catalog", nil)
	wUnauthorized := httptest.NewRecorder()
	router.ServeHTTP(wUnauthorized, reqUnauthorized)
	if wUnauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", wUnauthorized.Code)
	}

	// 17. Test Forgot Password Endpoint (Rate-Limited Public)
	forgotBody, _ := json.Marshal(dtos.ForgotPasswordRequest{
		Email: "lead@techcorp.com",
	})
	reqForgot := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password/forgot", bytes.NewReader(forgotBody))
	wForgot := httptest.NewRecorder()
	router.ServeHTTP(wForgot, reqForgot)
	if wForgot.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/v1/auth/password/forgot, got %d: %s", wForgot.Code, wForgot.Body.String())
	}

	// 18. Test CLI Device Login Initiate (RFC 8628)
	reqCLI := httptest.NewRequest(http.MethodPost, "/api/v1/auth/cli/device/initiate", nil)
	wCLI := httptest.NewRecorder()
	router.ServeHTTP(wCLI, reqCLI)
	if wCLI.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/v1/auth/cli/device/initiate, got %d: %s", wCLI.Code, wCLI.Body.String())
	}
	var cliResp cliauth.DeviceLoginInitiateResult
	_ = json.NewDecoder(wCLI.Body).Decode(&cliResp)
	if cliResp.DeviceCode == "" || cliResp.UserCode == "" {
		t.Fatalf("expected non-empty device_code and user_code, got %+v", cliResp)
	}

	// 19. Test OAuth Authorize URL with CSRF State
	reqOAuth := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/github/authorize", nil)
	wOAuth := httptest.NewRecorder()
	router.ServeHTTP(wOAuth, reqOAuth)
	if wOAuth.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/v1/auth/oauth/github/authorize, got %d: %s", wOAuth.Code, wOAuth.Body.String())
	}
	var oauthResp map[string]string
	_ = json.NewDecoder(wOAuth.Body).Decode(&oauthResp)
	if oauthResp["authorization_url"] == "" || oauthResp["state"] == "" {
		t.Fatalf("expected authorization_url and state in OAuth response, got %+v", oauthResp)
	}
}
