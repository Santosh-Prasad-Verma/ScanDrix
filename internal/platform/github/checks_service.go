// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/scandrix/backend/internal/platform/domain/types"
)

// CheckStatus represents pipeline state for a check run.
type CheckStatus string

const (
	CheckStatusQueued     CheckStatus = "queued"
	CheckStatusInProgress CheckStatus = "in_progress"
	CheckStatusCompleted  CheckStatus = "completed"
)

// CheckConclusion represents final outcome of a check run.
type CheckConclusion string

const (
	CheckConclusionSuccess        CheckConclusion = "success"
	CheckConclusionFailure        CheckConclusion = "failure"
	CheckConclusionNeutral        CheckConclusion = "neutral"
	CheckConclusionCancelled      CheckConclusion = "cancelled"
	CheckConclusionSkipped        CheckConclusion = "skipped"
	CheckConclusionTimedOut       CheckConclusion = "timed_out"
	CheckConclusionActionRequired CheckConclusion = "action_required"
)

// CheckAnnotationLevel specifies severity of code review finding.
type CheckAnnotationLevel string

const (
	AnnotationNotice  CheckAnnotationLevel = "notice"
	AnnotationWarning CheckAnnotationLevel = "warning"
	AnnotationFailure CheckAnnotationLevel = "failure"
)

// CheckAnnotation details an inline issue surfaced on a check run.
type CheckAnnotation struct {
	Path            string               `json:"path"`
	StartLine       int                  `json:"start_line"`
	EndLine         int                  `json:"end_line"`
	StartColumn     *int                 `json:"start_column,omitempty"`
	EndColumn       *int                 `json:"end_column,omitempty"`
	AnnotationLevel CheckAnnotationLevel `json:"annotation_level"`
	Message         string               `json:"message"`
	Title           string               `json:"title,omitempty"`
	RawDetails      string               `json:"raw_details,omitempty"`
}

// CheckRunOutput contains markdown review summary and annotations.
type CheckRunOutput struct {
	Title       string            `json:"title"`
	Summary     string            `json:"summary"`
	Text        string            `json:"text,omitempty"`
	Annotations []CheckAnnotation `json:"annotations,omitempty"`
}

// CreateCheckRunParams carries parameters to initiate a check run.
type CreateCheckRunParams struct {
	Owner       string         `json:"owner"`
	Repo        string         `json:"repo"`
	Name        string         `json:"name"`
	HeadSHA     string         `json:"head_sha"`
	Status      CheckStatus    `json:"status"`
	StartedAt   time.Time      `json:"started_at"`
	DetailsURL  string         `json:"details_url,omitempty"`
	ExternalID  string         `json:"external_id,omitempty"`
	Output      *CheckRunOutput `json:"output,omitempty"`
}

// UpdateCheckRunParams carries parameters to conclude or update a check run.
type UpdateCheckRunParams struct {
	Owner       string           `json:"owner"`
	Repo        string           `json:"repo"`
	CheckRunID  int64            `json:"check_run_id"`
	Status      CheckStatus      `json:"status"`
	Conclusion  *CheckConclusion `json:"conclusion,omitempty"`
	CompletedAt *time.Time       `json:"completed_at,omitempty"`
	Output      *CheckRunOutput  `json:"output,omitempty"`
}

// CheckRunResponse represents GitHub check run response payload.
type CheckRunResponse struct {
	ID          int64            `json:"id"`
	HeadSHA     string           `json:"head_sha"`
	Name        string           `json:"name"`
	Status      CheckStatus      `json:"status"`
	Conclusion  *CheckConclusion `json:"conclusion"`
	HTMLURL     string           `json:"html_url"`
	DetailsURL  string           `json:"details_url"`
	ExternalID  string           `json:"external_id"`
}

// IChecksService defines the checks API contract for automated reviews.
type IChecksService interface {
	FindCheckRun(ctx context.Context, data types.OrganizationAndTeamData, owner, repo, headSHA, name string) (*int64, error)
	CreateCheckRun(ctx context.Context, data types.OrganizationAndTeamData, params CreateCheckRunParams) (*CheckRunResponse, error)
	UpdateCheckRun(ctx context.Context, data types.OrganizationAndTeamData, params UpdateCheckRunParams) (*CheckRunResponse, error)
}

// GithubChecksService provides GitHub Checks API operations.
type GithubChecksService struct {
	client     *Client
	httpClient *http.Client
	logger     *slog.Logger
}

// NewGithubChecksService creates a new GitHub checks service.
func NewGithubChecksService(client *Client, logger *slog.Logger) *GithubChecksService {
	if logger == nil {
		logger = slog.Default()
	}
	return &GithubChecksService{
		client:     client,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		logger:     logger,
	}
}

// FindCheckRun searches for an existing in-progress check run on a commit ref.
func (s *GithubChecksService) FindCheckRun(
	ctx context.Context,
	data types.OrganizationAndTeamData,
	owner, repo, headSHA, name string,
) (*int64, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/commits/%s/check-runs?check_name=%s&filter=latest&per_page=1",
		owner, repo, headSHA, name)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	s.applyAuth(req, data)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list check-runs returned HTTP %d", resp.StatusCode)
	}

	var list struct {
		CheckRuns []struct {
			ID     int64       `json:"id"`
			Status CheckStatus `json:"status"`
		} `json:"check_runs"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, err
	}

	if len(list.CheckRuns) == 0 {
		return nil, nil
	}

	first := list.CheckRuns[0]
	// If completed, cannot be reopened in GitHub — return nil to signal fresh creation
	if first.Status == CheckStatusCompleted {
		return nil, nil
	}

	return &first.ID, nil
}

// CreateCheckRun registers a new check run on GitHub.
func (s *GithubChecksService) CreateCheckRun(
	ctx context.Context,
	data types.OrganizationAndTeamData,
	params CreateCheckRunParams,
) (*CheckRunResponse, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/check-runs", params.Owner, params.Repo)

	body := map[string]any{
		"name":       params.Name,
		"head_sha":   params.HeadSHA,
		"status":     params.Status,
		"started_at": params.StartedAt.Format(time.RFC3339),
	}
	if params.DetailsURL != "" {
		body["details_url"] = params.DetailsURL
	}
	if params.ExternalID != "" {
		body["external_id"] = params.ExternalID
	}
	if params.Output != nil {
		body["output"] = params.Output
	}

	jsonBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, err
	}
	s.applyAuth(req, data)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("create check-run returned HTTP %d", resp.StatusCode)
	}

	var created CheckRunResponse
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return nil, err
	}

	return &created, nil
}

// UpdateCheckRun modifies status, conclusion, and annotations on an existing check run.
func (s *GithubChecksService) UpdateCheckRun(
	ctx context.Context,
	data types.OrganizationAndTeamData,
	params UpdateCheckRunParams,
) (*CheckRunResponse, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/check-runs/%d",
		params.Owner, params.Repo, params.CheckRunID)

	body := map[string]any{
		"status": params.Status,
	}
	if params.Conclusion != nil {
		body["conclusion"] = *params.Conclusion
	}
	if params.CompletedAt != nil {
		body["completed_at"] = params.CompletedAt.Format(time.RFC3339)
	}
	if params.Output != nil {
		body["output"] = params.Output
	}

	jsonBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "PATCH", url, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, err
	}
	s.applyAuth(req, data)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update check-run returned HTTP %d", resp.StatusCode)
	}

	var updated CheckRunResponse
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		return nil, err
	}

	return &updated, nil
}

func (s *GithubChecksService) applyAuth(req *http.Request, data types.OrganizationAndTeamData) {
	token := data.AuthToken
	if token == "" && s.client != nil {
		token = s.client.token
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}
