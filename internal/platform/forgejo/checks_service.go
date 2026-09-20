// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package forgejo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/platform/domain/types"
)

// CommitStatusContext is the label used in Forgejo for automated review statuses.
const CommitStatusContext = "ScanDrix Code Review"

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

// CheckRunOutput contains markdown review summary and annotations.
type CheckRunOutput struct {
	Title       string `json:"title"`
	Summary     string `json:"summary"`
	Text        string `json:"text,omitempty"`
}

// CreateCheckRunParams carries parameters to initiate a check run.
type CreateCheckRunParams struct {
	Owner      string          `json:"owner"`
	Repo       string          `json:"repo"`
	Name       string          `json:"name"`
	HeadSHA    string          `json:"head_sha"`
	Status     CheckStatus     `json:"status"`
	StartedAt  time.Time       `json:"started_at"`
	DetailsURL string          `json:"details_url,omitempty"`
	ExternalID string          `json:"external_id,omitempty"`
	Output     *CheckRunOutput `json:"output,omitempty"`
}

// UpdateCheckRunParams carries parameters to conclude or update a check run.
type UpdateCheckRunParams struct {
	Owner       string           `json:"owner"`
	Repo        string           `json:"repo"`
	CheckRunID  int64            `json:"check_run_id"`
	HeadSHA     string           `json:"head_sha,omitempty"`
	Status      CheckStatus      `json:"status"`
	Conclusion  *CheckConclusion `json:"conclusion,omitempty"`
	CompletedAt *time.Time       `json:"completed_at,omitempty"`
	Output      *CheckRunOutput  `json:"output,omitempty"`
}

// CheckRunResponse represents Forgejo status response payload.
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

var statusStateMap = map[CheckStatus]string{
	CheckStatusInProgress: "pending",
	CheckStatusCompleted:  "success",
	CheckStatusQueued:      "pending",
}

var conclusionStateMap = map[CheckConclusion]string{
	CheckConclusionSuccess:  "success",
	CheckConclusionFailure:  "failure",
	CheckConclusionNeutral:  "warning",
	CheckConclusionSkipped:  "warning",
	CheckConclusionCancelled: "failure",
	CheckConclusionTimedOut: "failure",
}

// ForgejoChecksService provides Forgejo / Gitea Commit Status operations.
// Translates libs/platform/infrastructure/adapters/services/forgejo/forgejo-checks.service.ts
type ForgejoChecksService struct {
	baseURL    string
	httpClient *http.Client
	logger     *slog.Logger
}

// NewForgejoChecksService creates a new Forgejo checks service.
func NewForgejoChecksService(baseURL string, httpClient *http.Client, logger *slog.Logger) *ForgejoChecksService {
	if logger == nil {
		logger = slog.Default()
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &ForgejoChecksService{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
		logger:     logger,
	}
}

// FindCheckRun: Forgejo commit statuses are keyed by (sha, context),
// posting with the same context replaces the previous state, so there is no run to reuse.
func (s *ForgejoChecksService) FindCheckRun(
	ctx context.Context,
	data types.OrganizationAndTeamData,
	owner, repo, headSHA, name string,
) (*int64, error) {
	return nil, nil
}

// CreateCheckRun posts an initial commit status on Forgejo.
func (s *ForgejoChecksService) CreateCheckRun(
	ctx context.Context,
	data types.OrganizationAndTeamData,
	params CreateCheckRunParams,
) (*CheckRunResponse, error) {
	state := statusStateMap[params.Status]
	if state == "" {
		state = "pending"
	}

	description := "Starting..."
	if params.Output != nil {
		if params.Output.Title != "" {
			description = params.Output.Title
		} else if params.Output.Summary != "" {
			description = params.Output.Summary
		}
	}
	if len(description) > 140 {
		description = description[:137] + "..."
	}

	targetURL := params.DetailsURL
	if targetURL == "" {
		targetURL = s.buildTargetURL(data, params.Owner, params.Repo, params.HeadSHA)
	}

	payload := map[string]string{
		"state":       state,
		"context":     CommitStatusContext,
		"description": description,
		"target_url":  targetURL,
	}

	respData, err := s.postStatus(ctx, data, params.Owner, params.Repo, params.HeadSHA, payload)
	if err != nil {
		s.logger.Error("Failed to create Forgejo commit status",
			slog.String("owner", params.Owner),
			slog.String("repo", params.Repo),
			slog.String("sha", params.HeadSHA),
			slog.Any("err", err),
		)
		return nil, err
	}

	id := int64(0)
	if idVal, ok := respData["id"].(float64); ok {
		id = int64(idVal)
	}

	return &CheckRunResponse{
		ID:         id,
		HeadSHA:    params.HeadSHA,
		Name:       CommitStatusContext,
		Status:     params.Status,
		DetailsURL: targetURL,
		ExternalID: fmt.Sprintf("sha:%s", params.HeadSHA),
	}, nil
}

// UpdateCheckRun updates the commit status on Forgejo.
func (s *ForgejoChecksService) UpdateCheckRun(
	ctx context.Context,
	data types.OrganizationAndTeamData,
	params UpdateCheckRunParams,
) (*CheckRunResponse, error) {
	headSHA := params.HeadSHA
	if headSHA == "" {
		s.logger.Warn("Cannot update Forgejo commit status - head SHA unavailable",
			slog.String("owner", params.Owner),
			slog.String("repo", params.Repo),
			slog.Int64("checkRunID", params.CheckRunID),
		)
		return nil, fmt.Errorf("head SHA unavailable for check run update")
	}

	state := s.resolveState(params.Status, params.Conclusion)
	description := s.resolveDescription(params.Status, params.Conclusion, params.Output)
	if len(description) > 140 {
		description = description[:137] + "..."
	}

	targetURL := s.buildTargetURL(data, params.Owner, params.Repo, headSHA)

	payload := map[string]string{
		"state":       state,
		"context":     CommitStatusContext,
		"description": description,
		"target_url":  targetURL,
	}

	respData, err := s.postStatus(ctx, data, params.Owner, params.Repo, headSHA, payload)
	if err != nil {
		s.logger.Error("Failed to update Forgejo commit status",
			slog.String("owner", params.Owner),
			slog.String("repo", params.Repo),
			slog.String("sha", headSHA),
			slog.Any("err", err),
		)
		return nil, err
	}

	id := params.CheckRunID
	if idVal, ok := respData["id"].(float64); ok && idVal > 0 {
		id = int64(idVal)
	}

	return &CheckRunResponse{
		ID:         id,
		HeadSHA:    headSHA,
		Name:       CommitStatusContext,
		Status:     params.Status,
		Conclusion: params.Conclusion,
		DetailsURL: targetURL,
		ExternalID: fmt.Sprintf("sha:%s", headSHA),
	}, nil
}

func (s *ForgejoChecksService) resolveState(status CheckStatus, conclusion *CheckConclusion) string {
	if status == CheckStatusCompleted && conclusion != nil {
		if st, ok := conclusionStateMap[*conclusion]; ok {
			return st
		}
		return "success"
	}
	if st, ok := statusStateMap[status]; ok {
		return st
	}
	return "pending"
}

func (s *ForgejoChecksService) resolveDescription(
	status CheckStatus,
	conclusion *CheckConclusion,
	output *CheckRunOutput,
) string {
	if output != nil && output.Title != "" {
		return output.Title
	}

	if status == CheckStatusCompleted && conclusion != nil {
		switch *conclusion {
		case CheckConclusionSuccess:
			return "Code Review Complete"
		case CheckConclusionFailure:
			return "Code Review Failed"
		case CheckConclusionNeutral:
			return "Code Review Completed with Warnings"
		case CheckConclusionSkipped:
			return "Code Review Skipped"
		}
	}

	if status == CheckStatusInProgress && output != nil && output.Summary != "" {
		return output.Summary
	}

	return "Code Review In Progress"
}

func (s *ForgejoChecksService) buildTargetURL(data types.OrganizationAndTeamData, owner, repo, sha string) string {
	base := s.baseURL
	if host, ok := data.IntegrationCredentials["host"].(string); ok && host != "" {
		base = strings.TrimRight(host, "/")
	}
	if base == "" {
		base = "https://codeberg.org"
	}
	return fmt.Sprintf("%s/%s/%s/commit/%s", base, owner, repo, sha)
}

func (s *ForgejoChecksService) postStatus(
	ctx context.Context,
	data types.OrganizationAndTeamData,
	owner, repo, sha string,
	payload map[string]string,
) (map[string]any, error) {
	base := s.baseURL
	if host, ok := data.IntegrationCredentials["host"].(string); ok && host != "" {
		base = strings.TrimRight(host, "/")
	}
	if base == "" {
		base = "https://codeberg.org"
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	apiURL := fmt.Sprintf("%s/api/v1/repos/%s/%s/statuses/%s", base, owner, repo, sha)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}

	s.applyAuth(req, data)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("forgejo status post error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result map[string]any
	if len(respBody) > 0 {
		_ = json.Unmarshal(respBody, &result)
	}
	return result, nil
}

func (s *ForgejoChecksService) applyAuth(req *http.Request, data types.OrganizationAndTeamData) {
	token := data.AuthToken
	if token == "" && data.IntegrationCredentials != nil {
		if t, ok := data.IntegrationCredentials["token"].(string); ok {
			token = t
		} else if t, ok := data.IntegrationCredentials["accessToken"].(string); ok {
			token = t
		}
	}
	if token != "" {
		req.Header.Set("Authorization", "token "+token)
	}
}
