// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// GITLAB ANALYTICS AND AUDIT MODELS

// ProjectAuditEvent represents a security or configuration audit record in GitLab.
type ProjectAuditEvent struct {
	ID         int64     `json:"id"`
	AuthorID   int64     `json:"author_id"`
	EntityID   int64     `json:"entity_id"`
	EntityType string    `json:"entity_type"`
	Details    any       `json:"details"`
	CreatedAt  time.Time `json:"created_at"`
}

// ProjectPushRules models branch and commit validation rules enforced by GitLab.
type ProjectPushRules struct {
	ID                       int64  `json:"id"`
	ProjectID                int64  `json:"project_id"`
	CommitMessageRegex       string `json:"commit_message_regex,omitempty"`
	CommitMessageNegative    string `json:"commit_message_negative_regex,omitempty"`
	BranchNameRegex          string `json:"branch_name_regex,omitempty"`
	AuthorEmailRegex         string `json:"author_email_regex,omitempty"`
	FileNameRegex            string `json:"file_name_regex,omitempty"`
	MaxFileSize              int    `json:"max_file_size,omitempty"`
	DenyDeleteTag            bool   `json:"deny_delete_tag,omitempty"`
	MemberCheck              bool   `json:"member_check,omitempty"`
	PreventSecrets           bool   `json:"prevent_secrets,omitempty"`
	RejectUnsignedCommits    bool   `json:"reject_unsigned_commits,omitempty"`
	CommitCommitterCheck     bool   `json:"commit_committer_check,omitempty"`
	RejectNonDCOCommits      bool   `json:"reject_non_dco_commits,omitempty"`
}

// ContributorStats models author contribution metrics.
type ContributorStats struct {
	Name      string `json:"name"`
	Email     string `json:"email"`
	Commits   int    `json:"commits"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

// MergeRequestMetrics models lifecycle analytics for a merge request.
type MergeRequestMetrics struct {
	MergedAt                 *time.Time `json:"merged_at,omitempty"`
	ClosedAt                 *time.Time `json:"closed_at,omitempty"`
	FirstCommentAt           *time.Time `json:"first_comment_at,omitempty"`
	FirstCommitAt            *time.Time `json:"first_commit_at,omitempty"`
	LastCommitAt             *time.Time `json:"last_commit_at,omitempty"`
	DiffSize                 int        `json:"diff_size"`
	ModifiedPathsCount       int        `json:"modified_paths_count"`
	CommitsCount             int        `json:"commits_count"`
	TotalDiscussionsCount    int        `json:"total_discussions_count"`
	ResolvedDiscussionsCount int        `json:"resolved_discussions_count"`
}

// PipelineAnalyticsCharts models CI/CD aggregate metrics.
type PipelineAnalyticsCharts struct {
	Count       int `json:"count"`
	Success     int `json:"success"`
	Failed      int `json:"failed"`
	SuccessRate float64 `json:"success_rate"`
}

// GITLAB ANALYTICS AND AUDIT SERVICE

// AnalyticsAndAuditService manages GitLab audit events, push rules, and developer metrics.
type AnalyticsAndAuditService struct {
	client *Client
}

// NewAnalyticsAndAuditService initializes AnalyticsAndAuditService.
func NewAnalyticsAndAuditService(client *Client) *AnalyticsAndAuditService {
	return &AnalyticsAndAuditService{client: client}
}

// GetProjectAuditEvents lists audit events for a project with optional time window.
func (s *AnalyticsAndAuditService) GetProjectAuditEvents(
	ctx context.Context,
	projectID any,
	createdAfter *time.Time,
	page, perPage int,
) ([]ProjectAuditEvent, error) {
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 30
	}

	q := url.Values{}
	q.Set("page", strconv.Itoa(page))
	q.Set("per_page", strconv.Itoa(perPage))
	if createdAfter != nil {
		q.Set("created_after", createdAfter.Format(time.RFC3339))
	}

	endpoint := fmt.Sprintf("%s/api/v4/projects/%v/audit_events?%s", s.client.baseURL, projectID, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab get audit events failed with status %d", resp.StatusCode)
	}

	var events []ProjectAuditEvent
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		return nil, err
	}

	return events, nil
}

// GetPushRules retrieves push rule restrictions configured on the project.
func (s *AnalyticsAndAuditService) GetPushRules(
	ctx context.Context,
	projectID any,
) (*ProjectPushRules, error) {
	endpoint := fmt.Sprintf("%s/api/v4/projects/%v/push_rule", s.client.baseURL, projectID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // No push rules configured
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab get push rules failed with status %d", resp.StatusCode)
	}

	var rules ProjectPushRules
	if err := json.NewDecoder(resp.Body).Decode(&rules); err != nil {
		return nil, err
	}

	return &rules, nil
}

// GetContributorsStats retrieves repository contributor commits and diff counts.
func (s *AnalyticsAndAuditService) GetContributorsStats(
	ctx context.Context,
	projectID any,
) ([]ContributorStats, error) {
	endpoint := fmt.Sprintf("%s/api/v4/projects/%v/repository/contributors", s.client.baseURL, projectID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab get contributors failed with status %d", resp.StatusCode)
	}

	var contributors []ContributorStats
	if err := json.NewDecoder(resp.Body).Decode(&contributors); err != nil {
		return nil, err
	}

	return contributors, nil
}

// GetMergeRequestMetrics retrieves detailed resolution and lifecycle stats for an MR.
func (s *AnalyticsAndAuditService) GetMergeRequestMetrics(
	ctx context.Context,
	projectID any,
	mrIID int,
) (*MergeRequestMetrics, error) {
	endpoint := fmt.Sprintf("%s/api/v4/projects/%v/merge_requests/%d/time_tracking_stats", s.client.baseURL, projectID, mrIID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab get mr metrics failed with status %d", resp.StatusCode)
	}

	var metrics MergeRequestMetrics
	if err := json.NewDecoder(resp.Body).Decode(&metrics); err != nil {
		return nil, err
	}

	return &metrics, nil
}
