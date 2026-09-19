// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev
// ═══════════════════════════════════════════════════════════════

package ci

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// HeadlessRunner coordinates headless review execution, CI environment inspection, and gate evaluation.
type HeadlessRunner struct {
	env CIEnvironment
}

// NewHeadlessRunner creates a runner pre-loaded with detected CI environment properties.
func NewHeadlessRunner() *HeadlessRunner {
	return &HeadlessRunner{
		env: DetectCIEnvironment(),
	}
}

// Environment returns the detected CI runner metadata.
func (r *HeadlessRunner) Environment() CIEnvironment {
	return r.env
}

// DetectCIEnvironment auto-detects runner vendor and context from environment variables.
func DetectCIEnvironment() CIEnvironment {
	env := CIEnvironment{
		Platform:      PlatformGeneric,
		WorkspacePath: ".",
	}

	// 1. GitHub Actions
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		env.Platform = PlatformGitHubActions
		repo := os.Getenv("GITHUB_REPOSITORY")
		if parts := strings.Split(repo, "/"); len(parts) == 2 {
			env.RepoOwner = parts[0]
			env.RepoName = parts[1]
		}
		env.CommitSHA = os.Getenv("GITHUB_SHA")
		env.BaseRef = os.Getenv("GITHUB_BASE_REF")
		env.HeadRef = os.Getenv("GITHUB_HEAD_REF")
		env.RunID = os.Getenv("GITHUB_RUN_ID")
		env.JobID = os.Getenv("GITHUB_JOB")
		env.WorkspacePath = os.Getenv("GITHUB_WORKSPACE")
		env.EventName = os.Getenv("GITHUB_EVENT_NAME")
		env.Actor = os.Getenv("GITHUB_ACTOR")
		env.ServerURL = os.Getenv("GITHUB_SERVER_URL")
		if env.ServerURL == "" {
			env.ServerURL = "https://github.com"
		}
		if os.Getenv("GITHUB_EVENT_NAME") == "pull_request" {
			env.IsPR = true
			ref := os.Getenv("GITHUB_REF")
			if strings.HasPrefix(ref, "refs/pull/") {
				parts := strings.Split(ref, "/")
				if len(parts) >= 3 {
					env.PullRequestID, _ = strconv.Atoi(parts[2])
				}
			}
		}
		env.APIToken = os.Getenv("GITHUB_TOKEN")
		return env
	}

	// 2. GitLab CI
	if os.Getenv("GITLAB_CI") == "true" {
		env.Platform = PlatformGitLabCI
		env.RepoOwner = os.Getenv("CI_PROJECT_NAMESPACE")
		env.RepoName = os.Getenv("CI_PROJECT_NAME")
		env.CommitSHA = os.Getenv("CI_COMMIT_SHA")
		env.BaseRef = os.Getenv("CI_MERGE_REQUEST_TARGET_BRANCH_NAME")
		env.HeadRef = os.Getenv("CI_MERGE_REQUEST_SOURCE_BRANCH_NAME")
		env.JobID = os.Getenv("CI_JOB_ID")
		env.RunID = os.Getenv("CI_PIPELINE_ID")
		env.WorkspacePath = os.Getenv("CI_PROJECT_DIR")
		env.Actor = os.Getenv("GITLAB_USER_LOGIN")
		env.ServerURL = os.Getenv("CI_SERVER_URL")
		if iid := os.Getenv("CI_MERGE_REQUEST_IID"); iid != "" {
			env.IsPR = true
			env.PullRequestID, _ = strconv.Atoi(iid)
		}
		env.APIToken = os.Getenv("CI_JOB_TOKEN")
		return env
	}

	// 3. Azure Pipelines
	if strings.EqualFold(os.Getenv("TF_BUILD"), "true") {
		env.Platform = PlatformAzurePipelines
		repo := os.Getenv("BUILD_REPOSITORY_NAME")
		if parts := strings.Split(repo, "/"); len(parts) == 2 {
			env.RepoOwner = parts[0]
			env.RepoName = parts[1]
		}
		env.CommitSHA = os.Getenv("BUILD_SOURCEVERSION")
		env.BaseRef = os.Getenv("SYSTEM_PULLREQUEST_TARGETBRANCH")
		env.HeadRef = os.Getenv("SYSTEM_PULLREQUEST_SOURCEBRANCH")
		env.JobID = os.Getenv("SYSTEM_JOBID")
		env.RunID = os.Getenv("BUILD_BUILDID")
		env.WorkspacePath = os.Getenv("BUILD_SOURCESDIRECTORY")
		if prID := os.Getenv("SYSTEM_PULLREQUEST_PULLREQUESTNUMBER"); prID != "" {
			env.IsPR = true
			env.PullRequestID, _ = strconv.Atoi(prID)
		}
		env.APIToken = os.Getenv("SYSTEM_ACCESSTOKEN")
		return env
	}

	// 4. Bitbucket Pipelines
	if os.Getenv("BITBUCKET_BUILD_NUMBER") != "" {
		env.Platform = PlatformBitbucketPipelines
		repo := os.Getenv("BITBUCKET_REPO_FULL_NAME")
		if parts := strings.Split(repo, "/"); len(parts) == 2 {
			env.RepoOwner = parts[0]
			env.RepoName = parts[1]
		}
		env.CommitSHA = os.Getenv("BITBUCKET_COMMIT")
		env.BaseRef = os.Getenv("BITBUCKET_PR_DESTINATION_BRANCH")
		env.HeadRef = os.Getenv("BITBUCKET_BRANCH")
		env.RunID = os.Getenv("BITBUCKET_BUILD_NUMBER")
		env.WorkspacePath = os.Getenv("BITBUCKET_CLONE_DIR")
		if prID := os.Getenv("BITBUCKET_PR_ID"); prID != "" {
			env.IsPR = true
			env.PullRequestID, _ = strconv.Atoi(prID)
		}
		return env
	}

	// 5. Forgejo / Gitea Actions
	if os.Getenv("FORGEJO") == "true" || os.Getenv("GITEA_ACTIONS") == "true" {
		env.Platform = PlatformForgejoCI
		repo := os.Getenv("GITHUB_REPOSITORY")
		if parts := strings.Split(repo, "/"); len(parts) == 2 {
			env.RepoOwner = parts[0]
			env.RepoName = parts[1]
		}
		env.CommitSHA = os.Getenv("GITHUB_SHA")
		env.WorkspacePath = os.Getenv("GITHUB_WORKSPACE")
		env.APIToken = os.Getenv("GITHUB_TOKEN")
		return env
	}

	return env
}

// RunHeadlessReview performs an end-to-end automated review and evaluates the quality gate.
func (r *HeadlessRunner) RunHeadlessReview(
	ctx context.Context,
	cfg HeadlessReviewConfig,
	sampleFindings []CIFinding, // allow injection or mock for deterministic testing
) (*HeadlessReviewResult, error) {
	start := time.Now()

	workspace := cfg.WorkspaceDir
	if workspace == "" {
		workspace = r.env.WorkspacePath
	}
	if workspace == "" {
		workspace = "."
	}

	// Check if commit message requests skip
	if cfg.SkipCommitFlag {
		if r.shouldSkipCommit(workspace) {
			slog.Info("Review skipped due to commit message directive [skip scandrix]")
			return &HeadlessReviewResult{
				GatePassed:  true,
				ExitCode:    ExitSuccess,
				GateReason:  "Review skipped via commit message flag",
				GeneratedAt: time.Now().UTC(),
				Duration:    time.Since(start),
				Environment: r.env,
			}, nil
		}
	}

	findings := sampleFindings
	if findings == nil {
		findings = []CIFinding{}
	}

	// Filter findings by include/exclude path rules if specified
	if len(cfg.IncludePaths) > 0 || len(cfg.ExcludePaths) > 0 {
		findings = r.filterFindingsByPaths(findings, cfg.IncludePaths, cfg.ExcludePaths)
	}

	// Cap max findings if configured
	if cfg.MaxFindings > 0 && len(findings) > cfg.MaxFindings {
		findings = findings[:cfg.MaxFindings]
	}

	// Aggregate metrics
	result := &HeadlessReviewResult{
		TotalFindings: len(findings),
		Findings:      findings,
		GeneratedAt:   time.Now().UTC(),
		Duration:      time.Since(start),
		Environment:   r.env,
	}

	for _, f := range findings {
		switch f.Severity {
		case FailCritical:
			result.CriticalCount++
		case FailError:
			result.ErrorCount++
		case FailWarning:
			result.WarningCount++
		case FailInfo:
			result.InfoCount++
		}
	}

	// Evaluate Quality Gate
	r.evaluateQualityGate(result, cfg.FailOnSeverity)

	// Generate configured report files
	if err := r.generateReportArtifacts(result, cfg); err != nil {
		slog.Error("Failed to write report artifacts", "error", err)
	}

	return result, nil
}

func (r *HeadlessRunner) evaluateQualityGate(res *HeadlessReviewResult, threshold SeverityThreshold) {
	if threshold == "" {
		threshold = FailError // default enterprise gate
	}

	switch threshold {
	case FailNever:
		res.GatePassed = true
		res.ExitCode = ExitSuccess
		res.GateReason = "Quality gate set to 'never' fail"

	case FailCritical:
		if res.CriticalCount > 0 {
			res.GatePassed = false
			res.ExitCode = ExitQualityGateFailed
			res.GateReason = fmt.Sprintf("Quality gate failed: %d critical findings detected", res.CriticalCount)
		} else {
			res.GatePassed = true
			res.ExitCode = ExitSuccess
			res.GateReason = "Quality gate passed (0 critical findings)"
		}

	case FailError:
		if res.CriticalCount > 0 || res.ErrorCount > 0 {
			res.GatePassed = false
			res.ExitCode = ExitQualityGateFailed
			res.GateReason = fmt.Sprintf("Quality gate failed: %d critical and %d error findings detected", res.CriticalCount, res.ErrorCount)
		} else {
			res.GatePassed = true
			res.ExitCode = ExitSuccess
			res.GateReason = "Quality gate passed (0 critical or error findings)"
		}

	case FailWarning:
		if res.CriticalCount > 0 || res.ErrorCount > 0 || res.WarningCount > 0 {
			res.GatePassed = false
			res.ExitCode = ExitQualityGateFailed
			res.GateReason = fmt.Sprintf("Quality gate failed: %d findings detected at or above warning level", res.CriticalCount+res.ErrorCount+res.WarningCount)
		} else {
			res.GatePassed = true
			res.ExitCode = ExitSuccess
			res.GateReason = "Quality gate passed (0 warnings or errors)"
		}

	case FailInfo:
		if res.TotalFindings > 0 {
			res.GatePassed = false
			res.ExitCode = ExitQualityGateFailed
			res.GateReason = fmt.Sprintf("Quality gate failed: %d findings detected", res.TotalFindings)
		} else {
			res.GatePassed = true
			res.ExitCode = ExitSuccess
			res.GateReason = "Quality gate passed (0 findings)"
		}
	}
}

func (r *HeadlessRunner) generateReportArtifacts(res *HeadlessReviewResult, cfg HeadlessReviewConfig) error {
	if cfg.OutputSARIF != "" {
		sarifData, err := FormatSARIF(res.Findings)
		if err == nil {
			_ = os.WriteFile(cfg.OutputSARIF, sarifData, 0644)
		}
	}

	if cfg.OutputJUnit != "" {
		junitData, err := FormatJUnitXML(res.Findings, res.GatePassed, res.Duration)
		if err == nil {
			_ = os.WriteFile(cfg.OutputJUnit, junitData, 0644)
		}
	}

	if cfg.OutputGitLab != "" {
		gitlabData, err := FormatGitLabCodeQuality(res.Findings)
		if err == nil {
			_ = os.WriteFile(cfg.OutputGitLab, gitlabData, 0644)
		}
	}

	if cfg.OutputMarkdown != "" {
		summaryMD := FormatPRSummaryMarkdown(res)
		_ = os.WriteFile(cfg.OutputMarkdown, []byte(summaryMD), 0644)
	}

	return nil
}

func (r *HeadlessRunner) shouldSkipCommit(workspaceDir string) bool {
	cmd := exec.Command("git", "log", "-1", "--pretty=%B")
	cmd.Dir = workspaceDir
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	msg := strings.ToLower(string(out))
	return strings.Contains(msg, "[skip scandrix]") ||
		strings.Contains(msg, "[scandrix skip]") ||
		strings.Contains(msg, "[skip ci]")
}

func (r *HeadlessRunner) filterFindingsByPaths(findings []CIFinding, includes, excludes []string) []CIFinding {
	var filtered []CIFinding
	for _, f := range findings {
		if len(excludes) > 0 {
			excluded := false
			for _, pat := range excludes {
				if strings.Contains(f.FilePath, pat) {
					excluded = true
					break
				}
			}
			if excluded {
				continue
			}
		}

		if len(includes) > 0 {
			included := false
			for _, pat := range includes {
				if strings.Contains(f.FilePath, pat) {
					included = true
					break
				}
			}
			if !included {
				continue
			}
		}

		filtered = append(filtered, f)
	}
	return filtered
}
