package forgejo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// -------------------------------------------------------------------------------------
// Extended Forgejo / Gitea Models
// -------------------------------------------------------------------------------------

// PullReviewComment models an inline comment attached to a review.
type PullReviewComment struct {
	ID             int64     `json:"id"`
	ReviewID       int64     `json:"pull_request_review_id"`
	Path           string    `json:"path"`
	OldLineNum     int       `json:"old_line_num"`
	NewLineNum     int       `json:"new_line_num"`
	Body           string    `json:"body"`
	DiffHash       string    `json:"diff_hash,omitempty"`
	CommitID       string    `json:"commit_id"`
	User           string    `json:"user"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	HTMLURL        string    `json:"html_url"`
	PullRequestURL string    `json:"pull_request_url"`
}

// PullRequestChangedFile models a file modified within a pull request.
type PullRequestChangedFile struct {
	Filename    string `json:"filename"`
	Status      string `json:"status"` // added, modified, deleted, renamed
	Additions   int    `json:"additions"`
	Deletions   int    `json:"deletions"`
	Changes     int    `json:"changes"`
	HTMLURL     string `json:"html_url"`
	RawURL      string `json:"raw_url"`
	ContentsURL string `json:"contents_url"`
	Patch       string `json:"patch,omitempty"`
}

// CommitStatusRecord models a single commit status check in Forgejo.
type CommitStatusRecord struct {
	ID          int64     `json:"id"`
	State       string    `json:"state"` // pending, success, error, failure, warning
	TargetURL   string    `json:"target_url"`
	Description string    `json:"description"`
	Context     string    `json:"context"`
	Creator     string    `json:"creator"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CombinedCommitStatus models the aggregate status of all checks on a commit.
type CombinedCommitStatus struct {
	State      string               `json:"state"`
	SHA        string               `json:"sha"`
	TotalCount int                  `json:"total_count"`
	Statuses   []CommitStatusRecord `json:"statuses"`
	Repository string               `json:"repository"`
	CommitURL  string               `json:"commit_url"`
}

// CollaboratorPermission models collaborator access level on a repository.
type CollaboratorPermission struct {
	User       string `json:"user"`
	Permission string `json:"permission"` // read, write, admin, owner
	RoleName   string `json:"role_name,omitempty"`
}

// -------------------------------------------------------------------------------------
// Pull Request Review Management Methods
// -------------------------------------------------------------------------------------

// ListPullReviews returns all reviews submitted on a pull request.
func (s *ForgejoAdvancedService) ListPullReviews(
	ctx context.Context,
	token, owner, repo string,
	prIndex int64,
) ([]PullReviewResult, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/reviews",
		s.baseURL, owner, repo, prIndex)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create list pull reviews request: %w", err)
	}
	s.applyAuth(req, token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute list pull reviews request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list pull reviews error: status %d: %s", resp.StatusCode, string(b))
	}

	var raw []struct {
		ID          int64     `json:"id"`
		State       string    `json:"state"`
		Body        string    `json:"body"`
		HTMLURL     string    `json:"html_url"`
		SubmittedAt time.Time `json:"submitted_at"`
		User        struct {
			UserName string `json:"username"`
		} `json:"user"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode pull reviews response: %w", err)
	}

	results := make([]PullReviewResult, len(raw))
	for i, r := range raw {
		results[i] = PullReviewResult{
			ID:          r.ID,
			State:       r.State,
			Body:        r.Body,
			Reviewer:    r.User.UserName,
			SubmittedAt: r.SubmittedAt,
			HTMLURL:     r.HTMLURL,
		}
	}
	return results, nil
}

// GetPullReview retrieves a specific review by ID.
func (s *ForgejoAdvancedService) GetPullReview(
	ctx context.Context,
	token, owner, repo string,
	prIndex, reviewID int64,
) (*PullReviewResult, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/reviews/%d",
		s.baseURL, owner, repo, prIndex, reviewID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create get pull review request: %w", err)
	}
	s.applyAuth(req, token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute get pull review request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get pull review error: status %d: %s", resp.StatusCode, string(b))
	}

	var r struct {
		ID          int64     `json:"id"`
		State       string    `json:"state"`
		Body        string    `json:"body"`
		HTMLURL     string    `json:"html_url"`
		SubmittedAt time.Time `json:"submitted_at"`
		User        struct {
			UserName string `json:"username"`
		} `json:"user"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("decode pull review response: %w", err)
	}

	return &PullReviewResult{
		ID:          r.ID,
		State:       r.State,
		Body:        r.Body,
		Reviewer:    r.User.UserName,
		SubmittedAt: r.SubmittedAt,
		HTMLURL:     r.HTMLURL,
	}, nil
}

// DismissPullReview dismisses an existing review with a rationale comment.
func (s *ForgejoAdvancedService) DismissPullReview(
	ctx context.Context,
	token, owner, repo string,
	prIndex, reviewID int64,
	message string,
) error {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/reviews/%d/dismissals",
		s.baseURL, owner, repo, prIndex, reviewID)

	payload := map[string]string{
		"message": message,
	}
	bodyBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("create dismiss review request: %w", err)
	}
	s.applyAuth(req, token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute dismiss review request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("dismiss review error: status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// ListReviewComments retrieves all inline comments belonging to a specific review.
func (s *ForgejoAdvancedService) ListReviewComments(
	ctx context.Context,
	token, owner, repo string,
	prIndex, reviewID int64,
) ([]PullReviewComment, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/reviews/%d/comments",
		s.baseURL, owner, repo, prIndex, reviewID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create list review comments request: %w", err)
	}
	s.applyAuth(req, token)
	req.Header.Set("Accept", "application/json")

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
		ID             int64     `json:"id"`
		ReviewID       int64     `json:"pull_request_review_id"`
		Path           string    `json:"path"`
		OldLineNum     int       `json:"old_line_num"`
		NewLineNum     int       `json:"new_line_num"`
		Body           string    `json:"body"`
		DiffHash       string    `json:"diff_hash"`
		CommitID       string    `json:"commit_id"`
		CreatedAt      time.Time `json:"created_at"`
		UpdatedAt      time.Time `json:"updated_at"`
		HTMLURL        string    `json:"html_url"`
		PullRequestURL string    `json:"pull_request_url"`
		User           struct {
			UserName string `json:"username"`
		} `json:"user"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode review comments response: %w", err)
	}

	comments := make([]PullReviewComment, len(raw))
	for i, c := range raw {
		comments[i] = PullReviewComment{
			ID:             c.ID,
			ReviewID:       c.ReviewID,
			Path:           c.Path,
			OldLineNum:     c.OldLineNum,
			NewLineNum:     c.NewLineNum,
			Body:           c.Body,
			DiffHash:       c.DiffHash,
			CommitID:       c.CommitID,
			User:           c.User.UserName,
			CreatedAt:      c.CreatedAt,
			UpdatedAt:      c.UpdatedAt,
			HTMLURL:        c.HTMLURL,
			PullRequestURL: c.PullRequestURL,
		}
	}
	return comments, nil
}

// -------------------------------------------------------------------------------------
// Pull Request Diffs and Changed Files
// -------------------------------------------------------------------------------------

// GetPullRequestDiff downloads the unified raw diff of a pull request.
func (s *ForgejoAdvancedService) GetPullRequestDiff(
	ctx context.Context,
	token, owner, repo string,
	prIndex int64,
	binary bool,
) (string, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/pulls/%d.diff",
		s.baseURL, owner, repo, prIndex)
	if binary {
		endpoint += "?binary=true"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("create get pr diff request: %w", err)
	}
	s.applyAuth(req, token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("execute get pr diff request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("get pr diff error: status %d: %s", resp.StatusCode, string(b))
	}

	diffBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read pr diff body: %w", err)
	}
	return string(diffBytes), nil
}

// GetPullRequestPatch downloads the git patch of a pull request.
func (s *ForgejoAdvancedService) GetPullRequestPatch(
	ctx context.Context,
	token, owner, repo string,
	prIndex int64,
) (string, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/pulls/%d.patch",
		s.baseURL, owner, repo, prIndex)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("create get pr patch request: %w", err)
	}
	s.applyAuth(req, token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("execute get pr patch request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("get pr patch error: status %d: %s", resp.StatusCode, string(b))
	}

	patchBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read pr patch body: %w", err)
	}
	return string(patchBytes), nil
}

// GetPullRequestFiles retrieves the structured list of changed files with diff patches.
func (s *ForgejoAdvancedService) GetPullRequestFiles(
	ctx context.Context,
	token, owner, repo string,
	prIndex int64,
) ([]PullRequestChangedFile, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/files",
		s.baseURL, owner, repo, prIndex)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create get pr files request: %w", err)
	}
	s.applyAuth(req, token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute get pr files request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get pr files error: status %d: %s", resp.StatusCode, string(b))
	}

	var files []PullRequestChangedFile
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, fmt.Errorf("decode pr files response: %w", err)
	}
	return files, nil
}

// -------------------------------------------------------------------------------------
// Branch Protection Management Methods
// -------------------------------------------------------------------------------------

// GetBranchProtections lists all branch protection rules on a repository.
func (s *ForgejoAdvancedService) GetBranchProtections(
	ctx context.Context,
	token, owner, repo string,
) ([]BranchProtection, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/branch_protections",
		s.baseURL, owner, repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create get branch protections request: %w", err)
	}
	s.applyAuth(req, token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute get branch protections request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get branch protections error: status %d: %s", resp.StatusCode, string(b))
	}

	var protections []BranchProtection
	if err := json.NewDecoder(resp.Body).Decode(&protections); err != nil {
		return nil, fmt.Errorf("decode branch protections response: %w", err)
	}
	return protections, nil
}


// CreateBranchProtection creates a branch protection rule.
func (s *ForgejoAdvancedService) CreateBranchProtection(
	ctx context.Context,
	token, owner, repo string,
	bp BranchProtection,
) (*BranchProtection, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/branch_protections",
		s.baseURL, owner, repo)

	bodyBytes, err := json.Marshal(bp)
	if err != nil {
		return nil, fmt.Errorf("marshal branch protection: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create branch protection request: %w", err)
	}
	s.applyAuth(req, token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute create branch protection request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create branch protection error: status %d: %s", resp.StatusCode, string(b))
	}

	var created BranchProtection
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return nil, fmt.Errorf("decode created branch protection: %w", err)
	}
	return &created, nil
}

// UpdateBranchProtection modifies an existing branch protection rule.
func (s *ForgejoAdvancedService) UpdateBranchProtection(
	ctx context.Context,
	token, owner, repo, branchName string,
	bp BranchProtection,
) (*BranchProtection, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/branch_protections/%s",
		s.baseURL, owner, repo, url.PathEscape(branchName))

	bodyBytes, err := json.Marshal(bp)
	if err != nil {
		return nil, fmt.Errorf("marshal update branch protection: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create update branch protection request: %w", err)
	}
	s.applyAuth(req, token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute update branch protection request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("update branch protection error: status %d: %s", resp.StatusCode, string(b))
	}

	var updated BranchProtection
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		return nil, fmt.Errorf("decode updated branch protection: %w", err)
	}
	return &updated, nil
}

// DeleteBranchProtection removes a branch protection rule.
func (s *ForgejoAdvancedService) DeleteBranchProtection(
	ctx context.Context,
	token, owner, repo, branchName string,
) error {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/branch_protections/%s",
		s.baseURL, owner, repo, url.PathEscape(branchName))

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create delete branch protection request: %w", err)
	}
	s.applyAuth(req, token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute delete branch protection request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete branch protection error: status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// -------------------------------------------------------------------------------------
// Commit Status and Combined Status Methods
// -------------------------------------------------------------------------------------

// GetCombinedCommitStatus retrieves aggregate status for a git commit.
func (s *ForgejoAdvancedService) GetCombinedCommitStatus(
	ctx context.Context,
	token, owner, repo, sha string,
) (*CombinedCommitStatus, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/commits/%s/status",
		s.baseURL, owner, repo, sha)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create get combined status request: %w", err)
	}
	s.applyAuth(req, token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute get combined status request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get combined status error: status %d: %s", resp.StatusCode, string(b))
	}

	var combined CombinedCommitStatus
	if err := json.NewDecoder(resp.Body).Decode(&combined); err != nil {
		return nil, fmt.Errorf("decode combined status response: %w", err)
	}
	return &combined, nil
}

// ListCommitStatuses lists individual commit statuses for a commit.
func (s *ForgejoAdvancedService) ListCommitStatuses(
	ctx context.Context,
	token, owner, repo, sha string,
	page, limit int,
) ([]CommitStatusRecord, error) {
	u, err := url.Parse(fmt.Sprintf("%s/repos/%s/%s/commits/%s/statuses", s.baseURL, owner, repo, sha))
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	q := u.Query()
	if page > 0 {
		q.Set("page", strconv.Itoa(page))
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create list commit statuses request: %w", err)
	}
	s.applyAuth(req, token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute list commit statuses request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list commit statuses error: status %d: %s", resp.StatusCode, string(b))
	}

	var statuses []CommitStatusRecord
	if err := json.NewDecoder(resp.Body).Decode(&statuses); err != nil {
		return nil, fmt.Errorf("decode commit statuses response: %w", err)
	}
	return statuses, nil
}

// -------------------------------------------------------------------------------------
// Repository Topics and Collaborators
// -------------------------------------------------------------------------------------

// ListRepoTopics retrieves topics/tags assigned to a repository.
func (s *ForgejoAdvancedService) ListRepoTopics(
	ctx context.Context,
	token, owner, repo string,
) ([]string, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/topics",
		s.baseURL, owner, repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create list repo topics request: %w", err)
	}
	s.applyAuth(req, token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute list repo topics request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list repo topics error: status %d: %s", resp.StatusCode, string(b))
	}

	var res struct {
		Topics []string `json:"topics"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("decode repo topics response: %w", err)
	}
	return res.Topics, nil
}

// SetRepoTopics updates the topics assigned to a repository.
func (s *ForgejoAdvancedService) SetRepoTopics(
	ctx context.Context,
	token, owner, repo string,
	topics []string,
) error {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/topics",
		s.baseURL, owner, repo)

	payload := map[string][]string{
		"topics": topics,
	}
	bodyBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("create set repo topics request: %w", err)
	}
	s.applyAuth(req, token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute set repo topics request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("set repo topics error: status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// CheckCollaborator verifies if a user is a collaborator and determines their access level.
func (s *ForgejoAdvancedService) CheckCollaborator(
	ctx context.Context,
	token, owner, repo, username string,
) (bool, string, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/collaborators/%s",
		s.baseURL, owner, repo, url.PathEscape(username))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, "", fmt.Errorf("create check collaborator request: %w", err)
	}
	s.applyAuth(req, token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return false, "", fmt.Errorf("execute check collaborator request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return false, "", nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return false, "", fmt.Errorf("check collaborator error: status %d: %s", resp.StatusCode, string(b))
	}

	var perm struct {
		Permission string `json:"permission"`
		RoleName   string `json:"role_name"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&perm)
	role := perm.Permission
	if role == "" {
		role = perm.RoleName
	}
	return true, role, nil
}

func (s *ForgejoAdvancedService) applyAuth(req *http.Request, token string) {
	if token != "" {
		if strings.HasPrefix(token, "token ") || strings.HasPrefix(token, "Bearer ") {
			req.Header.Set("Authorization", token)
		} else {
			req.Header.Set("Authorization", "token "+token)
		}
	}
}
