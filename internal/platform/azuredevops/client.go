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

// resolveRepoPath formats the repo URL path supporting "org/project/repo" or "project/repo" or "repo".
func (a *Adapter) buildRepoURL(repo string, subpath string) string {
	parts := strings.Split(strings.Trim(repo, "/"), "/")
	if len(parts) >= 2 {
		// e.g. "my-project/my-repo" -> "{baseURL}/my-project/_apis/git/repositories/my-repo/{subpath}"
		project := parts[0]
		repoName := parts[1]
		if len(parts) == 3 {
			// e.g. "my-org/my-project/my-repo"
			project = parts[1]
			repoName = parts[2]
		}
		return fmt.Sprintf("%s/%s/_apis/git/repositories/%s/%s", a.baseURL, project, repoName, strings.TrimPrefix(subpath, "/"))
	}
	return fmt.Sprintf("%s/_apis/git/repositories/%s/%s", a.baseURL, repo, strings.TrimPrefix(subpath, "/"))
}

func (a *Adapter) FetchPullRequest(ctx context.Context, repo string, pullNumber int) (*platform.PullRequestDetails, error) {
	url := a.buildRepoURL(repo, fmt.Sprintf("pullrequests/%d?api-version=7.1-preview.1", pullNumber))
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
			UniqueName  string `json:"uniqueName"`
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

	author := data.CreatedBy.UniqueName
	if author == "" {
		author = data.CreatedBy.DisplayName
	}

	return &platform.PullRequestDetails{
		Number:       data.PullRequestID,
		Title:        data.Title,
		Author:       author,
		HeadSHA:      data.LastMergeSourceCommit.CommitID,
		BaseSHA:      data.LastMergeTargetCommit.CommitID,
		SourceBranch: strings.TrimPrefix(data.SourceRefName, "refs/heads/"),
		TargetBranch: strings.TrimPrefix(data.TargetRefName, "refs/heads/"),
		CreatedAt:    data.CreationDate,
		IsDraft:      data.IsDraft,
	}, nil
}

func (a *Adapter) FetchDiff(ctx context.Context, repo string, pullNumber int) (string, error) {
	// Azure DevOps provides iteration changes
	url := a.buildRepoURL(repo, fmt.Sprintf("pullrequests/%d/iterations?api-version=7.1-preview.1", pullNumber))
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

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("azure devops fetch diff iterations returned status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (a *Adapter) PostInlineComments(ctx context.Context, repo string, pullNumber int, comments []platform.InlineCommentSpec) error {
	for _, c := range comments {
		url := a.buildRepoURL(repo, fmt.Sprintf("pullrequests/%d/threads?api-version=7.1-preview.1", pullNumber))

		threadContext := map[string]any{
			"filePath": c.FilePath,
			"rightFileStart": map[string]int{
				"line": c.Line,
			},
		}
		if c.StartLine > 0 && c.StartLine != c.Line {
			threadContext["rightFileStart"] = map[string]int{
				"line": c.StartLine,
			}
			threadContext["rightFileEnd"] = map[string]int{
				"line": c.Line,
			}
		}

		payload := map[string]any{
			"comments": []map[string]any{
				{
					"parentCommentId": 0,
					"content":         c.Body,
					"commentType":     1, // text
				},
			},
			"threadContext": threadContext,
			"status":        1, // Active
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
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("azure devops post thread comment returned: %d", resp.StatusCode)
		}
	}
	return nil
}

func (a *Adapter) PostReviewSummary(ctx context.Context, repo string, pullNumber int, summary string, conclusion platform.ReviewConclusion) error {
	url := a.buildRepoURL(repo, fmt.Sprintf("pullrequests/%d/threads?api-version=7.1-preview.1", pullNumber))
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
	url := a.buildRepoURL(repo, fmt.Sprintf("commits/%s/statuses?api-version=7.1-preview.1", commitSHA))
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
	url := a.buildRepoURL(repo, "refs?filter=heads/&api-version=7.1-preview.1")
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
		return nil, fmt.Errorf("azure devops list branches returned: %d", resp.StatusCode)
	}

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
	url := a.buildRepoURL(repo, fmt.Sprintf("items?path=%s&versionDescriptor.version=%s&api-version=7.1-preview.1", path, ref))
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
		return nil, fmt.Errorf("azure devops get file content returned: %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func (a *Adapter) ApprovePullRequest(ctx context.Context, repo string, pullNumber int, message string) error {
	// Vote on PR with vote: 10 (Approved) or vote: 5 (Approved with suggestions)
	url := a.buildRepoURL(repo, fmt.Sprintf("pullrequests/%d/reviewers/@me?api-version=7.1-preview.1", pullNumber))
	payload := map[string]any{
		"vote": 10,
	}

	bodyBytes, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(bodyBytes))
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
		return fmt.Errorf("azure devops approve PR returned: %d", resp.StatusCode)
	}
	return nil
}

func (a *Adapter) MergePullRequest(ctx context.Context, repo string, pullNumber int, mergeMethod string) error {
	// Azure DevOps completes PRs by updating status to 3 (Completed) with completionOptions
	url := a.buildRepoURL(repo, fmt.Sprintf("pullrequests/%d?api-version=7.1-preview.1", pullNumber))

	// Fetch PR first to obtain lastMergeSourceCommit ID
	prDetails, err := a.FetchPullRequest(ctx, repo, pullNumber)
	if err != nil {
		return err
	}

	payload := map[string]any{
		"status": 3, // Completed
		"lastMergeSourceCommit": map[string]string{
			"commitId": prDetails.HeadSHA,
		},
		"completionOptions": map[string]any{
			"deleteSourceBranch": true,
			"squashMerge":        mergeMethod == "squash",
		},
	}

	bodyBytes, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewReader(bodyBytes))
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
		return fmt.Errorf("azure devops merge PR returned: %d", resp.StatusCode)
	}
	return nil
}

func (a *Adapter) VerifyWebhookSignature(secret string, payload []byte, signatureHeader string) bool {
	return verifyAzureServiceHookSignature(secret, payload, signatureHeader)
}

func (a *Adapter) ParseWebhookEvent(eventType string, payload []byte) (*platform.WebhookEventData, error) {
	return parseAzureWebhook(eventType, payload)
}

func (a *Adapter) setHeaders(req *http.Request) {
	auth := base64.StdEncoding.EncodeToString([]byte(":" + a.token))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
}
