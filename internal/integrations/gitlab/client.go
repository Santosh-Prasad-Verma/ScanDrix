package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Client interacts with GitLab REST API v4.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient initializes a GitLab API client.
func NewClient(baseURL, token string) *Client {
	if baseURL == "" {
		baseURL = "https://gitlab.com/api/v4"
	}
	return &Client{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// FetchMergeRequestDiff retrieves raw diff for a GitLab Merge Request.
func (c *Client) FetchMergeRequestDiff(ctx context.Context, projectPath string, mrIID int) (string, error) {
	encodedProject := url.PathEscape(projectPath)
	url := fmt.Sprintf("%s/projects/%s/merge_requests/%d/raw_diff", c.baseURL, encodedProject, mrIID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	if c.token != "" {
		req.Header.Set("PRIVATE-TOKEN", c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("gitlab diff request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("gitlab diff error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// PostDiscussion creates a line discussion on a GitLab Merge Request.
func (c *Client) PostDiscussion(ctx context.Context, projectPath string, mrIID int, noteBody string) error {
	encodedProject := url.PathEscape(projectPath)
	url := fmt.Sprintf("%s/projects/%s/merge_requests/%d/discussions", c.baseURL, encodedProject, mrIID)

	payload := map[string]string{"body": noteBody}
	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBytes))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("PRIVATE-TOKEN", c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("gitlab post discussion failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("gitlab post discussion rejected (HTTP %d): %s", resp.StatusCode, string(body))
	}

	return nil
}
