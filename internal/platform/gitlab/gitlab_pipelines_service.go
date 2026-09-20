package gitlab

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

// PipelineStatus represents the state of a GitLab CI/CD pipeline or job.
type PipelineStatus string

const (
	PipelineStatusCreated   PipelineStatus = "created"
	PipelineStatusWaiting   PipelineStatus = "waiting_for_resource"
	PipelineStatusPreparing PipelineStatus = "preparing"
	PipelineStatusPending   PipelineStatus = "pending"
	PipelineStatusRunning   PipelineStatus = "running"
	PipelineStatusSuccess   PipelineStatus = "success"
	PipelineStatusFailed    PipelineStatus = "failed"
	PipelineStatusCanceled  PipelineStatus = "canceled"
	PipelineStatusSkipped   PipelineStatus = "skipped"
	PipelineStatusManual    PipelineStatus = "manual"
	PipelineStatusScheduled PipelineStatus = "scheduled"
)

// CommitStatusState represents the state of a commit status check.
type CommitStatusState string

const (
	CommitStatusPending  CommitStatusState = "pending"
	CommitStatusRunning  CommitStatusState = "running"
	CommitStatusSuccess  CommitStatusState = "success"
	CommitStatusFailed   CommitStatusState = "failed"
	CommitStatusCanceled CommitStatusState = "canceled"
)

// CommitStatusRequest defines parameters for posting a commit status check.
type CommitStatusRequest struct {
	State       CommitStatusState `json:"state"`
	Ref         string            `json:"ref,omitempty"`
	Name        string            `json:"name,omitempty"`
	TargetURL   string            `json:"target_url,omitempty"`
	Description string            `json:"description,omitempty"`
	Coverage    *float64          `json:"coverage,omitempty"`
	PipelineID  *int              `json:"pipeline_id,omitempty"`
}

// CommitStatusResponse models the response from the commit statuses endpoint.
type CommitStatusResponse struct {
	ID          int               `json:"id"`
	SHA         string            `json:"sha"`
	Ref         string            `json:"ref"`
	Status      CommitStatusState `json:"status"`
	Name        string            `json:"name"`
	TargetURL   string            `json:"target_url"`
	Description string            `json:"description"`
	CreatedAt   time.Time         `json:"created_at"`
	StartedAt   *time.Time        `json:"started_at,omitempty"`
	FinishedAt  *time.Time        `json:"finished_at,omitempty"`
	Author      GitLabUser        `json:"author"`
	Coverage    *float64          `json:"coverage,omitempty"`
}

// PipelineInfo models a GitLab CI/CD pipeline summary.
type PipelineInfo struct {
	ID         int            `json:"id"`
	IID        int            `json:"iid"`
	ProjectID  int            `json:"project_id"`
	SHA        string         `json:"sha"`
	Ref        string         `json:"ref"`
	Status     PipelineStatus `json:"status"`
	Source     string         `json:"source"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	WebURL     string         `json:"web_url"`
	Duration   int            `json:"duration"`
	QueuedDuration int        `json:"queued_duration"`
	Coverage   string         `json:"coverage"`
}

// PipelineJob models an individual job within a GitLab CI pipeline.
type PipelineJob struct {
	ID             int            `json:"id"`
	Name           string         `json:"name"`
	Stage          string         `json:"stage"`
	Status         PipelineStatus `json:"status"`
	Ref            string         `json:"ref"`
	Tag            bool           `json:"tag"`
	Coverage       *float64       `json:"coverage,omitempty"`
	AllowFailure   bool           `json:"allow_failure"`
	CreatedAt      time.Time      `json:"created_at"`
	StartedAt      *time.Time     `json:"started_at,omitempty"`
	FinishedAt     *time.Time     `json:"finished_at,omitempty"`
	Duration       float64        `json:"duration"`
	QueuedDuration float64        `json:"queued_duration"`
	WebURL         string         `json:"web_url"`
}

// ApprovalRule models a GitLab merge request approval rule.
type ApprovalRule struct {
	ID                   int          `json:"id"`
	Name                 string       `json:"name"`
	RuleType             string       `json:"rule_type"`
	EligibleApprovers    []GitLabUser `json:"eligible_approvers"`
	ApprovalsRequired    int          `json:"approvals_required"`
	Users                []GitLabUser `json:"users"`
	Groups               []GitLabGroup `json:"groups"`
	ContainsHiddenGroups bool         `json:"contains_hidden_groups"`
	Section              string       `json:"section,omitempty"`
}

// GitLabUser represents minimal user information in GitLab.
type GitLabUser struct {
	ID        int    `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	State     string `json:"state"`
	AvatarURL string `json:"avatar_url"`
	WebURL    string `json:"web_url"`
}

// GitLabGroup represents minimal group information.
type GitLabGroup struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	FullName string `json:"full_name"`
}

// PipelineBridge models a cross-project pipeline trigger bridge.
type PipelineBridge struct {
	ID           int            `json:"id"`
	Name         string         `json:"name"`
	Stage        string         `json:"stage"`
	Status       PipelineStatus `json:"status"`
	CreatedAt    time.Time      `json:"created_at"`
	StartedAt    *time.Time     `json:"started_at,omitempty"`
	FinishedAt   *time.Time     `json:"finished_at,omitempty"`
	DownstreamPipeline *PipelineInfo `json:"downstream_pipeline,omitempty"`
}

// GitLabPipelinesService provides CI/CD pipeline and approval rule management for GitLab v4.
type GitLabPipelinesService struct {
	httpClient *http.Client
	baseURL    string
	token      string
}

// NewGitLabPipelinesService creates a new GitLab pipelines service instance.
func NewGitLabPipelinesService(token string, httpClient *http.Client, baseURL ...string) *GitLabPipelinesService {
	url := "https://gitlab.com/api/v4"
	if len(baseURL) > 0 && baseURL[0] != "" {
		url = strings.TrimRight(baseURL[0], "/")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &GitLabPipelinesService{
		httpClient: httpClient,
		baseURL:    url,
		token:      token,
	}
}

func (s *GitLabPipelinesService) doRequest(ctx context.Context, method, endpoint string, body any) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	reqURL := fmt.Sprintf("%s%s", s.baseURL, endpoint)
	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}

	if s.token != "" {
		req.Header.Set("PRIVATE-TOKEN", s.token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	return resp, nil
}

// CreateCommitStatus posts a commit status to GitLab for the given SHA.
func (s *GitLabPipelinesService) CreateCommitStatus(ctx context.Context, projectID string, sha string, req CommitStatusRequest) (*CommitStatusResponse, error) {
	endpoint := fmt.Sprintf("/projects/%s/statuses/%s", url.PathEscape(projectID), sha)
	resp, err := s.doRequest(ctx, http.MethodPost, endpoint, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gitlab create commit status error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var status CommitStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("failed to decode commit status response: %w", err)
	}
	return &status, nil
}

// ListCommitStatuses lists all commit statuses for a given commit SHA.
func (s *GitLabPipelinesService) ListCommitStatuses(ctx context.Context, projectID string, sha string) ([]CommitStatusResponse, error) {
	endpoint := fmt.Sprintf("/projects/%s/repository/commits/%s/statuses", url.PathEscape(projectID), sha)
	resp, err := s.doRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gitlab list commit statuses error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var statuses []CommitStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&statuses); err != nil {
		return nil, fmt.Errorf("failed to decode commit statuses: %w", err)
	}
	return statuses, nil
}

// TriggerPipeline triggers a new CI/CD pipeline for the given project and branch/tag ref.
func (s *GitLabPipelinesService) TriggerPipeline(ctx context.Context, projectID string, ref string, variables map[string]string) (*PipelineInfo, error) {
	payload := map[string]any{
		"ref": ref,
	}
	if len(variables) > 0 {
		var varsList []map[string]string
		for k, v := range variables {
			varsList = append(varsList, map[string]string{"key": k, "value": v, "variable_type": "env_var"})
		}
		payload["variables"] = varsList
	}

	endpoint := fmt.Sprintf("/projects/%s/pipeline", url.PathEscape(projectID))
	resp, err := s.doRequest(ctx, http.MethodPost, endpoint, payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gitlab trigger pipeline error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var pipeline PipelineInfo
	if err := json.NewDecoder(resp.Body).Decode(&pipeline); err != nil {
		return nil, fmt.Errorf("failed to decode pipeline response: %w", err)
	}
	return &pipeline, nil
}

// GetPipeline returns detailed pipeline metadata.
func (s *GitLabPipelinesService) GetPipeline(ctx context.Context, projectID string, pipelineID int) (*PipelineInfo, error) {
	endpoint := fmt.Sprintf("/projects/%s/pipelines/%d", url.PathEscape(projectID), pipelineID)
	resp, err := s.doRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gitlab get pipeline error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var pipeline PipelineInfo
	if err := json.NewDecoder(resp.Body).Decode(&pipeline); err != nil {
		return nil, fmt.Errorf("failed to decode pipeline: %w", err)
	}
	return &pipeline, nil
}

// ListPipelineJobs returns all jobs for a pipeline.
func (s *GitLabPipelinesService) ListPipelineJobs(ctx context.Context, projectID string, pipelineID int, scope string) ([]PipelineJob, error) {
	endpoint := fmt.Sprintf("/projects/%s/pipelines/%d/jobs", url.PathEscape(projectID), pipelineID)
	if scope != "" {
		endpoint += "?scope=" + url.QueryEscape(scope)
	}
	resp, err := s.doRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gitlab list pipeline jobs error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var jobs []PipelineJob
	if err := json.NewDecoder(resp.Body).Decode(&jobs); err != nil {
		return nil, fmt.Errorf("failed to decode pipeline jobs: %w", err)
	}
	return jobs, nil
}

// RetryPipeline retries failed jobs in a pipeline.
func (s *GitLabPipelinesService) RetryPipeline(ctx context.Context, projectID string, pipelineID int) (*PipelineInfo, error) {
	endpoint := fmt.Sprintf("/projects/%s/pipelines/%d/retry", url.PathEscape(projectID), pipelineID)
	resp, err := s.doRequest(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gitlab retry pipeline error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var pipeline PipelineInfo
	if err := json.NewDecoder(resp.Body).Decode(&pipeline); err != nil {
		return nil, fmt.Errorf("failed to decode pipeline retry response: %w", err)
	}
	return &pipeline, nil
}

// CancelPipeline cancels a running pipeline.
func (s *GitLabPipelinesService) CancelPipeline(ctx context.Context, projectID string, pipelineID int) (*PipelineInfo, error) {
	endpoint := fmt.Sprintf("/projects/%s/pipelines/%d/cancel", url.PathEscape(projectID), pipelineID)
	resp, err := s.doRequest(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gitlab cancel pipeline error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var pipeline PipelineInfo
	if err := json.NewDecoder(resp.Body).Decode(&pipeline); err != nil {
		return nil, fmt.Errorf("failed to decode pipeline cancel response: %w", err)
	}
	return &pipeline, nil
}

// ListMergeRequestApprovalRules retrieves all approval rules configured for a merge request.
func (s *GitLabPipelinesService) ListMergeRequestApprovalRules(ctx context.Context, projectID string, mrIID int) ([]ApprovalRule, error) {
	endpoint := fmt.Sprintf("/projects/%s/merge_requests/%d/approval_rules", url.PathEscape(projectID), mrIID)
	resp, err := s.doRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gitlab list approval rules error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var rules []ApprovalRule
	if err := json.NewDecoder(resp.Body).Decode(&rules); err != nil {
		return nil, fmt.Errorf("failed to decode approval rules: %w", err)
	}
	return rules, nil
}

// CreateMergeRequestApprovalRule creates a custom approval rule on a merge request.
func (s *GitLabPipelinesService) CreateMergeRequestApprovalRule(ctx context.Context, projectID string, mrIID int, name string, approvalsRequired int, userIDs []int) (*ApprovalRule, error) {
	payload := map[string]any{
		"name":               name,
		"approvals_required": approvalsRequired,
		"user_ids":           userIDs,
	}

	endpoint := fmt.Sprintf("/projects/%s/merge_requests/%d/approval_rules", url.PathEscape(projectID), mrIID)
	resp, err := s.doRequest(ctx, http.MethodPost, endpoint, payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gitlab create approval rule error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var rule ApprovalRule
	if err := json.NewDecoder(resp.Body).Decode(&rule); err != nil {
		return nil, fmt.Errorf("failed to decode approval rule response: %w", err)
	}
	return &rule, nil
}

// DeleteMergeRequestApprovalRule removes an approval rule from a merge request.
func (s *GitLabPipelinesService) DeleteMergeRequestApprovalRule(ctx context.Context, projectID string, mrIID int, ruleID int) error {
	endpoint := fmt.Sprintf("/projects/%s/merge_requests/%d/approval_rules/%d", url.PathEscape(projectID), mrIID, ruleID)
	resp, err := s.doRequest(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("gitlab delete approval rule error (status %d): %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// ListPipelineBridges retrieves downstream pipeline bridges for a given pipeline.
func (s *GitLabPipelinesService) ListPipelineBridges(ctx context.Context, projectID string, pipelineID int) ([]PipelineBridge, error) {
	endpoint := fmt.Sprintf("/projects/%s/pipelines/%d/bridges", url.PathEscape(projectID), pipelineID)
	resp, err := s.doRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gitlab list pipeline bridges error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var bridges []PipelineBridge
	if err := json.NewDecoder(resp.Body).Decode(&bridges); err != nil {
		return nil, fmt.Errorf("failed to decode pipeline bridges: %w", err)
	}
	return bridges, nil
}

// FormatPipelineDuration returns human-readable formatted string for seconds.
func FormatPipelineDuration(seconds int) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	minutes := seconds / 60
	remSeconds := seconds % 60
	if minutes < 60 {
		return fmt.Sprintf("%dm %ds", minutes, remSeconds)
	}
	hours := minutes / 60
	remMinutes := minutes % 60
	return fmt.Sprintf("%dh %dm %ds", hours, remMinutes, remSeconds)
}

// ParseGitLabCoverage parses float coverage value from string like "85.5%".
func ParseGitLabCoverage(raw string) (float64, error) {
	trimmed := strings.TrimSuffix(strings.TrimSpace(raw), "%")
	return strconv.ParseFloat(trimmed, 64)
}
