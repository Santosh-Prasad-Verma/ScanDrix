// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package formatters

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/cli/types"
)

func TestHTMLFormatter(t *testing.T) {
	formatter := NewHTMLFormatter("dark")

	res := &types.ReviewResult{
		ReviewID:      "rev-html-test",
		Status:        "failed",
		Summary:       "1 security defect identified.",
		FilesAnalyzed: 4,
		Duration:      350 * time.Millisecond,
		Stats: types.ReviewStats{
			CriticalCount: 1,
			FixableCount:  1,
			TotalIssues:   1,
		},
		Issues: []types.ReviewIssue{
			{
				ID:             "iss-1",
				File:           "auth/jwt.go",
				Line:           42,
				Severity:       types.SeverityCritical,
				Category:       "security_vulnerability",
				Message:        "Insecure JWT algorithm accepted",
				Recommendation: "Reject algorithm none immediately.",
				Suggestion:     "Enforce HMAC verification.",
				Fixable:        true,
				Fix: &types.CodeFix{
					NewCode: "+ if token.Header[\"alg\"] == \"none\" { return nil, err }",
				},
			},
		},
	}

	var buf bytes.Buffer
	if err := formatter.Format(&buf, res); err != nil {
		t.Fatalf("unexpected HTML formatting error: %v", err)
	}

	htmlStr := buf.String()
	if !strings.Contains(htmlStr, "<!DOCTYPE html>") {
		t.Errorf("expected DOCTYPE in HTML output")
	}
	if !strings.Contains(htmlStr, "ScanDrix Code Review Report") {
		t.Errorf("expected ScanDrix title in HTML output")
	}
	if !strings.Contains(htmlStr, "Insecure JWT algorithm accepted") {
		t.Errorf("expected issue message in HTML output")
	}
	if !strings.Contains(htmlStr, "auth/jwt.go:42") {
		t.Errorf("expected file and line in HTML output")
	}
}

func TestGitLabCIFormatter(t *testing.T) {
	formatter := NewGitLabCIFormatter()

	res := &types.ReviewResult{
		Issues: []types.ReviewIssue{
			{
				ID:       "iss-gl",
				File:     "database/query.go",
				Line:     18,
				EndLine:  20,
				Severity: types.SeverityCritical,
				Category: "security_vulnerability",
				RuleID:   "no-sql-injection",
				Message:  "Potential SQL injection via concatenated query parameter",
			},
		},
	}

	var buf bytes.Buffer
	if err := formatter.Format(&buf, res); err != nil {
		t.Fatalf("unexpected GitLab CI formatting error: %v", err)
	}

	var parsed []GitLabCodeQualityIssue
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("failed decoding GitLab CI JSON: %v", err)
	}

	if len(parsed) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(parsed))
	}

	iss := parsed[0]
	if iss.Severity != "blocker" {
		t.Errorf("expected 'blocker' for critical severity, got %s", iss.Severity)
	}
	if iss.Location.Path != "database/query.go" {
		t.Errorf("expected path database/query.go, got %s", iss.Location.Path)
	}
	if iss.Location.Lines.Begin != 18 || iss.Location.Lines.End != 20 {
		t.Errorf("expected lines 18-20, got begin=%d end=%d", iss.Location.Lines.Begin, iss.Location.Lines.End)
	}
}
