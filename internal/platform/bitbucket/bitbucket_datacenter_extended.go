// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

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
)

// BuildStatusState represents Bitbucket Server/DC build status state.
type BuildStatusState string

const (
	BuildStatusSuccessful BuildStatusState = "SUCCESSFUL"
	BuildStatusFailed     BuildStatusState = "FAILED"
	BuildStatusInProgress BuildStatusState = "INPROGRESS"
)

// BuildStatusRequest represents a build/commit status payload for Bitbucket Server/DC.
type BuildStatusRequest struct {
	State       BuildStatusState `json:"state"`
	Key         string           `json:"key"`
	Name        string           `json:"name"`
	URL         string           `json:"url"`
	Description string           `json:"description,omitempty"`
	DateAdded   *int64           `json:"dateAdded,omitempty"`
}

// BuildStatusResponse models the build status returned by Bitbucket Server/DC.
type BuildStatusResponse struct {
	State       BuildStatusState `json:"state"`
	Key         string           `json:"key"`
	Name        string           `json:"name"`
	URL         string           `json:"url"`
	Description string           `json:"description"`
	DateAdded   int64            `json:"dateAdded"`
}

// ServerBranchRestriction models Bitbucket Server 2.0 branch permission rule.
type ServerBranchRestriction struct {
	ID     int64    `json:"id,omitempty"`
	Type   string   `json:"type"` // "fast-forward-only", "no-deletes", "pull-request-only", "read-only"
	Matcher struct {
		ID        string `json:"id"`
		DisplayID string `json:"displayId"`
		Type      struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"type"`
		Active bool `json:"active"`
	} `json:"matcher"`
	Users  []string `json:"users,omitempty"`
	Groups []string `json:"groups,omitempty"`
}

// PullRequestParticipant represents a reviewer or participant on a Bitbucket PR.
type PullRequestParticipant struct {
	User struct {
		Name         string `json:"name"`
		EmailAddress string `json:"emailAddress"`
		ID           int    `json:"id"`
		DisplayName  string `json:"displayName"`
		Slug         string `json:"slug"`
	} `json:"user"`
	Role     string `json:"role"` // "REVIEWER", "AUTHOR", "PARTICIPANT"
	Approved bool   `json:"approved"`
	Status   string `json:"status"` // "APPROVED", "UNAPPROVED", "NEEDS_WORK"
}

// MergeCheckResponse represents merge readiness and veto checks on a PR.
type MergeCheckResponse struct {
	CanMerge bool `json:"canMerge"`
	Conflicted bool `json:"conflicted"`
	Outcome  string `json:"outcome"` // "CLEAN", "CONFLICTED"
	Vetoes   []struct {
		SummaryMessage  string `json:"summaryMessage"`
		DetailedMessage string `json:"detailedMessage"`
	} `json:"vetoes,omitempty"`
}

// BitbucketDataCenterExtended provides extended Bitbucket Server / Data Center REST API endpoints.
type BitbucketDataCenterExtended struct {
	httpClient *http.Client
	baseURL    string
}

// NewBitbucketDataCenterExtended creates a new BitbucketDataCenterExtended client.
func NewBitbucketDataCenterExtended(baseURL string, httpClient *http.Client) *BitbucketDataCenterExtended {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &BitbucketDataCenterExtended{
		httpClient: httpClient,
		baseURL:    strings.TrimRight(baseURL, "/"),
	}
}

func (c *BitbucketDataCenterExtended) doRequest(ctx context.Context, token, method, path string, body any) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	reqURL := fmt.Sprintf("%s%s", c.baseURL, path)
	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	return resp, nil
}

// PostCommitBuildStatus publishes a build status for a commit in Bitbucket Server/DC.
func (c *BitbucketDataCenterExtended) PostCommitBuildStatus(ctx context.Context, token, commitSHA string, req BuildStatusRequest) error {
	path := fmt.Sprintf("/rest/build-status/1.0/commits/%s", commitSHA)
	resp, err := c.doRequest(ctx, token, http.MethodPost, path, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("bitbucket post build status error (status %d): %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// GetCommitBuildStatuses retrieves build statuses for a commit.
func (c *BitbucketDataCenterExtended) GetCommitBuildStatuses(ctx context.Context, token, commitSHA string) ([]BuildStatusResponse, error) {
	path := fmt.Sprintf("/rest/build-status/1.0/commits/%s", commitSHA)
	resp, err := c.doRequest(ctx, token, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("bitbucket get build status error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var paged struct {
		Values []BuildStatusResponse `json:"values"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&paged); err != nil {
		return nil, fmt.Errorf("failed to decode build statuses: %w", err)
	}
	return paged.Values, nil
}

// ListBranchRestrictions retrieves branch permissions for a repository.
func (c *BitbucketDataCenterExtended) ListBranchRestrictions(ctx context.Context, token, projectKey, repoSlug string) ([]ServerBranchRestriction, error) {
	path := fmt.Sprintf("/rest/branch-permissions/2.0/projects/%s/repos/%s/restrictions", projectKey, repoSlug)
	resp, err := c.doRequest(ctx, token, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("bitbucket list branch restrictions error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var paged struct {
		Values []ServerBranchRestriction `json:"values"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&paged); err != nil {
		return nil, fmt.Errorf("failed to decode branch restrictions: %w", err)
	}
	return paged.Values, nil
}

// CreateBranchRestriction creates a branch permission restriction.
func (c *BitbucketDataCenterExtended) CreateBranchRestriction(ctx context.Context, token, projectKey, repoSlug string, restriction ServerBranchRestriction) (*ServerBranchRestriction, error) {
	path := fmt.Sprintf("/rest/branch-permissions/2.0/projects/%s/repos/%s/restrictions", projectKey, repoSlug)
	resp, err := c.doRequest(ctx, token, http.MethodPost, path, restriction)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("bitbucket create branch restriction error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var created ServerBranchRestriction
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return nil, fmt.Errorf("failed to decode branch restriction: %w", err)
	}
	return &created, nil
}

// DeleteBranchRestriction deletes a branch permission restriction by ID.
func (c *BitbucketDataCenterExtended) DeleteBranchRestriction(ctx context.Context, token, projectKey, repoSlug string, restrictionID int64) error {
	path := fmt.Sprintf("/rest/branch-permissions/2.0/projects/%s/repos/%s/restrictions/%d", projectKey, repoSlug, restrictionID)
	resp, err := c.doRequest(ctx, token, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("bitbucket delete branch restriction error (status %d): %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// GetPullRequestParticipants returns all participants on a PR.
func (c *BitbucketDataCenterExtended) GetPullRequestParticipants(ctx context.Context, token, projectKey, repoSlug string, prID int) ([]PullRequestParticipant, error) {
	path := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d/participants", projectKey, repoSlug, prID)
	resp, err := c.doRequest(ctx, token, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("bitbucket get participants error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var paged struct {
		Values []PullRequestParticipant `json:"values"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&paged); err != nil {
		return nil, fmt.Errorf("failed to decode participants: %w", err)
	}
	return paged.Values, nil
}

// CheckPullRequestMergeability queries the merge status and vetoes for a PR.
func (c *BitbucketDataCenterExtended) CheckPullRequestMergeability(ctx context.Context, token, projectKey, repoSlug string, prID int) (*MergeCheckResponse, error) {
	path := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d/merge", projectKey, repoSlug, prID)
	resp, err := c.doRequest(ctx, token, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("bitbucket check mergeability error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var check MergeCheckResponse
	if err := json.NewDecoder(resp.Body).Decode(&check); err != nil {
		return nil, fmt.Errorf("failed to decode mergeability response: %w", err)
	}
	return &check, nil
}
