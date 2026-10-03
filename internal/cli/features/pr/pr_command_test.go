// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package pr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/scandrix/backend/internal/cli/services/api"
	"github.com/scandrix/backend/internal/cli/types"
)

func TestFormatSuggestionMarkdown(t *testing.T) {
	md := FormatSuggestionMarkdown("SEC-001", "Do not hardcode secrets", "token = \"123\"", "token = os.Getenv(\"TOKEN\")")
	if md == "" {
		t.Fatal("Expected non-empty markdown")
	}
	if !containsStr(md, "**ScanDrix [SEC-001]**") {
		t.Errorf("Expected rule header, got: %s", md)
	}
	if !containsStr(md, "```suggestion\ntoken = os.Getenv(\"TOKEN\")\n```") {
		t.Errorf("Expected suggestion block, got: %s", md)
	}
}

func TestApplyLocalSuggestion(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "example.go")
	content := "package main\n\nfunc main() {\n\tsecret := \"bad\"\n\tprintln(secret)\n}\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	replacement := "\tsecret := os.Getenv(\"SECRET\")"
	err := ApplyLocalSuggestion(tmpDir, testFile, 4, 4, replacement)
	if err != nil {
		t.Fatalf("ApplyLocalSuggestion failed: %v", err)
	}

	patched, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed reading patched file: %v", err)
	}

	if !containsStr(string(patched), "os.Getenv(\"SECRET\")") {
		t.Errorf("Patched file missing replacement: %s", string(patched))
	}
}

func TestFilterIssuesAndComputeSummary(t *testing.T) {
	issues := []types.ReviewIssue{
		{RuleID: "R1", Severity: types.SeverityCritical, Category: "security", Message: "SQL Injection"},
		{RuleID: "R2", Severity: types.SeverityError, Category: "security", Message: "Hardcoded Key"},
		{RuleID: "R3", Severity: types.SeverityWarning, Category: "performance", Message: "N+1 Query"},
		{RuleID: "R4", Severity: types.SeverityInfo, Category: "style", Message: "Missing comment"},
	}

	// Filter by min severity = high / error
	filtered := FilterIssues(issues, "high", nil)
	if len(filtered) != 2 {
		t.Errorf("Expected 2 high/critical issues, got %d", len(filtered))
	}

	// Filter by category = performance
	perfFiltered := FilterIssues(issues, "low", []string{"performance"})
	if len(perfFiltered) != 1 || perfFiltered[0].RuleID != "R3" {
		t.Errorf("Expected 1 performance issue, got %+v", perfFiltered)
	}

	// Compute summary
	sum := ComputeSummary(42, issues)
	if sum.TotalIssues != 4 {
		t.Errorf("Expected 4 issues, got %d", sum.TotalIssues)
	}
	if sum.CriticalCount != 1 || sum.HighCount != 1 || sum.MediumCount != 1 || sum.LowCount != 0 {
		t.Errorf("Unexpected severity counts: %+v", sum)
	}
	if sum.Passed {
		t.Error("Summary should have failed due to critical/high issues")
	}

	// Compute summary for clean issues
	cleanSum := ComputeSummary(42, []types.ReviewIssue{
		{RuleID: "R4", Severity: types.SeverityInfo, Category: "style"},
	})
	if !cleanSum.Passed {
		t.Error("Summary with only info/low issues should pass")
	}
}

func TestPRCommentHandler_PostReviewComment(t *testing.T) {
	var capturedPR int
	var capturedBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/scm/comments" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var payload ReviewCommentPayload
		json.NewDecoder(r.Body).Decode(&payload)
		capturedPR = payload.PRNumber
		capturedBody = payload.Body

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"created"}`))
	}))
	defer server.Close()

	client := api.NewClient(server.URL, "token", "")
	handler := NewPRCommentHandler(client)

	err := handler.PostReviewComment(context.Background(), ReviewCommentPayload{
		PRNumber: 99,
		FilePath: "main.go",
		Line:     10,
		Body:     "Please fix this",
	})
	if err != nil {
		t.Fatalf("PostReviewComment failed: %v", err)
	}

	if capturedPR != 99 || capturedBody != "Please fix this" {
		t.Errorf("Unexpected captured comment: PR=%d, Body=%s", capturedPR, capturedBody)
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || (len(s) > len(sub) && containsSub(s, sub)))
}

func containsSub(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
