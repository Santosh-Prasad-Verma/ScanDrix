// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package ci

import (
	"os"
	"strconv"
	"strings"
)

// CIProvider identifies the detected continuous integration environment.
type CIProvider string

const (
	ProviderNone               CIProvider = "none"
	ProviderGitHubActions      CIProvider = "github_actions"
	ProviderGitLabCI           CIProvider = "gitlab_ci"
	ProviderBitbucketPipelines CIProvider = "bitbucket_pipelines"
	ProviderAzureDevOps        CIProvider = "azure_devops"
	ProviderGenericCI          CIProvider = "generic_ci"
)

// CIContext encapsulates metadata extracted from CI environment variables.
type CIContext struct {
	Provider    CIProvider `json:"provider"`
	IsCI        bool       `json:"is_ci"`
	RepoSlug    string     `json:"repo_slug"`
	CommitSHA   string     `json:"commit_sha"`
	Branch      string     `json:"branch"`
	IsPR        bool       `json:"is_pr"`
	PRNumber    int        `json:"pr_number,omitempty"`
	Actor       string     `json:"actor,omitempty"`
	JobID       string     `json:"job_id,omitempty"`
	StepSummary string     `json:"step_summary_file,omitempty"`
}

// DetectCI inspects the runtime environment and returns the active CI context.
func DetectCI() *CIContext {
	ctx := &CIContext{
		Provider: ProviderNone,
		IsCI:     false,
	}

	// 1. GitHub Actions
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		ctx.Provider = ProviderGitHubActions
		ctx.IsCI = true
		ctx.RepoSlug = os.Getenv("GITHUB_REPOSITORY")
		ctx.CommitSHA = os.Getenv("GITHUB_SHA")
		ctx.Branch = os.Getenv("GITHUB_REF_NAME")
		ctx.Actor = os.Getenv("GITHUB_ACTOR")
		ctx.JobID = os.Getenv("GITHUB_RUN_ID")
		ctx.StepSummary = os.Getenv("GITHUB_STEP_SUMMARY")

		ref := os.Getenv("GITHUB_REF")
		if strings.HasPrefix(ref, "refs/pull/") {
			ctx.IsPR = true
			parts := strings.Split(ref, "/")
			if len(parts) >= 3 {
				if num, err := strconv.Atoi(parts[2]); err == nil {
					ctx.PRNumber = num
				}
			}
		}
		return ctx
	}

	// 2. GitLab CI
	if os.Getenv("GITLAB_CI") == "true" {
		ctx.Provider = ProviderGitLabCI
		ctx.IsCI = true
		ctx.RepoSlug = os.Getenv("CI_PROJECT_PATH")
		ctx.CommitSHA = os.Getenv("CI_COMMIT_SHA")
		ctx.Branch = os.Getenv("CI_COMMIT_REF_NAME")
		ctx.Actor = os.Getenv("GITLAB_USER_LOGIN")
		ctx.JobID = os.Getenv("CI_JOB_ID")

		if mrIID := os.Getenv("CI_MERGE_REQUEST_IID"); mrIID != "" {
			ctx.IsPR = true
			if num, err := strconv.Atoi(mrIID); err == nil {
				ctx.PRNumber = num
			}
		}
		return ctx
	}

	// 3. Bitbucket Pipelines
	if os.Getenv("BITBUCKET_BUILD_NUMBER") != "" {
		ctx.Provider = ProviderBitbucketPipelines
		ctx.IsCI = true
		ctx.RepoSlug = os.Getenv("BITBUCKET_REPO_FULL_NAME")
		ctx.CommitSHA = os.Getenv("BITBUCKET_COMMIT")
		ctx.Branch = os.Getenv("BITBUCKET_BRANCH")
		ctx.JobID = os.Getenv("BITBUCKET_BUILD_NUMBER")

		if prID := os.Getenv("BITBUCKET_PR_ID"); prID != "" {
			ctx.IsPR = true
			if num, err := strconv.Atoi(prID); err == nil {
				ctx.PRNumber = num
			}
		}
		return ctx
	}

	// 4. Azure DevOps
	if strings.EqualFold(os.Getenv("TF_BUILD"), "true") {
		ctx.Provider = ProviderAzureDevOps
		ctx.IsCI = true
		ctx.RepoSlug = os.Getenv("BUILD_REPOSITORY_NAME")
		ctx.CommitSHA = os.Getenv("BUILD_SOURCEVERSION")
		ctx.Branch = os.Getenv("BUILD_SOURCEBRANCHNAME")
		ctx.JobID = os.Getenv("BUILD_BUILDID")

		if prID := os.Getenv("SYSTEM_PULLREQUEST_PULLREQUESTID"); prID != "" {
			ctx.IsPR = true
			if num, err := strconv.Atoi(prID); err == nil {
				ctx.PRNumber = num
			}
		}
		return ctx
	}

	// 5. Generic CI indicator
	if os.Getenv("CI") == "true" || os.Getenv("CONTINUOUS_INTEGRATION") == "true" {
		ctx.Provider = ProviderGenericCI
		ctx.IsCI = true
		return ctx
	}

	return ctx
}
