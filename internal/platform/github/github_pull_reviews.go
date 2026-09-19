package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// -------------------------------------------------------------------------------------
// GitHub Pull Request Review Models
// -------------------------------------------------------------------------------------

// PullRequestReviewItem represents a review on a GitHub pull request.
type PullRequestReviewItem struct {
	ID             int64     `json:"id"`
	User           string    `json:"user"`
	Body           string    `json:"body"`
	State          string    `json:"state"` // APPROVED, CHANGES_REQUESTED, COMMENTED, DISMISSED, PENDING
	HTMLURL        string    `json:"html_url"`
	PullRequestURL string    `json:"pull_request_url"`
	CommitID       string    `json:"commit_id"`
	SubmittedAt    time.Time `json:"submitted_at"`
}

// ReviewCommentItem represents an inline review comment.
type ReviewCommentItem struct {
	ID             int64     `json:"id"`
	ReviewID       int64     `json:"pull_request_review_id"`
	Path           string    `json:"path"`
	Line           int       `json:"line"`
	StartLine      int       `json:"start_line,omitempty"`
	Side           string    `json:"side"`
	Body           string    `json:"body"`
	CommitID       string    `json:"commit_id"`
	User           string    `json:"user"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	HTMLURL        string    `json:"html_url"`
	InReplyToID    int64     `json:"in_reply_to_id,omitempty"`
}

// RequestedReviewersResult models users and teams requested for review.
type RequestedReviewersResult struct {
	Users []string `json:"users"`
	Teams []string `json:"teams"`
}

// MergeabilityStatus models whether a PR can be merged without conflicts.
type MergeabilityStatus struct {
	Mergeable     *bool  `json:"mergeable"`
	MergeableState string `json:"mergeable_state"` // clean, dirty, blocked, unstable, draft, behind
	Rebaseable    *bool  `json:"rebaseable"`
}

// MergePullRequestInput defines parameters for merging a PR.
type MergePullRequestInput struct {
	CommitTitle   string `json:"commit_title,omitempty"`
	CommitMessage string `json:"commit_message,omitempty"`
	SHA           string `json:"sha,omitempty"`
	MergeMethod   string `json:"merge_method,omitempty"` // merge, squash, rebase
}

// MergePullRequestOutput models the result of merging a PR.
type MergePullRequestOutput struct {
	SHA     string `json:"sha"`
	Merged  bool   `json:"merged"`
	Message string `json:"message"`
}

// -------------------------------------------------------------------------------------
// GitHub Pull Request Review API Methods
// -------------------------------------------------------------------------------------

// ListPullRequestReviews returns all reviews submitted on a pull request.
func (s *GitHubAdvancedService) ListPullRequestReviews(
	ctx context.Context,
	token, owner, repo string,
	prNumber int,
) ([]PullRequestReviewItem, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/reviews",
		s.baseURL, owner, repo, prNumber)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create list pr reviews request: %w", err)
	}
	s.setHeaders(req, token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute list pr reviews request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list pr reviews error: status %d: %s", resp.StatusCode, string(b))
	}

	var raw []struct {
		ID             int64     `json:"id"`
		Body           string    `json:"body"`
		State          string    `json:"state"`
		HTMLURL        string    `json:"html_url"`
		PullRequestURL string    `json:"pull_request_url"`
		CommitID       string    `json:"commit_id"`
		SubmittedAt    time.Time `json:"submitted_at"`
		User           struct {
			Login string `json:"login"`
		} `json:"user"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode pr reviews response: %w", err)
	}

	reviews := make([]PullRequestReviewItem, len(raw))
	for i, r := range raw {
		reviews[i] = PullRequestReviewItem{
			ID:             r.ID,
			User:           r.User.Login,
			Body:           r.Body,
			State:          r.State,
			HTMLURL:        r.HTMLURL,
			PullRequestURL: r.PullRequestURL,
			CommitID:       r.CommitID,
			SubmittedAt:    r.SubmittedAt,
		}
	}
	return reviews, nil
}

// GetPullRequestReview retrieves a specific review by its ID.
func (s *GitHubAdvancedService) GetPullRequestReview(
	ctx context.Context,
	token, owner, repo string,
	prNumber, reviewID int,
) (*PullRequestReviewItem, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/reviews/%d",
		s.baseURL, owner, repo, prNumber, reviewID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create get pr review request: %w", err)
	}
	s.setHeaders(req, token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute get pr review request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get pr review error: status %d: %s", resp.StatusCode, string(b))
	}

	var r struct {
		ID             int64     `json:"id"`
		Body           string    `json:"body"`
		State          string    `json:"state"`
		HTMLURL        string    `json:"html_url"`
		PullRequestURL string    `json:"pull_request_url"`
		CommitID       string    `json:"commit_id"`
		SubmittedAt    time.Time `json:"submitted_at"`
		User           struct {
			Login string `json:"login"`
		} `json:"user"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("decode pr review response: %w", err)
	}

	return &PullRequestReviewItem{
		ID:             r.ID,
		User:           r.User.Login,
		Body:           r.Body,
		State:          r.State,
		HTMLURL:        r.HTMLURL,
		PullRequestURL: r.PullRequestURL,
		CommitID:       r.CommitID,
		SubmittedAt:    r.SubmittedAt,
	}, nil
}

// DismissPullRequestReview dismisses a pull request review with a rationale message.
func (s *GitHubAdvancedService) DismissPullRequestReview(
	ctx context.Context,
	token, owner, repo string,
	prNumber, reviewID int,
	message string,
) (*PullRequestReviewItem, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/reviews/%d/dismissals",
		s.baseURL, owner, repo, prNumber, reviewID)

	payload := map[string]string{
		"message": message,
	}
	bodyBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create dismiss review request: %w", err)
	}
	s.setHeaders(req, token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute dismiss review request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("dismiss review error: status %d: %s", resp.StatusCode, string(b))
	}

	var r struct {
		ID             int64     `json:"id"`
		Body           string    `json:"body"`
		State          string    `json:"state"`
		HTMLURL        string    `json:"html_url"`
		PullRequestURL string    `json:"pull_request_url"`
		CommitID       string    `json:"commit_id"`
		SubmittedAt    time.Time `json:"submitted_at"`
		User           struct {
			Login string `json:"login"`
		} `json:"user"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("decode dismissed review response: %w", err)
	}

	return &PullRequestReviewItem{
		ID:             r.ID,
		User:           r.User.Login,
		Body:           r.Body,
		State:          r.State,
		HTMLURL:        r.HTMLURL,
		PullRequestURL: r.PullRequestURL,
		CommitID:       r.CommitID,
		SubmittedAt:    r.SubmittedAt,
	}, nil
}

// ListReviewComments retrieves comments created as part of a review.
func (s *GitHubAdvancedService) ListReviewComments(
	ctx context.Context,
	token, owner, repo string,
	prNumber, reviewID int,
) ([]ReviewCommentItem, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/reviews/%d/comments",
		s.baseURL, owner, repo, prNumber, reviewID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create list review comments request: %w", err)
	}
	s.setHeaders(req, token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute list review comments request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list review comments error: status %d: %s", resp.StatusCode, string(b))
	}

	var raw []struct {
		ID          int64     `json:"id"`
		ReviewID    int64     `json:"pull_request_review_id"`
		Path        string    `json:"path"`
		Line        int       `json:"line"`
		StartLine   int       `json:"start_line"`
		Side        string    `json:"side"`
		Body        string    `json:"body"`
		CommitID    string    `json:"commit_id"`
		CreatedAt   time.Time `json:"created_at"`
		UpdatedAt   time.Time `json:"updated_at"`
		HTMLURL     string    `json:"html_url"`
		InReplyToID int64     `json:"in_reply_to_id"`
		User        struct {
			Login string `json:"login"`
		} `json:"user"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode review comments response: %w", err)
	}

	comments := make([]ReviewCommentItem, len(raw))
	for i, c := range raw {
		comments[i] = ReviewCommentItem{
			ID:          c.ID,
			ReviewID:    c.ReviewID,
			Path:        c.Path,
			Line:        c.Line,
			StartLine:   c.StartLine,
			Side:        c.Side,
			Body:        c.Body,
			CommitID:    c.CommitID,
			User:        c.User.Login,
			CreatedAt:   c.CreatedAt,
			UpdatedAt:   c.UpdatedAt,
			HTMLURL:     c.HTMLURL,
			InReplyToID: c.InReplyToID,
		}
	}
	return comments, nil
}

// CreateReviewCommentReply creates a reply to an existing PR review comment.
func (s *GitHubAdvancedService) CreateReviewCommentReply(
	ctx context.Context,
	token, owner, repo string,
	prNumber, inReplyToCommentID int,
	body string,
) (*ReviewCommentItem, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/comments/%d/replies",
		s.baseURL, owner, repo, prNumber, inReplyToCommentID)

	payload := map[string]string{
		"body": body,
	}
	bodyBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create review comment reply request: %w", err)
	}
	s.setHeaders(req, token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute review comment reply request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create review comment reply error: status %d: %s", resp.StatusCode, string(b))
	}

	var c struct {
		ID          int64     `json:"id"`
		ReviewID    int64     `json:"pull_request_review_id"`
		Path        string    `json:"path"`
		Line        int       `json:"line"`
		StartLine   int       `json:"start_line"`
		Side        string    `json:"side"`
		Body        string    `json:"body"`
		CommitID    string    `json:"commit_id"`
		CreatedAt   time.Time `json:"created_at"`
		UpdatedAt   time.Time `json:"updated_at"`
		HTMLURL     string    `json:"html_url"`
		InReplyToID int64     `json:"in_reply_to_id"`
		User        struct {
			Login string `json:"login"`
		} `json:"user"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&c); err != nil {
		return nil, fmt.Errorf("decode comment reply response: %w", err)
	}

	return &ReviewCommentItem{
		ID:          c.ID,
		ReviewID:    c.ReviewID,
		Path:        c.Path,
		Line:        c.Line,
		StartLine:   c.StartLine,
		Side:        c.Side,
		Body:        c.Body,
		CommitID:    c.CommitID,
		User:        c.User.Login,
		CreatedAt:   c.CreatedAt,
		UpdatedAt:   c.UpdatedAt,
		HTMLURL:     c.HTMLURL,
		InReplyToID: c.InReplyToID,
	}, nil
}

// RequestReviewers requests reviews from users or teams for a pull request.
func (s *GitHubAdvancedService) RequestReviewers(
	ctx context.Context,
	token, owner, repo string,
	prNumber int,
	reviewers, teamReviewers []string,
) error {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/requested_reviewers",
		s.baseURL, owner, repo, prNumber)

	payload := map[string]any{}
	if len(reviewers) > 0 {
		payload["reviewers"] = reviewers
	}
	if len(teamReviewers) > 0 {
		payload["team_reviewers"] = teamReviewers
	}

	bodyBytes, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("create request reviewers request: %w", err)
	}
	s.setHeaders(req, token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute request reviewers request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("request reviewers error: status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// RemoveReviewers removes requested reviews for users or teams from a pull request.
func (s *GitHubAdvancedService) RemoveReviewers(
	ctx context.Context,
	token, owner, repo string,
	prNumber int,
	reviewers, teamReviewers []string,
) error {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/requested_reviewers",
		s.baseURL, owner, repo, prNumber)

	payload := map[string]any{}
	if len(reviewers) > 0 {
		payload["reviewers"] = reviewers
	}
	if len(teamReviewers) > 0 {
		payload["team_reviewers"] = teamReviewers
	}

	bodyBytes, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("create remove reviewers request: %w", err)
	}
	s.setHeaders(req, token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute remove reviewers request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("remove reviewers error: status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// GetRequestedReviewers retrieves current users and teams requested for review.
func (s *GitHubAdvancedService) GetRequestedReviewers(
	ctx context.Context,
	token, owner, repo string,
	prNumber int,
) (*RequestedReviewersResult, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/requested_reviewers",
		s.baseURL, owner, repo, prNumber)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create get requested reviewers request: %w", err)
	}
	s.setHeaders(req, token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute get requested reviewers request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get requested reviewers error: status %d: %s", resp.StatusCode, string(b))
	}

	var res struct {
		Users []struct {
			Login string `json:"login"`
		} `json:"users"`
		Teams []struct {
			Slug string `json:"slug"`
		} `json:"teams"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("decode requested reviewers response: %w", err)
	}

	users := make([]string, len(res.Users))
	for i, u := range res.Users {
		users[i] = u.Login
	}
	teams := make([]string, len(res.Teams))
	for i, t := range res.Teams {
		teams[i] = t.Slug
	}

	return &RequestedReviewersResult{
		Users: users,
		Teams: teams,
	}, nil
}

// CheckMergeability checks if a PR is mergeable and its current mergeable state.
func (s *GitHubAdvancedService) CheckMergeability(
	ctx context.Context,
	token, owner, repo string,
	prNumber int,
) (*MergeabilityStatus, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/pulls/%d",
		s.baseURL, owner, repo, prNumber)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create check mergeability request: %w", err)
	}
	s.setHeaders(req, token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute check mergeability request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("check mergeability error: status %d: %s", resp.StatusCode, string(b))
	}

	var pr struct {
		Mergeable      *bool  `json:"mergeable"`
		MergeableState string `json:"mergeable_state"`
		Rebaseable     *bool  `json:"rebaseable"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, fmt.Errorf("decode mergeability response: %w", err)
	}

	return &MergeabilityStatus{
		Mergeable:      pr.Mergeable,
		MergeableState: pr.MergeableState,
		Rebaseable:     pr.Rebaseable,
	}, nil
}

// MergePullRequest executes the merge of a pull request.
func (s *GitHubAdvancedService) MergePullRequest(
	ctx context.Context,
	token, owner, repo string,
	prNumber int,
	input MergePullRequestInput,
) (*MergePullRequestOutput, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/merge",
		s.baseURL, owner, repo, prNumber)

	bodyBytes, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("marshal merge pr input: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create merge pr request: %w", err)
	}
	s.setHeaders(req, token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute merge pr request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("merge pr error: status %d: %s", resp.StatusCode, string(b))
	}

	var output MergePullRequestOutput
	if err := json.NewDecoder(resp.Body).Decode(&output); err != nil {
		return nil, fmt.Errorf("decode merge pr response: %w", err)
	}
	return &output, nil
}
