package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/platform"
	"github.com/scandrix/backend/pkg/models"
)

func init() {
	platform.RegisterAdapter(models.ProviderGitHub, func(cfg platform.AdapterConfig) (platform.SCMAdapter, error) {
		baseURL := cfg.BaseURL
		if baseURL == "" {
			baseURL = "https://api.github.com"
		}
		return NewAdapter(baseURL, cfg.Token), nil
	})
}

// Adapter implements SCMAdapter for GitHub.
type Adapter struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewAdapter creates an authenticated GitHub API client.
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
	return models.ProviderGitHub
}

func (a *Adapter) FetchPullRequest(ctx context.Context, repo string, pullNumber int) (*platform.PullRequestDetails, error) {
	url := fmt.Sprintf("%s/repos/%s/pulls/%d", a.baseURL, repo, pullNumber)
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
		return nil, fmt.Errorf("github fetch pr returned status: %d", resp.StatusCode)
	}

	var data struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		User   struct {
			Login string `json:"login"`
		} `json:"user"`
		Head struct {
			SHA string `json:"sha"`
			Ref string `json:"ref"`
		} `json:"head"`
		Base struct {
			SHA string `json:"sha"`
			Ref string `json:"ref"`
		} `json:"base"`
		CreatedAt time.Time `json:"created_at"`
		Draft     bool      `json:"draft"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	return &platform.PullRequestDetails{
		Number:       data.Number,
		Title:        data.Title,
		Author:       data.User.Login,
		HeadSHA:      data.Head.SHA,
		BaseSHA:      data.Base.SHA,
		SourceBranch: data.Head.Ref,
		TargetBranch: data.Base.Ref,
		CreatedAt:    data.CreatedAt,
		IsDraft:      data.Draft,
	}, nil
}

func (a *Adapter) FetchDiff(ctx context.Context, repo string, pullNumber int) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/pulls/%d", a.baseURL, repo, pullNumber)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	a.setHeaders(req)
	req.Header.Set("Accept", "application/vnd.github.v3.diff")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github diff returned status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (a *Adapter) PostInlineComments(ctx context.Context, repo string, pullNumber int, comments []platform.InlineCommentSpec) error {
	if len(comments) == 0 {
		return nil
	}

	type ghComment struct {
		Path string `json:"path"`
		Line int    `json:"line"`
		Body string `json:"body"`
	}

	ghComments := make([]ghComment, 0, len(comments))
	for _, c := range comments {
		ghComments = append(ghComments, ghComment{
			Path: c.FilePath,
			Line: c.Line,
			Body: c.Body,
		})
	}

	payload := map[string]any{
		"event":    "COMMENT",
		"comments": ghComments,
	}

	bodyBytes, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/repos/%s/pulls/%d/reviews", a.baseURL, repo, pullNumber)
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
		return fmt.Errorf("github submit review comments returned status: %d", resp.StatusCode)
	}
	return nil
}

func (a *Adapter) PostReviewSummary(ctx context.Context, repo string, pullNumber int, summary string, conclusion platform.ReviewConclusion) error {
	event := "COMMENT"
	if conclusion == platform.ConclusionFailure {
		event = "REQUEST_CHANGES"
	} else if conclusion == platform.ConclusionSuccess {
		event = "APPROVE"
	}

	payload := map[string]any{
		"body":  summary,
		"event": event,
	}

	bodyBytes, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/repos/%s/pulls/%d/reviews", a.baseURL, repo, pullNumber)
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
		return fmt.Errorf("github post summary returned status: %d", resp.StatusCode)
	}
	return nil
}

func (a *Adapter) SetCommitStatus(ctx context.Context, repo, commitSHA, contextName string, state platform.CommitStatusState, targetURL, description string) error {
	ghState := "pending"
	switch state {
	case platform.StatusSuccess:
		ghState = "success"
	case platform.StatusFailure:
		ghState = "failure"
	case platform.StatusError:
		ghState = "error"
	}

	payload := map[string]any{
		"state":       ghState,
		"context":     contextName,
		"target_url":  targetURL,
		"description": description,
	}

	bodyBytes, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/repos/%s/statuses/%s", a.baseURL, repo, commitSHA)
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
		return fmt.Errorf("github set status returned %d", resp.StatusCode)
	}
	return nil
}

func (a *Adapter) ListBranches(ctx context.Context, repo string) ([]string, error) {
	url := fmt.Sprintf("%s/repos/%s/branches", a.baseURL, repo)
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
		return nil, fmt.Errorf("github list branches returned: %d", resp.StatusCode)
	}

	var data []struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	branches := make([]string, 0, len(data))
	for _, b := range data {
		branches = append(branches, b.Name)
	}
	return branches, nil
}

func (a *Adapter) GetFileContent(ctx context.Context, repo, ref, path string) ([]byte, error) {
	url := fmt.Sprintf("%s/repos/%s/contents/%s?ref=%s", a.baseURL, repo, path, ref)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	a.setHeaders(req)
	req.Header.Set("Accept", "application/vnd.github.v3.raw")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("file not found on github")
	}
	return io.ReadAll(resp.Body)
}

func (a *Adapter) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+a.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")
}
