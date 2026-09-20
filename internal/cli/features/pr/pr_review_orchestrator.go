// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package pr

import (
	"context"
	"fmt"
	"strings"

	"github.com/scandrix/backend/internal/cli/pr"
	"github.com/scandrix/backend/internal/cli/services/api"
	"github.com/scandrix/backend/internal/cli/types"
	"github.com/scandrix/backend/internal/cli/utils"
)

// PRReviewOptions configures the review orchestration run.
type PRReviewOptions struct {
	MinSeverity string
	Categories  []string
	AutoApply   bool
	PostComment bool
	Token       string
}

// PRReviewSummary aggregates the review results for a pull request.
type PRReviewSummary struct {
	PRNumber      int                 `json:"prNumber"`
	TotalIssues   int                 `json:"totalIssues"`
	CriticalCount int                 `json:"criticalCount"`
	HighCount     int                 `json:"highCount"`
	MediumCount   int                 `json:"mediumCount"`
	LowCount      int                 `json:"lowCount"`
	Passed        bool                `json:"passed"`
	Issues        []types.ReviewIssue `json:"issues"`
}

// PRReviewOrchestrator manages the end-to-end pull request review workflow.
type PRReviewOrchestrator struct {
	apiClient      *api.Client
	commentHandler *PRCommentHandler
}

// NewPRReviewOrchestrator creates a new review orchestrator.
func NewPRReviewOrchestrator(apiClient *api.Client) *PRReviewOrchestrator {
	if apiClient == nil {
		apiClient = api.NewClient("", "", "")
	}
	return &PRReviewOrchestrator{
		apiClient:      apiClient,
		commentHandler: NewPRCommentHandler(apiClient),
	}
}

// FilterIssues filters issues according to minimum severity and category whitelist.
func FilterIssues(issues []types.ReviewIssue, minSeverity string, categories []string) []types.ReviewIssue {
	severityRanks := map[string]int{
		"critical": 4,
		"high":     3,
		"error":    3,
		"medium":   2,
		"warning":  2,
		"low":      1,
		"info":     0,
	}

	minRank := 0
	if minSeverity != "" {
		minRank = severityRanks[strings.ToLower(minSeverity)]
	}

	categoryMap := make(map[string]bool)
	for _, c := range categories {
		categoryMap[strings.ToLower(strings.TrimSpace(c))] = true
	}

	var filtered []types.ReviewIssue
	for _, iss := range issues {
		rank := severityRanks[strings.ToLower(string(iss.Severity))]
		if rank < minRank {
			continue
		}
		if len(categoryMap) > 0 {
			cat := strings.ToLower(iss.Category)
			if !categoryMap[cat] {
				continue
			}
		}
		filtered = append(filtered, iss)
	}
	return filtered
}

// ComputeSummary aggregates statistics and computes pass/fail status.
func ComputeSummary(prNumber int, issues []types.ReviewIssue) PRReviewSummary {
	summary := PRReviewSummary{
		PRNumber:    prNumber,
		TotalIssues: len(issues),
		Passed:      true,
		Issues:      issues,
	}

	for _, iss := range issues {
		switch strings.ToLower(string(iss.Severity)) {
		case "critical":
			summary.CriticalCount++
			summary.Passed = false
		case "high", "error":
			summary.HighCount++
			summary.Passed = false
		case "medium", "warning":
			summary.MediumCount++
		case "low":
			summary.LowCount++
		}
	}
	return summary
}

// ReviewPR runs the review orchestration workflow for a pull request.
func (o *PRReviewOrchestrator) ReviewPR(ctx context.Context, input string, opts PRReviewOptions) (*PRReviewSummary, error) {
	namespace, prNumber, err := pr.ParsePRInput(input)
	if err != nil {
		return nil, fmt.Errorf("failed parsing PR input: %w", err)
	}

	utils.Info("Fetching PR #%d from %s...", prNumber, namespace)
	diff, err := pr.FetchDiffFromGitHubAPI(ctx, namespace, prNumber, opts.Token)
	if err != nil {
		// Fallback to local git fetch
		var gitErr error
		diff, gitErr = pr.FetchDiffFromGit(ctx, prNumber, "main")
		if gitErr != nil {
			return nil, fmt.Errorf("unable to retrieve PR diff via API (%v) or Git: %w", err, gitErr)
		}
	}

	utils.Info("Analyzing PR diff (%d bytes)...", len(diff))

	// Submit review to API
	var reviewResp struct {
		ReviewID string              `json:"reviewId"`
		Issues   []types.ReviewIssue `json:"issues"`
	}

	err = o.apiClient.Do(ctx, "POST", "/v1/review/submit", map[string]any{
		"repo":     namespace,
		"prNumber": prNumber,
		"diff":     diff,
	}, &reviewResp)

	if err != nil {
		return nil, fmt.Errorf("review submission failed: %w", err)
	}

	filtered := FilterIssues(reviewResp.Issues, opts.MinSeverity, opts.Categories)
	summary := ComputeSummary(prNumber, filtered)

	if opts.PostComment && o.commentHandler != nil {
		for _, iss := range filtered {
			body := FormatSuggestionMarkdown(iss.RuleID, iss.Message, "", iss.Suggestion)
			_ = o.commentHandler.PostReviewComment(ctx, ReviewCommentPayload{
				PRNumber: prNumber,
				RepoID:   namespace,
				FilePath: iss.File,
				Line:     iss.Line,
				Body:     body,
			})
		}
	}

	return &summary, nil
}
