package azuredevops

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/platform"
	"github.com/scandrix/backend/pkg/models"
)

func init() {
	platform.RegisterAdapter(models.ProviderAzure, func(cfg platform.AdapterConfig) (platform.SCMAdapter, error) {
		baseURL := cfg.BaseURL
		if baseURL == "" {
			baseURL = "https://dev.azure.com"
		}
		return NewAdapter(baseURL, cfg.Token), nil
	})
}

// Adapter implements SCMAdapter for Azure DevOps Git REST API v7.1.
type Adapter struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewAdapter creates an authenticated Azure DevOps client.
func NewAdapter(baseURL, token string) *Adapter {
	return &Adapter{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (a *Adapter) Provider() models.SCMProvider {
	return models.ProviderAzure
}

func (a *Adapter) FetchPullRequest(ctx context.Context, repo string, pullNumber int) (*platform.PullRequestDetails, error) {
	url := fmt.Sprintf("%s/_apis/git/repositories/%s/pullrequests/%d?api-version=7.1-preview.1", a.baseURL, repo, pullNumber)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	a.setHeaders(req)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("azure devops fetch pr returned status: %d", resp.StatusCode)
	}

	var data struct {
		PullRequestID int    `json:"pullRequestId"`
		Title         string `json:"title"`
		CreatedBy     struct {
			DisplayName string `json:"displayName"`
		} `json:"createdBy"`
		LastMergeSourceCommit struct {
			CommitID string `json:"commitId"`
		} `json:"lastMergeSourceCommit"`
		LastMergeTargetCommit struct {
			CommitID string `json:"commitId"`
		} `json:"lastMergeTargetCommit"`
		SourceRefName string    `json:"sourceRefName"`
		TargetRefName string    `json:"targetRefName"`
		CreationDate  time.Time `json:"creationDate"`
		IsDraft       bool      `json:"isDraft"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	return &platform.PullRequestDetails{
		Number:       data.PullRequestID,
		Title:        data.Title,
		Author:       data.CreatedBy.DisplayName,
		HeadSHA:      data.LastMergeSourceCommit.CommitID,
		BaseSHA:      data.LastMergeTargetCommit.CommitID,
		SourceBranch: strings.TrimPrefix(data.SourceRefName, "refs/heads/"),
		TargetBranch: strings.TrimPrefix(data.TargetRefName, "refs/heads/"),
		CreatedAt:    data.CreationDate,
		IsDraft:      data.IsDraft,
	}, nil
}

func (a *Adapter) FetchDiff(ctx context.Context, repo string, pullNumber int) (string, error) {
	// Azure DevOps provides iteration changes or raw diff via Git items endpoint
	url := fmt.Sprintf("%s/_apis/git/repositories/%s/pullrequests/%d/iterations?api-version=7.1-preview.1", a.baseURL, repo, pullNumber)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	a.setHeaders(req)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (a *Adapter) PostInlineComments(ctx context.Context, repo string, pullNumber int, comments []platform.InlineCommentSpec) error {
	for _, c := range comments {
		url := fmt.Sprintf("%s/_apis/git/repositories/%s/pullrequests/%d/threads?api-version=7.1-preview.1", a.baseURL, repo, pullNumber)
		payload := map[string]any{
			"comments": []map[string]any{
				{
					"parentCommentId": 0,
					"content":         c.Body,
					"commentType":     1, // text
				},
			},
			"threadContext": map[string]any{
				"filePath": c.FilePath,
				"rightFileStart": map[string]int{
					"line": c.Line,
				},
			},
			"status": 1, // Active
		}

		bodyBytes, _ := json.Marshal(payload)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
		if err != nil {
			return err
		}
		a.setHeaders(req)

		resp, err := a.httpClient.Do(req)
		if err != nil {
			return err
		}
		resp.Body.Close()
	}
	return nil
}

func (a *Adapter) PostReviewSummary(ctx context.Context, repo string, pullNumber int, summary string, conclusion platform.ReviewConclusion) error {
	url := fmt.Sprintf("%s/_apis/git/repositories/%s/pullrequests/%d/threads?api-version=7.1-preview.1", a.baseURL, repo, pullNumber)
	payload := map[string]any{
		"comments": []map[string]any{
			{
				"parentCommentId": 0,
				"content":         summary,
				"commentType":     1,
			},
		},
		"status": 1,
	}

	bodyBytes, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	a.setHeaders(req)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("azure devops post summary returned: %d", resp.StatusCode)
	}
	return nil
}

func (a *Adapter) SetCommitStatus(ctx context.Context, repo, commitSHA, contextName string, state platform.CommitStatusState, targetURL, description string) error {
	url := fmt.Sprintf("%s/_apis/git/repositories/%s/commits/%s/statuses?api-version=7.1-preview.1", a.baseURL, repo, commitSHA)
	azState := 1 // pending
	switch state {
	case platform.StatusSuccess:
		azState = 2 // succeeded
	case platform.StatusFailure:
		azState = 3 // failed
	case platform.StatusError:
		azState = 4 // error
	}

	payload := map[string]any{
		"state":       azState,
		"description": description,
		"targetUrl":   targetURL,
		"context": map[string]string{
			"name":  contextName,
			"genre": "security-gate",
		},
	}

	bodyBytes, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	a.setHeaders(req)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("azure devops set status returned: %d", resp.StatusCode)
	}
	return nil
}

func (a *Adapter) ListBranches(ctx context.Context, repo string) ([]string, error) {
	url := fmt.Sprintf("%s/_apis/git/repositories/%s/refs?filter=heads/&api-version=7.1-preview.1", a.baseURL, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	a.setHeaders(req)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data struct {
		Value []struct {
			Name string `json:"name"`
		} `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	branches := make([]string, 0, len(data.Value))
	for _, b := range data.Value {
		branches = append(branches, strings.TrimPrefix(b.Name, "refs/heads/"))
	}
	return branches, nil
}

func (a *Adapter) GetFileContent(ctx context.Context, repo, ref, path string) ([]byte, error) {
	url := fmt.Sprintf("%s/_apis/git/repositories/%s/items?path=%s&versionDescriptor.version=%s&api-version=7.1-preview.1", a.baseURL, repo, path, ref)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	a.setHeaders(req)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

func (a *Adapter) setHeaders(req *http.Request) {
	// Azure DevOps PAT authentication uses Basic with base64(":" + token)
	auth := base64.StdEncoding.EncodeToString([]byte(":" + a.token))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Content-Type", "application/json")
}
