package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/platform"
	"github.com/scandrix/backend/pkg/models"
)

func init() {
	platform.RegisterAdapter(models.ProviderGitLab, func(cfg platform.AdapterConfig) (platform.SCMAdapter, error) {
		baseURL := cfg.BaseURL
		if baseURL == "" {
			baseURL = "https://gitlab.com"
		}
		return NewAdapter(baseURL, cfg.Token), nil
	})
}

// Adapter implements SCMAdapter for GitLab.
type Adapter struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewAdapter creates an authenticated GitLab API client.
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
	return models.ProviderGitLab
}

func (a *Adapter) FetchPullRequest(ctx context.Context, repo string, pullNumber int) (*platform.PullRequestDetails, error) {
	encodedRepo := url.PathEscape(repo)
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d", a.baseURL, encodedRepo, pullNumber)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
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
		return nil, fmt.Errorf("gitlab fetch mr returned status: %d", resp.StatusCode)
	}

	var data struct {
		IID          int       `json:"iid"`
		Title        string    `json:"title"`
		Author       struct {
			Username string `json:"username"`
		} `json:"author"`
		SHA          string    `json:"sha"`
		DiffRefs     struct {
			BaseSHA string `json:"base_sha"`
			HeadSHA string `json:"head_sha"`
		} `json:"diff_refs"`
		SourceBranch string    `json:"source_branch"`
		TargetBranch string    `json:"target_branch"`
		CreatedAt    time.Time `json:"created_at"`
		Draft        bool      `json:"draft"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	headSHA := data.DiffRefs.HeadSHA
	if headSHA == "" {
		headSHA = data.SHA
	}

	return &platform.PullRequestDetails{
		Number:       data.IID,
		Title:        data.Title,
		Author:       data.Author.Username,
		HeadSHA:      headSHA,
		BaseSHA:      data.DiffRefs.BaseSHA,
		SourceBranch: data.SourceBranch,
		TargetBranch: data.TargetBranch,
		CreatedAt:    data.CreatedAt,
		IsDraft:      data.Draft,
	}, nil
}

func (a *Adapter) FetchDiff(ctx context.Context, repo string, pullNumber int) (string, error) {
	encodedRepo := url.PathEscape(repo)
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d/raw_diff", a.baseURL, encodedRepo, pullNumber)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
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
		return "", fmt.Errorf("gitlab diff returned status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (a *Adapter) PostInlineComments(ctx context.Context, repo string, pullNumber int, comments []platform.InlineCommentSpec) error {
	encodedRepo := url.PathEscape(repo)
	for _, c := range comments {
		endpoint := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d/discussions", a.baseURL, encodedRepo, pullNumber)
		payload := map[string]any{
			"body": c.Body,
			"position": map[string]any{
				"position_type": "text",
				"new_path":      c.FilePath,
				"new_line":      c.Line,
			},
		}

		bodyBytes, _ := json.Marshal(payload)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
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
	encodedRepo := url.PathEscape(repo)
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d/notes", a.baseURL, encodedRepo, pullNumber)

	payload := map[string]any{"body": summary}
	bodyBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
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
		return fmt.Errorf("gitlab post summary returned status: %d", resp.StatusCode)
	}
	return nil
}

func (a *Adapter) SetCommitStatus(ctx context.Context, repo, commitSHA, contextName string, state platform.CommitStatusState, targetURL, description string) error {
	encodedRepo := url.PathEscape(repo)
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/statuses/%s", a.baseURL, encodedRepo, commitSHA)

	glState := "pending"
	switch state {
	case platform.StatusSuccess:
		glState = "success"
	case platform.StatusFailure:
		glState = "failed"
	case platform.StatusError:
		glState = "canceled"
	}

	payload := map[string]any{
		"state":       glState,
		"name":        contextName,
		"target_url":  targetURL,
		"description": description,
	}

	bodyBytes, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
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
		return fmt.Errorf("gitlab set status returned: %d", resp.StatusCode)
	}
	return nil
}

func (a *Adapter) ListBranches(ctx context.Context, repo string) ([]string, error) {
	encodedRepo := url.PathEscape(repo)
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/repository/branches", a.baseURL, encodedRepo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
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
	encodedRepo := url.PathEscape(repo)
	encodedPath := url.PathEscape(path)
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/repository/files/%s/raw?ref=%s", a.baseURL, encodedRepo, encodedPath, ref)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
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

func (a *Adapter) ApprovePullRequest(ctx context.Context, repo string, pullNumber int, message string) error {
	encodedRepo := url.PathEscape(repo)
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d/approve", a.baseURL, encodedRepo, pullNumber)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
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
		return fmt.Errorf("gitlab approve merge request returned: %d", resp.StatusCode)
	}
	return nil
}

func (a *Adapter) MergePullRequest(ctx context.Context, repo string, pullNumber int, mergeMethod string) error {
	encodedRepo := url.PathEscape(repo)
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d/merge", a.baseURL, encodedRepo, pullNumber)
	payload := map[string]any{
		"should_remove_source_branch": true,
	}
	if mergeMethod == "squash" {
		payload["squash"] = true
	}
	bodyBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(bodyBytes))
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
		return fmt.Errorf("gitlab merge merge request returned: %d", resp.StatusCode)
	}
	return nil
}

func (a *Adapter) VerifyWebhookSignature(secret string, payload []byte, signatureHeader string) bool {
	if secret == "" || signatureHeader == "" {
		return false
	}
	// GitLab sends the plain webhook token in the X-Gitlab-Token header
	return secret == signatureHeader
}

func (a *Adapter) ParseWebhookEvent(eventType string, payload []byte) (*platform.WebhookEventData, error) {
	var event platform.WebhookEventData
	event.RawPayload = payload

	switch eventType {
	case "Merge Request Hook", "merge_request":
		event.Type = platform.WebhookEventPullRequest
		var mrEvent struct {
			ObjectKind string `json:"object_kind"`
			User       struct {
				Username string `json:"username"`
			} `json:"user"`
			Project struct {
				PathWithNamespace string `json:"path_with_namespace"`
				DefaultBranch     string `json:"default_branch"`
			} `json:"project"`
			ObjectAttributes struct {
				IID          int       `json:"iid"`
				Title        string    `json:"title"`
				Action       string    `json:"action"`
				LastCommit   struct{ ID string } `json:"last_commit"`
				SourceBranch string    `json:"source_branch"`
				TargetBranch string    `json:"target_branch"`
				WorkInProgress bool    `json:"work_in_progress"`
				CreatedAt    time.Time `json:"created_at"`
			} `json:"object_attributes"`
		}
		if err := json.Unmarshal(payload, &mrEvent); err != nil {
			return nil, err
		}
		event.Action = mrEvent.ObjectAttributes.Action
		event.Repository = mrEvent.Project.PathWithNamespace
		event.DefaultBranch = mrEvent.Project.DefaultBranch
		event.Sender = mrEvent.User.Username
		event.PullRequest = &platform.PullRequestDetails{
			Number:       mrEvent.ObjectAttributes.IID,
			Title:        mrEvent.ObjectAttributes.Title,
			Author:       mrEvent.User.Username,
			HeadSHA:      mrEvent.ObjectAttributes.LastCommit.ID,
			SourceBranch: mrEvent.ObjectAttributes.SourceBranch,
			TargetBranch: mrEvent.ObjectAttributes.TargetBranch,
			CreatedAt:    mrEvent.ObjectAttributes.CreatedAt,
			IsDraft:      mrEvent.ObjectAttributes.WorkInProgress,
		}
	case "Push Hook", "push":
		event.Type = platform.WebhookEventPush
		var pushEvent struct {
			After     string `json:"after"`
			UserName  string `json:"user_username"`
			Project   struct {
				PathWithNamespace string `json:"path_with_namespace"`
				DefaultBranch     string `json:"default_branch"`
			} `json:"project"`
		}
		if err := json.Unmarshal(payload, &pushEvent); err != nil {
			return nil, err
		}
		event.CommitSHA = pushEvent.After
		event.Repository = pushEvent.Project.PathWithNamespace
		event.DefaultBranch = pushEvent.Project.DefaultBranch
		event.Sender = pushEvent.UserName
	case "Note Hook", "note":
		event.Type = platform.WebhookEventReviewComment
		var noteEvent struct {
			User struct {
				Username string `json:"username"`
			} `json:"user"`
			Project struct {
				PathWithNamespace string `json:"path_with_namespace"`
			} `json:"project"`
			ObjectAttributes struct {
				ID   int64  `json:"id"`
				Note string `json:"note"`
			} `json:"object_attributes"`
		}
		if err := json.Unmarshal(payload, &noteEvent); err != nil {
			return nil, err
		}
		event.Repository = noteEvent.Project.PathWithNamespace
		event.CommentID = noteEvent.ObjectAttributes.ID
		event.CommentBody = noteEvent.ObjectAttributes.Note
		event.Sender = noteEvent.User.Username
	default:
		event.Type = platform.WebhookEventType(eventType)
	}

	return &event, nil
}

func (a *Adapter) setHeaders(req *http.Request) {
	req.Header.Set("PRIVATE-TOKEN", a.token)
	req.Header.Set("Content-Type", "application/json")
}

