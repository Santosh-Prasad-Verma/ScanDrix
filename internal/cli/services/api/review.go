// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/scandrix/backend/internal/cli/services/git"
	"github.com/scandrix/backend/internal/cli/utils"
	"github.com/scandrix/backend/pkg/models"
)

// REVIEW DATA CONTRACTS & DTOs

const (
	MaxDiffChars           = 20_000_000       // 20M characters diff limit
	MaxSerializedBodyBytes = 24 * 1024 * 1024 // 24MB payload guard
	PollMinDelay           = 1 * time.Second
	PollMaxDelay           = 5 * time.Second
	PollMaxWait            = 30 * time.Minute
)

// ReviewRequest encapsulates options, diff, metrics, and inlined files sent to the review engine.
type ReviewRequest struct {
	Diff         string            `json:"diff"`
	RulesOnly    bool              `json:"rules_only"`
	Fast         bool              `json:"fast"`
	Heavy        bool              `json:"heavy"`
	Focus        string            `json:"focus,omitempty"`
	Context      string            `json:"context,omitempty"`
	Files        []string          `json:"files,omitempty"`
	InlinedFiles []git.FileContent `json:"inlined_files,omitempty"`
	Staged       bool              `json:"staged,omitempty"`
	Commit       string            `json:"commit,omitempty"`
	Branch       string            `json:"branch,omitempty"`
	Repository   string            `json:"repository,omitempty"`
	Metrics      *git.GitInfo      `json:"metrics,omitempty"`
	Fingerprint  string            `json:"fingerprint,omitempty"`
	GithubPAT    string            `json:"github_pat,omitempty"`
}

// ReviewResponse models the response returned from the review API.
type ReviewResponse struct {
	ReviewID      string               `json:"review_id"`
	Status        string               `json:"status"`
	Summary       string               `json:"summary"`
	FilesAnalyzed int                  `json:"files_analyzed"`
	DurationMs    int64                `json:"duration_ms"`
	Findings      []models.CodeFinding `json:"findings"`
	RemainingUses int                  `json:"remaining_uses,omitempty"`
}

// EnqueueJobResponse models an accepted async review job.
type EnqueueJobResponse struct {
	JobID     string `json:"job_id"`
	JobIDAlt  string `json:"jobId"`
	Status    string `json:"status"`
	StatusURL string `json:"status_url"`
}

// JobStatusResponse models the polling response for an async review job.
type JobStatusResponse struct {
	JobID    string          `json:"job_id"`
	Status   string          `json:"status"` // PENDING, PROCESSING, COMPLETED, FAILED
	Result   *ReviewResponse `json:"result,omitempty"`
	Error    string          `json:"error,omitempty"`
	Progress string          `json:"progress,omitempty"`
}

// REVIEW OPERATIONS (Submit, Async Polling, Trial, Suggestions, PR)

// validateReviewLimits ensures payload sizes are within safe limits.
func validateReviewLimits(req ReviewRequest) error {
	if len(req.Diff) > MaxDiffChars {
		return utils.NewCommandError(
			"REVIEW_TOO_LARGE",
			fmt.Sprintf("Diff is too large (%d characters, limit %d). Narrow the review scope.", len(req.Diff), MaxDiffChars),
			1,
			nil,
		)
	}
	bodyBytes, err := json.Marshal(req)
	if err == nil && len(bodyBytes) > MaxSerializedBodyBytes {
		return utils.NewCommandError(
			"REVIEW_TOO_LARGE",
			fmt.Sprintf("Review payload is too large (%.1f MB, limit %.1f MB). Narrow the review scope or use --branch/--commit.", float64(len(bodyBytes))/(1024*1024), float64(MaxSerializedBodyBytes)/(1024*1024)),
			1,
			nil,
		)
	}
	return nil
}

// SubmitReview triggers an authenticated code review with async polling support.
func (c *Client) SubmitReview(ctx context.Context, req ReviewRequest) (*ReviewResponse, error) {
	return c.SubmitReviewWithProgress(ctx, req, nil)
}

// SubmitReviewWithProgress triggers an authenticated code review and reports job status transitions.
func (c *Client) SubmitReviewWithProgress(ctx context.Context, req ReviewRequest, onProgress func(status string)) (*ReviewResponse, error) {
	if err := validateReviewLimits(req); err != nil {
		return nil, err
	}

	headers := map[string]string{
		"X-ScanDrix-Async": "1",
		"X-Async":          "1",
	}

	status, respBytes, err := c.DoRaw(ctx, http.MethodPost, "/cli/review", headers, req)
	if err != nil {
		// Fallback to legacy /api/v1/reviews
		var errFallback error
		status, respBytes, errFallback = c.DoRaw(ctx, http.MethodPost, "/api/v1/reviews", headers, req)
		if errFallback != nil {
			return nil, err
		}
	}

	// 1. Asynchronous Enqueue (202 Accepted)
	if status == http.StatusAccepted {
		var enqueueResp EnqueueJobResponse
		if err := json.Unmarshal(respBytes, &enqueueResp); err == nil {
			jobID := enqueueResp.JobID
			if jobID == "" {
				jobID = enqueueResp.JobIDAlt
			}
			if jobID != "" {
				return c.pollReviewJob(ctx, jobID, onProgress)
			}
		}
	}

	// 2. Synchronous Response (200 OK)
	var resp ReviewResponse
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		return nil, fmt.Errorf("failed decoding review response: %w", err)
	}
	return &resp, nil
}

func (c *Client) pollReviewJob(ctx context.Context, jobID string, onProgress func(status string)) (*ReviewResponse, error) {
	startTime := time.Now()
	delay := PollMinDelay
	lastStatus := ""

	endpoint := fmt.Sprintf("/cli/review/jobs/%s", url.PathEscape(jobID))
	fallbackEndpoint := fmt.Sprintf("/api/v1/reviews/jobs/%s", url.PathEscape(jobID))

	for time.Since(startTime) < PollMaxWait {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}

		delay = delay * 2
		if delay > PollMaxDelay {
			delay = PollMaxDelay
		}

		var jobStatus JobStatusResponse
		if err := c.Do(ctx, http.MethodGet, endpoint, nil, &jobStatus); err != nil {
			if errFallback := c.Do(ctx, http.MethodGet, fallbackEndpoint, nil, &jobStatus); errFallback != nil {
				// Transient poll error, continue
				continue
			}
		}

		currentStatus := jobStatus.Status
		if currentStatus != lastStatus {
			lastStatus = currentStatus
			if onProgress != nil {
				onProgress(currentStatus)
			}
		}

		switch currentStatus {
		case "COMPLETED":
			if jobStatus.Result == nil {
				return nil, fmt.Errorf("review job %s completed but returned no result", jobID)
			}
			return jobStatus.Result, nil
		case "FAILED":
			errMsg := jobStatus.Error
			if errMsg == "" {
				errMsg = fmt.Sprintf("review job %s failed without error details", jobID)
			}
			return nil, fmt.Errorf("review failed: %s", errMsg)
		}
	}

	return nil, fmt.Errorf("review job %s did not complete within timeout (%v)", jobID, PollMaxWait)
}

// GetTrialStatus queries the remaining daily trial review quota for a device fingerprint.
func (c *Client) GetTrialStatus(ctx context.Context, fingerprint string) (*utils.TrialStatus, error) {
	endpoint := "/cli/trial/status?fingerprint=" + url.QueryEscape(fingerprint)
	var status utils.TrialStatus
	if err := c.Do(ctx, http.MethodGet, endpoint, nil, &status); err != nil {
		fallback := "/api/v1/reviews/trial/status?fingerprint=" + url.QueryEscape(fingerprint)
		if errFallback := c.Do(ctx, http.MethodGet, fallback, nil, &status); errFallback != nil {
			return nil, err
		}
	}
	return &status, nil
}

// SubmitTrialReview performs an unauthenticated / trial review.
func (c *Client) SubmitTrialReview(ctx context.Context, req ReviewRequest) (*ReviewResponse, error) {
	if err := validateReviewLimits(req); err != nil {
		return nil, err
	}
	var resp ReviewResponse
	if err := c.Do(ctx, http.MethodPost, "/cli/trial/review", req, &resp); err != nil {
		if errFallback := c.Do(ctx, http.MethodPost, "/api/v1/reviews/trial", req, &resp); errFallback != nil {
			return nil, err
		}
	}
	return &resp, nil
}

// FetchSuggestions retrieves review suggestions for a remote pull request.
func (c *Client) FetchSuggestions(ctx context.Context, repoID string, prNumber int) ([]models.CodeFinding, error) {
	endpoint := fmt.Sprintf("/pull-requests/suggestions?prNumber=%d", prNumber)
	if repoID != "" {
		endpoint += "&repositoryId=" + url.QueryEscape(repoID)
	}

	var findings []models.CodeFinding
	if err := c.Do(ctx, http.MethodGet, endpoint, nil, &findings); err != nil {
		fallback := fmt.Sprintf("/api/v1/findings?pr_number=%d", prNumber)
		if repoID != "" {
			fallback += "&repo_id=" + url.QueryEscape(repoID)
		}
		if errFallback := c.Do(ctx, http.MethodGet, fallback, nil, &findings); errFallback != nil {
			return nil, err
		}
	}
	return findings, nil
}

// RunBusinessValidation triggers a PM spec verification via /cli/business-validation.
func (c *Client) RunBusinessValidation(ctx context.Context, repo, taskURL, taskID, diff string) (map[string]any, error) {
	payload := map[string]any{}
	if repo != "" {
		payload["repository"] = repo
	}
	if taskURL != "" {
		payload["taskUrl"] = taskURL
	}
	if taskID != "" {
		payload["taskId"] = taskID
	}
	if diff != "" {
		payload["diff"] = diff
	}

	var resp map[string]any
	if err := c.Do(ctx, http.MethodPost, "/cli/business-validation", payload, &resp); err != nil {
		if errFallback := c.Do(ctx, http.MethodPost, "/api/v1/integrations/pm/validate", payload, &resp); errFallback != nil {
			return nil, err
		}
	}
	return resp, nil
}

// PostPRComment posts a review summary to a pull request via backend integration.
func (c *Client) PostPRComment(ctx context.Context, repoNamespace string, prNumber int, commentBody string) error {
	payload := map[string]any{
		"repo_namespace": repoNamespace,
		"pull_number":    prNumber,
		"body":           commentBody,
	}
	return c.Do(ctx, http.MethodPost, "/api/v1/integrations/comments", payload, nil)
}
