// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package pr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/cli/configcli"
	"github.com/scandrix/backend/pkg/models"
)

var validBranchRegex = regexp.MustCompile(`^[a-zA-Z0-9._/-]+$`)

// PRClient handles fetching remote pull request diffs, querying suggestions, and posting review comments.
type PRClient struct {
	serverURL  string
	authToken  string
	httpClient *http.Client
}

// NewPRClient creates a PR workflow client.
func NewPRClient(serverURL, authToken string) *PRClient {
	if serverURL == "" {
		cfg := configcli.Load(".")
		serverURL = cfg.ServerURL
	}
	return &PRClient{
		serverURL:  serverURL,
		authToken:  authToken,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// FetchDiffFromGitHubAPI fetches unified diff for a pull request directly from the GitHub REST API.
func FetchDiffFromGitHubAPI(ctx context.Context, namespace string, prNumber int, token string) (string, error) {
	if namespace == "" || prNumber <= 0 {
		return "", fmt.Errorf("invalid namespace or pull request number")
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s/pulls/%d", namespace, prNumber)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed creating pr diff request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3.diff")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("github api request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("github api returned status %d: %s", resp.StatusCode, string(body))
	}

	diffBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed reading diff response: %w", err)
	}

	return string(diffBytes), nil
}

// FetchDiffFromGit attempts to fetch and extract the PR diff locally using Git refs.
func FetchDiffFromGit(ctx context.Context, prNumber int, baseBranch string) (string, error) {
	if prNumber <= 0 {
		return "", fmt.Errorf("invalid PR number: %d (must be positive)", prNumber)
	}

	if baseBranch == "" {
		baseBranch = "main"
	}

	// Prevent command / argument injection via crafted branch names or flags (Master Rule 5.3)
	if strings.HasPrefix(baseBranch, "-") || strings.Contains(baseBranch, "..") || !validBranchRegex.MatchString(baseBranch) {
		return "", fmt.Errorf("invalid base branch name %q: contains illegal characters or flags", baseBranch)
	}

	// 1. Fetch PR ref from origin (GitHub format)
	refSpec := fmt.Sprintf("pull/%d/head:pr-%d", prNumber, prNumber)
	fetchCmd := exec.CommandContext(ctx, "git", "fetch", "origin", refSpec)
	var fetchErrBuf bytes.Buffer
	fetchCmd.Stderr = &fetchErrBuf
	_ = fetchCmd.Run() // Best effort; may fail if origin is GitLab or local ref exists

	// 2. Diff against base branch with explicit revision terminator '--'
	diffCmd := exec.CommandContext(ctx, "git", "diff", "--no-color", fmt.Sprintf("origin/%s...pr-%d", baseBranch, prNumber), "--")
	var out, diffErrBuf bytes.Buffer
	diffCmd.Stdout = &out
	diffCmd.Stderr = &diffErrBuf
	if err := diffCmd.Run(); err == nil && out.Len() > 0 {
		return out.String(), nil
	}

	// Fallback to GitLab merge request ref syntax
	refSpecGL := fmt.Sprintf("merge-requests/%d/head:mr-%d", prNumber, prNumber)
	fetchGLCmd := exec.CommandContext(ctx, "git", "fetch", "origin", refSpecGL)
	var fetchGLErrBuf bytes.Buffer
	fetchGLCmd.Stderr = &fetchGLErrBuf
	_ = fetchGLCmd.Run()

	diffCmdGL := exec.CommandContext(ctx, "git", "diff", "--no-color", fmt.Sprintf("origin/%s...mr-%d", baseBranch, prNumber), "--")
	var outGL, diffGLErrBuf bytes.Buffer
	diffCmdGL.Stdout = &outGL
	diffCmdGL.Stderr = &diffGLErrBuf
	if err := diffCmdGL.Run(); err == nil && outGL.Len() > 0 {
		return outGL.String(), nil
	}

	lastErr := strings.TrimSpace(diffGLErrBuf.String())
	if lastErr == "" {
		lastErr = strings.TrimSpace(diffErrBuf.String())
	}
	if lastErr == "" {
		lastErr = strings.TrimSpace(fetchErrBuf.String())
	}
	if lastErr != "" {
		return "", fmt.Errorf("unable to fetch diff for PR #%d locally via git: %s", prNumber, lastErr)
	}
	return "", fmt.Errorf("unable to fetch diff for PR #%d locally via git (verify remotes and network)", prNumber)
}

// PostReviewComment posts a markdown review summary comment to the remote PR via the ScanDrix API gateway.
func (c *PRClient) PostReviewComment(ctx context.Context, repoNamespace string, prNumber int, markdownSummary string) error {
	if c.authToken == "" {
		return fmt.Errorf("authentication token required to post review comments")
	}

	payload := map[string]any{
		"repo_namespace": repoNamespace,
		"pull_number":    prNumber,
		"body":           markdownSummary,
	}
	bodyBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/v1/integrations/comments", c.serverURL),
		bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.authToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed connecting to ScanDrix API gateway: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed posting comment (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// SuggestionFilterOptions defines query parameters for fetching PR suggestions.
type SuggestionFilterOptions struct {
	PRURL      string `json:"pr_url,omitempty"`
	PRNumber   int    `json:"pr_number,omitempty"`
	RepoID     string `json:"repo_id,omitempty"`
	Severities string `json:"severities,omitempty"`
	Categories string `json:"categories,omitempty"`
	Format     string `json:"format,omitempty"`
}

// FetchPRSuggestions queries the server for review suggestions on a PR.
func (c *PRClient) FetchPRSuggestions(ctx context.Context, opts SuggestionFilterOptions) ([]models.CodeFinding, error) {
	url := fmt.Sprintf("%s/api/v1/findings?pr_number=%d", c.serverURL, opts.PRNumber)
	if opts.RepoID != "" {
		url += "&repo_id=" + opts.RepoID
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return []models.CodeFinding{}, nil
	}

	var findings []models.CodeFinding
	if err := json.NewDecoder(resp.Body).Decode(&findings); err != nil {
		return nil, err
	}
	return findings, nil
}

// BusinessValidationRequest models request for PM / task spec verification.
type BusinessValidationRequest struct {
	TaskURL    string `json:"task_url,omitempty"`
	TaskID     string `json:"task_id,omitempty"`
	RawDiff    string `json:"raw_diff"`
	Repository string `json:"repository,omitempty"`
}

// BusinessValidationResponse models response of business validation check.
type BusinessValidationResponse struct {
	Valid          bool     `json:"valid"`
	TaskTitle      string   `json:"task_title,omitempty"`
	Requirements   []string `json:"requirements_met,omitempty"`
	MissingDetails []string `json:"missing_requirements,omitempty"`
	Score          float64  `json:"compliance_score"`
}

// RunBusinessValidation validates code changes against task specifications.
func (c *PRClient) RunBusinessValidation(ctx context.Context, req BusinessValidationRequest) (*BusinessValidationResponse, error) {
	bodyBytes, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.serverURL+"/api/v1/integrations/pm/validate", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.authToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.authToken)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed connecting to PM integration validation gateway (%s): %w", c.serverURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var out BusinessValidationResponse
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return nil, fmt.Errorf("failed decoding business validation response: %w", err)
		}
		return &out, nil
	}

	respBody, _ := io.ReadAll(resp.Body)
	return nil, fmt.Errorf("business validation failed (HTTP %d): %s", resp.StatusCode, string(respBody))
}

// ParsePRInput parses a PR number or full PR URL into repo namespace and PR number.
func ParsePRInput(input string) (namespace string, prNumber int, err error) {
	input = strings.TrimSpace(input)

	// Case 1: Simple integer, e.g. "42"
	var num int
	if n, _ := fmt.Sscanf(input, "%d", &num); n == 1 && !strings.Contains(input, "/") {
		return "", num, nil
	}

	// Case 2: GitHub URL: https://github.com/owner/repo/pull/42
	if strings.Contains(input, "/pull/") {
		parts := strings.Split(input, "/pull/")
		if len(parts) == 2 {
			_, _ = fmt.Sscanf(parts[1], "%d", &prNumber)
			urlParts := strings.Split(strings.TrimPrefix(parts[0], "https://"), "/")
			if len(urlParts) >= 3 {
				namespace = fmt.Sprintf("%s/%s", urlParts[1], urlParts[2])
			}
			return namespace, prNumber, nil
		}
	}

	// Case 3: GitLab MR URL: https://gitlab.com/owner/repo/-/merge_requests/42
	if strings.Contains(input, "/merge_requests/") {
		parts := strings.Split(input, "/merge_requests/")
		if len(parts) == 2 {
			_, _ = fmt.Sscanf(parts[1], "%d", &prNumber)
			urlClean := strings.TrimSuffix(parts[0], "/-")
			urlParts := strings.Split(strings.TrimPrefix(urlClean, "https://"), "/")
			if len(urlParts) >= 3 {
				namespace = strings.Join(urlParts[1:], "/")
			}
			return namespace, prNumber, nil
		}
	}

	return "", 0, fmt.Errorf("invalid PR identifier %q (expected number or URL)", input)
}
