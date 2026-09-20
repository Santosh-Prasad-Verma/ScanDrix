// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ReleaseAsset represents an asset attached to a GitHub release.
type ReleaseAsset struct {
	ID                 int64     `json:"id"`
	Name               string    `json:"name"`
	Label              string    `json:"label"`
	State              string    `json:"state"` // "uploaded"
	ContentType        string    `json:"content_type"`
	Size               int64     `json:"size"`
	DownloadCount      int       `json:"download_count"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	BrowserDownloadURL string    `json:"browser_download_url"`
}

// Release represents a GitHub release.
type Release struct {
	ID              int64          `json:"id"`
	TagName         string         `json:"tag_name"`
	TargetCommitish string         `json:"target_commitish"`
	Name            string         `json:"name"`
	Body            string         `json:"body"`
	Draft           bool           `json:"draft"`
	Prerelease      bool           `json:"prerelease"`
	CreatedAt       time.Time      `json:"created_at"`
	PublishedAt     *time.Time     `json:"published_at,omitempty"`
	HTMLURL         string         `json:"html_url"`
	Assets          []ReleaseAsset `json:"assets"`
}

// CreateReleaseRequest represents parameters for creating a new release.
type CreateReleaseRequest struct {
	TagName         string `json:"tag_name"`
	TargetCommitish string `json:"target_commitish,omitempty"`
	Name            string `json:"name,omitempty"`
	Body            string `json:"body,omitempty"`
	Draft           bool   `json:"draft,omitempty"`
	Prerelease      bool   `json:"prerelease,omitempty"`
	GenerateNotes   bool   `json:"generate_release_notes,omitempty"`
}

// CollaboratorUser represents a collaborator on a repository.
type CollaboratorUser struct {
	ID          int64             `json:"id"`
	Login       string            `json:"login"`
	RoleName    string            `json:"role_name"`
	Permissions map[string]bool   `json:"permissions"`
}

// GitHubReleasesService provides repository releases and metadata operations.
type GitHubReleasesService struct {
	httpClient *http.Client
	baseURL    string
}

// NewGitHubReleasesService creates a new GitHub releases service instance.
func NewGitHubReleasesService(httpClient *http.Client, baseURL ...string) *GitHubReleasesService {
	url := "https://api.github.com"
	if len(baseURL) > 0 && baseURL[0] != "" {
		url = strings.TrimRight(baseURL[0], "/")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &GitHubReleasesService{
		httpClient: httpClient,
		baseURL:    url,
	}
}

func (s *GitHubReleasesService) doRequest(ctx context.Context, token, method, path string, body any) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	reqURL := fmt.Sprintf("%s%s", s.baseURL, path)
	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if token != "" {
		req.Header.Set("Authorization", "token "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	return resp, nil
}

// ListReleases returns releases for a repository.
func (s *GitHubReleasesService) ListReleases(ctx context.Context, token, owner, repo string, perPage, page int) ([]Release, error) {
	if perPage <= 0 {
		perPage = 30
	}
	if page <= 0 {
		page = 1
	}
	path := fmt.Sprintf("/repos/%s/%s/releases?per_page=%d&page=%d", owner, repo, perPage, page)
	resp, err := s.doRequest(ctx, token, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github list releases error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var releases []Release
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("failed to decode releases: %w", err)
	}
	return releases, nil
}

// GetLatestRelease returns the latest published release.
func (s *GitHubReleasesService) GetLatestRelease(ctx context.Context, token, owner, repo string) (*Release, error) {
	path := fmt.Sprintf("/repos/%s/%s/releases/latest", owner, repo)
	resp, err := s.doRequest(ctx, token, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github get latest release error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to decode release: %w", err)
	}
	return &release, nil
}

// CreateRelease creates a new release on a repository.
func (s *GitHubReleasesService) CreateRelease(ctx context.Context, token, owner, repo string, req CreateReleaseRequest) (*Release, error) {
	path := fmt.Sprintf("/repos/%s/%s/releases", owner, repo)
	resp, err := s.doRequest(ctx, token, http.MethodPost, path, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github create release error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to decode created release: %w", err)
	}
	return &release, nil
}

// GetRepositoryTopics returns repository topics.
func (s *GitHubReleasesService) GetRepositoryTopics(ctx context.Context, token, owner, repo string) ([]string, error) {
	path := fmt.Sprintf("/repos/%s/%s/topics", owner, repo)
	resp, err := s.doRequest(ctx, token, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github get topics error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var data struct {
		Names []string `json:"names"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode topics: %w", err)
	}
	return data.Names, nil
}

// ReplaceRepositoryTopics sets topics for a repository.
func (s *GitHubReleasesService) ReplaceRepositoryTopics(ctx context.Context, token, owner, repo string, topics []string) ([]string, error) {
	path := fmt.Sprintf("/repos/%s/%s/topics", owner, repo)
	resp, err := s.doRequest(ctx, token, http.MethodPut, path, map[string][]string{"names": topics})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github replace topics error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var data struct {
		Names []string `json:"names"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode replaced topics: %w", err)
	}
	return data.Names, nil
}

// ListCollaborators returns repository collaborators.
func (s *GitHubReleasesService) ListCollaborators(ctx context.Context, token, owner, repo string, affiliation string) ([]CollaboratorUser, error) {
	path := fmt.Sprintf("/repos/%s/%s/collaborators", owner, repo)
	if affiliation != "" {
		path += "?affiliation=" + affiliation
	}
	resp, err := s.doRequest(ctx, token, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github list collaborators error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var users []CollaboratorUser
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		return nil, fmt.Errorf("failed to decode collaborators: %w", err)
	}
	return users, nil
}
