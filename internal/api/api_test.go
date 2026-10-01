package api_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/auth/cliauth"
	"github.com/scandrix/backend/internal/auth/mailer"
	"github.com/scandrix/backend/internal/auth/oauth"
	"github.com/scandrix/backend/internal/enterprise/license"
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

	// 1. Readiness/health must FAIL CLOSED when the database is unreachable.
	// This router is built with a nil repo, so a 503 is the correct answer. The
	// previous inline handler defaulted dbStatus to "ok" when cfg.Repo was nil
	// and reported healthy with no database at all.
	reqHealth := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	wHealth := httptest.NewRecorder()
	router.ServeHTTP(wHealth, reqHealth)
	if wHealth.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 from /healthz with no database, got %d", wHealth.Code)
	}

	// Liveness must stay 200 regardless: it reports the process, not deps, so a
	// database blip does not get the container killed and restarted.
	reqLive := httptest.NewRequest(http.MethodGet, "/livez", nil)
	wLive := httptest.NewRecorder()
	router.ServeHTTP(wLive, reqLive)
	if wLive.Code != http.StatusOK {
		t.Fatalf("expected 200 from /livez, got %d", wLive.Code)
	}

	// 2. Test User Registration (Repo is nil -> Expect 503 Service Unavailable fail-closed)
	regBody, _ := json.Marshal(dtos.RegisterRequest{
		Email:         "lead@techcorp.com",
		Password:      "Zq7-Kv4-Mn9-Tb2-Xc6-Rp8",
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
	//
	// This router is built with Repo: nil, so the honest answer is 503. It
	// used to answer 200 with a fabricated 100% pass rate, reporting a
	// perfect security posture for a workspace with no data at all
	// (AUDIT_REMEDIATION.md F-07).
	reqCockpit := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/cockpit", nil)
	reqCockpit.Header.Set("Authorization", "Bearer "+accessToken)
	wCockpit := httptest.NewRecorder()
	router.ServeHTTP(wCockpit, reqCockpit)
	if wCockpit.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for /api/v1/workspaces/cockpit with no repository, got %d: %s",
			wCockpit.Code, wCockpit.Body.String())
	}
	if strings.Contains(wCockpit.Body.String(), "pass_rate_percentage") {
		t.Fatalf("a nil repository must not yield a metric: %s", wCockpit.Body.String())
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

	// 7. Test Teams API (expects 503 without DB)
	teamBody, _ := json.Marshal(dtos.CreateTeamRequest{
		Name:        "Security Architecture",
		Description: "Owns security controls and compliance checks",
	})
	reqTeam := httptest.NewRequest(http.MethodPost, "/api/v1/teams", bytes.NewReader(teamBody))
	reqTeam.Header.Set("Authorization", "Bearer "+accessToken)
	wTeam := httptest.NewRecorder()
	router.ServeHTTP(wTeam, reqTeam)
	if wTeam.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for /api/v1/teams (no DB), got %d", wTeam.Code)
	}

	// 8. Test Repository Tracking API (expects 503 without DB)
	repoBody, _ := json.Marshal(dtos.TrackRepositoryRequest{
		NamespacePath: "acme/auth-service",
		DefaultBranch: "main",
	})
	reqRepo := httptest.NewRequest(http.MethodPost, "/api/v1/repos/track", bytes.NewReader(repoBody))
	reqRepo.Header.Set("Authorization", "Bearer "+accessToken)
	wRepo := httptest.NewRecorder()
	router.ServeHTTP(wRepo, reqRepo)
	if wRepo.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for /api/v1/repos/track (no DB), got %d", wRepo.Code)
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

func TestSecondaryAuthRoutesRateLimiting(t *testing.T) {
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
		Repo:         nil,
		AuthService:  authService,
		StreamHub:    streamHub,
		Evaluator:    evaluator,
		DeviceFlow:   deviceFlow,
		OAuthService: oauthService,
		Mailer:       mockMailer,
		AppBaseURL:   "https://app.scandrix.dev",
		JWTSecret:    jwtSecret,
	})

	// 1. Verify GET /user/email returns response under normal rate
	reqEmail := httptest.NewRequest(http.MethodGet, "/user/email?email=test@example.com", nil)
	reqEmail.RemoteAddr = "192.0.2.1:12345"
	wEmail := httptest.NewRecorder()
	router.ServeHTTP(wEmail, reqEmail)
	if wEmail.Code != http.StatusOK {
		t.Fatalf("expected 200 for /user/email under normal conditions, got %d: %s", wEmail.Code, wEmail.Body.String())
	}

	// 2. Verify POST /cli/authorize/approve is protected (repo is nil -> 503 rather than panic or 404)
	approveBody := `{"user_code":"ABCD-EFGH","email":"test@example.com","password":"Zq7-Kv4-Mn9-Tb2-Xc6-Rp8","action":"login"}`
	reqApprove := httptest.NewRequest(http.MethodPost, "/cli/authorize/approve", bytes.NewBufferString(approveBody))
	reqApprove.RemoteAddr = "192.0.2.1:12345"
	wApprove := httptest.NewRecorder()
	router.ServeHTTP(wApprove, reqApprove)
	if wApprove.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for /cli/authorize/approve when repo is nil, got %d: %s", wApprove.Code, wApprove.Body.String())
	}
}

func TestAPIWebhooksIngressRouting(t *testing.T) {
	jwtSecret := "test-jwt-secret-key-123456789012"
	authService := auth.NewAuthenticator(jwtSecret)
	billingSecret := "test-billing-wh-secret"
	ghSecret := "test-gh-wh-secret"

	router := api.BuildRouter(api.RouterConfig{
		AuthService:          authService,
		BillingWebhookSecret: billingSecret,
		GitHubWebhookSecret:  ghSecret,
	})

	// 1. Test Billing Webhook route via /billing/webhook/payment-failed
	body := []byte(`{"workspaceId":"` + uuid.New().String() + `","amount":9900,"currency":"USD","failureReason":"card_declined"}`)
	mac := hmac.New(sha256.New, []byte(billingSecret))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))

	req1 := httptest.NewRequest(http.MethodPost, "/billing/webhook/payment-failed", bytes.NewReader(body))
	req1.Header.Set("X-Scandrix-Signature", sig)
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("expected 200 for /billing/webhook/payment-failed, got %d: %s", w1.Code, w1.Body.String())
	}

	// 2. Test Billing Webhook route via /api/v1/billing/webhook/payment-failed
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/billing/webhook/payment-failed", bytes.NewReader(body))
	req2.Header.Set("X-ScanDrix-Signature", sig)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/v1/billing/webhook/payment-failed, got %d: %s", w2.Code, w2.Body.String())
	}

	// 3. Test Git Webhook route via /github/webhook with invalid sig -> 401
	ghBody := []byte(`{"action":"opened","repository":{"full_name":"owner/repo"}}`)
	req3 := httptest.NewRequest(http.MethodPost, "/github/webhook", bytes.NewReader(ghBody))
	req3.Header.Set("X-GitHub-Event", "pull_request")
	req3.Header.Set("X-Hub-Signature-256", "sha256=invalid")
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, req3)
	if w3.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthorized /github/webhook, got %d", w3.Code)
	}

	// 4. Test Git Webhook route via /api/v1/webhooks/github with invalid sig -> 401
	req4 := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github", bytes.NewReader(ghBody))
	req4.Header.Set("X-GitHub-Event", "pull_request")
	req4.Header.Set("X-Hub-Signature-256", "sha256=invalid")
	w4 := httptest.NewRecorder()
	router.ServeHTTP(w4, req4)
	if w4.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthorized /api/v1/webhooks/github, got %d", w4.Code)
	}
}

func TestAPIRouterMCPEndToEnd(t *testing.T) {
	os.Setenv("SCANDRIX_MCP_SERVER_ENABLED", "true")
	defer os.Unsetenv("SCANDRIX_MCP_SERVER_ENABLED")

	jwtSecret := "test-jwt-secret-key-123456789012"
	authService := auth.NewAuthenticator(jwtSecret)

	router := api.BuildRouter(api.RouterConfig{
		Repo:        nil,
		AuthService: authService,
		AppBaseURL:  "https://app.scandrix.dev",
		JWTSecret:   jwtSecret,
	})

	// MCP requires a session: the transport is authenticated and disabled by
	// default (AUDIT_REMEDIATION.md F-15f), so these requests carry a token.
	mcpTok, _, err := authService.GenerateTokenPairWithEmail(
		uuid.New(), uuid.New(), models.RoleOwner, "mcp-e2e@scandrix.internal")
	if err != nil {
		t.Fatalf("mint mcp token: %v", err)
	}
	mcpAuth := "Bearer " + mcpTok

	// 1. GET /api/v1/mcp must return 405 Method Not Allowed with Allow: POST
	reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/mcp", nil)
	reqGet.Header.Set("Authorization", mcpAuth)
	wGet := httptest.NewRecorder()
	router.ServeHTTP(wGet, reqGet)
	if wGet.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for GET /api/v1/mcp, got %d: %s", wGet.Code, wGet.Body.String())
	}
	if wGet.Header().Get("Allow") != "POST" {
		t.Fatalf("expected Allow: POST, got %s", wGet.Header().Get("Allow"))
	}

	// 2. POST /api/v1/mcp with initialize must return 200 OK
	initPayload := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`)
	reqPost := httptest.NewRequest(http.MethodPost, "/api/v1/mcp", bytes.NewReader(initPayload))
	reqPost.Header.Set("Content-Type", "application/json")
	reqPost.Header.Set("Authorization", mcpAuth)
	wPost := httptest.NewRecorder()
	router.ServeHTTP(wPost, reqPost)
	if wPost.Code != http.StatusOK {
		t.Fatalf("expected 200 for POST /api/v1/mcp, got %d: %s", wPost.Code, wPost.Body.String())
	}

	// 3. POST /api/v1/mcp/issues with tools/list must return 200 OK
	listPayload := []byte(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	reqIssues := httptest.NewRequest(http.MethodPost, "/api/v1/mcp/issues", bytes.NewReader(listPayload))
	reqIssues.Header.Set("Content-Type", "application/json")
	reqIssues.Header.Set("Authorization", mcpAuth)
	wIssues := httptest.NewRecorder()
	router.ServeHTTP(wIssues, reqIssues)
	if wIssues.Code != http.StatusOK {
		t.Fatalf("expected 200 for POST /api/v1/mcp/issues, got %d: %s", wIssues.Code, wIssues.Body.String())
	}

	// 4. GET /api/v1/mcp/issues must return 405 Method Not Allowed
	reqIssuesGet := httptest.NewRequest(http.MethodGet, "/api/v1/mcp/issues", nil)
	reqIssuesGet.Header.Set("Authorization", mcpAuth)
	wIssuesGet := httptest.NewRecorder()
	router.ServeHTTP(wIssuesGet, reqIssuesGet)
	if wIssuesGet.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for GET /api/v1/mcp/issues, got %d", wIssuesGet.Code)
	}
}

// TestCapabilitiesRouteIsReachableThroughTheFullRouter proves the capabilities
// TestSSODomainVerificationRequiresSession pins AUDIT_REMEDIATION.md F-11.
//
// /sso/domains/verify-dns and /confirm-token were registered on the
// unauthenticated route block and resolved the target workspace from the request
// body when the context carried none, so an anonymous caller could drive domain
// verification for a workspace it did not belong to. They must now be
// unreachable without a valid session.
func TestSSODomainVerificationRequiresSession(t *testing.T) {
	jwtSecret := "test-jwt-secret-key-123456789012"
	authService := auth.NewAuthenticator(jwtSecret)
	router := api.BuildRouter(api.RouterConfig{
		Repo:        nil,
		AuthService: authService,
		AppBaseURL:  "https://app.scandrix.dev",
		JWTSecret:   jwtSecret,
	})

	victimWorkspace := uuid.New()
	body := fmt.Sprintf(`{"domain":"victim-corp.com","workspace_id":%q}`, victimWorkspace)

	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/api/v1/auth/sso/domains/verify-dns"},
		{http.MethodPost, "/api/v1/auth/sso/domains/confirm-token"},
		{http.MethodPost, "/api/v1/auth/sso/domains/request-verification"},
		{http.MethodGet, "/api/v1/auth/sso/domains/status"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// Exactly 401: a 404 here would mean the route is unreachable for
			// legitimate callers too, which would pass a weaker assertion while
			// silently breaking the feature.
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401 for anonymous %s, got %d", tc.path, w.Code)
			}
		})
	}
}

// endpoint is mounted on both the root and the /api/v1 router, and answers a
// community plan for a workspace with no license configured. Building the real
// router also proves no route registration panics at startup.
func TestCapabilitiesRouteIsReachableThroughTheFullRouter(t *testing.T) {
	jwtSecret := "test-jwt-secret-key-123456789012"
	authService := auth.NewAuthenticator(jwtSecret)
	streamHub := review.NewStreamHub()
	evaluator, _ := rules.NewEvaluator(rules.DefaultCatalog())
	deviceFlow := cliauth.NewDeviceFlowManager(cliauth.NewInMemorySessionStore(), "https://app.scandrix.dev")
	oauthService := oauth.NewOAuthService(
		oauth.ProviderConfig{ClientID: "mock-gh-client-id", ClientSecret: "mock-gh-secret"},
		oauth.ProviderConfig{ClientID: "mock-gl-client-id", ClientSecret: "mock-gl-secret"},
	)

	router := api.BuildRouter(api.RouterConfig{
		Repo:         nil,
		AuthService:  authService,
		StreamHub:    streamHub,
		Evaluator:    evaluator,
		DeviceFlow:   deviceFlow,
		OAuthService: oauthService,
		Mailer:       mailer.NewNoopSender(),
		AppBaseURL:   "https://app.scandrix.dev",
		JWTSecret:    jwtSecret,
	})

	wsID := uuid.New()
	userID := uuid.New()
	token, err := authService.GenerateToken(userID, wsID, models.RoleMember)
	if err != nil {
		t.Fatalf("failed generating token: %v", err)
	}

	for _, path := range []string{"/capabilities", "/api/v1/capabilities"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d: %s", path, rec.Code, rec.Body.String())
		}

		var caps license.Capabilities
		if err := json.Unmarshal(rec.Body.Bytes(), &caps); err != nil {
			t.Fatalf("failed decoding %s response: %v", path, err)
		}
		if caps.Tier != string(license.TierCommunity) {
			t.Fatalf("expected COMMUNITY from %s with no license configured, got %s", path, caps.Tier)
		}
		if caps.Features[string(license.FeatureSSOSAML)] {
			t.Fatalf("%s must not report SSO for an unlicensed workspace", path)
		}
		if !caps.Features[string(license.FeatureBYOK)] {
			t.Fatalf("%s must report BYOK for an unlicensed workspace", path)
		}
	}
}
