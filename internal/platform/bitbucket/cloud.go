package bitbucket

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/platform"
	"github.com/scandrix/backend/pkg/models"
)

// CloudClient implements Bitbucket Cloud 2.0 REST API.
type CloudClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewCloudClient creates a new client for Bitbucket Cloud (api.bitbucket.org/2.0).
func NewCloudClient(baseURL, token string) *CloudClient {
	if baseURL == "" {
		baseURL = "https://api.bitbucket.org/2.0"
	}
	return &CloudClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *CloudClient) Provider() models.SCMProvider {
	return models.ProviderBitbucket
}

func (c *CloudClient) FetchPullRequest(ctx context.Context, repo string, pullNumber int) (*platform.PullRequestDetails, error) {
	url := fmt.Sprintf("%s/repositories/%s/pullrequests/%d", c.baseURL, repo, pullNumber)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bitbucket cloud fetch pr returned status: %d", resp.StatusCode)
	}

	var data struct {
		ID     int    `json:"id"`
		Title  string `json:"title"`
		Author struct {
			DisplayName string `json:"display_name"`
			Nickname    string `json:"nickname"`
		} `json:"author"`
		Source struct {
			Commit struct {
				Hash string `json:"hash"`
			} `json:"commit"`
			Branch struct {
				Name string `json:"name"`
			} `json:"branch"`
		} `json:"source"`
		Destination struct {
			Commit struct {
				Hash string `json:"hash"`
			} `json:"commit"`
			Branch struct {
				Name string `json:"name"`
			} `json:"branch"`
		} `json:"destination"`
		CreatedOn time.Time `json:"created_on"`
		Draft     bool      `json:"draft"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	authorName := data.Author.Nickname
	if authorName == "" {
		authorName = data.Author.DisplayName
	}

	return &platform.PullRequestDetails{
		Number:       data.ID,
		Title:        data.Title,
		Author:       authorName,
		HeadSHA:      data.Source.Commit.Hash,
		BaseSHA:      data.Destination.Commit.Hash,
		SourceBranch: data.Source.Branch.Name,
		TargetBranch: data.Destination.Branch.Name,
		CreatedAt:    data.CreatedOn,
		IsDraft:      data.Draft,
	}, nil
}

func (c *CloudClient) FetchDiff(ctx context.Context, repo string, pullNumber int) (string, error) {
	url := fmt.Sprintf("%s/repositories/%s/pullrequests/%d/diff", c.baseURL, repo, pullNumber)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("bitbucket cloud fetch diff returned status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (c *CloudClient) PostInlineComments(ctx context.Context, repo string, pullNumber int, comments []platform.InlineCommentSpec) error {
	for _, comment := range comments {
		url := fmt.Sprintf("%s/repositories/%s/pullrequests/%d/comments", c.baseURL, repo, pullNumber)
		payload := map[string]any{
			"content": map[string]string{
				"raw": comment.Body,
			},
			"inline": map[string]any{
				"to":   comment.Line,
				"path": comment.FilePath,
			},
		}
		if comment.StartLine > 0 && comment.StartLine != comment.Line {
			payload["inline"].(map[string]any)["from"] = comment.StartLine
		}

		bodyBytes, _ := json.Marshal(payload)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
		if err != nil {
			return err
		}
		c.setHeaders(req)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return err
		}
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("bitbucket cloud post inline comment failed with status: %d", resp.StatusCode)
		}
	}
	return nil
}

func (c *CloudClient) PostReviewSummary(ctx context.Context, repo string, pullNumber int, summary string, conclusion platform.ReviewConclusion) error {
	url := fmt.Sprintf("%s/repositories/%s/pullrequests/%d/comments", c.baseURL, repo, pullNumber)
	payload := map[string]any{
		"content": map[string]string{
			"raw": summary,
		},
	}

	bodyBytes, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("bitbucket cloud post review summary returned: %d", resp.StatusCode)
	}
	return nil
}

func (c *CloudClient) SetCommitStatus(ctx context.Context, repo, commitSHA, contextName string, state platform.CommitStatusState, targetURL, description string) error {
	url := fmt.Sprintf("%s/repositories/%s/commit/%s/statuses/build", c.baseURL, repo, commitSHA)
	bbState := "INPROGRESS"
	switch state {
	case platform.StatusSuccess:
		bbState = "SUCCESSFUL"
	case platform.StatusFailure, platform.StatusError:
		bbState = "FAILED"
	}

	payload := map[string]any{
		"key":         contextName,
		"state":       bbState,
		"name":        "ScanDrix Security Gate",
		"url":         targetURL,
		"description": description,
	}

	bodyBytes, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("bitbucket cloud set status returned: %d", resp.StatusCode)
	}
	return nil
}

func (c *CloudClient) ListBranches(ctx context.Context, repo string) ([]string, error) {
	url := fmt.Sprintf("%s/repositories/%s/refs/branches", c.baseURL, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bitbucket cloud list branches returned: %d", resp.StatusCode)
	}

	var data struct {
		Values []struct {
			Name string `json:"name"`
		} `json:"values"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	branches := make([]string, 0, len(data.Values))
	for _, b := range data.Values {
		branches = append(branches, b.Name)
	}
	return branches, nil
}

func (c *CloudClient) GetFileContent(ctx context.Context, repo, ref, path string) ([]byte, error) {
	url := fmt.Sprintf("%s/repositories/%s/src/%s/%s", c.baseURL, repo, ref, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bitbucket cloud get file content returned: %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func (c *CloudClient) ApprovePullRequest(ctx context.Context, repo string, pullNumber int, message string) error {
	url := fmt.Sprintf("%s/repositories/%s/pullrequests/%d/approve", c.baseURL, repo, pullNumber)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("bitbucket cloud approve PR returned: %d", resp.StatusCode)
	}
	return nil
}

func (c *CloudClient) MergePullRequest(ctx context.Context, repo string, pullNumber int, mergeMethod string) error {
	url := fmt.Sprintf("%s/repositories/%s/pullrequests/%d/merge", c.baseURL, repo, pullNumber)
	payload := map[string]any{
		"close_source_branch": true,
	}
	if mergeMethod == "squash" {
		payload["merge_strategy"] = "squash"
	} else if mergeMethod == "fast_forward" {
		payload["merge_strategy"] = "fast_forward"
	} else {
		payload["merge_strategy"] = "merge_commit"
	}

	bodyBytes, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("bitbucket cloud merge PR returned: %d", resp.StatusCode)
	}
	return nil
}

func (c *CloudClient) VerifyWebhookSignature(secret string, payload []byte, signatureHeader string) bool {
	return verifyBitbucketSignature(secret, payload, signatureHeader)
}

func (c *CloudClient) ParseWebhookEvent(eventType string, payload []byte) (*platform.WebhookEventData, error) {
	return parseBitbucketWebhook(eventType, payload)
}

func (c *CloudClient) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
}
