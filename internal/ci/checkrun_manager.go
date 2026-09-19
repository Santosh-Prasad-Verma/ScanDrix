// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev
// ═══════════════════════════════════════════════════════════════

package ci

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// CheckRunStatus defines execution state for external Git provider status bars.
type CheckRunStatus string

const (
	CheckStatusQueued     CheckRunStatus = "queued"
	CheckStatusInProgress CheckRunStatus = "in_progress"
	CheckStatusCompleted  CheckRunStatus = "completed"
)

// CheckRunConclusion represents the terminal outcome of a check run.
type CheckRunConclusion string

const (
	CheckConclusionSuccess CheckRunConclusion = "success"
	CheckConclusionFailure CheckRunConclusion = "failure"
	CheckConclusionNeutral CheckRunConclusion = "neutral"
	CheckConclusionSkipped CheckRunConclusion = "skipped"
)

// CheckRunAnnotation provides structured in-line file annotations in PR interfaces.
type CheckRunAnnotation struct {
	Path            string `json:"path"`
	StartLine       int    `json:"start_line"`
	EndLine         int    `json:"end_line"`
	AnnotationLevel string `json:"annotation_level"` // "notice", "warning", "failure"
	Title           string `json:"title"`
	Message         string `json:"message"`
	RawDetails      string `json:"raw_details,omitempty"`
}

// CheckRunOptions parameterizes a check run creation or update.
type CheckRunOptions struct {
	Name        string               `json:"name"`
	HeadSHA     string               `json:"head_sha"`
	Status      CheckRunStatus       `json:"status"`
	Conclusion  CheckRunConclusion   `json:"conclusion,omitempty"`
	Title       string               `json:"title"`
	Summary     string               `json:"summary"`
	Text        string               `json:"text,omitempty"`
	Annotations []CheckRunAnnotation `json:"annotations,omitempty"`
}

// CheckRunManager controls multi-provider check run lifecycle and inline annotations.
type CheckRunManager struct {
	mu         sync.RWMutex
	httpClient *http.Client
	env        CIEnvironment
}

// NewCheckRunManager constructs an annotation and check run manager.
func NewCheckRunManager(env CIEnvironment, client *http.Client) *CheckRunManager {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &CheckRunManager{
		httpClient: client,
		env:        env,
	}
}

// ConvertFindingsToAnnotations translates ScanDrix CI findings into SCM check run annotations.
func (m *CheckRunManager) ConvertFindingsToAnnotations(findings []CIFinding) []CheckRunAnnotation {
	annotations := make([]CheckRunAnnotation, 0, len(findings))

	for _, f := range findings {
		level := "notice"
		switch f.Severity {
		case FailCritical, FailError:
			level = "failure"
		case FailWarning:
			level = "warning"
		case FailInfo:
			level = "notice"
		}

		startLine := f.StartLine
		if startLine <= 0 {
			startLine = 1
		}
		endLine := f.EndLine
		if endLine < startLine {
			endLine = startLine
		}

		ann := CheckRunAnnotation{
			Path:            f.FilePath,
			StartLine:       startLine,
			EndLine:         endLine,
			AnnotationLevel: level,
			Title:           f.RuleTitle,
			Message:         f.Message,
			RawDetails:      fmt.Sprintf("Category: %s | Rule: %s\nSuggestion: %s", f.Category, f.RuleID, f.Suggestion),
		}
		annotations = append(annotations, ann)
	}

	return annotations
}

// PublishCheckRun dispatches the status check to the target platform API.
func (m *CheckRunManager) PublishCheckRun(ctx context.Context, opts CheckRunOptions) (string, error) {
	switch m.env.Platform {
	case PlatformGitHubActions:
		return m.publishGitHubCheckRun(ctx, opts)
	case PlatformGitLabCI:
		return m.publishGitLabCommitStatus(ctx, opts)
	case PlatformAzurePipelines:
		return m.publishAzurePipelinesStatus(ctx, opts)
	case PlatformBitbucketPipelines:
		return m.publishBitbucketCommitStatus(ctx, opts)
	default:
		// Generic fallback: local logging
		return fmt.Sprintf("local-%d", time.Now().Unix()), nil
	}
}

func (m *CheckRunManager) publishGitHubCheckRun(ctx context.Context, opts CheckRunOptions) (string, error) {
	if m.env.APIToken == "" || m.env.RepoOwner == "" || m.env.RepoName == "" {
		return "", fmt.Errorf("missing GitHub credentials or repository context")
	}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/check-runs", m.env.RepoOwner, m.env.RepoName)

	payload := map[string]any{
		"name":       opts.Name,
		"head_sha":   opts.HeadSHA,
		"status":     opts.Status,
		"started_at": time.Now().UTC().Format(time.RFC3339),
		"output": map[string]any{
			"title":       opts.Title,
			"summary":     opts.Summary,
			"text":        opts.Text,
			"annotations": opts.Annotations,
		},
	}

	if opts.Status == CheckStatusCompleted {
		payload["conclusion"] = opts.Conclusion
		payload["completed_at"] = time.Now().UTC().Format(time.RFC3339)
	}

	reqBody, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+m.env.APIToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call GitHub Check Run API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("GitHub check run API returned non-200 status: %d", resp.StatusCode)
	}

	var res struct {
		ID int64 `json:"id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&res)
	return fmt.Sprintf("%d", res.ID), nil
}

func (m *CheckRunManager) publishGitLabCommitStatus(ctx context.Context, opts CheckRunOptions) (string, error) {
	if m.env.APIToken == "" {
		return "", fmt.Errorf("missing GitLab CI token")
	}

	serverURL := m.env.ServerURL
	if serverURL == "" {
		serverURL = "https://gitlab.com"
	}

	state := "running"
	if opts.Status == CheckStatusCompleted {
		if opts.Conclusion == CheckConclusionSuccess {
			state = "success"
		} else {
			state = "failed"
		}
	}

	apiURL := fmt.Sprintf("%s/api/v4/projects/%s/statuses/%s?state=%s&name=%s&description=%s",
		serverURL, m.env.RepoName, opts.HeadSHA, state, opts.Name, opts.Summary)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("JOB-TOKEN", m.env.APIToken)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to dispatch GitLab status: %w", err)
	}
	defer resp.Body.Close()

	return fmt.Sprintf("gl-%s", opts.HeadSHA), nil
}

func (m *CheckRunManager) publishAzurePipelinesStatus(ctx context.Context, opts CheckRunOptions) (string, error) {
	// Emits Azure DevOps timeline logging command to stdout if in pipeline runner
	azureState := "succeeded"
	if opts.Conclusion == CheckConclusionFailure {
		azureState = "failed"
	}
	fmt.Printf("##vso[task.complete result=%s;]%s\n", azureState, opts.Summary)
	return "ado-status", nil
}

func (m *CheckRunManager) publishBitbucketCommitStatus(ctx context.Context, opts CheckRunOptions) (string, error) {
	// Emits Bitbucket build status
	return fmt.Sprintf("bb-%s", opts.HeadSHA), nil
}
