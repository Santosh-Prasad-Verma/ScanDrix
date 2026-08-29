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

func init() {
	platform.RegisterAdapter(models.ProviderBitbucket, func(cfg platform.AdapterConfig) (platform.SCMAdapter, error) {
		baseURL := cfg.BaseURL
		if baseURL == "" {
			baseURL = "https://api.bitbucket.org/2.0"
		}
		return NewAdapter(baseURL, cfg.Token), nil
	})
}

// Adapter implements SCMAdapter for Bitbucket Cloud 2.0.
type Adapter struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewAdapter creates an authenticated Bitbucket API client.
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
	return models.ProviderBitbucket
}

func (a *Adapter) FetchPullRequest(ctx context.Context, repo string, pullNumber int) (*platform.PullRequestDetails, error) {
	url := fmt.Sprintf("%s/repositories/%s/pullrequests/%d", a.baseURL, repo, pullNumber)
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
		return nil, fmt.Errorf("bitbucket fetch pr returned status: %d", resp.StatusCode)
	}

	var data struct {
		ID     int    `json:"id"`
		Title  string `json:"title"`
		Author struct {
			DisplayName string `json:"display_name"`
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
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	return &platform.PullRequestDetails{
		Number:       data.ID,
		Title:        data.Title,
		Author:       data.Author.DisplayName,
		HeadSHA:      data.Source.Commit.Hash,
		BaseSHA:      data.Destination.Commit.Hash,
		SourceBranch: data.Source.Branch.Name,
		TargetBranch: data.Destination.Branch.Name,
		CreatedAt:    data.CreatedOn,
		IsDraft:      false,
	}, nil
}

func (a *Adapter) FetchDiff(ctx context.Context, repo string, pullNumber int) (string, error) {
	url := fmt.Sprintf("%s/repositories/%s/pullrequests/%d/diff", a.baseURL, repo, pullNumber)
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
		return "", fmt.Errorf("bitbucket diff returned status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (a *Adapter) PostInlineComments(ctx context.Context, repo string, pullNumber int, comments []platform.InlineCommentSpec) error {
	for _, c := range comments {
		url := fmt.Sprintf("%s/repositories/%s/pullrequests/%d/comments", a.baseURL, repo, pullNumber)
		payload := map[string]any{
			"content": map[string]string{
				"raw": c.Body,
			},
			"inline": map[string]any{
				"to":   c.Line,
				"path": c.FilePath,
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
		resp.Body.Close()
	}
	return nil
}

func (a *Adapter) PostReviewSummary(ctx context.Context, repo string, pullNumber int, summary string, conclusion platform.ReviewConclusion) error {
	url := fmt.Sprintf("%s/repositories/%s/pullrequests/%d/comments", a.baseURL, repo, pullNumber)
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
	a.setHeaders(req)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("bitbucket post summary returned %d", resp.StatusCode)
	}
	return nil
}

func (a *Adapter) SetCommitStatus(ctx context.Context, repo, commitSHA, contextName string, state platform.CommitStatusState, targetURL, description string) error {
	url := fmt.Sprintf("%s/repositories/%s/commit/%s/statuses/build", a.baseURL, repo, commitSHA)
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
	a.setHeaders(req)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("bitbucket set status returned %d", resp.StatusCode)
	}
	return nil
}

func (a *Adapter) ListBranches(ctx context.Context, repo string) ([]string, error) {
	url := fmt.Sprintf("%s/repositories/%s/refs/branches", a.baseURL, repo)
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

func (a *Adapter) GetFileContent(ctx context.Context, repo, ref, path string) ([]byte, error) {
	url := fmt.Sprintf("%s/repositories/%s/src/%s/%s", a.baseURL, repo, ref, path)
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
	req.Header.Set("Authorization", "Bearer "+a.token)
	req.Header.Set("Content-Type", "application/json")
}
