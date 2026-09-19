// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev
// ═══════════════════════════════════════════════════════════════

package ci

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectCIEnvironment(t *testing.T) {
	// Test GitHub Actions detection
	os.Setenv("GITHUB_ACTIONS", "true")
	os.Setenv("GITHUB_REPOSITORY", "scandrix/backend")
	os.Setenv("GITHUB_SHA", "abcdef1234567890")
	os.Setenv("GITHUB_REF", "refs/pull/42/merge")
	os.Setenv("GITHUB_EVENT_NAME", "pull_request")
	defer func() {
		os.Unsetenv("GITHUB_ACTIONS")
		os.Unsetenv("GITHUB_REPOSITORY")
		os.Unsetenv("GITHUB_SHA")
		os.Unsetenv("GITHUB_REF")
		os.Unsetenv("GITHUB_EVENT_NAME")
	}()

	env := DetectCIEnvironment()
	assert.Equal(t, PlatformGitHubActions, env.Platform)
	assert.Equal(t, "scandrix", env.RepoOwner)
	assert.Equal(t, "backend", env.RepoName)
	assert.Equal(t, "abcdef1234567890", env.CommitSHA)
	assert.True(t, env.IsPR)
	assert.Equal(t, 42, env.PullRequestID)
}

func TestDetectCIEnvironment_GitLab(t *testing.T) {
	os.Setenv("GITLAB_CI", "true")
	os.Setenv("CI_PROJECT_NAMESPACE", "scandrix")
	os.Setenv("CI_PROJECT_NAME", "backend")
	os.Setenv("CI_COMMIT_SHA", "glsha123")
	os.Setenv("CI_MERGE_REQUEST_IID", "99")
	defer func() {
		os.Unsetenv("GITLAB_CI")
		os.Unsetenv("CI_PROJECT_NAMESPACE")
		os.Unsetenv("CI_PROJECT_NAME")
		os.Unsetenv("CI_COMMIT_SHA")
		os.Unsetenv("CI_MERGE_REQUEST_IID")
	}()

	env := DetectCIEnvironment()
	assert.Equal(t, PlatformGitLabCI, env.Platform)
	assert.Equal(t, "scandrix", env.RepoOwner)
	assert.Equal(t, "backend", env.RepoName)
	assert.True(t, env.IsPR)
	assert.Equal(t, 99, env.PullRequestID)
}

func TestHeadlessRunner_QualityGateEvaluation(t *testing.T) {
	runner := NewHeadlessRunner()
	ctx := context.Background()

	findings := []CIFinding{
		{
			RuleID:    "SEC-01",
			RuleTitle: "SQL Injection",
			Severity:  FailCritical,
			FilePath:  "internal/db.go",
			StartLine: 10,
			Message:   "Vulnerability found",
		},
		{
			RuleID:    "PERF-02",
			RuleTitle: "Loop Allocation",
			Severity:  FailWarning,
			FilePath:  "internal/alloc.go",
			StartLine: 20,
			Message:   "Heap alloc warning",
		},
	}

	// 1. FailCritical gate -> MUST fail because 1 critical exists
	cfgCritical := HeadlessReviewConfig{
		FailOnSeverity: FailCritical,
	}
	res, err := runner.RunHeadlessReview(ctx, cfgCritical, findings)
	require.NoError(t, err)
	assert.False(t, res.GatePassed)
	assert.Equal(t, ExitQualityGateFailed, res.ExitCode)
	assert.Equal(t, 1, res.CriticalCount)
	assert.Equal(t, 1, res.WarningCount)

	// 2. FailNever gate -> MUST pass
	cfgNever := HeadlessReviewConfig{
		FailOnSeverity: FailNever,
	}
	res2, err := runner.RunHeadlessReview(ctx, cfgNever, findings)
	require.NoError(t, err)
	assert.True(t, res2.GatePassed)
	assert.Equal(t, ExitSuccess, res2.ExitCode)

	// 3. Test report file generation
	tmpDir := t.TempDir()
	sarifPath := filepath.Join(tmpDir, "report.sarif")
	junitPath := filepath.Join(tmpDir, "report.junit.xml")
	glPath := filepath.Join(tmpDir, "codequality.json")
	summaryPath := filepath.Join(tmpDir, "summary.md")

	cfgReports := HeadlessReviewConfig{
		FailOnSeverity: FailNever,
		OutputSARIF:    sarifPath,
		OutputJUnit:    junitPath,
		OutputGitLab:   glPath,
		OutputMarkdown: summaryPath,
	}
	_, err = runner.RunHeadlessReview(ctx, cfgReports, findings)
	require.NoError(t, err)

	assert.FileExists(t, sarifPath)
	assert.FileExists(t, junitPath)
	assert.FileExists(t, glPath)
	assert.FileExists(t, summaryPath)
}

func TestHeadlessRunner_PathFiltering(t *testing.T) {
	runner := NewHeadlessRunner()
	ctx := context.Background()

	findings := []CIFinding{
		{RuleID: "R1", FilePath: "internal/service.go", Severity: FailWarning},
		{RuleID: "R2", FilePath: "docs/architecture.md", Severity: FailWarning},
		{RuleID: "R3", FilePath: "tests/unit_test.go", Severity: FailWarning},
	}

	cfg := HeadlessReviewConfig{
		FailOnSeverity: FailNever,
		ExcludePaths:   []string{"docs/", "tests/"},
	}

	res, err := runner.RunHeadlessReview(ctx, cfg, findings)
	require.NoError(t, err)
	assert.Equal(t, 1, res.TotalFindings)
	assert.Equal(t, "internal/service.go", res.Findings[0].FilePath)
}
