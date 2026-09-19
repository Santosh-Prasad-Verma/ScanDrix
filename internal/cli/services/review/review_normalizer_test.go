// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package review

import (
	"testing"
	"time"

	"github.com/scandrix/backend/internal/cli/types"
	"github.com/scandrix/backend/pkg/models"
)

func TestNormalizeSeverity(t *testing.T) {
	tests := []struct {
		input string
		want  types.Severity
	}{
		{"critical", types.SeverityCritical},
		{"CRITICAL", types.SeverityCritical},
		{"high", types.SeverityError},
		{"error", types.SeverityError},
		{"medium", types.SeverityWarning},
		{"warning", types.SeverityWarning},
		{"low", types.SeverityInfo},
		{"info", types.SeverityInfo},
		{"", types.SeverityInfo},
		{"unknown", types.SeverityInfo},
	}

	for _, tt := range tests {
		got := NormalizeSeverity(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeSeverity(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestToFindingSeverity(t *testing.T) {
	if ToFindingSeverity(types.SeverityCritical) != models.SeverityCritical {
		t.Errorf("unexpected finding severity for critical")
	}
	if ToFindingSeverity(types.SeverityError) != models.SeverityHigh {
		t.Errorf("unexpected finding severity for error")
	}
	if ToFindingSeverity(types.SeverityWarning) != models.SeverityMedium {
		t.Errorf("unexpected finding severity for warning")
	}
	if ToFindingSeverity(types.SeverityInfo) != models.SeverityLow {
		t.Errorf("unexpected finding severity for info")
	}
}

func TestNormalizeRawSuggestions(t *testing.T) {
	rawFiles := []RawAPISuggestion{
		{
			FilePath:           "internal/auth/jwt.go",
			RelevantLinesStart: 42,
			RelevantLinesEnd:   45,
			Severity:           "high",
			SuggestionContent:  "JWT signature algorithm none accepted",
			OneSentenceSummary: "Enforce HS256/RS256 verification",
			Label:              "sec-jwt-001",
			SuggestedDiff:      "- alg: none\n+ alg: RS256",
		},
	}

	rawPR := []RawAPISuggestion{
		{
			Severity:           "medium",
			SuggestionContent:  "PR missing security review approval",
			OneSentenceSummary: "Require CISO signoff",
			Label:              "policy-pr-002",
		},
	}

	issues := NormalizeRawSuggestions(rawFiles, rawPR)
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(issues))
	}

	if issues[0].File != "internal/auth/jwt.go" || issues[0].Severity != types.SeverityError {
		t.Errorf("unexpected first issue: %+v", issues[0])
	}
	if !issues[0].Fixable || issues[0].Fix == nil {
		t.Errorf("expected first issue to be fixable")
	}

	if issues[1].File != "PR" || issues[1].Line != 0 || issues[1].Severity != types.SeverityWarning {
		t.Errorf("unexpected second issue: %+v", issues[1])
	}

	stats := CalculateReviewStats(issues)
	if stats.TotalIssues != 2 || stats.ErrorCount != 1 || stats.WarningCount != 1 || stats.FixableCount != 1 {
		t.Errorf("unexpected stats: %+v", stats)
	}

	res := BuildReviewResult("rev-123", "Completed with issues", issues, 0, 1500*time.Millisecond)
	if res.Status != "failed" || !res.IsBlocking || res.ExitCode != 1 {
		t.Errorf("expected failed status with exitCode 1, got: %+v", res)
	}
	if res.FilesAnalyzed != 1 {
		t.Errorf("expected 1 file analyzed, got %d", res.FilesAnalyzed)
	}
}
