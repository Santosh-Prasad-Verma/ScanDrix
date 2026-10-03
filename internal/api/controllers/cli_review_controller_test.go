package controllers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/clireview"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/internal/try"
)

// seedCLIReviewOrg owns the review seeded by setupCliReviewTest. The lookup is
// tenant-scoped, so requests must present this organization.
const seedCLIReviewOrg = "11111111-1111-1111-1111-111111111111"

func setupCliReviewTest() (*controllers.CliReviewController, *controllers.CliReviewsController, *clireview.KeyValidator) {
	evaluator, _ := rules.NewEvaluator(rules.DefaultCatalog())
	engine := clireview.NewEngine(evaluator, nil)
	keyValidator := clireview.NewKeyValidator("jwt-secret-for-test-32bytes-12345", nil)

	// Register a valid team key
	keyValidator.RegisterTeamKey("scandrix_team_test_123", clireview.TeamKeyRecord{
		KeyID:          "key-1",
		TeamID:         "team-1",
		TeamName:       "Core Engineering",
		OrganizationID: "org-1",
		OrgName:        "Acme Corp",
		Active:         true,
	})

	trialLimiter := clireview.NewTrialRateLimiter(2, 500*time.Millisecond)
	sessionStore := clireview.NewSessionStore()
	classifier := clireview.NewSessionClassifier()
	ingester := clireview.NewSessionIngester(sessionStore, classifier)

	cliReviewCtrl := controllers.NewCliReviewController(
		engine,
		keyValidator,
		trialLimiter,
		ingester,
		sessionStore,
	)

	dashStore := clireview.NewDashboardStore()
	cliReviewsCtrl := controllers.NewCliReviewsController(dashStore)

	// Seed one historical review
	dashStore.RecordReview(clireview.CliReviewSummary{
		ID:             "exec-uuid-1",
		OrganizationID: seedCLIReviewOrg,
		CorrelationID:  "corr-1",
		Summary:        "Zero defects found",
		IssuesCount:    0,
		FilesAnalyzed:  2,
		Duration:       120,
		Status:         "COMPLETED",
		CreatedAt:      time.Now().UTC(),
		Branch:         "feature/auth",
	})

	return cliReviewCtrl, cliReviewsCtrl, keyValidator
}

func TestCliReviewController_AuthAndFastReview(t *testing.T) {
	cliCtrl, _, _ := setupCliReviewTest()
	router := cliCtrl.ReviewRoutes()

	// 1. Unauthorized request without team key
	reqBody := `{"diff":"diff --git a/a.go b/a.go\n+func Hello(){}"}`
	req, _ := http.NewRequest("POST", "/", bytes.NewBufferString(reqBody))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized without auth, got %d", rec.Code)
	}

	// 2. Fast synchronous review with valid team key
	reqFast, _ := http.NewRequest("POST", "/", bytes.NewBufferString(`{
		"diff": "diff --git a/a.go b/a.go\n+func Hello(){}",
		"config": {
			"fast": true
		}
	}`))
	reqFast.Header.Set("X-Team-Key", "scandrix_team_test_123")
	recFast := httptest.NewRecorder()
	router.ServeHTTP(recFast, reqFast)

	if recFast.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for fast review, got %d: %s", recFast.Code, recFast.Body.String())
	}

	var resp clireview.CliReviewResponse
	if err := json.Unmarshal(recFast.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed decoding fast review response: %v", err)
	}
	if resp.Summary == "" {
		t.Fatalf("expected summary to be populated")
	}
}

func TestCliReviewController_AsyncReviewAndJobPolling(t *testing.T) {
	cliCtrl, _, _ := setupCliReviewTest()
	router := cliCtrl.ReviewRoutes()

	// Enqueue async review
	reqAsync, _ := http.NewRequest("POST", "/", bytes.NewBufferString(`{
		"diff": "diff --git a/token.go b/token.go\n+const Secret = \"pass123\""
	}`))
	reqAsync.Header.Set("X-Team-Key", "scandrix_team_test_123")
	recAsync := httptest.NewRecorder()
	router.ServeHTTP(recAsync, reqAsync)

	if recAsync.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted for async enqueue, got %d", recAsync.Code)
	}

	var enqResp map[string]any
	_ = json.Unmarshal(recAsync.Body.Bytes(), &enqResp)
	jobIDStr, ok := enqResp["jobId"].(string)
	if !ok || jobIDStr == "" {
		t.Fatalf("expected jobId in response: %+v", enqResp)
	}

	// Wait briefly for execution
	time.Sleep(50 * time.Millisecond)

	// Poll job status. This endpoint now requires authentication: it returned
	// another tenant's file paths, line numbers and findings to anonymous
	// callers (AUDIT_REMEDIATION.md F-15d).
	reqPoll, _ := http.NewRequest("GET", "/jobs/"+jobIDStr, nil)
	reqPoll.Header.Set("X-Team-Key", "scandrix_team_test_123")
	recPoll := httptest.NewRecorder()
	router.ServeHTTP(recPoll, reqPoll)

	if recPoll.Code != http.StatusOK {
		t.Fatalf("expected 200 OK polling job status, got %d: %s", recPoll.Code, recPoll.Body.String())
	}

	// Control: the same poll without credentials must be refused.
	reqAnon, _ := http.NewRequest("GET", "/jobs/"+jobIDStr, nil)
	recAnon := httptest.NewRecorder()
	router.ServeHTTP(recAnon, reqAnon)
	if recAnon.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous job polling returned %d; it must be 401", recAnon.Code)
	}
}

func TestCliReviewController_TrialReview(t *testing.T) {
	cliCtrl, _, _ := setupCliReviewTest()
	trialRouter := cliCtrl.TrialRoutes()

	// 1. Check status
	reqStatus, _ := http.NewRequest("GET", "/status?fingerprint=fp-test-1", nil)
	recStatus := httptest.NewRecorder()
	trialRouter.ServeHTTP(recStatus, reqStatus)

	if recStatus.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for trial status, got %d", recStatus.Code)
	}

	// 2. Perform 1st trial review
	trialReq1, _ := http.NewRequest("POST", "/review", bytes.NewBufferString(`{
		"diff": "diff --git a/a.go b/a.go\n+var x = 1",
		"fingerprint": "fp-test-1"
	}`))
	recTrial1 := httptest.NewRecorder()
	trialRouter.ServeHTTP(recTrial1, trialReq1)

	if recTrial1.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for 1st trial review, got %d: %s", recTrial1.Code, recTrial1.Body.String())
	}

	// 3. Perform 2nd trial review
	trialReq2, _ := http.NewRequest("POST", "/review", bytes.NewBufferString(`{
		"diff": "diff --git a/a.go b/a.go\n+var x = 2",
		"fingerprint": "fp-test-1"
	}`))
	recTrial2 := httptest.NewRecorder()
	trialRouter.ServeHTTP(recTrial2, trialReq2)

	if recTrial2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for 2nd trial review, got %d", recTrial2.Code)
	}

	// 4. 3rd review should be rate limited (429)
	trialReq3, _ := http.NewRequest("POST", "/review", bytes.NewBufferString(`{
		"diff": "diff --git a/a.go b/a.go\n+var x = 3",
		"fingerprint": "fp-test-1"
	}`))
	recTrial3 := httptest.NewRecorder()
	trialRouter.ServeHTTP(recTrial3, trialReq3)

	if recTrial3.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests, got %d", recTrial3.Code)
	}
}

func TestCliReviewController_PublicPRAndFeaturedReviews(t *testing.T) {
	cliCtrl, _, _ := setupCliReviewTest()
	publicRouter := cliCtrl.PublicRoutes()

	// 1. Invalid PR URL returns 400
	reqBadPR, _ := http.NewRequest("POST", "/review-pr", bytes.NewBufferString(`{
		"prUrl": "invalid-url",
		"fingerprint": "fp-123"
	}`))
	recBadPR := httptest.NewRecorder()
	publicRouter.ServeHTTP(recBadPR, reqBadPR)

	if recBadPR.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", recBadPR.Code)
	}

	// 2. List featured showcase reviews
	reqFeatured, _ := http.NewRequest("GET", "/featured-reviews", nil)
	recFeatured := httptest.NewRecorder()
	publicRouter.ServeHTTP(recFeatured, reqFeatured)

	if recFeatured.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for featured reviews, got %d", recFeatured.Code)
	}

	var featuredList []try.FeaturedReviewSummary
	if err := json.Unmarshal(recFeatured.Body.Bytes(), &featuredList); err != nil {
		t.Fatalf("failed unmarshaling featured reviews: %v", err)
	}
	if len(featuredList) == 0 {
		t.Fatalf("expected seeded featured reviews, got 0")
	}

	// 3. Get featured review detail by slug
	slug := featuredList[0].Slug
	reqSlug, _ := http.NewRequest("GET", "/featured-reviews/"+slug, nil)
	recSlug := httptest.NewRecorder()
	publicRouter.ServeHTTP(recSlug, reqSlug)

	if recSlug.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for slug %s, got %d", slug, recSlug.Code)
	}
}

func TestCliReviewController_SessionsEventsAndMemory(t *testing.T) {
	cliCtrl, _, _ := setupCliReviewTest()
	sessionRouter := cliCtrl.SessionsRoutes()
	memoryRouter := cliCtrl.MemoryRoutes()

	// Ingest telemetry event
	reqEvent, _ := http.NewRequest("POST", "/events", bytes.NewBufferString(`{
		"sessionId": "sess-test-1",
		"eventType": "stop",
		"payload": {
			"summary": "Decided to adopt Chi router for high performance HTTP routing"
		}
	}`))
	recEvent := httptest.NewRecorder()
	sessionRouter.ServeHTTP(recEvent, reqEvent)

	if recEvent.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for session event, got %d: %s", recEvent.Code, recEvent.Body.String())
	}

	// Ingest memory capture
	reqMem, _ := http.NewRequest("POST", "/captures", bytes.NewBufferString(`{
		"captureId": "cap-test-1",
		"summary": "Architecture decisions recorded"
	}`))
	recMem := httptest.NewRecorder()
	memoryRouter.ServeHTTP(recMem, reqMem)

	if recMem.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for memory capture, got %d: %s", recMem.Code, recMem.Body.String())
	}
}

func TestCliReviewsController_DashboardHistory(t *testing.T) {
	_, cliReviewsCtrl, _ := setupCliReviewTest()
	router := cliReviewsCtrl.Routes()

	// 1. Without workspace context -> 401
	reqNoAuth, _ := http.NewRequest("GET", "/executions", nil)
	recNoAuth := httptest.NewRecorder()
	router.ServeHTTP(recNoAuth, reqNoAuth)

	if recNoAuth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without workspace context, got %d", recNoAuth.Code)
	}

	// 2. With workspace context -> 200
	wsUUID := uuid.New()
	ctx := auth.WithWorkspaceContext(context.Background(), wsUUID)
	reqWithAuth, _ := http.NewRequestWithContext(ctx, "GET", "/executions?page=1&pageSize=10", nil)
	recWithAuth := httptest.NewRecorder()
	router.ServeHTTP(recWithAuth, reqWithAuth)

	if recWithAuth.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for executions list, got %d: %s", recWithAuth.Code, recWithAuth.Body.String())
	}

	// 3. Single-review lookup must also require an authenticated tenant.
	// This handler resolved no workspace at all before F-15c, so the lookup
	// was unscoped.
	reqNoAuthDetail, _ := http.NewRequest("GET", "/exec-uuid-1", nil)
	recNoAuthDetail := httptest.NewRecorder()
	router.ServeHTTP(recNoAuthDetail, reqNoAuthDetail)

	if recNoAuthDetail.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for execution lookup without workspace context, got %d", recNoAuthDetail.Code)
	}

	// 4. Another organization must not be able to read this review by ID.
	otherCtx := auth.WithWorkspaceContext(context.Background(), uuid.New())
	reqOtherOrg, _ := http.NewRequestWithContext(otherCtx, "GET", "/exec-uuid-1", nil)
	recOtherOrg := httptest.NewRecorder()
	router.ServeHTTP(recOtherOrg, reqOtherOrg)

	if recOtherOrg.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when another organization reads the review, got %d: %s",
			recOtherOrg.Code, recOtherOrg.Body.String())
	}

	// 5. The owning organization gets the review.
	ownerCtx := auth.WithWorkspaceContext(context.Background(), uuid.MustParse(seedCLIReviewOrg))
	reqDetail, _ := http.NewRequestWithContext(ownerCtx, "GET", "/exec-uuid-1", nil)
	recDetail := httptest.NewRecorder()
	router.ServeHTTP(recDetail, reqDetail)

	if recDetail.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for execution lookup, got %d: %s", recDetail.Code, recDetail.Body.String())
	}
}

// The list endpoint accepted an organization filter and then ignored it, so
// once any review was recorded every organization saw all of them.
func TestCliReviewsController_ListIsTenantScoped(t *testing.T) {
	_, cliReviewsCtrl, _ := setupCliReviewTest()
	router := cliReviewsCtrl.Routes()

	ctx := auth.WithWorkspaceContext(context.Background(), uuid.New())
	req, _ := http.NewRequestWithContext(ctx, "GET", "/executions?page=1&pageSize=10", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Items []clireview.CliReviewSummary `json:"items"`
		Total int                          `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	for _, item := range body.Items {
		if item.OrganizationID == "" || item.OrganizationID != seedCLIReviewOrg {
			t.Fatalf("list leaked a review owned by %q to another organization", item.OrganizationID)
		}
	}
}

// The /cli/business-validation endpoint used to answer
// {"success":true,"status":"QUEUED"} after decoding a request and doing nothing:
// no queue, no rule engine, no review. The CLI reported a queued job that never
// existed (AUDIT_REMEDIATION.md F-63 follow-on / fabricated-response class).
//
// It was first changed to an honest 501. The route has since been removed
// entirely, because the capability cannot be implemented by wiring: the
// IBusinessRulesValidationAgent it depends on has no implementation, and the
// Deep* pipeline stages it belongs to (~15 of them) are likewise never
// instantiated. A route that can only ever fail is a standing promise the
// product cannot keep, so the honest answer is that it does not exist.
func TestBusinessValidationRouteIsAbsent(t *testing.T) {
	ctrl := controllers.NewCliReviewController(nil, nil, nil, nil, nil)
	router := ctrl.Routes()

	body := strings.NewReader(`{"prUrl":"https://github.com/acme/repo/pull/1","prNumber":1}`)
	req := httptest.NewRequest(http.MethodPost, "/business-validation", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	// Not 200 (never fabricate), and not 501 either - the route is gone.
	if rec.Code == http.StatusOK {
		t.Fatalf("must not report success for work that is never done, got 200: %s", rec.Body.String())
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for the removed route, got %d: %s", rec.Code, rec.Body.String())
	}
}

// F-15d: the authenticated mount must be tenant-scoped, and the public trial
// mount must serve trial jobs while refusing tenant jobs.
//
// Both routes read the same job table, so the mount decides the gate: the
// authenticated handler requires an organization match, the public handler
// requires trial mode. Getting either wrong is a cross-tenant read.
func TestJobStatusIsTenantScoped(t *testing.T) {
	engine := clireview.NewEngine(nil, nil)

	tenantAJob, err := engine.EnqueueReview(context.Background(), clireview.EnqueueCliReviewInput{
		OrganizationID: "org-1",
		TeamID:         "team-1",
		Input:          clireview.CliReviewInput{Diff: "diff --git a/x b/x"},
	})
	if err != nil {
		t.Fatalf("enqueue tenant job: %v", err)
	}
	trialJob, err := engine.EnqueueReview(context.Background(), clireview.EnqueueCliReviewInput{
		IsTrialMode: true,
		Input:       clireview.CliReviewInput{Diff: "diff --git a/x b/x"},
	})
	if err != nil {
		t.Fatalf("enqueue trial job: %v", err)
	}

	keyValidator := clireview.NewKeyValidator("jwt-secret-for-test-32bytes-12345", nil)
	// Tenant A, and tenant B, so the cross-tenant attempt is real.
	keyValidator.RegisterTeamKey("scandrix_team_org_a", clireview.TeamKeyRecord{
		KeyID: "key-a", TeamID: "team-1", TeamName: "A",
		OrganizationID: "org-1", OrgName: "Org A", Active: true,
	})
	keyValidator.RegisterTeamKey("scandrix_team_org_b", clireview.TeamKeyRecord{
		KeyID: "key-b", TeamID: "team-2", TeamName: "B",
		OrganizationID: "org-2", OrgName: "Org B", Active: true,
	})

	ctrl := controllers.NewCliReviewController(engine, keyValidator, nil, nil, nil)
	authed := ctrl.ReviewRoutes() // /cli/review/jobs/{jobId}
	public := ctrl.PublicRoutes() // /cli/public/review/jobs/{jobId}

	call := func(router http.Handler, path, key string) int {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if key != "" {
			req.Header.Set("X-Team-Key", key)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec.Code
	}

	// Authenticated mount: the owner reads its own job.
	if code := call(authed, "/jobs/"+tenantAJob.JobID.String(), "scandrix_team_org_a"); code != http.StatusOK {
		t.Errorf("owning tenant should read its job, got %d", code)
	}
	// Another tenant must not, and gets 404 rather than 403 so that job
	// existence is not itself disclosed.
	if code := call(authed, "/jobs/"+tenantAJob.JobID.String(), "scandrix_team_org_b"); code != http.StatusNotFound {
		t.Errorf("cross-tenant read must be 404, got %d", code)
	}
	// No credential at all.
	if code := call(authed, "/jobs/"+tenantAJob.JobID.String(), ""); code != http.StatusUnauthorized {
		t.Errorf("unauthenticated read must be 401, got %d", code)
	}
	// A trial job has no owning org, so it is not served here.
	if code := call(authed, "/jobs/"+trialJob.JobID.String(), "scandrix_team_org_a"); code != http.StatusNotFound {
		t.Errorf("trial job must not be served on the authenticated mount, got %d", code)
	}

	// Public mount: trial jobs yes, tenant jobs never.
	if code := call(public, "/review/jobs/"+trialJob.JobID.String(), ""); code == http.StatusNotFound ||
		code == http.StatusUnauthorized {
		t.Errorf("public mount must serve a trial job without auth, got %d", code)
	}
	if code := call(public, "/review/jobs/"+tenantAJob.JobID.String(), ""); code != http.StatusNotFound {
		t.Errorf("public mount must not serve a tenant job, got %d", code)
	}
	// Even a valid tenant key must not unlock the public mount for a tenant job.
	if code := call(public, "/review/jobs/"+tenantAJob.JobID.String(), "scandrix_team_org_a"); code != http.StatusNotFound {
		t.Errorf("public mount must stay trial-only, got %d", code)
	}
}
