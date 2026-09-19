// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package github

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

// GitHubUser represents basic GitHub user information.
type GitHubUser struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	AvatarURL string `json:"avatar_url,omitempty"`
	HTMLURL   string `json:"html_url,omitempty"`
	Type      string `json:"type,omitempty"`
}

// GitHubTeam represents a team in a GitHub organization.
type GitHubTeam struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description,omitempty"`
}

// GitHubDeployment represents a GitHub deployment event.
type GitHubDeployment struct {
	ID                    int64             `json:"id"`
	SHA                   string            `json:"sha"`
	Ref                   string            `json:"ref"`
	Task                  string            `json:"task"`
	Environment           string            `json:"environment"`
	Description           string            `json:"description,omitempty"`
	Creator               GitHubUser        `json:"creator"`
	CreatedAt             time.Time         `json:"created_at"`
	UpdatedAt             time.Time         `json:"updated_at"`
	StatusesURL           string            `json:"statuses_url"`
	RepositoryURL         string            `json:"repository_url"`
	TransientEnvironment  bool              `json:"transient_environment"`
	ProductionEnvironment bool              `json:"production_environment"`
	Payload               map[string]any    `json:"payload,omitempty"`
}

// CreateDeploymentRequest contains fields to initiate a deployment.
type CreateDeploymentRequest struct {
	Ref                   string         `json:"ref"`
	Task                  string         `json:"task,omitempty"`
	AutoMerge             *bool          `json:"auto_merge,omitempty"`
	RequiredContexts      []string       `json:"required_contexts,omitempty"`
	Payload               map[string]any `json:"payload,omitempty"`
	Environment           string         `json:"environment,omitempty"`
	Description           string         `json:"description,omitempty"`
	TransientEnvironment  bool           `json:"transient_environment,omitempty"`
	ProductionEnvironment bool           `json:"production_environment,omitempty"`
}

// GitHubDeploymentStatus represents status of a deployment.
type GitHubDeploymentStatus struct {
	ID             int64      `json:"id"`
	State          string     `json:"state"` // "error", "failure", "inactive", "in_progress", "queued", "pending", "success"
	Creator        GitHubUser `json:"creator"`
	Description    string     `json:"description,omitempty"`
	Environment    string     `json:"environment,omitempty"`
	TargetURL      string     `json:"target_url,omitempty"`
	LogURL         string     `json:"log_url,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	DeploymentURL  string     `json:"deployment_url"`
	RepositoryURL  string     `json:"repository_url"`
}

// CreateDeploymentStatusRequest contains parameters to post a deployment status.
type CreateDeploymentStatusRequest struct {
	State          string `json:"state"`
	TargetURL      string `json:"target_url,omitempty"`
	LogURL         string `json:"log_url,omitempty"`
	Description    string `json:"description,omitempty"`
	Environment    string `json:"environment,omitempty"`
	AutoInactive   *bool  `json:"auto_inactive,omitempty"`
}

// GitHubBranchProtection represents branch protection rules.
type GitHubBranchProtection struct {
	URL                         string                         `json:"url"`
	RequiredStatusChecks        *GitHubRequiredStatusChecks    `json:"required_status_checks,omitempty"`
	EnforceAdmins               *GitHubEnforceAdmins           `json:"enforce_admins,omitempty"`
	RequiredPullRequestReviews  *GitHubRequiredPRReviews       `json:"required_pull_request_reviews,omitempty"`
	Restrictions                *GitHubBranchRestrictions      `json:"restrictions,omitempty"`
	RequiredLinearHistory       *GitHubProtectionToggle        `json:"required_linear_history,omitempty"`
	AllowForcePushes            *GitHubProtectionToggle        `json:"allow_force_pushes,omitempty"`
	AllowDeletions              *GitHubProtectionToggle        `json:"allow_deletions,omitempty"`
	BlockCreations              *GitHubProtectionToggle        `json:"block_creations,omitempty"`
	RequiredConversationResolution *GitHubProtectionToggle     `json:"required_conversation_resolution,omitempty"`
}

// GitHubRequiredStatusChecks represents status check requirements on a branch.
type GitHubRequiredStatusChecks struct {
	Strict   bool                       `json:"strict"`
	Contexts []string                   `json:"contexts"`
	Checks   []GitHubRequiredCheckEntry `json:"checks,omitempty"`
}

// GitHubRequiredCheckEntry represents a specific app check requirement.
type GitHubRequiredCheckEntry struct {
	Context string `json:"context"`
	AppID   int64  `json:"app_id,omitempty"`
}

// GitHubEnforceAdmins indicates whether branch protection applies to administrators.
type GitHubEnforceAdmins struct {
	URL     string `json:"url"`
	Enabled bool   `json:"enabled"`
}

// GitHubRequiredPRReviews specifies PR review requirements.
type GitHubRequiredPRReviews struct {
	DismissStaleReviews          bool `json:"dismiss_stale_reviews"`
	RequireCodeOwnerReviews      bool `json:"require_code_owner_reviews"`
	RequiredApprovingReviewCount int  `json:"required_approving_review_count"`
	RequireLastPushApproval      bool `json:"require_last_push_approval,omitempty"`
}

// GitHubBranchRestrictions specifies users/teams allowed to push.
type GitHubBranchRestrictions struct {
	Users []GitHubUser `json:"users"`
	Teams []GitHubTeam `json:"teams"`
	Apps  []any        `json:"apps,omitempty"`
}

// GitHubProtectionToggle represents a simple enabled/disabled boolean toggle.
type GitHubProtectionToggle struct {
	Enabled bool `json:"enabled"`
}

// BranchProtectionRequest contains configuration to set branch protection.
type BranchProtectionRequest struct {
	RequiredStatusChecks           *GitHubRequiredStatusChecks `json:"required_status_checks"`
	EnforceAdmins                  bool                        `json:"enforce_admins"`
	RequiredPullRequestReviews     *GitHubRequiredPRReviews    `json:"required_pull_request_reviews"`
	Restrictions                   *GitHubBranchRestrictions   `json:"restrictions"`
	RequiredLinearHistory          bool                        `json:"required_linear_history,omitempty"`
	AllowForcePushes               bool                        `json:"allow_force_pushes,omitempty"`
	AllowDeletions                 bool                        `json:"allow_deletions,omitempty"`
	BlockCreations                 bool                        `json:"block_creations,omitempty"`
	RequiredConversationResolution bool                        `json:"required_conversation_resolution,omitempty"`
}

// GitHubDeploymentsAndProtectionsService provides GitHub API methods for deployments and protections.
type GitHubDeploymentsAndProtectionsService struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewGitHubDeploymentsAndProtectionsService creates an instance of the service.
func NewGitHubDeploymentsAndProtectionsService(baseURL, token string, client *http.Client) *GitHubDeploymentsAndProtectionsService {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	if baseURL == "" {
		baseURL = "https://api.github.com"
	}
	return &GitHubDeploymentsAndProtectionsService{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		httpClient: client,
	}
}

func (s *GitHubDeploymentsAndProtectionsService) applyHeaders(req *http.Request) {
	if strings.HasPrefix(s.token, "Bearer ") {
		req.Header.Set("Authorization", s.token)
	} else if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")
}

// CreateDeployment creates a new deployment.
func (s *GitHubDeploymentsAndProtectionsService) CreateDeployment(ctx context.Context, owner, repo string, req CreateDeploymentRequest) (*GitHubDeployment, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/deployments", s.baseURL, url.PathEscape(owner), url.PathEscape(repo))

	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	s.applyHeaders(httpReq)

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create deployment failed (status %d): %s", resp.StatusCode, string(body))
	}

	var deployment GitHubDeployment
	if err := json.NewDecoder(resp.Body).Decode(&deployment); err != nil {
		return nil, err
	}
	return &deployment, nil
}

// CreateDeploymentStatus adds a status check to a deployment.
func (s *GitHubDeploymentsAndProtectionsService) CreateDeploymentStatus(ctx context.Context, owner, repo string, deploymentID int64, req CreateDeploymentStatusRequest) (*GitHubDeploymentStatus, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/deployments/%d/statuses", s.baseURL, url.PathEscape(owner), url.PathEscape(repo), deploymentID)

	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	s.applyHeaders(httpReq)

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create deployment status failed (status %d): %s", resp.StatusCode, string(body))
	}

	var status GitHubDeploymentStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, err
	}
	return &status, nil
}

// ListDeployments lists deployments for a repository.
func (s *GitHubDeploymentsAndProtectionsService) ListDeployments(ctx context.Context, owner, repo string, environment string) ([]GitHubDeployment, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/deployments", s.baseURL, url.PathEscape(owner), url.PathEscape(repo))
	if environment != "" {
		endpoint += "?environment=" + url.QueryEscape(environment)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	s.applyHeaders(httpReq)

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list deployments failed (status %d): %s", resp.StatusCode, string(body))
	}

	var deployments []GitHubDeployment
	if err := json.NewDecoder(resp.Body).Decode(&deployments); err != nil {
		return nil, err
	}
	return deployments, nil
}

// ListDeploymentStatuses lists statuses for a deployment.
func (s *GitHubDeploymentsAndProtectionsService) ListDeploymentStatuses(ctx context.Context, owner, repo string, deploymentID int64) ([]GitHubDeploymentStatus, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/deployments/%d/statuses", s.baseURL, url.PathEscape(owner), url.PathEscape(repo), deploymentID)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	s.applyHeaders(httpReq)

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list deployment statuses failed (status %d): %s", resp.StatusCode, string(body))
	}

	var statuses []GitHubDeploymentStatus
	if err := json.NewDecoder(resp.Body).Decode(&statuses); err != nil {
		return nil, err
	}
	return statuses, nil
}

// GetBranchProtection retrieves protection settings for a branch.
func (s *GitHubDeploymentsAndProtectionsService) GetBranchProtection(ctx context.Context, owner, repo, branch string) (*GitHubBranchProtection, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/branches/%s/protection", s.baseURL, url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(branch))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	s.applyHeaders(httpReq)

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get branch protection failed (status %d): %s", resp.StatusCode, string(body))
	}

	var protection GitHubBranchProtection
	if err := json.NewDecoder(resp.Body).Decode(&protection); err != nil {
		return nil, err
	}
	return &protection, nil
}

// UpdateBranchProtection sets protection settings on a branch.
func (s *GitHubDeploymentsAndProtectionsService) UpdateBranchProtection(ctx context.Context, owner, repo, branch string, req BranchProtectionRequest) (*GitHubBranchProtection, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/branches/%s/protection", s.baseURL, url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(branch))

	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	s.applyHeaders(httpReq)

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("update branch protection failed (status %d): %s", resp.StatusCode, string(body))
	}

	var protection GitHubBranchProtection
	if err := json.NewDecoder(resp.Body).Decode(&protection); err != nil {
		return nil, err
	}
	return &protection, nil
}

// DeleteBranchProtection removes protection from a branch.
func (s *GitHubDeploymentsAndProtectionsService) DeleteBranchProtection(ctx context.Context, owner, repo, branch string) error {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/branches/%s/protection", s.baseURL, url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(branch))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	s.applyHeaders(httpReq)

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete branch protection failed (status %d): %s", resp.StatusCode, string(body))
	}
	return nil
}

// CreateRepositoryDispatch triggers a repository_dispatch event in GitHub Actions.
func (s *GitHubDeploymentsAndProtectionsService) CreateRepositoryDispatch(ctx context.Context, owner, repo, eventType string, clientPayload map[string]any) error {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/dispatches", s.baseURL, url.PathEscape(owner), url.PathEscape(repo))

	payload := map[string]any{
		"event_type": eventType,
	}
	if clientPayload != nil {
		payload["client_payload"] = clientPayload
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	s.applyHeaders(httpReq)

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create repository dispatch failed (status %d): %s", resp.StatusCode, string(body))
	}
	return nil
}
