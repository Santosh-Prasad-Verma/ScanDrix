package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ReviewCommentPayload models an inline diff comment posted to a GitHub pull request review.
type ReviewCommentPayload struct {
	Path      string `json:"path"`
	Position  int    `json:"position,omitempty"`
	Line      int    `json:"line,omitempty"`
	Side      string `json:"side,omitempty"` // "RIGHT" for additions
	Body      string `json:"body"`
	StartLine int    `json:"start_line,omitempty"`
	StartSide string `json:"start_side,omitempty"`
}

// PullReviewSubmission models the GitHub API payload for submitting an overall review.
type PullReviewSubmission struct {
	CommitID string                 `json:"commit_id,omitempty"`
	Body     string                 `json:"body"`
	Event    string                 `json:"event"` // "COMMENT", "APPROVE", "REQUEST_CHANGES"
	Comments []ReviewCommentPayload `json:"comments,omitempty"`
}

// Client interacts with GitHub REST API v3 to retrieve diffs and publish automated reviews.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient creates a new GitHub API client instance.
func NewClient(token string) *Client {
	return &Client{
		baseURL: "https://api.github.com",
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// FetchPullRequestDiff retrieves the unified raw git diff for a specific pull request.
func (c *Client) FetchPullRequestDiff(ctx context.Context, owner, repo string, pullNumber int) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d", c.baseURL, owner, repo, pullNumber)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create pr diff request: %w", err)
	}

	// Request raw unified diff format from GitHub API
	req.Header.Set("Accept", "application/vnd.github.v3.diff")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("github api request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed fetching diff (HTTP %d): %s", resp.StatusCode, string(body))
	}

	diffBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed reading diff response: %w", err)
	}

	return string(diffBytes), nil
}

// SubmitPullRequestReview publishes review findings as inline comments and summary verdict on GitHub.
func (c *Client) SubmitPullRequestReview(ctx context.Context, owner, repo string, pullNumber int, submission PullReviewSubmission) error {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/reviews", c.baseURL, owner, repo, pullNumber)

	payloadBytes, err := json.Marshal(submission)
	if err != nil {
		return fmt.Errorf("failed marshaling review submission: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payloadBytes))
	if err != nil {
		return fmt.Errorf("failed creating review post request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("github api post review failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("github post review rejected (HTTP %d): %s", resp.StatusCode, string(body))
	}

	return nil
}
