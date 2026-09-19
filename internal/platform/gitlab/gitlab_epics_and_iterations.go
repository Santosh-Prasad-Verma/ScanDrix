// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

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
)

// GitLabEpic represents a group-level epic in GitLab.
type GitLabEpic struct {
	ID          int64      `json:"id"`
	IID         int64      `json:"iid"`
	GroupID     int64      `json:"group_id"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	State       string     `json:"state"` // "opened", "closed"
	WebURL      string     `json:"web_url"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ClosedAt    *time.Time `json:"closed_at,omitempty"`
	StartDate   *string    `json:"start_date,omitempty"`
	DueDate     *string    `json:"due_date,omitempty"`
}

// CreateGitLabEpicRequest holds fields to create an epic.
type CreateGitLabEpicRequest struct {
	Title       string  `json:"title"`
	Description string  `json:"description,omitempty"`
	StartDate   *string `json:"start_date,omitempty"`
	DueDate     *string `json:"due_date,omitempty"`
}

// UpdateGitLabEpicRequest holds fields to update an epic.
type UpdateGitLabEpicRequest struct {
	Title       string  `json:"title,omitempty"`
	Description string  `json:"description,omitempty"`
	StateEvent  string  `json:"state_event,omitempty"` // "reopen", "close"
	StartDate   *string `json:"start_date,omitempty"`
	DueDate     *string `json:"due_date,omitempty"`
}

// GitLabIteration represents an iteration/sprint cadence in GitLab.
type GitLabIteration struct {
	ID          int64     `json:"id"`
	IID         int64     `json:"iid"`
	Sequence    int       `json:"sequence"`
	Title       string    `json:"title,omitempty"`
	Description string    `json:"description,omitempty"`
	State       int       `json:"state"` // 1=upcoming, 2=current, 3=closed
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	StartDate   string    `json:"start_date"`
	DueDate     string    `json:"due_date"`
	WebURL      string    `json:"web_url"`
}

// GitLabMilestone represents a project or group milestone.
type GitLabMilestone struct {
	ID          int64      `json:"id"`
	IID         int64      `json:"iid"`
	ProjectID   int64      `json:"project_id,omitempty"`
	GroupID     int64      `json:"group_id,omitempty"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	State       string     `json:"state"` // "active", "closed"
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	DueDate     *string    `json:"due_date,omitempty"`
	StartDate   *string    `json:"start_date,omitempty"`
	WebURL      string     `json:"web_url"`
}

// CreateGitLabMilestoneRequest holds parameters to create a milestone.
type CreateGitLabMilestoneRequest struct {
	Title       string  `json:"title"`
	Description string  `json:"description,omitempty"`
	StartDate   *string `json:"start_date,omitempty"`
	DueDate     *string `json:"due_date,omitempty"`
}

// UpdateGitLabMilestoneRequest holds parameters to update a milestone.
type UpdateGitLabMilestoneRequest struct {
	Title       string  `json:"title,omitempty"`
	Description string  `json:"description,omitempty"`
	StateEvent  string  `json:"state_event,omitempty"` // "activate", "close"
	StartDate   *string `json:"start_date,omitempty"`
	DueDate     *string `json:"due_date,omitempty"`
}

// GitLabCommitComment represents an inline or general comment on a commit.
type GitLabCommitComment struct {
	Note     string `json:"note"`
	Path     string `json:"path,omitempty"`
	Line     int    `json:"line,omitempty"`
	LineType string `json:"line_type,omitempty"` // "new", "old"
	Author   struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
		Name     string `json:"name"`
	} `json:"author"`
	CreatedAt time.Time `json:"created_at"`
}

// GitLabEpicsAndIterationsService provides GitLab API client for epics, iterations, milestones, and commit notes.
type GitLabEpicsAndIterationsService struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewGitLabEpicsAndIterationsService creates a new instance of GitLabEpicsAndIterationsService.
func NewGitLabEpicsAndIterationsService(baseURL, token string, client *http.Client) *GitLabEpicsAndIterationsService {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &GitLabEpicsAndIterationsService{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		httpClient: client,
	}
}

func (s *GitLabEpicsAndIterationsService) applyAuth(req *http.Request) {
	if strings.HasPrefix(s.token, "Bearer ") {
		req.Header.Set("Authorization", s.token)
	} else if s.token != "" {
		req.Header.Set("PRIVATE-TOKEN", s.token)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
}

// ListGroupEpics lists epics for a group.
func (s *GitLabEpicsAndIterationsService) ListGroupEpics(ctx context.Context, groupID string, state string) ([]GitLabEpic, error) {
	endpoint := fmt.Sprintf("%s/api/v4/groups/%s/epics", s.baseURL, url.PathEscape(groupID))
	if state != "" {
		endpoint += "?state=" + url.QueryEscape(state)
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
		return nil, fmt.Errorf("list group epics failed (status %d): %s", resp.StatusCode, string(body))
	}

	var epics []GitLabEpic
	if err := json.NewDecoder(resp.Body).Decode(&epics); err != nil {
		return nil, err
	}
	return epics, nil
}

// GetGroupEpic retrieves an epic by IID.
func (s *GitLabEpicsAndIterationsService) GetGroupEpic(ctx context.Context, groupID string, epicIID int64) (*GitLabEpic, error) {
	endpoint := fmt.Sprintf("%s/api/v4/groups/%s/epics/%d", s.baseURL, url.PathEscape(groupID), epicIID)

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
		return nil, fmt.Errorf("get group epic failed (status %d): %s", resp.StatusCode, string(body))
	}

	var epic GitLabEpic
	if err := json.NewDecoder(resp.Body).Decode(&epic); err != nil {
		return nil, err
	}
	return &epic, nil
}

// CreateGroupEpic creates an epic in a group.
func (s *GitLabEpicsAndIterationsService) CreateGroupEpic(ctx context.Context, groupID string, req CreateGitLabEpicRequest) (*GitLabEpic, error) {
	endpoint := fmt.Sprintf("%s/api/v4/groups/%s/epics", s.baseURL, url.PathEscape(groupID))

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
		return nil, fmt.Errorf("create group epic failed (status %d): %s", resp.StatusCode, string(body))
	}

	var epic GitLabEpic
	if err := json.NewDecoder(resp.Body).Decode(&epic); err != nil {
		return nil, err
	}
	return &epic, nil
}

// UpdateGroupEpic updates an existing epic.
func (s *GitLabEpicsAndIterationsService) UpdateGroupEpic(ctx context.Context, groupID string, epicIID int64, req UpdateGitLabEpicRequest) (*GitLabEpic, error) {
	endpoint := fmt.Sprintf("%s/api/v4/groups/%s/epics/%d", s.baseURL, url.PathEscape(groupID), epicIID)

	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(bodyBytes))
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
		return nil, fmt.Errorf("update group epic failed (status %d): %s", resp.StatusCode, string(body))
	}

	var epic GitLabEpic
	if err := json.NewDecoder(resp.Body).Decode(&epic); err != nil {
		return nil, err
	}
	return &epic, nil
}

// DeleteGroupEpic deletes an epic.
func (s *GitLabEpicsAndIterationsService) DeleteGroupEpic(ctx context.Context, groupID string, epicIID int64) error {
	endpoint := fmt.Sprintf("%s/api/v4/groups/%s/epics/%d", s.baseURL, url.PathEscape(groupID), epicIID)

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
		return fmt.Errorf("delete group epic failed (status %d): %s", resp.StatusCode, string(body))
	}
	return nil
}

// ListGroupIterations lists iterations for a group.
func (s *GitLabEpicsAndIterationsService) ListGroupIterations(ctx context.Context, groupID string, state string) ([]GitLabIteration, error) {
	endpoint := fmt.Sprintf("%s/api/v4/groups/%s/iterations", s.baseURL, url.PathEscape(groupID))
	if state != "" {
		endpoint += "?state=" + url.QueryEscape(state)
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
		return nil, fmt.Errorf("list group iterations failed (status %d): %s", resp.StatusCode, string(body))
	}

	var iterations []GitLabIteration
	if err := json.NewDecoder(resp.Body).Decode(&iterations); err != nil {
		return nil, err
	}
	return iterations, nil
}

// ListProjectMilestones lists milestones for a project.
func (s *GitLabEpicsAndIterationsService) ListProjectMilestones(ctx context.Context, projectID string, state string) ([]GitLabMilestone, error) {
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/milestones", s.baseURL, url.PathEscape(projectID))
	if state != "" {
		endpoint += "?state=" + url.QueryEscape(state)
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
		return nil, fmt.Errorf("list project milestones failed (status %d): %s", resp.StatusCode, string(body))
	}

	var milestones []GitLabMilestone
	if err := json.NewDecoder(resp.Body).Decode(&milestones); err != nil {
		return nil, err
	}
	return milestones, nil
}

// CreateProjectMilestone creates a milestone in a project.
func (s *GitLabEpicsAndIterationsService) CreateProjectMilestone(ctx context.Context, projectID string, req CreateGitLabMilestoneRequest) (*GitLabMilestone, error) {
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/milestones", s.baseURL, url.PathEscape(projectID))

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
		return nil, fmt.Errorf("create project milestone failed (status %d): %s", resp.StatusCode, string(body))
	}

	var milestone GitLabMilestone
	if err := json.NewDecoder(resp.Body).Decode(&milestone); err != nil {
		return nil, err
	}
	return &milestone, nil
}

// CreateCommitComment posts an inline or general comment on a commit.
func (s *GitLabEpicsAndIterationsService) CreateCommitComment(ctx context.Context, projectID, commitSHA, note, path string, line int, lineType string) (*GitLabCommitComment, error) {
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/repository/commits/%s/comments",
		s.baseURL, url.PathEscape(projectID), url.PathEscape(commitSHA))

	payload := map[string]any{
		"note": note,
	}
	if path != "" {
		payload["path"] = path
	}
	if line > 0 {
		payload["line"] = line
	}
	if lineType != "" {
		payload["line_type"] = lineType
	}

	bodyBytes, err := json.Marshal(payload)
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
		return nil, fmt.Errorf("create commit comment failed (status %d): %s", resp.StatusCode, string(body))
	}

	var comment GitLabCommitComment
	if err := json.NewDecoder(resp.Body).Decode(&comment); err != nil {
		return nil, err
	}
	return &comment, nil
}
