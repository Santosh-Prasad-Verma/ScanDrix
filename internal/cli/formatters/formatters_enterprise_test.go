// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package formatters

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/cli/types"
)

func sampleReviewResult() *types.ReviewResult {
	return &types.ReviewResult{
		Summary: "Found 2 issues",
		Issues: []types.ReviewIssue{
			{
				ID:             "ISSUE-1",
				Severity:       types.SeverityCritical,
				Category:       "security",
				File:           "auth/jwt.go",
				Line:           15,
				EndLine:        18,
				Message:        "Hardcoded JWT Secret",
				Recommendation: "Read secret from environment",
				Suggestion:     "os.Getenv(\"JWT_SECRET\")",
				Fixable:        true,
			},
			{
				ID:             "ISSUE-2",
				Severity:       types.SeverityWarning,
				Category:       "performance",
				File:           "db/query.go",
				Line:           45,
				EndLine:        50,
				Message:        "Unindexed SQL query",
				Recommendation: "Add index on user_id",
				Fixable:        false,
			},
		},
	}
}

func TestFormatJUnitXML(t *testing.T) {
	var buf bytes.Buffer
	err := FormatJUnitXML(&buf, sampleReviewResult())
	if err != nil {
		t.Fatalf("FormatJUnitXML failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "<testsuites") || !strings.Contains(output, "Hardcoded JWT Secret") {
		t.Fatalf("unexpected JUnit XML output: %s", output)
	}

	var suites JUnitTestSuites
	if err := xml.Unmarshal(buf.Bytes(), &suites); err != nil {
		t.Fatalf("failed parsing generated JUnit XML: %v", err)
	}

	if suites.Tests != 2 || suites.Errors != 1 || suites.Failures != 1 {
		t.Errorf("unexpected counts: tests=%d, errors=%d, failures=%d", suites.Tests, suites.Errors, suites.Failures)
	}
}

func TestFormatSonarQube(t *testing.T) {
	var buf bytes.Buffer
	err := FormatSonarQube(&buf, sampleReviewResult())
	if err != nil {
		t.Fatalf("FormatSonarQube failed: %v", err)
	}

	var report SonarQubeReport
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatalf("failed parsing SonarQube JSON: %v", err)
	}

	if len(report.Issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(report.Issues))
	}

	if report.Issues[0].Severity != "BLOCKER" || report.Issues[0].Type != "VULNERABILITY" {
		t.Errorf("unexpected Sonar issue mapping: %+v", report.Issues[0])
	}
}

func TestFormatCSV(t *testing.T) {
	var buf bytes.Buffer
	err := FormatCSV(&buf, sampleReviewResult())
	if err != nil {
		t.Fatalf("FormatCSV failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 { // header + 2 issues
		t.Fatalf("expected 3 lines in CSV, got %d", len(lines))
	}

	if !strings.Contains(lines[0], "ID,Severity,Category") {
		t.Errorf("unexpected CSV header: %s", lines[0])
	}
	if !strings.Contains(lines[1], "ISSUE-1,critical,security") {
		t.Errorf("unexpected CSV record: %s", lines[1])
	}
}
