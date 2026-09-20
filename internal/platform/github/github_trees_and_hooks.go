// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package github

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
)

// GitHubTreeEntry represents an entry inside a Git tree object.
type GitHubTreeEntry struct {
	Path string `json:"path"`
	Mode string `json:"mode"` // "100644", "100755", "040000", "160000", "120000"
	Type string `json:"type"` // "blob", "tree", "commit"
	SHA  string `json:"sha"`
	Size int64  `json:"size,omitempty"`
	URL  string `json:"url"`
}

// GitHubTree represents a Git tree object.
type GitHubTree struct {
	SHA       string            `json:"sha"`
	URL       string            `json:"url"`
	Tree      []GitHubTreeEntry `json:"tree"`
	Truncated bool              `json:"truncated"`
}

// CreateGitHubTreeEntry represents an entry to create or modify in a tree.
type CreateGitHubTreeEntry struct {
	Path    string `json:"path"`
	Mode    string `json:"mode"`
	Type    string `json:"type"`
	SHA     string `json:"sha,omitempty"`
	Content string `json:"content,omitempty"`
}

// CreateGitHubTreeRequest holds parameters for creating a new tree.
type CreateGitHubTreeRequest struct {
	BaseTree string                  `json:"base_tree,omitempty"`
	Tree     []CreateGitHubTreeEntry `json:"tree"`
}

// GitHubBlob represents a Git blob object.
type GitHubBlob struct {
	SHA      string `json:"sha"`
	Size     int64  `json:"size"`
	URL      string `json:"url"`
	Content  string `json:"content"`
	Encoding string `json:"encoding"` // "base64", "utf-8"
}

// CreateGitHubBlobRequest holds parameters for creating a Git blob.
type CreateGitHubBlobRequest struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding,omitempty"` // "utf-8" (default) or "base64"
}

// GitHubHookConfig holds webhook target URL, secret, and content type.
type GitHubHookConfig struct {
	URL         string `json:"url"`
	ContentType string `json:"content_type,omitempty"`
	Secret      string `json:"secret,omitempty"`
	InsecureSSL string `json:"insecure_ssl,omitempty"`
}

// GitHubHook represents a repository webhook in GitHub.
type GitHubHook struct {
	ID        int64            `json:"id"`
	Name      string           `json:"name"`
	Active    bool             `json:"active"`
	Events    []string         `json:"events"`
	Config    GitHubHookConfig `json:"config"`
	UpdatedAt time.Time        `json:"updated_at"`
	CreatedAt time.Time        `json:"created_at"`
	URL       string           `json:"url"`
	PingURL   string           `json:"ping_url,omitempty"`
	TestURL   string           `json:"test_url,omitempty"`
}

// CreateGitHubHookRequest holds parameters to create a repository webhook.
type CreateGitHubHookRequest struct {
	Name   string           `json:"name"`
	Active bool             `json:"active"`
	Events []string         `json:"events"`
	Config GitHubHookConfig `json:"config"`
}

// UpdateGitHubHookRequest holds parameters to update a repository webhook.
type UpdateGitHubHookRequest struct {
	Active *bool             `json:"active,omitempty"`
	Events []string          `json:"events,omitempty"`
	Config *GitHubHookConfig `json:"config,omitempty"`
}

// GitHubTreesAndHooksService provides methods for Git trees, blobs, and repository webhooks.
type GitHubTreesAndHooksService struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewGitHubTreesAndHooksService creates a new instance of GitHubTreesAndHooksService.
func NewGitHubTreesAndHooksService(baseURL, token string, client *http.Client) *GitHubTreesAndHooksService {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &GitHubTreesAndHooksService{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		httpClient: client,
	}
}

func (s *GitHubTreesAndHooksService) applyAuth(req *http.Request) {
	if strings.HasPrefix(s.token, "token ") || strings.HasPrefix(s.token, "Bearer ") {
		req.Header.Set("Authorization", s.token)
	} else if s.token != "" {
		req.Header.Set("Authorization", "token "+s.token)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
}

// GetTree retrieves a Git tree by SHA or branch name.
func (s *GitHubTreesAndHooksService) GetTree(ctx context.Context, owner, repo, treeSHA string, recursive bool) (*GitHubTree, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/git/trees/%s", s.baseURL, url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(treeSHA))
	if recursive {
		endpoint += "?recursive=1"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	s.applyAuth(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get tree failed (status %d): %s", resp.StatusCode, string(body))
	}

	var tree GitHubTree
	if err := json.NewDecoder(resp.Body).Decode(&tree); err != nil {
		return nil, err
	}
	return &tree, nil
}

// CreateTree creates a new Git tree from existing tree entries or base tree.
func (s *GitHubTreesAndHooksService) CreateTree(ctx context.Context, owner, repo string, req CreateGitHubTreeRequest) (*GitHubTree, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/git/trees", s.baseURL, url.PathEscape(owner), url.PathEscape(repo))

	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	s.applyAuth(httpReq)

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create tree failed (status %d): %s", resp.StatusCode, string(body))
	}

	var tree GitHubTree
	if err := json.NewDecoder(resp.Body).Decode(&tree); err != nil {
		return nil, err
	}
	return &tree, nil
}

// GetBlob retrieves a Git blob by its SHA.
func (s *GitHubTreesAndHooksService) GetBlob(ctx context.Context, owner, repo, fileSHA string) (*GitHubBlob, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/git/blobs/%s", s.baseURL, url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(fileSHA))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	s.applyAuth(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get blob failed (status %d): %s", resp.StatusCode, string(body))
	}

	var blob GitHubBlob
	if err := json.NewDecoder(resp.Body).Decode(&blob); err != nil {
		return nil, err
	}
	return &blob, nil
}

// CreateBlob creates a new Git blob.
func (s *GitHubTreesAndHooksService) CreateBlob(ctx context.Context, owner, repo string, req CreateGitHubBlobRequest) (string, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/git/blobs", s.baseURL, url.PathEscape(owner), url.PathEscape(repo))

	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	s.applyAuth(httpReq)

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create blob failed (status %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		SHA string `json:"sha"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.SHA, nil
}

// ListHooks retrieves webhooks registered for a repository.
func (s *GitHubTreesAndHooksService) ListHooks(ctx context.Context, owner, repo string) ([]GitHubHook, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/hooks", s.baseURL, url.PathEscape(owner), url.PathEscape(repo))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	s.applyAuth(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list hooks failed (status %d): %s", resp.StatusCode, string(body))
	}

	var hooks []GitHubHook
	if err := json.NewDecoder(resp.Body).Decode(&hooks); err != nil {
		return nil, err
	}
	return hooks, nil
}

// GetHook retrieves a single webhook by ID.
func (s *GitHubTreesAndHooksService) GetHook(ctx context.Context, owner, repo string, hookID int64) (*GitHubHook, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/hooks/%d", s.baseURL, url.PathEscape(owner), url.PathEscape(repo), hookID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	s.applyAuth(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get hook failed (status %d): %s", resp.StatusCode, string(body))
	}

	var hook GitHubHook
	if err := json.NewDecoder(resp.Body).Decode(&hook); err != nil {
		return nil, err
	}
	return &hook, nil
}

// CreateHook creates a new repository webhook.
func (s *GitHubTreesAndHooksService) CreateHook(ctx context.Context, owner, repo string, req CreateGitHubHookRequest) (*GitHubHook, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/hooks", s.baseURL, url.PathEscape(owner), url.PathEscape(repo))

	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	s.applyAuth(httpReq)

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create hook failed (status %d): %s", resp.StatusCode, string(body))
	}

	var hook GitHubHook
	if err := json.NewDecoder(resp.Body).Decode(&hook); err != nil {
		return nil, err
	}
	return &hook, nil
}

// UpdateHook updates a repository webhook.
func (s *GitHubTreesAndHooksService) UpdateHook(ctx context.Context, owner, repo string, hookID int64, req UpdateGitHubHookRequest) (*GitHubHook, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/hooks/%d", s.baseURL, url.PathEscape(owner), url.PathEscape(repo), hookID)

	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	s.applyAuth(httpReq)

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("update hook failed (status %d): %s", resp.StatusCode, string(body))
	}

	var hook GitHubHook
	if err := json.NewDecoder(resp.Body).Decode(&hook); err != nil {
		return nil, err
	}
	return &hook, nil
}

// DeleteHook deletes a repository webhook.
func (s *GitHubTreesAndHooksService) DeleteHook(ctx context.Context, owner, repo string, hookID int64) error {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/hooks/%d", s.baseURL, url.PathEscape(owner), url.PathEscape(repo), hookID)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	s.applyAuth(httpReq)

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete hook failed (status %d): %s", resp.StatusCode, string(body))
	}
	return nil
}

// PingHook sends a ping event to the webhook.
func (s *GitHubTreesAndHooksService) PingHook(ctx context.Context, owner, repo string, hookID int64) error {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/hooks/%d/pings", s.baseURL, url.PathEscape(owner), url.PathEscape(repo), hookID)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}
	s.applyAuth(httpReq)

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ping hook failed (status %d): %s", resp.StatusCode, string(body))
	}
	return nil
}
