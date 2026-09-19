// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/cli/engine"
	"github.com/scandrix/backend/internal/llm"
	"github.com/scandrix/backend/internal/prompts"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/pkg/models"
)

func TestV3PRReviewE2E(t *testing.T) {
	// 1. Unified Diff with a critical vulnerability and benign changes
	rawDiff := `diff --git a/services/auth/login.go b/services/auth/login.go
index a1b2c3d..e4f5g6h 100644
--- a/services/auth/login.go
+++ b/services/auth/login.go
@@ -20,6 +20,10 @@ func Authenticate(user, pass string) (*Session, error) {
+	// Dangerous back door bypass
+	if pass == "SUPER_SECRET_ADMIN_TOKEN" {
+		return &Session{Role: "admin", User: user}, nil
+	}
 	return nil, ErrUnauthorized
 }
diff --git a/services/user/profile.go b/services/user/profile.go
index b2c3d4e..f5g6h7i 100644
--- a/services/user/profile.go
+++ b/services/user/profile.go
@@ -15,4 +15,5 @@ func GetProfile(id string) *Profile {
+	// Added sanitization check
 	return &Profile{ID: id}
 }
`

	// 2. Parse unified diff
	patches, err := diff.ParseUnifiedDiff(strings.NewReader(rawDiff))
	if err != nil {
		t.Fatalf("Failed to parse diff: %v", err)
	}
	if len(patches) != 2 {
		t.Fatalf("Expected 2 patches, got %d", len(patches))
	}

	// 3. Mock LLM Server responding with v3.0 Canonical Finding Schema
	v3ResponsePayload := map[string]any{
		"engine_version": "3.0",
		"review_verdict": "REQUEST_CHANGES",
		"risk_score":     90,
		"summary":        "Identified critical hardcoded authentication bypass backdoor in login service.",
		"positive_observations": []string{
			"Profile retrieval includes sanitization checks.",
		},
		"statistics": map[string]any{
			"files_analyzed":    2,
			"total_findings":    1,
			"blocking_findings": 1,
			"critical":          1,
			"high":              0,
			"medium":            0,
			"low":               0,
			"info":              0,
		},
		"findings": []map[string]any{
			{
				"id":                           "SDXF-SEC-001",
				"category":                     "SECURITY",
				"severity":                     "CRITICAL",
				"confidence":                   "HIGH",
				"blocking":                     true,
				"file_path":                    "services/auth/login.go",
				"start_line":                   22,
				"end_line":                     24,
				"title":                        "Hardcoded Administrative Authentication Bypass",
				"description":                  "Hardcoded comparison allows anyone supplying the backdoor string to achieve administrative session elevation.",
				"evidence":                     "if pass == \"SUPER_SECRET_ADMIN_TOKEN\" {",
				"impact":                       "Full unauthenticated administrative account takeover.",
				"exploit_scenario":             "An attacker passes the backdoor token in the password field to obtain admin rights.",
				"root_cause":                   "Backdoor bypass hardcoded in production authentication handler.",
				"preconditions":                "Login route exposed to public network.",
				"existing_mitigations_checked": "No upstream filter prevents reaching this condition.",
				"remediation":                  "Remove hardcoded token and authenticate exclusively against password hashes.",
				"suggested_diff":               "- if pass == \"SUPER_SECRET_ADMIN_TOKEN\" {\n- \treturn &Session{Role: \"admin\", User: user}, nil\n- }",
				"blocking_justification":       "Authentication bypass constitutes an immediate critical security threat.",
				"cwe_id":                       "CWE-798",
				"owasp":                        "A07:2021-Identification and Authentication Failures",
				"references": []string{
					"https://cwe.mitre.org/data/definitions/798.html",
				},
			},
		},
		"policy_compliance": map[string]any{
			"custom_policies_evaluated": true,
			"violations": []map[string]any{
				{
					"policy":      "No Hardcoded Credentials",
					"violation":   "Hardcoded backdoor token found in login logic",
					"finding_ids": []string{"SDXF-SEC-001"},
				},
			},
		},
	}

	responseJSON, _ := json.Marshal(v3ResponsePayload)

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify standard completion call
		w.Header().Set("Content-Type", "application/json")
		// OpenAI compatible chat completion envelope
		completionResp := map[string]any{
			"id":      "chatcmpl-test-123",
			"object":  "chat.completion",
			"created": 1700000000,
			"model":   "gemini-2.5-flash",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": string(responseJSON),
					},
					"finish_reason": "stop",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(completionResp)
	}))
	defer mockServer.Close()

	// 4. Initialize LLM Gateway targeting the mock server
	gw := llm.NewGateway(
		"",
		"test-key-mock",
		"",
		"",
		llm.WithOpenAIBaseURL(mockServer.URL+"/v1"),
	)

	reviewReq := llm.ReviewRequest{
		WorkspaceID:   uuid.New(),
		RepoNamespace: "github.com/scandrix/enterprise-auth",
		PullTitle:     "feat: updates to login and user profile",
		DiffContent:   rawDiff,
		CustomRules:   "- Strictly block any hardcoded credentials or bypasses",
	}

	res, err := gw.AnalyzeDiff(context.Background(), reviewReq)
	if err != nil {
		t.Fatalf("AnalyzeDiff failed: %v", err)
	}

	if res == nil {
		t.Fatal("Expected non-nil ReviewResponse")
	}

	if res.EngineVersion != "3.0" {
		t.Errorf("Expected engine version '3.0', got '%s'", res.EngineVersion)
	}

	if res.ReviewVerdict != "REQUEST_CHANGES" {
		t.Errorf("Expected verdict 'REQUEST_CHANGES', got '%s'", res.ReviewVerdict)
	}

	if len(res.Findings) != 1 {
		t.Fatalf("Expected 1 finding, got %d", len(res.Findings))
	}

	finding := res.Findings[0]
	if finding.Severity != "CRITICAL" {
		t.Errorf("Expected severity CRITICAL, got %s", finding.Severity)
	}
	if finding.Confidence != "HIGH" {
		t.Errorf("Expected confidence HIGH, got %s", finding.Confidence)
	}
	if !finding.Blocking {
		t.Errorf("Expected finding to be blocking")
	}
	if finding.Evidence != "if pass == \"SUPER_SECRET_ADMIN_TOKEN\" {" {
		t.Errorf("Evidence mismatch: %s", finding.Evidence)
	}
	if finding.SuggestedDiff == "" {
		t.Errorf("SuggestedDiff should not be empty")
	}

	// 5. Convert to models.CodeFinding and test Output Formatter exports
	modelFindings := make([]models.CodeFinding, 0, len(res.Findings))
	for _, f := range res.Findings {
		modelFindings = append(modelFindings, models.CodeFinding{
			ReviewID:      uuid.New(),
			WorkspaceID:   reviewReq.WorkspaceID,
			FilePath:      f.FilePath,
			StartLine:     f.StartLine,
			EndLine:       f.EndLine,
			Severity:      models.SeverityCritical,
			Category:      f.Category,
			Title:         f.Title,
			Description:   f.Description,
			Remediation:   f.Remediation,
			SuggestedDiff: f.SuggestedDiff,
		})
	}
	if len(modelFindings) != 1 {
		t.Fatalf("Expected 1 converted CodeFinding, got %d", len(modelFindings))
	}

	cliResult := &engine.CLIResult{
		Status:        "failed",
		FilesReviewed: 2,
		TotalFindings: 1,
		CriticalCount: 1,
		Findings:      modelFindings,
		IsBlocking:    true,
	}

	formatter := engine.NewOutputFormatter()

	// Test SARIF Export
	var sarifBuf bytes.Buffer
	if err := formatter.Render(&sarifBuf, cliResult, engine.FormatSARIF); err != nil {
		t.Fatalf("SARIF render failed: %v", err)
	}
	if !strings.Contains(sarifBuf.String(), "Hardcoded Administrative Authentication Bypass") {
		t.Errorf("SARIF missing finding title: %s", sarifBuf.String())
	}

	// Test CSV Export
	var csvBuf bytes.Buffer
	if err := formatter.Render(&csvBuf, cliResult, engine.FormatCSV); err != nil {
		t.Fatalf("CSV render failed: %v", err)
	}
	if !strings.Contains(csvBuf.String(), "services/auth/login.go") || !strings.Contains(csvBuf.String(), "CRITICAL") {
		t.Errorf("CSV missing expected details: %s", csvBuf.String())
	}

	// Test Markdown Export
	var mdBuf bytes.Buffer
	if err := formatter.Render(&mdBuf, cliResult, engine.FormatMarkdown); err != nil {
		t.Fatalf("Markdown render failed: %v", err)
	}
	if !strings.Contains(mdBuf.String(), "Hardcoded Administrative Authentication Bypass") {
		t.Errorf("Markdown missing expected details: %s", mdBuf.String())
	}
}

func TestV3PromptDirectivesIntegrity(t *testing.T) {
	// Verify that shared foundation tokens are completely resolved
	req := llm.ReviewRequest{
		WorkspaceID:   uuid.New(),
		RepoNamespace: "github.com/scandrix/sample",
		PullTitle:     "test PR",
		DiffContent:   "dummy diff",
	}

	systemPrompt := prompts.ApplyFoundations(prompts.FoundationPromptInjectionGuardrails)
	if strings.Contains(systemPrompt, "{{FOUNDATION_") {
		t.Errorf("Found unexpanded token in FoundationPromptInjectionGuardrails: %s", systemPrompt)
	}

	evidenceGate := prompts.ApplyFoundations(prompts.FoundationEvidenceGate)
	if !strings.Contains(evidenceGate, "FINDING EVIDENCE GATE") {
		t.Errorf("Evidence gate header missing")
	}

	qualityGate := prompts.ApplyFoundations(prompts.FoundationQualityGate)
	if !strings.Contains(qualityGate, "FINAL QUALITY GATE") {
		t.Errorf("Quality gate header missing")
	}

	_ = req
}
