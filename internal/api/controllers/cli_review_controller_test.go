package controllers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/clireview"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/internal/try"
)

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
		ID:            "exec-uuid-1",
		CorrelationID: "corr-1",
		Summary:       "Zero defects found",
		IssuesCount:   0,
		FilesAnalyzed: 2,
		Duration:      120,
		Status:        "COMPLETED",
		CreatedAt:     time.Now().UTC(),
		Branch:        "feature/auth",
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

	// Poll job status
	reqPoll, _ := http.NewRequest("GET", "/jobs/"+jobIDStr, nil)
	recPoll := httptest.NewRecorder()
	router.ServeHTTP(recPoll, reqPoll)

	if recPoll.Code != http.StatusOK {
		t.Fatalf("expected 200 OK polling job status, got %d", recPoll.Code)
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

	// 3. Get single review by execution UUID
	reqDetail, _ := http.NewRequest("GET", "/exec-uuid-1", nil)
	recDetail := httptest.NewRecorder()
	router.ServeHTTP(recDetail, reqDetail)

	if recDetail.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for execution lookup, got %d", recDetail.Code)
	}
}
