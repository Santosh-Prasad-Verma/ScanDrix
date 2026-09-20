// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package bitbucket

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// BitbucketAdvancedService provides extended capabilities for Bitbucket Cloud (REST 2.0)
// and Bitbucket Server/Data Center (REST 1.0) APIs, including activities, comment tasks,
// branch restrictions, and diff anchors.
type BitbucketAdvancedService struct {
	httpClient *http.Client
	baseURL    string
	isServer   bool
}

// NewBitbucketAdvancedService creates an instance of BitbucketAdvancedService.
func NewBitbucketAdvancedService(httpClient *http.Client, baseURL ...string) *BitbucketAdvancedService {
	url := "https://api.bitbucket.org/2.0"
	if len(baseURL) > 0 && baseURL[0] != "" {
		url = strings.TrimRight(baseURL[0], "/")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &BitbucketAdvancedService{
		httpClient: httpClient,
		baseURL:    url,
		isServer:   false,
	}
}

// SetIsServer toggles Bitbucket Server / Data Center REST 1.0 mode.
func (s *BitbucketAdvancedService) SetIsServer(isServer bool) *BitbucketAdvancedService {
	s.isServer = isServer
	return s
}

// PullRequestActivity models an activity log entry on a Bitbucket pull request.
type PullRequestActivity struct {
	Action      string    `json:"action"` // "approval", "comment", "update", "merge"
	User        string    `json:"user"`
	CreatedOn   time.Time `json:"created_on"`
	CommentBody string    `json:"comment_body,omitempty"`
	CommentID   int64     `json:"comment_id,omitempty"`
}

// CommentTask models a task associated with an inline PR comment.
type CommentTask struct {
	ID        int64     `json:"id"`
	CommentID int64     `json:"comment_id"`
	Content   string    `json:"content"`
	State     string    `json:"state"` // "UNRESOLVED", "RESOLVED"
	CreatedOn time.Time `json:"created_on"`
	Creator   string    `json:"creator"`
}

// BranchRestriction models branch protection rules in Bitbucket Cloud/Server.
type BranchRestriction struct {
	ID    int64    `json:"id"`
	Kind  string   `json:"kind"` // "push", "force", "delete", "require_approvals_to_merge"
	Value *int     `json:"value,omitempty"`
	Users []string `json:"users,omitempty"`
}

// DiffAnchor models line coordinate matching for Bitbucket inline comments.
type DiffAnchor struct {
	Path     string `json:"path"`
	LineFrom int    `json:"line_from,omitempty"`
	LineTo   int    `json:"line_to,omitempty"`
	FileType string `json:"file_type,omitempty"` // "FROM", "TO"
}

// GetPullRequestActivities retrieves chronological activity items for a pull request.
func (s *BitbucketAdvancedService) GetPullRequestActivities(
	ctx context.Context,
	token, workspace, repoSlug string,
	prID int,
) ([]PullRequestActivity, error) {
	var endpoint string
	if s.isServer {
		endpoint = fmt.Sprintf("%s/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d/activities",
			s.baseURL, workspace, repoSlug, prID)
	} else {
		endpoint = fmt.Sprintf("%s/repositories/%s/%s/pullrequests/%d/activity",
			s.baseURL, workspace, repoSlug, prID)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if strings.HasPrefix(token, "Bearer ") {
		req.Header.Set("Authorization", token)
	} else {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("bitbucket api error: status %d: %s", resp.StatusCode, string(b))
	}

	var raw struct {
		Values []map[string]any `json:"values"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode activities: %w", err)
	}

	activities := make([]PullRequestActivity, 0, len(raw.Values))
	for _, val := range raw.Values {
		act := PullRequestActivity{}
		if approval, ok := val["approval"].(map[string]any); ok {
			act.Action = "approval"
			if user, ok := approval["user"].(map[string]any); ok {
				act.User, _ = user["display_name"].(string)
			}
			if dateStr, ok := approval["date"].(string); ok {
				act.CreatedOn, _ = time.Parse(time.RFC3339, dateStr)
			}
		} else if comment, ok := val["comment"].(map[string]any); ok {
			act.Action = "comment"
			if content, ok := comment["content"].(map[string]any); ok {
				act.CommentBody, _ = content["raw"].(string)
			}
			if id, ok := comment["id"].(float64); ok {
				act.CommentID = int64(id)
			}
			if user, ok := comment["user"].(map[string]any); ok {
				act.User, _ = user["display_name"].(string)
			}
			if dateStr, ok := comment["created_on"].(string); ok {
				act.CreatedOn, _ = time.Parse(time.RFC3339, dateStr)
			}
		} else if update, ok := val["update"].(map[string]any); ok {
			act.Action = "update"
			if author, ok := update["author"].(map[string]any); ok {
				act.User, _ = author["display_name"].(string)
			}
			if dateStr, ok := update["date"].(string); ok {
				act.CreatedOn, _ = time.Parse(time.RFC3339, dateStr)
			}
		}
		activities = append(activities, act)
	}
	return activities, nil
}

// CreateCommentTask attaches an actionable task to a comment.
func (s *BitbucketAdvancedService) CreateCommentTask(
	ctx context.Context,
	token, workspace, repoSlug string,
	prID int,
	commentID int64,
	content string,
) (*CommentTask, error) {
	endpoint := fmt.Sprintf("%s/repositories/%s/%s/pullrequests/%d/comments/%d/tasks",
		s.baseURL, workspace, repoSlug, prID, commentID)

	payload := map[string]any{
		"content": map[string]string{"raw": content},
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimPrefix(token, "Bearer "))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("bitbucket api error: status %d: %s", resp.StatusCode, string(b))
	}

	var res struct {
		ID      int64  `json:"id"`
		State   string `json:"state"`
		Content struct {
			Raw string `json:"raw"`
		} `json:"content"`
		CreatedOn time.Time `json:"created_on"`
		Creator   struct {
			DisplayName string `json:"display_name"`
		} `json:"creator"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("decode task response: %w", err)
	}

	return &CommentTask{
		ID:        res.ID,
		CommentID: commentID,
		Content:   res.Content.Raw,
		State:     res.State,
		CreatedOn: res.CreatedOn,
		Creator:   res.Creator.DisplayName,
	}, nil
}

// GetBranchRestrictions retrieves branch protection restrictions for a repository.
func (s *BitbucketAdvancedService) GetBranchRestrictions(
	ctx context.Context,
	token, workspace, repoSlug string,
) ([]BranchRestriction, error) {
	endpoint := fmt.Sprintf("%s/repositories/%s/%s/branch-restrictions",
		s.baseURL, workspace, repoSlug)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimPrefix(token, "Bearer "))
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("bitbucket api error: status %d: %s", resp.StatusCode, string(b))
	}

	var raw struct {
		Values []struct {
			ID    int64  `json:"id"`
			Kind  string `json:"kind"`
			Value *int   `json:"value"`
			Users []struct {
				DisplayName string `json:"display_name"`
			} `json:"users"`
		} `json:"values"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode branch restrictions: %w", err)
	}

	restrictions := make([]BranchRestriction, len(raw.Values))
	for i, v := range raw.Values {
		users := make([]string, len(v.Users))
		for j, u := range v.Users {
			users[j] = u.DisplayName
		}
		restrictions[i] = BranchRestriction{
			ID:    v.ID,
			Kind:  v.Kind,
			Value: v.Value,
			Users: users,
		}
	}
	return restrictions, nil
}
