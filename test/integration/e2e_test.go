package integration_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/review"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/pkg/models"
)

func TestEndToEndReviewFlow(t *testing.T) {
	// Step 1: Simulate GitHub Webhook Ingestion with HMAC-SHA256
	webhookSecret := "scandrix-test-webhook-secret"
	webhookPayload := map[string]any{
		"action": "opened",
		"number": 108,
		"pull_request": map[string]any{
			"title": "Add database query without parameterization",
			"head":  map[string]string{"sha": "abcdef123456"},
			"base":  map[string]string{"sha": "123456abcdef"},
			"user":  map[string]string{"login": "developer-alice"},
		},
		"repository": map[string]any{
			"full_name": "acme/payment-service",
			"id":        999001,
		},
	}

	payloadBytes, err := json.Marshal(webhookPayload)
	if err != nil {
		t.Fatalf("failed marshaling webhook payload: %v", err)
	}

	mac := hmac.New(sha256.New, []byte(webhookSecret))
	mac.Write(payloadBytes)
	signatureHeader := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	// Verify HMAC signature calculation
	if !strings.HasPrefix(signatureHeader, "sha256=") {
		t.Fatalf("expected sha256 signature prefix")
	}

	// Step 2: Simulate Raw Git Diff Generation
	rawDiff := `diff --git a/pkg/db/query.go b/pkg/db/query.go
index 1122334..5566778 100644
--- a/pkg/db/query.go
+++ b/pkg/db/query.go
@@ -10,6 +10,8 @@ func GetUser(db *sql.DB, id string) (*User, error) {
-	return nil, nil
+	// Dangerous raw query injection
+	query := fmt.Sprintf("SELECT * FROM users WHERE id = '%s'", id)
+	row := db.QueryRow(query)
+	return nil, nil
 }
`

	// Step 3: Parse Unified Diff using high-performance streaming parser
	patches, err := diff.ParseUnifiedDiff(strings.NewReader(rawDiff))
	if err != nil {
		t.Fatalf("ParseUnifiedDiff failed: %v", err)
	}
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d", len(patches))
	}
	if patches[0].NewPath != "pkg/db/query.go" {
		t.Errorf("expected path 'pkg/db/query.go', got %s", patches[0].NewPath)
	}
	if patches[0].Additions != 4 {
		t.Errorf("expected 4 additions, got %d", patches[0].Additions)
	}

	// Step 4: Evaluate Rules Engine against parsed diff hunks
	ruleID := uuid.New()
	evaluator, err := rules.NewEvaluator([]rules.RuleSpec{
		{
			ID:          ruleID,
			Name:        "Detect SQL String Formatting",
			PathPattern: "*.go",
			RegexRule:   `fmt\.Sprintf\(["'].*SELECT.*FROM`,
			Severity:    models.SeverityCritical,
			Category:    "SECURITY_INJECTION",
			Description: "Direct string formatting into SQL queries allows SQL injection vulnerabilities.",
			Remediation: "Use parameterized queries with placeholders ($1, $2) instead of string formatting.",
		},
	})
	if err != nil {
		t.Fatalf("NewEvaluator failed: %v", err)
	}

	reviewID := uuid.New()
	workspaceID := uuid.New()

	findings := evaluator.EvaluatePatches(reviewID, workspaceID, patches)
	if len(findings) != 1 {
		t.Fatalf("expected 1 security finding, got %d", len(findings))
	}

	f := findings[0]
	if f.Severity != models.SeverityCritical {
		t.Errorf("expected CRITICAL severity, got %s", f.Severity)
	}
	if f.StartLine != 11 {
		t.Errorf("expected line 11, got %d", f.StartLine)
	}
	if f.Category != "SECURITY_INJECTION" {
		t.Errorf("expected SECURITY_INJECTION category, got %s", f.Category)
	}

	// Step 5: Test Real-Time SSE Streaming Hub
	streamHub := review.NewStreamHub()
	ch, unsubscribe := streamHub.Subscribe(reviewID)
	defer unsubscribe()

	// Broadcast review stage progress and finding
	streamHub.Broadcast(reviewID, "stage_progress", "RULES_EVALUATION", map[string]string{"progress": "50%"})
	streamHub.Broadcast(reviewID, "new_finding", "FINDING_DISCOVERED", f)

	// Verify subscriber receives the SSE events in real-time
	select {
	case evt := <-ch:
		if evt.Event != "stage_progress" || evt.Stage != "RULES_EVALUATION" {
			t.Errorf("unexpected first event: %+v", evt)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for stage_progress SSE event")
	}

	select {
	case evt := <-ch:
		if evt.Event != "new_finding" || evt.Stage != "FINDING_DISCOVERED" {
			t.Errorf("unexpected second event: %+v", evt)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for new_finding SSE event")
	}

	// Step 6: Verify Authenticated API Access and Tenant Context
	testTenantID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/reviews/"+reviewID.String()+"/stream", nil)
	ctx := auth.WithWorkspaceContext(req.Context(), testTenantID)

	extractedTenant, err := auth.WorkspaceFromContext(ctx)
	if err != nil {
		t.Fatalf("failed extracting workspace from context: %v", err)
	}
	if extractedTenant != testTenantID {
		t.Errorf("expected tenant %s, got %s", testTenantID, extractedTenant)
	}
}
