// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package forgejo

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

// ForgejoMilestone represents a milestone for grouping issues and pull requests.
type ForgejoMilestone struct {
	ID           int64      `json:"id"`
	Title        string     `json:"title"`
	Description  string     `json:"description,omitempty"`
	State        string     `json:"state"` // "open", "closed"
	OpenIssues   int        `json:"open_issues"`
	ClosedIssues int        `json:"closed_issues"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	Deadline     *time.Time `json:"deadline,omitempty"`
}

// CreateForgejoMilestoneRequest contains fields to create a milestone.
type CreateForgejoMilestoneRequest struct {
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	State       string     `json:"state,omitempty"` // "open", "closed"
	Deadline    *time.Time `json:"deadline,omitempty"`
}

// UpdateForgejoMilestoneRequest contains fields to update a milestone.
type UpdateForgejoMilestoneRequest struct {
	Title       string     `json:"title,omitempty"`
	Description string     `json:"description,omitempty"`
	State       string     `json:"state,omitempty"`
	Deadline    *time.Time `json:"deadline,omitempty"`
}

// ForgejoLabel represents a tag/label applied to issues or pull requests.
type ForgejoLabel struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Color       string `json:"color"` // hex code, e.g. "f29513"
	URL         string `json:"url,omitempty"`
}

// CreateForgejoLabelRequest contains fields to create a label.
type CreateForgejoLabelRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Color       string `json:"color"`
}

// UpdateForgejoLabelRequest contains fields to update a label.
type UpdateForgejoLabelRequest struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Color       string `json:"color,omitempty"`
}

// ForgejoBranch represents a repository branch.
type ForgejoBranch struct {
	Name      string            `json:"name"`
	Commit    *ForgejoCommitRef `json:"commit,omitempty"`
	Protected bool              `json:"protected"`
}

// ForgejoCommitRef provides minimal commit information on a branch.
type ForgejoCommitRef struct {
	ID        string    `json:"id"`
	Message   string    `json:"message"`
	URL       string    `json:"url"`
	Timestamp time.Time `json:"timestamp"`
}

// CreateForgejoBranchRequest contains parameters to create a branch.
type CreateForgejoBranchRequest struct {
	NewBranchName string `json:"new_branch_name"`
	OldRefName    string `json:"old_ref_name,omitempty"`
}

// ForgejoRelease represents a published release in Forgejo.
type ForgejoRelease struct {
	ID              int64     `json:"id"`
	TagName         string    `json:"tag_name"`
	TargetCommitish string    `json:"target_commitish"`
	Name            string    `json:"name"`
	Body            string    `json:"body"`
	Draft           bool      `json:"draft"`
	Prerelease      bool      `json:"prerelease"`
	CreatedAt       time.Time `json:"created_at"`
	PublishedAt     time.Time `json:"published_at"`
}

// CreateForgejoReleaseRequest contains parameters to create a release.
type CreateForgejoReleaseRequest struct {
	TagName         string `json:"tag_name"`
	TargetCommitish string `json:"target_commitish,omitempty"`
	Name            string `json:"name"`
	Body            string `json:"body,omitempty"`
	Draft           bool   `json:"draft,omitempty"`
	Prerelease      bool   `json:"prerelease,omitempty"`
}

// ForgejoMilestonesAndLabelsService provides Forgejo API methods for milestones, labels, branches, and releases.
type ForgejoMilestonesAndLabelsService struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewForgejoMilestonesAndLabelsService creates a new instance of ForgejoMilestonesAndLabelsService.
func NewForgejoMilestonesAndLabelsService(baseURL, token string, client *http.Client) *ForgejoMilestonesAndLabelsService {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &ForgejoMilestonesAndLabelsService{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		httpClient: client,
	}
}

func (s *ForgejoMilestonesAndLabelsService) applyAuth(req *http.Request) {
	if strings.HasPrefix(s.token, "token ") || strings.HasPrefix(s.token, "Bearer ") {
		req.Header.Set("Authorization", s.token)
	} else if s.token != "" {
		req.Header.Set("Authorization", "token "+s.token)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
}

// ListMilestones retrieves milestones for a repository.
func (s *ForgejoMilestonesAndLabelsService) ListMilestones(ctx context.Context, owner, repo, state string) ([]ForgejoMilestone, error) {
	endpoint := fmt.Sprintf("%s/api/v1/repos/%s/%s/milestones", s.baseURL, url.PathEscape(owner), url.PathEscape(repo))
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
		return nil, fmt.Errorf("list milestones failed (status %d): %s", resp.StatusCode, string(body))
	}

	var milestones []ForgejoMilestone
	if err := json.NewDecoder(resp.Body).Decode(&milestones); err != nil {
		return nil, err
	}
	return milestones, nil
}

// GetMilestone retrieves a single milestone by ID.
func (s *ForgejoMilestonesAndLabelsService) GetMilestone(ctx context.Context, owner, repo string, id int64) (*ForgejoMilestone, error) {
	endpoint := fmt.Sprintf("%s/api/v1/repos/%s/%s/milestones/%d", s.baseURL, url.PathEscape(owner), url.PathEscape(repo), id)

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
		return nil, fmt.Errorf("get milestone failed (status %d): %s", resp.StatusCode, string(body))
	}

	var milestone ForgejoMilestone
	if err := json.NewDecoder(resp.Body).Decode(&milestone); err != nil {
		return nil, err
	}
	return &milestone, nil
}

// CreateMilestone creates a new milestone.
func (s *ForgejoMilestonesAndLabelsService) CreateMilestone(ctx context.Context, owner, repo string, req CreateForgejoMilestoneRequest) (*ForgejoMilestone, error) {
	endpoint := fmt.Sprintf("%s/api/v1/repos/%s/%s/milestones", s.baseURL, url.PathEscape(owner), url.PathEscape(repo))

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
		return nil, fmt.Errorf("create milestone failed (status %d): %s", resp.StatusCode, string(body))
	}

	var milestone ForgejoMilestone
	if err := json.NewDecoder(resp.Body).Decode(&milestone); err != nil {
		return nil, err
	}
	return &milestone, nil
}

// UpdateMilestone updates an existing milestone.
func (s *ForgejoMilestonesAndLabelsService) UpdateMilestone(ctx context.Context, owner, repo string, id int64, req UpdateForgejoMilestoneRequest) (*ForgejoMilestone, error) {
	endpoint := fmt.Sprintf("%s/api/v1/repos/%s/%s/milestones/%d", s.baseURL, url.PathEscape(owner), url.PathEscape(repo), id)

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
		return nil, fmt.Errorf("update milestone failed (status %d): %s", resp.StatusCode, string(body))
	}

	var milestone ForgejoMilestone
	if err := json.NewDecoder(resp.Body).Decode(&milestone); err != nil {
		return nil, err
	}
	return &milestone, nil
}

// DeleteMilestone deletes a milestone.
func (s *ForgejoMilestonesAndLabelsService) DeleteMilestone(ctx context.Context, owner, repo string, id int64) error {
	endpoint := fmt.Sprintf("%s/api/v1/repos/%s/%s/milestones/%d", s.baseURL, url.PathEscape(owner), url.PathEscape(repo), id)

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
		return fmt.Errorf("delete milestone failed (status %d): %s", resp.StatusCode, string(body))
	}
	return nil
}

// ListLabels retrieves labels configured for a repository.
func (s *ForgejoMilestonesAndLabelsService) ListLabels(ctx context.Context, owner, repo string) ([]ForgejoLabel, error) {
	endpoint := fmt.Sprintf("%s/api/v1/repos/%s/%s/labels", s.baseURL, url.PathEscape(owner), url.PathEscape(repo))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
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
		return nil, fmt.Errorf("list labels failed (status %d): %s", resp.StatusCode, string(body))
	}

	var labels []ForgejoLabel
	if err := json.NewDecoder(resp.Body).Decode(&labels); err != nil {
		return nil, err
	}
	return labels, nil
}

// CreateLabel creates a new label.
func (s *ForgejoMilestonesAndLabelsService) CreateLabel(ctx context.Context, owner, repo string, req CreateForgejoLabelRequest) (*ForgejoLabel, error) {
	endpoint := fmt.Sprintf("%s/api/v1/repos/%s/%s/labels", s.baseURL, url.PathEscape(owner), url.PathEscape(repo))

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
		return nil, fmt.Errorf("create label failed (status %d): %s", resp.StatusCode, string(body))
	}

	var label ForgejoLabel
	if err := json.NewDecoder(resp.Body).Decode(&label); err != nil {
		return nil, err
	}
	return &label, nil
}

// AddLabelsToIssueOrPR applies label IDs to an issue or PR.
func (s *ForgejoMilestonesAndLabelsService) AddLabelsToIssueOrPR(ctx context.Context, owner, repo string, index int64, labelIDs []int64) ([]ForgejoLabel, error) {
	endpoint := fmt.Sprintf("%s/api/v1/repos/%s/%s/issues/%d/labels", s.baseURL, url.PathEscape(owner), url.PathEscape(repo), index)

	payload := map[string][]int64{"labels": labelIDs}
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

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("add labels failed (status %d): %s", resp.StatusCode, string(body))
	}

	var labels []ForgejoLabel
	if err := json.NewDecoder(resp.Body).Decode(&labels); err != nil {
		return nil, err
	}
	return labels, nil
}

// ListBranches retrieves branches for a repository.
func (s *ForgejoMilestonesAndLabelsService) ListBranches(ctx context.Context, owner, repo string) ([]ForgejoBranch, error) {
	endpoint := fmt.Sprintf("%s/api/v1/repos/%s/%s/branches", s.baseURL, url.PathEscape(owner), url.PathEscape(repo))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
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
		return nil, fmt.Errorf("list branches failed (status %d): %s", resp.StatusCode, string(body))
	}

	var branches []ForgejoBranch
	if err := json.NewDecoder(resp.Body).Decode(&branches); err != nil {
		return nil, err
	}
	return branches, nil
}

// CreateBranch creates a new branch from an old ref.
func (s *ForgejoMilestonesAndLabelsService) CreateBranch(ctx context.Context, owner, repo string, req CreateForgejoBranchRequest) (*ForgejoBranch, error) {
	endpoint := fmt.Sprintf("%s/api/v1/repos/%s/%s/branches", s.baseURL, url.PathEscape(owner), url.PathEscape(repo))

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
		return nil, fmt.Errorf("create branch failed (status %d): %s", resp.StatusCode, string(body))
	}

	var branch ForgejoBranch
	if err := json.NewDecoder(resp.Body).Decode(&branch); err != nil {
		return nil, err
	}
	return &branch, nil
}

// DeleteBranch deletes a branch.
func (s *ForgejoMilestonesAndLabelsService) DeleteBranch(ctx context.Context, owner, repo, branchName string) error {
	endpoint := fmt.Sprintf("%s/api/v1/repos/%s/%s/branches/%s", s.baseURL, url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(branchName))

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
		return fmt.Errorf("delete branch failed (status %d): %s", resp.StatusCode, string(body))
	}
	return nil
}

// ListReleases retrieves releases for a repository.
func (s *ForgejoMilestonesAndLabelsService) ListReleases(ctx context.Context, owner, repo string) ([]ForgejoRelease, error) {
	endpoint := fmt.Sprintf("%s/api/v1/repos/%s/%s/releases", s.baseURL, url.PathEscape(owner), url.PathEscape(repo))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
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
		return nil, fmt.Errorf("list releases failed (status %d): %s", resp.StatusCode, string(body))
	}

	var releases []ForgejoRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}
	return releases, nil
}

// CreateRelease creates a release in Forgejo.
func (s *ForgejoMilestonesAndLabelsService) CreateRelease(ctx context.Context, owner, repo string, req CreateForgejoReleaseRequest) (*ForgejoRelease, error) {
	endpoint := fmt.Sprintf("%s/api/v1/repos/%s/%s/releases", s.baseURL, url.PathEscape(owner), url.PathEscape(repo))

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
		return nil, fmt.Errorf("create release failed (status %d): %s", resp.StatusCode, string(body))
	}

	var release ForgejoRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}
	return &release, nil
}
