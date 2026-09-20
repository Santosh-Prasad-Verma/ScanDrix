// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

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

// ServerUser represents a Bitbucket Server user account.
type ServerUser struct {
	Name         string `json:"name"`
	EmailAddress string `json:"emailAddress,omitempty"`
	ID           int    `json:"id,omitempty"`
	DisplayName  string `json:"displayName,omitempty"`
	Active       bool   `json:"active,omitempty"`
	Slug         string `json:"slug,omitempty"`
	Type         string `json:"type,omitempty"`
}

// ServerCommentTask represents a task attached to a comment.
type ServerCommentTask struct {
	ID     int64       `json:"id"`
	Text   string      `json:"text"`
	State  string      `json:"state"` // "OPEN", "RESOLVED"
	Author *ServerUser `json:"author,omitempty"`
}

// ServerComment represents a Bitbucket Server pull request comment thread.
type ServerComment struct {
	ID          int64               `json:"id"`
	Version     int                 `json:"version"`
	Text        string              `json:"text"`
	Author      *ServerUser         `json:"author,omitempty"`
	CreatedDate int64               `json:"createdDate,omitempty"`
	UpdatedDate int64               `json:"updatedDate,omitempty"`
	Comments    []ServerComment     `json:"comments,omitempty"`
	Tasks       []ServerCommentTask `json:"tasks,omitempty"`
	Severity    string              `json:"severity,omitempty"`
	State       string              `json:"state,omitempty"` // "OPEN", "RESOLVED"
}

// ServerPRActivity represents an activity event on a Bitbucket Server PR.
type ServerPRActivity struct {
	ID            int64          `json:"id"`
	CreatedDate   int64          `json:"createdDate"`
	User          *ServerUser    `json:"user,omitempty"`
	Action        string         `json:"action"` // "COMMENTED", "APPROVED", "REVIEWED", "RESCOPED", "MERGED", "DECLINED", "REOPENED"
	CommentAction string         `json:"commentAction,omitempty"`
	Comment       *ServerComment `json:"comment,omitempty"`
}

// ServerMergeVeto explains why a pull request cannot be merged.
type ServerMergeVeto struct {
	SummaryMessage  string `json:"summaryMessage"`
	DetailedMessage string `json:"detailedMessage"`
}

// ServerMergeStatus represents whether a pull request can be merged.
type ServerMergeStatus struct {
	CanMerge   bool              `json:"canMerge"`
	Conflicted bool              `json:"conflicted"`
	Outcome    string            `json:"outcome"` // "CLEAN", "CONFLICTED"
	Vetoes     []ServerMergeVeto `json:"vetoes,omitempty"`
}

// ServerAuthor represents commit author details.
type ServerAuthor struct {
	Name         string `json:"name"`
	EmailAddress string `json:"emailAddress"`
}

// ServerCommitRef represents commit reference.
type ServerCommitRef struct {
	ID        string `json:"id"`
	DisplayID string `json:"displayId"`
}

// ServerCommit represents a commit in Bitbucket Server.
type ServerCommit struct {
	ID              string            `json:"id"`
	DisplayID       string            `json:"displayId"`
	Author          *ServerAuthor     `json:"author,omitempty"`
	AuthorTimestamp int64             `json:"authorTimestamp,omitempty"`
	Committer       *ServerAuthor     `json:"committer,omitempty"`
	Message         string            `json:"message"`
	Parents         []ServerCommitRef `json:"parents,omitempty"`
}

// BitbucketServerActivitiesService provides methods for Bitbucket Server/DC activities, diffs, and merges.
type BitbucketServerActivitiesService struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewBitbucketServerActivitiesService creates a new instance of BitbucketServerActivitiesService.
func NewBitbucketServerActivitiesService(baseURL, token string, client *http.Client) *BitbucketServerActivitiesService {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &BitbucketServerActivitiesService{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		httpClient: client,
	}
}

func (s *BitbucketServerActivitiesService) applyAuth(req *http.Request) {
	if strings.HasPrefix(s.token, "Bearer ") {
		req.Header.Set("Authorization", s.token)
	} else if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
}

// ListPullRequestActivities lists activity history on a pull request.
func (s *BitbucketServerActivitiesService) ListPullRequestActivities(ctx context.Context, projectKey, repoSlug string, prID int, limit, start int) ([]ServerPRActivity, error) {
	endpoint := fmt.Sprintf("%s/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d/activities?limit=%d&start=%d",
		s.baseURL, url.PathEscape(projectKey), url.PathEscape(repoSlug), prID, limit, start)

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
		return nil, fmt.Errorf("list activities failed (status %d): %s", resp.StatusCode, string(body))
	}

	var data struct {
		Values []ServerPRActivity `json:"values"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return data.Values, nil
}

// ListPullRequestCommits lists commits belonging to a pull request.
func (s *BitbucketServerActivitiesService) ListPullRequestCommits(ctx context.Context, projectKey, repoSlug string, prID int, limit, start int) ([]ServerCommit, error) {
	endpoint := fmt.Sprintf("%s/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d/commits?limit=%d&start=%d",
		s.baseURL, url.PathEscape(projectKey), url.PathEscape(repoSlug), prID, limit, start)

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
		return nil, fmt.Errorf("list pr commits failed (status %d): %s", resp.StatusCode, string(body))
	}

	var data struct {
		Values []ServerCommit `json:"values"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return data.Values, nil
}

// GetPullRequestDiff retrieves the unified raw diff of a pull request.
func (s *BitbucketServerActivitiesService) GetPullRequestDiff(ctx context.Context, projectKey, repoSlug string, prID int, contextLines int) (string, error) {
	endpoint := fmt.Sprintf("%s/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d.diff",
		s.baseURL, url.PathEscape(projectKey), url.PathEscape(repoSlug), prID)
	if contextLines > 0 {
		endpoint += fmt.Sprintf("?contextLines=%d", contextLines)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	s.applyAuth(req)
	req.Header.Set("Accept", "text/plain")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("get pr diff failed (status %d): %s", resp.StatusCode, string(body))
	}

	diffBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(diffBytes), nil
}

// GetMergeStatus evaluates whether a pull request can be merged cleanly.
func (s *BitbucketServerActivitiesService) GetMergeStatus(ctx context.Context, projectKey, repoSlug string, prID int) (*ServerMergeStatus, error) {
	endpoint := fmt.Sprintf("%s/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d/merge",
		s.baseURL, url.PathEscape(projectKey), url.PathEscape(repoSlug), prID)

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
		return nil, fmt.Errorf("get merge status failed (status %d): %s", resp.StatusCode, string(body))
	}

	var status ServerMergeStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, err
	}
	return &status, nil
}

// MergePullRequest executes the merge of a pull request.
func (s *BitbucketServerActivitiesService) MergePullRequest(ctx context.Context, projectKey, repoSlug string, prID int, version int, message string) (*ServerPRActivity, error) {
	endpoint := fmt.Sprintf("%s/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d/merge?version=%d",
		s.baseURL, url.PathEscape(projectKey), url.PathEscape(repoSlug), prID, version)

	payload := map[string]string{}
	if message != "" {
		payload["message"] = message
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	s.applyAuth(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("merge pull request failed (status %d): %s", resp.StatusCode, string(body))
	}

	var activity ServerPRActivity
	if err := json.NewDecoder(resp.Body).Decode(&activity); err != nil {
		return nil, err
	}
	return &activity, nil
}

// DeclinePullRequest declines a pull request.
func (s *BitbucketServerActivitiesService) DeclinePullRequest(ctx context.Context, projectKey, repoSlug string, prID int, version int) (*ServerPRActivity, error) {
	endpoint := fmt.Sprintf("%s/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d/decline?version=%d",
		s.baseURL, url.PathEscape(projectKey), url.PathEscape(repoSlug), prID, version)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
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
		return nil, fmt.Errorf("decline pull request failed (status %d): %s", resp.StatusCode, string(body))
	}

	var activity ServerPRActivity
	if err := json.NewDecoder(resp.Body).Decode(&activity); err != nil {
		return nil, err
	}
	return &activity, nil
}

// ReopenPullRequest reopens a declined pull request.
func (s *BitbucketServerActivitiesService) ReopenPullRequest(ctx context.Context, projectKey, repoSlug string, prID int, version int) (*ServerPRActivity, error) {
	endpoint := fmt.Sprintf("%s/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d/reopen?version=%d",
		s.baseURL, url.PathEscape(projectKey), url.PathEscape(repoSlug), prID, version)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
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
		return nil, fmt.Errorf("reopen pull request failed (status %d): %s", resp.StatusCode, string(body))
	}

	var activity ServerPRActivity
	if err := json.NewDecoder(resp.Body).Decode(&activity); err != nil {
		return nil, err
	}
	return &activity, nil
}

// AddPullRequestComment adds a comment or reply to a pull request.
func (s *BitbucketServerActivitiesService) AddPullRequestComment(ctx context.Context, projectKey, repoSlug string, prID int, text string, parentCommentID *int64) (*ServerComment, error) {
	endpoint := fmt.Sprintf("%s/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d/comments",
		s.baseURL, url.PathEscape(projectKey), url.PathEscape(repoSlug), prID)

	payload := map[string]any{
		"text": text,
	}
	if parentCommentID != nil {
		payload["parent"] = map[string]int64{"id": *parentCommentID}
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	s.applyAuth(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("add comment failed (status %d): %s", resp.StatusCode, string(body))
	}

	var comment ServerComment
	if err := json.NewDecoder(resp.Body).Decode(&comment); err != nil {
		return nil, err
	}
	return &comment, nil
}

// ResolveComment resolves a comment thread on a pull request.
func (s *BitbucketServerActivitiesService) ResolveComment(ctx context.Context, projectKey, repoSlug string, prID int, commentID int64, version int) (*ServerComment, error) {
	endpoint := fmt.Sprintf("%s/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d/comments/%d?version=%d",
		s.baseURL, url.PathEscape(projectKey), url.PathEscape(repoSlug), prID, commentID, version)

	payload := map[string]string{
		"state": "RESOLVED",
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(bodyBytes))
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
		return nil, fmt.Errorf("resolve comment failed (status %d): %s", resp.StatusCode, string(body))
	}

	var comment ServerComment
	if err := json.NewDecoder(resp.Body).Decode(&comment); err != nil {
		return nil, err
	}
	return &comment, nil
}
