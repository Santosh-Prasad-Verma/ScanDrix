// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package review

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	reviewSvc "github.com/scandrix/backend/internal/cli/services/review"
	"github.com/scandrix/backend/internal/cli/types"
	"github.com/scandrix/backend/pkg/models"
)

func TestReviewOrchestratorInitialization(t *testing.T) {
	orchestrator := NewReviewOrchestrator(nil, nil, nil, nil, nil)
	if orchestrator == nil {
		t.Fatalf("expected non-nil orchestrator")
	}
	if orchestrator.gitService == nil {
		t.Errorf("expected initialized gitService")
	}
	if orchestrator.authService == nil {
		t.Errorf("expected initialized authService")
	}
	if orchestrator.reviewService == nil {
		t.Errorf("expected initialized reviewService")
	}
	if orchestrator.contextService == nil {
		t.Errorf("expected initialized contextService")
	}
}

func TestConvertServiceResult(t *testing.T) {
	orchestrator := NewReviewOrchestrator(nil, nil, nil, nil, nil)

	findingID := uuid.New()
	serviceResult := &reviewSvc.ReviewResult{
		ReviewID:      "rev-12345",
		Status:        "failed",
		Summary:       "Security vulnerability identified in authentication token validation.",
		FilesAnalyzed: 3,
		TotalFindings: 1,
		CriticalCount: 1,
		DurationMs:    450,
		ExitCode:      1,
		Findings: []models.CodeFinding{
			{
				ID:            findingID,
				FilePath:      "internal/auth/jwt.go",
				StartLine:     42,
				EndLine:       45,
				Severity:      models.SeverityCritical,
				Category:      "security_vulnerability",
				Title:         "Insecure JWT algorithm none accepted",
				Description:   "Accepting 'none' algorithm bypasses signature verification entirely.",
				Remediation:   "Explicitly reject alg: none and enforce HMAC or RSA verification.",
				SuggestedDiff: "if token.Header[\"alg\"] == \"none\" {\n\treturn nil, errors.New(\"unsupported algorithm\")\n}",
			},
		},
	}

	result := orchestrator.convertServiceResult(serviceResult, "diff --git ...")
	if result == nil {
		t.Fatalf("expected non-nil converted result")
	}

	if result.ReviewID != "rev-12345" {
		t.Errorf("expected review ID rev-12345, got %s", result.ReviewID)
	}
	if len(result.Issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(result.Issues))
	}

	issue := result.Issues[0]
	if issue.ID != findingID.String() {
		t.Errorf("expected ID %s, got %s", findingID.String(), issue.ID)
	}
	if issue.Severity != types.SeverityCritical {
		t.Errorf("expected critical severity, got %s", issue.Severity)
	}
	if !issue.Fixable {
		t.Errorf("expected issue to be marked fixable")
	}
	if issue.Fix == nil || issue.Fix.NewCode == "" {
		t.Errorf("expected valid fix code")
	}
	if result.Stats.CriticalCount != 1 {
		t.Errorf("expected critical count 1, got %d", result.Stats.CriticalCount)
	}
	if result.Duration != 450*time.Millisecond {
		t.Errorf("expected duration 450ms, got %v", result.Duration)
	}
}

func TestFormatOutput(t *testing.T) {
	orchestrator := NewReviewOrchestrator(nil, nil, nil, nil, nil)
	result := &types.ReviewResult{
		ReviewID:      "rev-test",
		Status:        "passed",
		Summary:       "All security and quality checks passed.",
		FilesAnalyzed: 5,
		Issues:        []types.ReviewIssue{},
		Stats: types.ReviewStats{
			TotalIssues: 0,
		},
	}

	// JSON format
	jsonOut, err := orchestrator.formatOutput(result, "json")
	if err != nil {
		t.Fatalf("unexpected json format error: %v", err)
	}
	if len(jsonOut) == 0 {
		t.Errorf("expected non-empty json output")
	}

	// Markdown format
	mdOut, err := orchestrator.formatOutput(result, "markdown")
	if err != nil {
		t.Fatalf("unexpected markdown format error: %v", err)
	}
	if len(mdOut) == 0 {
		t.Errorf("expected non-empty markdown output")
	}

	// SARIF format
	sarifOut, err := orchestrator.formatOutput(result, "sarif")
	if err != nil {
		t.Fatalf("unexpected sarif format error: %v", err)
	}
	if len(sarifOut) == 0 {
		t.Errorf("expected non-empty sarif output")
	}

	// Terminal format
	termOut, err := orchestrator.formatOutput(result, "terminal")
	if err != nil {
		t.Fatalf("unexpected terminal format error: %v", err)
	}
	if len(termOut) == 0 {
		t.Errorf("expected non-empty terminal output")
	}
}

func TestExecuteReviewValidationFailure(t *testing.T) {
	orchestrator := NewReviewOrchestrator(nil, nil, nil, nil, nil)
	// Mutually exclusive flags: interactive + fix
	opts := ReviewOptions{
		Interactive: true,
		Fix:         true,
	}

	_, exitCode, err := orchestrator.ExecuteReview(context.Background(), opts)
	if err == nil {
		t.Errorf("expected error for interactive + fix")
	}
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}
}
