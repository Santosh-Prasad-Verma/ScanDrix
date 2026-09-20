package bitbucket

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client interacts with Bitbucket Cloud REST API 2.0.
type Client struct {
	baseURL     string
	username    string
	appPassword string
	httpClient  *http.Client
}

// NewClient initializes a Bitbucket Cloud client.
func NewClient(username, appPassword string) *Client {
	return &Client{
		baseURL:     "https://api.bitbucket.org/2.0",
		username:    username,
		appPassword: appPassword,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// FetchPullRequestDiff retrieves raw diff for a Bitbucket pull request.
func (c *Client) FetchPullRequestDiff(ctx context.Context, workspace, repoSlug string, pullID int) (string, error) {
	url := fmt.Sprintf("%s/repositories/%s/%s/pullrequests/%d/diff", c.baseURL, workspace, repoSlug, pullID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	if c.username != "" && c.appPassword != "" {
		req.SetBasicAuth(c.username, c.appPassword)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("bitbucket diff request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("bitbucket diff error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// PostComment publishes a review comment on a Bitbucket pull request.
func (c *Client) PostComment(ctx context.Context, workspace, repoSlug string, pullID int, commentText string) error {
	url := fmt.Sprintf("%s/repositories/%s/%s/pullrequests/%d/comments", c.baseURL, workspace, repoSlug, pullID)

	payload := map[string]any{
		"content": map[string]string{
			"raw": commentText,
		},
	}
	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBytes))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	if c.username != "" && c.appPassword != "" {
		req.SetBasicAuth(c.username, c.appPassword)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("bitbucket comment request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("bitbucket post comment error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	return nil
}
