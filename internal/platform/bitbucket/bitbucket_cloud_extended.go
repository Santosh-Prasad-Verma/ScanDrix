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
	"net/url"
	"strings"
	"time"
)

// -------------------------------------------------------------------------------------
// Bitbucket Cloud REST 2.0 Models
// -------------------------------------------------------------------------------------

// CloudCommitStatusState models Bitbucket Cloud commit status states.
type CloudCommitStatusState string

const (
	CloudStatusSuccessful CloudCommitStatusState = "SUCCESSFUL"
	CloudStatusFailed     CloudCommitStatusState = "FAILED"
	CloudStatusInProgress CloudCommitStatusState = "INPROGRESS"
	CloudStatusStopped    CloudCommitStatusState = "STOPPED"
)

// CloudCommitStatusInput is the payload for creating/updating a commit status.
type CloudCommitStatusInput struct {
	Key         string                 `json:"key"`
	State       CloudCommitStatusState `json:"state"`
	Name        string                 `json:"name"`
	URL         string                 `json:"url"`
	Description string                 `json:"description,omitempty"`
}

// CloudCommitStatusOutput models the status returned by Bitbucket Cloud.
type CloudCommitStatusOutput struct {
	Key         string                 `json:"key"`
	State       CloudCommitStatusState `json:"state"`
	Name        string                 `json:"name"`
	URL         string                 `json:"url"`
	Description string                 `json:"description"`
	CreatedOn   time.Time              `json:"created_on"`
	UpdatedOn   time.Time              `json:"updated_on"`
	Type        string                 `json:"type"` // "commit_status"
}

// CloudPRCommitSummary models an individual commit in a pull request.
type CloudPRCommitSummary struct {
	Hash    string    `json:"hash"`
	Message string    `json:"message"`
	Date    time.Time `json:"date"`
	Author  struct {
		Raw  string `json:"raw"`
		User struct {
			DisplayName string `json:"display_name"`
			UUID        string `json:"uuid"`
			Nickname    string `json:"nickname"`
		} `json:"user"`
	} `json:"author"`
}

// CloudDiffstatEntry represents a file modification summary in a pull request.
type CloudDiffstatEntry struct {
	Status       string `json:"status"` // added, modified, removed
	LinesAdded   int    `json:"lines_added"`
	LinesRemoved int    `json:"lines_removed"`
	OldPath      string `json:"old_path,omitempty"`
	NewPath      string `json:"new_path,omitempty"`
}

// CloudRepoEnvironment represents a deployment environment in Bitbucket Cloud.
type CloudRepoEnvironment struct {
	UUID            string `json:"uuid"`
	Name            string `json:"name"`
	EnvironmentType string `json:"environment_type"` // Test, Staging, Production
	Rank            int    `json:"rank"`
}

// CloudRepoEnvironmentInput specifies parameters to create an environment.
type CloudRepoEnvironmentInput struct {
	Name            string `json:"name"`
	EnvironmentType string `json:"environment_type"`
	Rank            int    `json:"rank,omitempty"`
}

// CloudMergeRequest specifies parameters for merging a PR.
type CloudMergeRequest struct {
	Message           string `json:"message,omitempty"`
	CloseSourceBranch bool   `json:"close_source_branch"`
	MergeStrategy     string `json:"merge_strategy,omitempty"` // merge_commit, squash, fast_forward
}

// CloudMergeResponse models the response when a pull request is merged.
type CloudMergeResponse struct {
	Hash               string `json:"hash"`
	State              string `json:"state"` // MERGED
	ClosedSourceBranch bool   `json:"closed_source_branch"`
}

// CloudMergeabilityStatus models whether a pull request can be merged.
type CloudMergeabilityStatus struct {
	CanMerge bool     `json:"can_merge"`
	Reasons  []string `json:"reasons,omitempty"`
}

// -------------------------------------------------------------------------------------
// Bitbucket Cloud Extended Service API Methods
// -------------------------------------------------------------------------------------

// CreateCloudCommitStatus creates or updates a commit status check in Bitbucket Cloud.
func (s *BitbucketAdvancedService) CreateCloudCommitStatus(
	ctx context.Context,
	token, workspace, repoSlug, commitSHA string,
	status CloudCommitStatusInput,
) (*CloudCommitStatusOutput, error) {
	endpoint := fmt.Sprintf("%s/repositories/%s/%s/commit/%s/statuses/build",
		s.baseURL, workspace, repoSlug, commitSHA)

	bodyBytes, err := json.Marshal(status)
	if err != nil {
		return nil, fmt.Errorf("marshal commit status: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create commit status request: %w", err)
	}
	s.applyCloudAuth(req, token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute commit status request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("bitbucket cloud commit status error: status %d: %s", resp.StatusCode, string(b))
	}

	var output CloudCommitStatusOutput
	if err := json.NewDecoder(resp.Body).Decode(&output); err != nil {
		return nil, fmt.Errorf("decode commit status response: %w", err)
	}
	return &output, nil
}

// GetCloudCommitStatuses retrieves all commit statuses for a specific commit SHA.
func (s *BitbucketAdvancedService) GetCloudCommitStatuses(
	ctx context.Context,
	token, workspace, repoSlug, commitSHA string,
) ([]CloudCommitStatusOutput, error) {
	endpoint := fmt.Sprintf("%s/repositories/%s/%s/commit/%s/statuses",
		s.baseURL, workspace, repoSlug, commitSHA)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create get commit statuses request: %w", err)
	}
	s.applyCloudAuth(req, token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute get commit statuses request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("bitbucket cloud get statuses error: status %d: %s", resp.StatusCode, string(b))
	}

	var wrapper struct {
		Values []CloudCommitStatusOutput `json:"values"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil, fmt.Errorf("decode commit statuses response: %w", err)
	}
	return wrapper.Values, nil
}

// GetCloudCommitStatusByKey retrieves a specific commit status by its key.
func (s *BitbucketAdvancedService) GetCloudCommitStatusByKey(
	ctx context.Context,
	token, workspace, repoSlug, commitSHA, key string,
) (*CloudCommitStatusOutput, error) {
	endpoint := fmt.Sprintf("%s/repositories/%s/%s/commit/%s/statuses/build/%s",
		s.baseURL, workspace, repoSlug, commitSHA, url.PathEscape(key))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create get commit status by key request: %w", err)
	}
	s.applyCloudAuth(req, token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute get commit status by key request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("bitbucket cloud get status by key error: status %d: %s", resp.StatusCode, string(b))
	}

	var output CloudCommitStatusOutput
	if err := json.NewDecoder(resp.Body).Decode(&output); err != nil {
		return nil, fmt.Errorf("decode commit status response: %w", err)
	}
	return &output, nil
}

// GetCloudPRCommits retrieves the list of commits belonging to a pull request.
func (s *BitbucketAdvancedService) GetCloudPRCommits(
	ctx context.Context,
	token, workspace, repoSlug string,
	prID int,
) ([]CloudPRCommitSummary, error) {
	endpoint := fmt.Sprintf("%s/repositories/%s/%s/pullrequests/%d/commits",
		s.baseURL, workspace, repoSlug, prID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create get pr commits request: %w", err)
	}
	s.applyCloudAuth(req, token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute get pr commits request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("bitbucket cloud get pr commits error: status %d: %s", resp.StatusCode, string(b))
	}

	var wrapper struct {
		Values []CloudPRCommitSummary `json:"values"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil, fmt.Errorf("decode pr commits response: %w", err)
	}
	return wrapper.Values, nil
}

// GetCloudPRDiffstat retrieves the diffstat entries for a pull request.
func (s *BitbucketAdvancedService) GetCloudPRDiffstat(
	ctx context.Context,
	token, workspace, repoSlug string,
	prID int,
) ([]CloudDiffstatEntry, error) {
	endpoint := fmt.Sprintf("%s/repositories/%s/%s/pullrequests/%d/diffstat",
		s.baseURL, workspace, repoSlug, prID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create get pr diffstat request: %w", err)
	}
	s.applyCloudAuth(req, token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute get pr diffstat request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("bitbucket cloud get diffstat error: status %d: %s", resp.StatusCode, string(b))
	}

	var wrapper struct {
		Values []struct {
			Status       string `json:"status"`
			LinesAdded   int    `json:"lines_added"`
			LinesRemoved int    `json:"lines_removed"`
			Old          *struct {
				Path string `json:"path"`
			} `json:"old"`
			New *struct {
				Path string `json:"path"`
			} `json:"new"`
		} `json:"values"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil, fmt.Errorf("decode diffstat response: %w", err)
	}

	entries := make([]CloudDiffstatEntry, len(wrapper.Values))
	for i, v := range wrapper.Values {
		entry := CloudDiffstatEntry{
			Status:       v.Status,
			LinesAdded:   v.LinesAdded,
			LinesRemoved: v.LinesRemoved,
		}
		if v.Old != nil {
			entry.OldPath = v.Old.Path
		}
		if v.New != nil {
			entry.NewPath = v.New.Path
		}
		entries[i] = entry
	}
	return entries, nil
}

// GetCloudPRPatch downloads the raw git patch representation of a PR.
func (s *BitbucketAdvancedService) GetCloudPRPatch(
	ctx context.Context,
	token, workspace, repoSlug string,
	prID int,
) (string, error) {
	endpoint := fmt.Sprintf("%s/repositories/%s/%s/pullrequests/%d/patch",
		s.baseURL, workspace, repoSlug, prID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("create get pr patch request: %w", err)
	}
	s.applyCloudAuth(req, token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("execute get pr patch request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("bitbucket cloud get patch error: status %d: %s", resp.StatusCode, string(b))
	}

	patchBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read pr patch body: %w", err)
	}
	return string(patchBytes), nil
}

// ListCloudEnvironments lists deployment environments configured for a repository.
func (s *BitbucketAdvancedService) ListCloudEnvironments(
	ctx context.Context,
	token, workspace, repoSlug string,
) ([]CloudRepoEnvironment, error) {
	endpoint := fmt.Sprintf("%s/repositories/%s/%s/environments",
		s.baseURL, workspace, repoSlug)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create list environments request: %w", err)
	}
	s.applyCloudAuth(req, token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute list environments request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("bitbucket cloud list environments error: status %d: %s", resp.StatusCode, string(b))
	}

	var wrapper struct {
		Values []CloudRepoEnvironment `json:"values"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil, fmt.Errorf("decode environments response: %w", err)
	}
	return wrapper.Values, nil
}

// CreateCloudEnvironment creates a new deployment environment for a repository.
func (s *BitbucketAdvancedService) CreateCloudEnvironment(
	ctx context.Context,
	token, workspace, repoSlug string,
	env CloudRepoEnvironmentInput,
) (*CloudRepoEnvironment, error) {
	endpoint := fmt.Sprintf("%s/repositories/%s/%s/environments",
		s.baseURL, workspace, repoSlug)

	bodyBytes, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("marshal environment input: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create environment request: %w", err)
	}
	s.applyCloudAuth(req, token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute create environment request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("bitbucket cloud create environment error: status %d: %s", resp.StatusCode, string(b))
	}

	var created CloudRepoEnvironment
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return nil, fmt.Errorf("decode created environment: %w", err)
	}
	return &created, nil
}

// DeleteCloudEnvironment removes an environment by its UUID.
func (s *BitbucketAdvancedService) DeleteCloudEnvironment(
	ctx context.Context,
	token, workspace, repoSlug, envUUID string,
) error {
	endpoint := fmt.Sprintf("%s/repositories/%s/%s/environments/%s",
		s.baseURL, workspace, repoSlug, envUUID)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create delete environment request: %w", err)
	}
	s.applyCloudAuth(req, token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute delete environment request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("bitbucket cloud delete environment error: status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// RequestCloudReviewers updates the list of requested reviewers on a pull request.
func (s *BitbucketAdvancedService) RequestCloudReviewers(
	ctx context.Context,
	token, workspace, repoSlug string,
	prID int,
	reviewerUUIDs []string,
) error {
	endpoint := fmt.Sprintf("%s/repositories/%s/%s/pullrequests/%d",
		s.baseURL, workspace, repoSlug, prID)

	reviewers := make([]map[string]string, len(reviewerUUIDs))
	for i, u := range reviewerUUIDs {
		reviewers[i] = map[string]string{"uuid": u}
	}

	payload := map[string]any{
		"reviewers": reviewers,
	}
	bodyBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("create request reviewers request: %w", err)
	}
	s.applyCloudAuth(req, token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute request reviewers request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("bitbucket cloud request reviewers error: status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// CheckCloudMergeability verifies if a PR has open conflicts or pending status blockers.
func (s *BitbucketAdvancedService) CheckCloudMergeability(
	ctx context.Context,
	token, workspace, repoSlug string,
	prID int,
) (*CloudMergeabilityStatus, error) {
	endpoint := fmt.Sprintf("%s/repositories/%s/%s/pullrequests/%d",
		s.baseURL, workspace, repoSlug, prID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create check mergeability request: %w", err)
	}
	s.applyCloudAuth(req, token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute check mergeability request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("bitbucket cloud check mergeability error: status %d: %s", resp.StatusCode, string(b))
	}

	var pr struct {
		State     string `json:"state"` // OPEN, MERGED, DECLINED
		Summary   string `json:"summary"`
		CloseFlag bool   `json:"close_source_branch"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, fmt.Errorf("decode mergeability response: %w", err)
	}

	if pr.State != "OPEN" {
		return &CloudMergeabilityStatus{
			CanMerge: false,
			Reasons:  []string{fmt.Sprintf("pull request is in %s state", pr.State)},
		}, nil
	}

	return &CloudMergeabilityStatus{
		CanMerge: true,
	}, nil
}

// MergeCloudPullRequest executes the merge of a pull request in Bitbucket Cloud.
func (s *BitbucketAdvancedService) MergeCloudPullRequest(
	ctx context.Context,
	token, workspace, repoSlug string,
	prID int,
	mergeReq CloudMergeRequest,
) (*CloudMergeResponse, error) {
	endpoint := fmt.Sprintf("%s/repositories/%s/%s/pullrequests/%d/merge",
		s.baseURL, workspace, repoSlug, prID)

	bodyBytes, err := json.Marshal(mergeReq)
	if err != nil {
		return nil, fmt.Errorf("marshal merge request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create merge pull request request: %w", err)
	}
	s.applyCloudAuth(req, token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute merge pull request request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("bitbucket cloud merge pull request error: status %d: %s", resp.StatusCode, string(b))
	}

	var mergeResp struct {
		State              string `json:"state"`
		ClosedSourceBranch bool   `json:"close_source_branch"`
		MergeCommit        struct {
			Hash string `json:"hash"`
		} `json:"merge_commit"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&mergeResp); err != nil {
		return nil, fmt.Errorf("decode merge response: %w", err)
	}

	return &CloudMergeResponse{
		Hash:               mergeResp.MergeCommit.Hash,
		State:              mergeResp.State,
		ClosedSourceBranch: mergeResp.ClosedSourceBranch,
	}, nil
}

func (s *BitbucketAdvancedService) applyCloudAuth(req *http.Request, token string) {
	if token != "" {
		if strings.HasPrefix(token, "Bearer ") || strings.HasPrefix(token, "token ") {
			req.Header.Set("Authorization", token)
		} else {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}
}
