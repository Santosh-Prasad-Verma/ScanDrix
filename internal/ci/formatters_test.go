// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev
// ═══════════════════════════════════════════════════════════════

package ci

import (
	"encoding/json"
	"encoding/xml"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getSampleFindings() []CIFinding {
	return []CIFinding{
		{
			ID:          "find_1",
			RuleID:      "SEC-SQL-001",
			RuleTitle:   "Potential SQL Injection via Raw Formatting",
			Category:    "SECURITY",
			Severity:    FailCritical,
			FilePath:    "internal/database/query.go",
			StartLine:   42,
			EndLine:     45,
			Message:     "Do not concatenate user input directly into SQL strings",
			Description: "Parameterized queries prevent injection attacks",
			Suggestion:  "- db.Query(fmt.Sprintf(...))\n+ db.Query(query, param)",
			CWETaxonomy: "CWE-89",
			Confidence:  0.95,
		},
		{
			ID:          "find_2",
			RuleID:      "PERF-ALLOC-002",
			RuleTitle:   "Heap Allocation in Hot Loop",
			Category:    "PERFORMANCE",
			Severity:    FailWarning,
			FilePath:    "internal/worker/pool.go",
			StartLine:   112,
			EndLine:     112,
			Message:     "Reallocate buffer outside the for-loop to reduce GC pause",
			Suggestion:  "buf := make([]byte, 1024)\nfor ...",
			Confidence:  0.88,
		},
	}
}

func TestFormatSARIF_Validation(t *testing.T) {
	findings := getSampleFindings()
	sarifBytes, err := FormatSARIF(findings)
	require.NoError(t, err)

	var report SARIFReport
	err = json.Unmarshal(sarifBytes, &report)
	require.NoError(t, err)

	assert.Equal(t, "2.1.0", report.Version)
	require.Len(t, report.Runs, 1)
	assert.Equal(t, "ScanDrix", report.Runs[0].Tool.Driver.Name)
	assert.Equal(t, "https://scandrix.dev", report.Runs[0].Tool.Driver.InformationURI)

	results := report.Runs[0].Results
	require.Len(t, results, 2)
	assert.Equal(t, "SEC-SQL-001", results[0].RuleID)
	assert.Equal(t, "error", results[0].Level)
	assert.Equal(t, "internal/database/query.go", results[0].Locations[0].PhysicalLocation.ArtifactLocation.URI)
	assert.Equal(t, 42, results[0].Locations[0].PhysicalLocation.Region.StartLine)

	assert.Equal(t, "PERF-ALLOC-002", results[1].RuleID)
	assert.Equal(t, "warning", results[1].Level)
}

func TestFormatJUnitXML_Validation(t *testing.T) {
	findings := getSampleFindings()
	xmlBytes, err := FormatJUnitXML(findings, false, 250*time.Millisecond)
	require.NoError(t, err)

	var suites JUnitTestSuites
	err = xml.Unmarshal(xmlBytes, &suites)
	require.NoError(t, err)

	assert.Equal(t, 2, suites.Tests)
	assert.Equal(t, 1, suites.Failures) // 1 critical is a failure, 1 warning is not
	require.Len(t, suites.TestSuites, 1)
	assert.Equal(t, "ScanDrix Automated Code Review", suites.TestSuites[0].Name)
}

func TestFormatGitLabCodeQuality_Validation(t *testing.T) {
	findings := getSampleFindings()
	glBytes, err := FormatGitLabCodeQuality(findings)
	require.NoError(t, err)

	var items []GitLabCodeQualityFinding
	err = json.Unmarshal(glBytes, &items)
	require.NoError(t, err)

	require.Len(t, items, 2)
	assert.Equal(t, "SEC-SQL-001", items[0].CheckName)
	assert.Equal(t, "critical", items[0].Severity)
	assert.NotEmpty(t, items[0].Fingerprint)
	assert.Equal(t, 42, items[0].Location.Lines.Begin)

	assert.Equal(t, "minor", items[1].Severity)
}

func TestFormatPRSummaryMarkdown_Validation(t *testing.T) {
	findings := getSampleFindings()
	res := &HeadlessReviewResult{
		GatePassed:    false,
		GateReason:    "Quality gate failed: 1 critical finding",
		TotalFindings: 2,
		CriticalCount: 1,
		WarningCount:  1,
		Findings:      findings,
		Duration:      150 * time.Millisecond,
		Environment: CIEnvironment{
			Platform: PlatformGitHubActions,
		},
	}

	md := FormatPRSummaryMarkdown(res)
	assert.Contains(t, md, "ScanDrix Automated Code Review — ❌ BLOCKED")
	assert.Contains(t, md, "🔴 **Critical** | 1")
	assert.Contains(t, md, "Potential SQL Injection")
	assert.Contains(t, md, "scandrix.dev")
}
