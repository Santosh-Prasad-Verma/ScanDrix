// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package forgejo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ForgejoAdvancedService provides extended capabilities for Gitea / Forgejo REST API v1,
// including PR reviews, commit statuses, branch protections, team permissions, and webhook delivery retries.
type ForgejoAdvancedService struct {
	httpClient *http.Client
	baseURL    string
}

// NewForgejoAdvancedService creates an instance of ForgejoAdvancedService.
func NewForgejoAdvancedService(httpClient *http.Client, baseURL ...string) *ForgejoAdvancedService {
	url := "https://codeberg.org/api/v1"
	if len(baseURL) > 0 && baseURL[0] != "" {
		url = strings.TrimRight(baseURL[0], "/")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &ForgejoAdvancedService{
		httpClient: httpClient,
		baseURL:    url,
	}
}

// PullReviewSubmission models submitting a comprehensive code review in Forgejo/Gitea.
type PullReviewSubmission struct {
	Event    string               `json:"event"` // "APPROVED", "REQUEST_CHANGES", "COMMENT"
	Body     string               `json:"body"`
	Comments []ReviewCommentDraft `json:"comments,omitempty"`
	CommitID string               `json:"commit_id,omitempty"`
}

// ReviewCommentDraft models an inline comment in a review submission.
type ReviewCommentDraft struct {
	Path     string `json:"path"`
	NewLine  int    `json:"new_position,omitempty"`
	OldLine  int    `json:"old_position,omitempty"`
	Body     string `json:"body"`
}

// PullReviewResult models the created pull review.
type PullReviewResult struct {
	ID           int64     `json:"id"`
	State        string    `json:"state"`
	Body         string    `json:"body"`
	Reviewer     string    `json:"reviewer"`
	SubmittedAt  time.Time `json:"submitted_at"`
	HTMLURL      string    `json:"html_url"`
}

// CommitStatusSubmission models a commit status update.
type CommitStatusSubmission struct {
	State       string `json:"state"` // "pending", "success", "error", "failure", "warning"
	TargetURL   string `json:"target_url,omitempty"`
	Description string `json:"description,omitempty"`
	Context     string `json:"context,omitempty"`
}

// BranchProtection models Forgejo branch protection settings.
type BranchProtection struct {
	BranchName            string   `json:"branch_name"`
	EnablePush            bool     `json:"enable_push"`
	EnablePushWhitelist   bool     `json:"enable_push_whitelist"`
	PushWhitelistUsernames []string `json:"push_whitelist_usernames"`
	EnableMergeWhitelist  bool     `json:"enable_merge_whitelist"`
	RequiredApprovals     int64    `json:"required_approvals"`
	BlockOnRejectedReviews bool    `json:"block_on_rejected_reviews"`
}

// SubmitPullReview posts a review with multiple inline comments and verdict to Forgejo.
func (s *ForgejoAdvancedService) SubmitPullReview(
	ctx context.Context,
	token, owner, repo string,
	index int64,
	review PullReviewSubmission,
) (*PullReviewResult, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/reviews",
		s.baseURL, owner, repo, index)

	bodyBytes, err := json.Marshal(review)
	if err != nil {
		return nil, fmt.Errorf("marshal review: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "token "+strings.TrimPrefix(token, "token "))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("forgejo api error: status %d: %s", resp.StatusCode, string(b))
	}

	var res struct {
		ID          int64     `json:"id"`
		State       string    `json:"state"`
		Body        string    `json:"body"`
		SubmittedAt time.Time `json:"submitted_at"`
		HTMLURL     string    `json:"html_url"`
		User        struct {
			Username string `json:"username"`
		} `json:"user"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("decode review response: %w", err)
	}

	return &PullReviewResult{
		ID:          res.ID,
		State:       res.State,
		Body:        res.Body,
		Reviewer:    res.User.Username,
		SubmittedAt: res.SubmittedAt,
		HTMLURL:     res.HTMLURL,
	}, nil
}

// CreateCommitStatus creates or updates a commit status check on a commit SHA.
func (s *ForgejoAdvancedService) CreateCommitStatus(
	ctx context.Context,
	token, owner, repo, sha string,
	status CommitStatusSubmission,
) error {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/statuses/%s",
		s.baseURL, owner, repo, sha)

	bodyBytes, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("marshal status: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "token "+strings.TrimPrefix(token, "token "))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("forgejo api error: status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// GetBranchProtection retrieves branch protection settings for a specific branch.
func (s *ForgejoAdvancedService) GetBranchProtection(
	ctx context.Context,
	token, owner, repo, branch string,
) (*BranchProtection, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/branch_protections/%s",
		s.baseURL, owner, repo, branch)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "token "+strings.TrimPrefix(token, "token "))
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("forgejo api error: status %d: %s", resp.StatusCode, string(b))
	}

	var bp BranchProtection
	if err := json.NewDecoder(resp.Body).Decode(&bp); err != nil {
		return nil, fmt.Errorf("decode branch protection: %w", err)
	}
	return &bp, nil
}
