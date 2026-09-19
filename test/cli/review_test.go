// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/cli/cmd"
	"github.com/scandrix/backend/internal/cli/engine"
	"github.com/scandrix/backend/internal/cli/formatters"
	"github.com/scandrix/backend/internal/cli/services/api"
	"github.com/scandrix/backend/internal/cli/services/auth"
	ctxService "github.com/scandrix/backend/internal/cli/services/context"
	"github.com/scandrix/backend/internal/cli/services/git"
	"github.com/scandrix/backend/internal/cli/services/review"
	"github.com/scandrix/backend/pkg/models"
)

// CODE REVIEW PIPELINE & FORMATTERS TEST SUITE

const sampleVulnerableDiff = `diff --git a/auth.go b/auth.go
index 1111111..2222222 100644
--- a/auth.go
+++ b/auth.go
@@ -10,6 +10,12 @@ func Authenticate(user, pass string) bool {
+	// Insecure hardcoded credential evaluation
+	if pass == "hardcoded_admin_secret_123" {
+		return true
+	}
+	query := fmt.Sprintf("SELECT * FROM users WHERE pass = '%s'", pass)
+	db.Query(query)
 	return false
 }
`

func TestReviewService_RemoteExecution(t *testing.T) {
	setupTempHome(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/reviews" && r.Method == http.MethodPost {
			var req api.ReviewRequest
			_ = json.NewDecoder(r.Body).Decode(&req)

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(api.ReviewResponse{
				ReviewID:      "rev-12345",
				Status:        "passed",
				Summary:       "Autonomous multi-critic analysis detected security findings.",
				FilesAnalyzed: 1,
				DurationMs:    120,
				Findings: []models.CodeFinding{
					{
						ID:          uuid.New(),
						Title:       "SQL Injection in Database Query",
						Description: "User input directly formatted into raw SQL string",
						Severity:    models.SeverityCritical,
						Category:    "security",
						FilePath:    "auth.go",
						StartLine:   15,
						EndLine:     16,
					},
					{
						ID:          uuid.New(),
						Title:       "Hardcoded Credential Match",
						Description: "Hardcoded secret string found in conditional branch",
						Severity:    models.SeverityHigh,
						Category:    "security",
						FilePath:    "auth.go",
						StartLine:   11,
						EndLine:     13,
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	// Create temporary git repo with sample diff patch
	tmpDir := t.TempDir()
	patchFile := filepath.Join(tmpDir, "sample.patch")
	_ = os.WriteFile(patchFile, []byte(sampleVulnerableDiff), 0644)

	client := api.NewClient(server.URL, "mock-auth-token", "")
	authSvc := auth.NewService(client)
	_, _ = authSvc.SaveSessionTokens("mock-auth-token", "refresh-token", "dev@scandrix.dev", 3600)
	gitSvc := git.DefaultService()
	ctxSvc := ctxService.DefaultService()
	runner := engine.NewCLIRunner()

	svc := review.NewService(client, authSvc, gitSvc, ctxSvc, runner)

	opts := review.ReviewOptions{
		File:           patchFile,
		FailOnSeverity: "HIGH",
	}

	res, err := svc.Analyze(context.Background(), opts)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	if res.TotalFindings != 2 {
		t.Errorf("expected 2 findings, got %d", res.TotalFindings)
	}
	if res.CriticalCount != 1 {
		t.Errorf("expected 1 critical finding, got %d", res.CriticalCount)
	}
	if res.HighCount != 1 {
		t.Errorf("expected 1 high finding, got %d", res.HighCount)
	}
	if !res.IsBlocking {
		t.Errorf("expected review result to be blocking when FailOnSeverity=HIGH")
	}
}

func TestReviewService_FocusFilter(t *testing.T) {
	setupTempHome(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.ReviewResponse{
			FilesAnalyzed: 2,
			Findings: []models.CodeFinding{
				{Title: "SQL Injection", Category: "database", Severity: models.SeverityCritical},
				{Title: "Authentication bypass", Category: "auth", Severity: models.SeverityCritical},
				{Title: "Missing index", Category: "perf", Severity: models.SeverityLow},
			},
		})
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	patchFile := filepath.Join(tmpDir, "sample.patch")
	_ = os.WriteFile(patchFile, []byte(sampleVulnerableDiff), 0644)

	client := api.NewClient(server.URL, "token", "")
	authSvc := auth.NewService(client)
	_, _ = authSvc.SaveSessionTokens("token", "refresh", "test@scandrix.dev", 3600)
	svc := review.NewService(client, authSvc, git.DefaultService(), ctxService.DefaultService())

	opts := review.ReviewOptions{
		File:  patchFile,
		Focus: "auth",
	}

	res, err := svc.Analyze(context.Background(), opts)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	if res.TotalFindings != 1 {
		t.Fatalf("expected 1 finding matching 'auth', got %d", res.TotalFindings)
	}
	if res.Findings[0].Category != "auth" {
		t.Errorf("expected finding category 'auth', got %s", res.Findings[0].Category)
	}
}

func TestFormatters_PromptOutput(t *testing.T) {
	var buf bytes.Buffer
	opts := formatters.PromptFormatOptions{
		FilesAnalyzed: 3,
		DurationMs:    450,
		Summary:       "Security issues identified",
		Findings: []models.CodeFinding{
			{
				FilePath:      "server.go",
				StartLine:     42,
				Severity:      models.SeverityCritical,
				Title:         "Command Injection",
				Description:   "Shell command built from untrusted parameter",
				Remediation:   "Use exec.Command without sh -c",
				SuggestedDiff: "- exec.Command(\"sh\", \"-c\", cmd)\n+ exec.Command(cmd)",
			},
		},
	}

	err := formatters.RenderPrompt(&buf, opts)
	if err != nil {
		t.Fatalf("RenderPrompt failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "REVIEW_ANALYSIS_COMPLETE") {
		t.Errorf("expected REVIEW_ANALYSIS_COMPLETE in output")
	}
	if !strings.Contains(out, "CRITICAL: 1") {
		t.Errorf("expected CRITICAL: 1 in severity breakdown")
	}
	if !strings.Contains(out, "Command Injection") {
		t.Errorf("expected finding title in output")
	}
	if !strings.Contains(out, "END_REVIEW") {
		t.Errorf("expected END_REVIEW in output")
	}
}

func TestReviewCommand_FlagRegistration(t *testing.T) {
	reviewCmd := cmd.RootCmd.Commands()
	var found bool
	for _, c := range reviewCmd {
		if c.Name() == "review" {
			found = true
			if c.Flags().Lookup("staged") == nil {
				t.Errorf("expected --staged flag on review command")
			}
			if c.Flags().Lookup("branch") == nil {
				t.Errorf("expected --branch flag on review command")
			}
			if c.Flags().Lookup("commit") == nil {
				t.Errorf("expected --commit flag on review command")
			}
			if c.Flags().Lookup("fix") == nil {
				t.Errorf("expected --fix flag on review command")
			}
			if c.Flags().Lookup("focus") == nil {
				t.Errorf("expected --focus flag on review command")
			}
			if c.Flags().Lookup("fail-on-severity") == nil {
				t.Errorf("expected --fail-on-severity flag on review command")
			}
			break
		}
	}
	if !found {
		t.Fatalf("review command not registered in RootCmd")
	}
}
