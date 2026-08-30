package bitbucket

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

// ServerClient implements Bitbucket Data Center / Server 1.0 REST API.
type ServerClient struct {
	baseURL    string
	token      string
	username   string
	httpClient *http.Client
}

// NewServerClient creates a client for Bitbucket Server / Data Center.
func NewServerClient(baseURL, token, username string) *ServerClient {
	return &ServerClient{
		baseURL:  strings.TrimRight(baseURL, "/"),
		token:    token,
		username: username,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (s *ServerClient) Provider() models.SCMProvider {
	return models.ProviderBitbucket
}

func (s *ServerClient) parseProjectAndRepo(repo string) (string, string, error) {
	parts := strings.Split(strings.Trim(repo, "/"), "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid Bitbucket Server repo format, expected 'PROJECT/repo': %s", repo)
	}
	return parts[0], parts[1], nil
}

func (s *ServerClient) FetchPullRequest(ctx context.Context, repo string, pullNumber int) (*platform.PullRequestDetails, error) {
	proj, repoSlug, err := s.parseProjectAndRepo(repo)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d", s.baseURL, proj, repoSlug, pullNumber)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	s.setHeaders(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bitbucket server fetch pr returned status: %d", resp.StatusCode)
	}

	var data struct {
		ID     int    `json:"id"`
		Title  string `json:"title"`
		Author struct {
			User struct {
				Name        string `json:"name"`
				DisplayName string `json:"displayName"`
			} `json:"user"`
		} `json:"author"`
		FromRef struct {
			ID           string `json:"id"`
			LatestCommit string `json:"latestCommit"`
			DisplayID    string `json:"displayId"`
		} `json:"fromRef"`
		ToRef struct {
			ID           string `json:"id"`
			LatestCommit string `json:"latestCommit"`
			DisplayID    string `json:"displayId"`
		} `json:"toRef"`
		CreatedDate int64 `json:"createdDate"`
		Open        bool  `json:"open"`
		Draft       bool  `json:"draft"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	author := data.Author.User.Name
	if author == "" {
		author = data.Author.User.DisplayName
	}

	return &platform.PullRequestDetails{
		Number:       data.ID,
		Title:        data.Title,
		Author:       author,
		HeadSHA:      data.FromRef.LatestCommit,
		BaseSHA:      data.ToRef.LatestCommit,
		SourceBranch: data.FromRef.DisplayID,
		TargetBranch: data.ToRef.DisplayID,
		CreatedAt:    time.UnixMilli(data.CreatedDate),
		IsDraft:      data.Draft,
	}, nil
}

func (s *ServerClient) FetchDiff(ctx context.Context, repo string, pullNumber int) (string, error) {
	proj, repoSlug, err := s.parseProjectAndRepo(repo)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d/diff", s.baseURL, proj, repoSlug, pullNumber)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	s.setHeaders(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("bitbucket server fetch diff returned status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (s *ServerClient) PostInlineComments(ctx context.Context, repo string, pullNumber int, comments []platform.InlineCommentSpec) error {
	proj, repoSlug, err := s.parseProjectAndRepo(repo)
	if err != nil {
		return err
	}

	for _, comment := range comments {
		url := fmt.Sprintf("%s/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d/comments", s.baseURL, proj, repoSlug, pullNumber)
		payload := map[string]any{
			"text": comment.Body,
			"anchor": map[string]any{
				"line":     comment.Line,
				"lineType": "ADDED",
				"fileType": "TO",
				"path":     comment.FilePath,
			},
		}

		bodyBytes, _ := json.Marshal(payload)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
		if err != nil {
			return err
		}
		s.setHeaders(req)

		resp, err := s.httpClient.Do(req)
		if err != nil {
			return err
		}
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("bitbucket server post inline comment returned: %d", resp.StatusCode)
		}
	}
	return nil
}

func (s *ServerClient) PostReviewSummary(ctx context.Context, repo string, pullNumber int, summary string, conclusion platform.ReviewConclusion) error {
	proj, repoSlug, err := s.parseProjectAndRepo(repo)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d/comments", s.baseURL, proj, repoSlug, pullNumber)
	payload := map[string]any{
		"text": summary,
	}

	bodyBytes, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	s.setHeaders(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("bitbucket server post review summary returned: %d", resp.StatusCode)
	}
	return nil
}

func (s *ServerClient) SetCommitStatus(ctx context.Context, repo, commitSHA, contextName string, state platform.CommitStatusState, targetURL, description string) error {
	url := fmt.Sprintf("%s/rest/build-status/1.0/commits/%s", s.baseURL, commitSHA)
	bbState := "INPROGRESS"
	switch state {
	case platform.StatusSuccess:
		bbState = "SUCCESSFUL"
	case platform.StatusFailure, platform.StatusError:
		bbState = "FAILED"
	}

	payload := map[string]any{
		"state":       bbState,
		"key":         contextName,
		"name":        "ScanDrix Security Gate",
		"url":         targetURL,
		"description": description,
	}

	bodyBytes, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	s.setHeaders(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("bitbucket server set commit status returned: %d", resp.StatusCode)
	}
	return nil
}

func (s *ServerClient) ListBranches(ctx context.Context, repo string) ([]string, error) {
	proj, repoSlug, err := s.parseProjectAndRepo(repo)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/rest/api/1.0/projects/%s/repos/%s/branches", s.baseURL, proj, repoSlug)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	s.setHeaders(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bitbucket server list branches returned: %d", resp.StatusCode)
	}

	var data struct {
		Values []struct {
			DisplayID string `json:"displayId"`
		} `json:"values"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	branches := make([]string, 0, len(data.Values))
	for _, b := range data.Values {
		branches = append(branches, b.DisplayID)
	}
	return branches, nil
}

func (s *ServerClient) GetFileContent(ctx context.Context, repo, ref, path string) ([]byte, error) {
	proj, repoSlug, err := s.parseProjectAndRepo(repo)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/rest/api/1.0/projects/%s/repos/%s/raw/%s?at=%s", s.baseURL, proj, repoSlug, path, ref)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	s.setHeaders(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bitbucket server get file content returned: %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func (s *ServerClient) ApprovePullRequest(ctx context.Context, repo string, pullNumber int, message string) error {
	proj, repoSlug, err := s.parseProjectAndRepo(repo)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d/approve", s.baseURL, proj, repoSlug, pullNumber)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	s.setHeaders(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("bitbucket server approve PR returned: %d", resp.StatusCode)
	}
	return nil
}

func (s *ServerClient) MergePullRequest(ctx context.Context, repo string, pullNumber int, mergeMethod string) error {
	proj, repoSlug, err := s.parseProjectAndRepo(repo)
	if err != nil {
		return err
	}

	// Fetch current PR to get version
	prDetailsUrl := fmt.Sprintf("%s/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d", s.baseURL, proj, repoSlug, pullNumber)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, prDetailsUrl, nil)
	if err != nil {
		return err
	}
	s.setHeaders(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var prData struct {
		Version int `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&prData); err != nil {
		return err
	}

	mergeUrl := fmt.Sprintf("%s/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d/merge?version=%d", s.baseURL, proj, repoSlug, pullNumber, prData.Version)
	mergeReq, err := http.NewRequestWithContext(ctx, http.MethodPost, mergeUrl, nil)
	if err != nil {
		return err
	}
	s.setHeaders(mergeReq)

	mResp, err := s.httpClient.Do(mergeReq)
	if err != nil {
		return err
	}
	defer mResp.Body.Close()

	if mResp.StatusCode < 200 || mResp.StatusCode >= 300 {
		return fmt.Errorf("bitbucket server merge PR returned: %d", mResp.StatusCode)
	}
	return nil
}

func (s *ServerClient) VerifyWebhookSignature(secret string, payload []byte, signatureHeader string) bool {
	return verifyBitbucketSignature(secret, payload, signatureHeader)
}

func (s *ServerClient) ParseWebhookEvent(eventType string, payload []byte) (*platform.WebhookEventData, error) {
	return parseBitbucketWebhook(eventType, payload)
}

func (s *ServerClient) setHeaders(req *http.Request) {
	if s.username != "" {
		basic := base64.StdEncoding.EncodeToString([]byte(s.username + ":" + s.token))
		req.Header.Set("Authorization", "Basic "+basic)
	} else {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
}
