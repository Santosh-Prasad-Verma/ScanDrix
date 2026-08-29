package forgejo

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
	platform.RegisterAdapter(models.ProviderForgejo, func(cfg platform.AdapterConfig) (platform.SCMAdapter, error) {
		baseURL := cfg.BaseURL
		if baseURL == "" {
			baseURL = "https://codeberg.org"
		}
		return NewAdapter(baseURL, cfg.Token), nil
	})
}

// Adapter implements SCMAdapter for Forgejo / Gitea API v1.
type Adapter struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewAdapter creates an authenticated Forgejo/Gitea client.
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
	return models.ProviderForgejo
}

func (a *Adapter) FetchPullRequest(ctx context.Context, repo string, pullNumber int) (*platform.PullRequestDetails, error) {
	url := fmt.Sprintf("%s/api/v1/repos/%s/pulls/%d", a.baseURL, repo, pullNumber)
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
		return nil, fmt.Errorf("forgejo fetch pr returned status: %d", resp.StatusCode)
	}

	var data struct {
		Index int    `json:"index"`
		Title string `json:"title"`
		User  struct {
			Username string `json:"username"`
		} `json:"user"`
		Head struct {
			SHA string `json:"sha"`
			Ref string `json:"ref"`
		} `json:"head"`
		Base struct {
			SHA string `json:"sha"`
			Ref string `json:"ref"`
		} `json:"base"`
		Created time.Time `json:"created_at"`
		Draft   bool      `json:"draft"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	return &platform.PullRequestDetails{
		Number:       data.Index,
		Title:        data.Title,
		Author:       data.User.Username,
		HeadSHA:      data.Head.SHA,
		BaseSHA:      data.Base.SHA,
		SourceBranch: data.Head.Ref,
		TargetBranch: data.Base.Ref,
		CreatedAt:    data.Created,
		IsDraft:      data.Draft,
	}, nil
}

func (a *Adapter) FetchDiff(ctx context.Context, repo string, pullNumber int) (string, error) {
	url := fmt.Sprintf("%s/api/v1/repos/%s/pulls/%d.diff", a.baseURL, repo, pullNumber)
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
		return "", fmt.Errorf("forgejo diff returned status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (a *Adapter) PostInlineComments(ctx context.Context, repo string, pullNumber int, comments []platform.InlineCommentSpec) error {
	type forgejoComment struct {
		Path    string `json:"path"`
		NewLine int    `json:"new_position"`
		Body    string `json:"body"`
	}

	fComments := make([]forgejoComment, 0, len(comments))
	for _, c := range comments {
		fComments = append(fComments, forgejoComment{
			Path:    c.FilePath,
			NewLine: c.Line,
			Body:    c.Body,
		})
	}

	payload := map[string]any{
		"event":    "COMMENT",
		"comments": fComments,
	}

	bodyBytes, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/api/v1/repos/%s/pulls/%d/reviews", a.baseURL, repo, pullNumber)
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
		return fmt.Errorf("forgejo post reviews returned status: %d", resp.StatusCode)
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
	url := fmt.Sprintf("%s/api/v1/repos/%s/pulls/%d/reviews", a.baseURL, repo, pullNumber)
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
		return fmt.Errorf("forgejo post summary returned status: %d", resp.StatusCode)
	}
	return nil
}

func (a *Adapter) SetCommitStatus(ctx context.Context, repo, commitSHA, contextName string, state platform.CommitStatusState, targetURL, description string) error {
	url := fmt.Sprintf("%s/api/v1/repos/%s/statuses/%s", a.baseURL, repo, commitSHA)
	fgState := "pending"
	switch state {
	case platform.StatusSuccess:
		fgState = "success"
	case platform.StatusFailure:
		fgState = "failure"
	case platform.StatusError:
		fgState = "error"
	}

	payload := map[string]any{
		"state":       fgState,
		"context":     contextName,
		"target_url":  targetURL,
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
		return fmt.Errorf("forgejo set status returned: %d", resp.StatusCode)
	}
	return nil
}

func (a *Adapter) ListBranches(ctx context.Context, repo string) ([]string, error) {
	url := fmt.Sprintf("%s/api/v1/repos/%s/branches", a.baseURL, repo)
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
	url := fmt.Sprintf("%s/api/v1/repos/%s/raw/%s?ref=%s", a.baseURL, repo, path, ref)
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
	req.Header.Set("Authorization", "token "+a.token)
	req.Header.Set("Content-Type", "application/json")
}
