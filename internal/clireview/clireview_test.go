package clireview_test

import (
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/clireview"
	"github.com/scandrix/backend/internal/rules"
)

func TestKeyValidator(t *testing.T) {
	jwtSecret := "test-secret-key-1234567890123456"
	validator := clireview.NewKeyValidator(jwtSecret, nil)

	// 1. Register a team key
	validator.RegisterTeamKey("scandrix_team_valid_123", clireview.TeamKeyRecord{
		KeyID:          "key-1",
		TeamID:         "team-1",
		TeamName:       "Core Team",
		OrganizationID: "org-1",
		OrgName:        "Acme Corp",
		Active:         true,
	})

	// Test Valid Team Key via TeamKey field
	res1 := validator.ValidateCliKey(clireview.ValidateCliKeyInput{
		TeamKey:  "scandrix_team_valid_123",
		DeviceID: "device-mac-1",
	})
	if !res1.Valid || *res1.TeamID != "team-1" || res1.DeviceToken == "" {
		t.Fatalf("expected valid team key validation with device token, got %+v", res1)
	}

	// Test Valid Team Key via Bearer header
	resBearer := validator.ValidateCliKey(clireview.ValidateCliKeyInput{
		AuthHeader: "Bearer scandrix_team_valid_123",
	})
	if !resBearer.Valid || *resBearer.TeamID != "team-1" {
		t.Fatalf("expected valid bearer team key, got %+v", resBearer)
	}

	// Test Invalid Team Key
	resInvalid := validator.ValidateCliKey(clireview.ValidateCliKeyInput{
		TeamKey: "scandrix_invalid_key",
	})
	if resInvalid.Valid {
		t.Fatalf("expected invalid team key to fail")
	}

	// 2. Test JWT Bearer Token
	claims := jwt.MapClaims{
		"email":            "dev@scandrix.dev",
		"organizationId":   "org-jwt-1",
		"teamId":           "team-jwt-1",
		"teamName":         "Platform",
		"organizationName": "Acme Global",
		"exp":              time.Now().Add(time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(jwtSecret))
	if err != nil {
		t.Fatalf("failed signing jwt: %v", err)
	}

	resJWT := validator.ValidateCliKey(clireview.ValidateCliKeyInput{
		AuthHeader: "Bearer " + tokenStr,
	})
	if !resJWT.Valid || resJWT.Email != "dev@scandrix.dev" || *resJWT.TeamID != "team-jwt-1" {
		t.Fatalf("expected valid JWT validation, got %+v", resJWT)
	}

	// Test Expired / Invalid JWT
	resBadJWT := validator.ValidateCliKey(clireview.ValidateCliKeyInput{
		AuthHeader: "Bearer invalid.jwt.token",
	})
	if resBadJWT.Valid {
		t.Fatalf("expected invalid JWT to fail")
	}
}

func TestRateLimiters(t *testing.T) {
	// 1. Trial Rate Limiter
	trialLimiter := clireview.NewTrialRateLimiter(2, 100*time.Millisecond)

	r1 := trialLimiter.CheckRateLimit("fp-1")
	if !r1.Allowed || r1.Remaining != 1 {
		t.Fatalf("expected 1st trial allowed with 1 remaining, got %+v", r1)
	}

	r2 := trialLimiter.CheckRateLimit("fp-1")
	if !r2.Allowed || r2.Remaining != 0 {
		t.Fatalf("expected 2nd trial allowed with 0 remaining, got %+v", r2)
	}

	r3 := trialLimiter.CheckRateLimit("fp-1")
	if r3.Allowed {
		t.Fatalf("expected 3rd trial blocked")
	}

	time.Sleep(120 * time.Millisecond)
	rAfter := trialLimiter.CheckRateLimit("fp-1")
	if !rAfter.Allowed {
		t.Fatalf("expected trial allowed after window expiration")
	}

	// 2. Authenticated Rate Limiter
	authLimiter := clireview.NewAuthenticatedRateLimiter(2)
	if !authLimiter.Acquire("team-1") {
		t.Fatalf("expected 1st acquire allowed")
	}
	if !authLimiter.Acquire("team-1") {
		t.Fatalf("expected 2nd acquire allowed")
	}
	if authLimiter.Acquire("team-1") {
		t.Fatalf("expected 3rd acquire blocked")
	}
	authLimiter.Release("team-1")
	if !authLimiter.Acquire("team-1") {
		t.Fatalf("expected acquire allowed after release")
	}
}

func TestPipelineAndEngine(t *testing.T) {
	evaluator, err := rules.NewEvaluator(rules.DefaultCatalog())
	if err != nil {
		t.Fatalf("failed creating rules evaluator: %v", err)
	}

	authLimiter := clireview.NewAuthenticatedRateLimiter(5)
	engine := clireview.NewEngine(evaluator, authLimiter)

	diffText := `diff --git a/auth/jwt.go b/auth/jwt.go
new file mode 100644
index 0000000..e69de29
--- /dev/null
+++ b/auth/jwt.go
@@ -0,0 +1,5 @@
+package auth
+
+func BadSecret() string {
+	return "hardcoded-secret-key-123"
+}
`

	input := clireview.CliReviewInput{
		Diff: diffText,
		Config: &clireview.CliReviewConfig{
			Fast: true,
		},
	}

	// Synchronous execution
	ctx := context.Background()
	res, err := engine.ExecuteReview(ctx, input)
	if err != nil {
		t.Fatalf("failed executing review: %v", err)
	}
	if res.FilesAnalyzed != 1 {
		t.Fatalf("expected 1 file analyzed, got %d", res.FilesAnalyzed)
	}

	// Asynchronous Enqueue & Wait
	enqueueRes, err := engine.EnqueueReview(ctx, clireview.EnqueueCliReviewInput{
		OrganizationID: "org-1",
		TeamID:         "team-1",
		Input:          input,
	})
	if err != nil {
		t.Fatalf("failed enqueuing review: %v", err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	asyncRes, err := engine.WaitForJob(waitCtx, enqueueRes.JobID, 20*time.Millisecond)
	if err != nil {
		t.Fatalf("failed waiting for job: %v", err)
	}
	if asyncRes.FilesAnalyzed != 1 {
		t.Fatalf("expected 1 file analyzed in async result, got %d", asyncRes.FilesAnalyzed)
	}

	// Check status
	status, ok := engine.GetJobStatus(enqueueRes.JobID)
	if !ok || status.Status != "COMPLETED" {
		t.Fatalf("expected job status COMPLETED, got %+v", status)
	}
}

func TestSessionClassifierAndIngester(t *testing.T) {
	store := clireview.NewSessionStore()
	classifier := clireview.NewSessionClassifier()
	ingester := clireview.NewSessionIngester(store, classifier)

	sessionID := "sess-" + uuid.New().String()

	// Ingest edit event
	_, err := ingester.IngestEvent(context.Background(), clireview.SessionEvent{
		SessionID: sessionID,
		EventType: "file_edit",
		Payload: map[string]any{
			"modifiedFiles": []string{"pkg/auth/token.go"},
			"summary":       "Decided to use Argon2id for password hashing instead of bcrypt because of quantum resistance requirements.",
		},
	})
	if err != nil {
		t.Fatalf("failed ingesting edit event: %v", err)
	}

	// Ingest stop event to trigger classification
	capture, err := ingester.IngestEvent(context.Background(), clireview.SessionEvent{
		SessionID: sessionID,
		EventType: "stop",
		Payload: map[string]any{
			"summary": "We adopted the repository pattern for all database access layers and chose strict convention for tenant isolation.",
		},
	})
	if err != nil {
		t.Fatalf("failed ingesting stop event: %v", err)
	}

	if capture.Status != "completed" {
		t.Fatalf("expected capture status completed, got %s", capture.Status)
	}
	if len(capture.ClassifiedDecisions) == 0 {
		t.Fatalf("expected classified decisions to be populated")
	}

	first := capture.ClassifiedDecisions[0]
	if first.Decision == "" {
		t.Fatalf("expected decision text to be non-empty")
	}
}

func TestTraceContextAndDashboard(t *testing.T) {
	store := clireview.NewSessionStore()
	sessionID := "trace-sess-1"
	store.SaveCapture(&clireview.CliSessionCapture{
		CaptureID: sessionID,
		Summary:   "Refactored authentication architecture",
		ClassifiedDecisions: []clireview.CliSessionClassifiedDecision{
			{
				Type:       clireview.DecisionArchitecturalDetail,
				Decision:   "Extract auth service into separate package",
				Confidence: 0.9,
			},
		},
	})

	traceSvc := clireview.NewTraceContextService(store)
	pack, err := traceSvc.BuildContextPack(sessionID)
	if err != nil {
		t.Fatalf("failed building context pack: %v", err)
	}
	if len(pack.Decisions) != 1 {
		t.Fatalf("expected 1 decision in pack, got %d", len(pack.Decisions))
	}

	comment := traceSvc.FormatTracePrComment(pack)
	if len(comment) == 0 || !testing.Verbose() && comment == "" {
		t.Fatalf("expected comment to be generated")
	}

	// Dashboard store
	dashStore := clireview.NewDashboardStore()
	dashStore.RecordReview(clireview.CliReviewSummary{
		ID:             "rev-1",
		OrganizationID: "org-1",
		CorrelationID:  "corr-1",
		Summary:        "Zero vulnerabilities found",
		IssuesCount:    0,
		FilesAnalyzed:  3,
		Duration:       150,
		Status:         "COMPLETED",
		CreatedAt:      time.Now().UTC(),
		Branch:         "feature/auth",
	})

	dashStore.RecordReview(clireview.CliReviewSummary{
		ID:             "rev-2",
		OrganizationID: "org-1",
		CorrelationID:  "corr-2",
		Summary:        "Found 1 critical flaw",
		IssuesCount:    1,
		FilesAnalyzed:  5,
		Duration:       300,
		Status:         "COMPLETED",
		CreatedAt:      time.Now().UTC(),
		Branch:         "main",
	})

	list := dashStore.GetCliReviews(clireview.CliReviewsQuery{
		OrganizationID: "org-1",
		Search:         "critical",
	})
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].ID != "rev-2" {
		t.Fatalf("expected 1 matched review with ID rev-2, got %+v", list)
	}

	r, err := dashStore.GetCliReviewByID("rev-1", "org-1")
	if err != nil || r.Summary != "Zero vulnerabilities found" {
		t.Fatalf("expected rev-1 lookup, got %+v, err: %v", r, err)
	}

	// Another organization must not be able to read the same record by ID.
	if _, err := dashStore.GetCliReviewByID("rev-1", "org-2"); err == nil {
		t.Fatal("expected rev-1 lookup from another organization to be refused")
	}

	// And the list must not leak it either.
	otherList := dashStore.GetCliReviews(clireview.CliReviewsQuery{OrganizationID: "org-2"})
	if otherList.Total != 0 {
		t.Fatalf("expected 0 reviews for org-2, got %d", otherList.Total)
	}
}

func TestPublicPrService(t *testing.T) {
	evaluator, err := rules.NewEvaluator(rules.DefaultCatalog())
	if err != nil {
		t.Fatalf("failed creating rules evaluator: %v", err)
	}
	engine := clireview.NewEngine(evaluator, nil)
	trialLimiter := clireview.NewTrialRateLimiter(2, 500*time.Millisecond)
	service := clireview.NewPublicPrService(trialLimiter, engine)

	ctx := context.Background()

	// 1. Invalid PR URL
	badRes := service.Execute(ctx, "not-a-github-url", "fp-1")
	if badRes.OK || badRes.StatusCode != 400 || badRes.Code != "invalid_url" {
		t.Fatalf("expected invalid_url error, got %+v", badRes)
	}

	// 2. Rate limit exhaustion
	trialLimiter.CheckRateLimit("fp-blocked")
	trialLimiter.CheckRateLimit("fp-blocked")

	rateLimitedRes := service.Execute(ctx, "https://github.com/scandrix/repo/pull/1", "fp-blocked")
	if rateLimitedRes.OK || rateLimitedRes.StatusCode != 429 || rateLimitedRes.Code != "rate_limited" {
		t.Fatalf("expected rate_limited response, got %+v", rateLimitedRes)
	}
}

func TestPrepareCliFilesAndFormatting(t *testing.T) {
	// 1. Test PrepareCliFiles with explicit files input
	input := clireview.CliReviewInput{
		Config: &clireview.CliReviewConfig{
			Files: []clireview.CliFileInput{
				{
					Path:    "pkg/auth/token.go",
					Content: "package auth",
					Status:  "modified",
					Diff:    "@@ -1,3 +1,3 @@\n-oldToken\n+newToken\n+anotherLine\n",
				},
			},
		},
	}

	files := clireview.PrepareCliFiles(input)
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Additions != 2 || files[0].Deletions != 1 {
		t.Fatalf("expected 2 additions, 1 deletion, got %+v", files[0])
	}

	// 2. Test FormatCliOutput
	zeroRes := clireview.FormatCliOutput(nil, 3, time.Now().Add(-100*time.Millisecond))
	if zeroRes.FilesAnalyzed != 3 || zeroRes.Summary == "" {
		t.Fatalf("unexpected zero issues response: %+v", zeroRes)
	}

	endLine := 10
	withIssues := clireview.FormatCliOutput([]clireview.CliReviewIssue{
		{
			File:     "pkg/auth/token.go",
			Line:     10,
			EndLine:  &endLine,
			Severity: "CRITICAL",
			Message:  "Detected hardcoded secret",
		},
	}, 1, time.Now().Add(-50*time.Millisecond))
	if len(withIssues.Issues) != 1 || withIssues.Summary == "" {
		t.Fatalf("unexpected with issues response: %+v", withIssues)
	}
}
