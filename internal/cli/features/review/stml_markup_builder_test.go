// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package review

import (
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/cli/types"
)

func TestEscapeSTML(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"<script>alert('xss')</script>", "&lt;script&gt;alert(&apos;xss&apos;)&lt;/script&gt;"},
		{`foo & "bar"`, "foo &amp; &quot;bar&quot;"},
	}

	for _, c := range cases {
		got := EscapeSTML(c.input)
		if got != c.expected {
			t.Errorf("EscapeSTML(%q) = %q, expected %q", c.input, got, c.expected)
		}
	}
}

func TestExtractMarkdownLinks(t *testing.T) {
	text := "See [docs](https://scandrix.dev/docs) and [OWASP](https://owasp.org/top10) for details."
	prose, links := ExtractMarkdownLinks(text)

	if !strings.Contains(prose, "See docs and OWASP for details.") {
		t.Errorf("unexpected prose: %s", prose)
	}
	if len(links) != 2 {
		t.Fatalf("expected 2 links, got %d", len(links))
	}
	if links[0].Label != "docs" || links[0].URL != "https://scandrix.dev/docs" {
		t.Errorf("unexpected link 0: %+v", links[0])
	}
	if links[1].Label != "OWASP" || links[1].URL != "https://owasp.org/top10" {
		t.Errorf("unexpected link 1: %+v", links[1])
	}
}

func TestSplitAdvice(t *testing.T) {
	// Case 1: Fenced code
	advice1 := "Use parameterized query:\n```go\ndb.QueryRow(\"SELECT ...\", id)\n```"
	lead1, code1 := SplitAdvice(advice1)
	if lead1 != "Use parameterized query:" || !strings.Contains(code1, "db.QueryRow") {
		t.Errorf("unexpected split for fenced: lead=%q, code=%q", lead1, code1)
	}

	// Case 2: Backticks
	advice2 := "Replace with `sync.Mutex`"
	lead2, code2 := SplitAdvice(advice2)
	if lead2 != "Replace with" || code2 != "sync.Mutex" {
		t.Errorf("unexpected split for backticks: lead=%q, code=%q", lead2, code2)
	}

	// Case 3: Plain text
	advice3 := "Sanitize all user inputs"
	lead3, code3 := SplitAdvice(advice3)
	if lead3 != "Sanitize all user inputs" || code3 != "" {
		t.Errorf("unexpected split for plain: lead=%q, code=%q", lead3, code3)
	}
}

func TestBuildSTMLMarkup(t *testing.T) {
	issue := types.ReviewIssue{
		File:     "pkg/auth/token.go",
		Line:     20,
		Severity: "critical",
		Category: "Security",
		Message:  "Hardcoded secret detected. Refer to [Policy](https://scandrix.dev/policy).",
	}

	markup := BuildSTMLMarkup(issue, "critical", issue.Message, "Use `os.Getenv`", "severity critical · Security")

	if !strings.Contains(markup, `<badge color="danger">CRITICAL</badge>`) {
		t.Errorf("expected critical danger badge in markup: %s", markup)
	}
	if !strings.Contains(markup, "— Drixy ·") {
		t.Errorf("expected Drixy attribution in markup: %s", markup)
	}
	if !strings.Contains(markup, "https://scandrix.dev/policy") {
		t.Errorf("expected policy URL in markup: %s", markup)
	}
}

func TestConvertReviewToHunkFindings(t *testing.T) {
	result := &types.ReviewResult{
		Summary: "Found 1 critical issue",
		Issues: []types.ReviewIssue{
			{
				File:     "main.go",
				Line:     15,
				EndLine:  18,
				Severity: "high",
				Message:  "SQL Injection vulnerability",
				Category: "Security",
				RuleID:   "SEC-SQL-01",
			},
		},
	}

	findings := ConvertReviewToHunkFindings(result)
	if findings.Version != 1 || len(findings.Findings) != 1 {
		t.Fatalf("unexpected findings: %+v", findings)
	}

	f := findings.Findings[0]
	if f.ID != "scandrix-0" || f.File != "main.go" || f.Line != 15 || f.EndLine != 18 {
		t.Fatalf("unexpected finding: %+v", f)
	}
	if f.Title != "SQL Injection vulnerability" || f.RuleID != "SEC-SQL-01" {
		t.Fatalf("unexpected finding title/rule: %+v", f)
	}
}
