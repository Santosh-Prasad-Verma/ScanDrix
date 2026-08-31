// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/pkg/models"
)

// CLIRunner coordinates code review execution across remote API services and local AST fallback engines.
type CLIRunner struct {
	evaluator  *rules.Evaluator
	httpClient *http.Client
}

// NewCLIRunner initializes the runner with the default OWASP rule engine and HTTP transport.
func NewCLIRunner() *CLIRunner {
	ev, _ := rules.NewEvaluator(rules.DefaultCatalog())
	return &CLIRunner{
		evaluator:  ev,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// RunReview processes the provided unified diff and returns comprehensive review findings.
func (r *CLIRunner) RunReview(ctx context.Context, rawDiff string, opts CLIOptions) (*CLIResult, error) {
	startTime := time.Now()

	result := &CLIResult{
		Status:   "passed",
		Findings: make([]models.CodeFinding, 0),
	}

	if strings.TrimSpace(rawDiff) == "" {
		result.Duration = time.Since(startTime)
		return result, nil
	}

	// 1. Check if remote API execution should be attempted
	shouldTryRemote := !opts.Offline && !opts.DryRun && opts.APIBaseURL != "" && (opts.APIKey != "" || opts.AccessToken != "")
	if shouldTryRemote {
		if remoteRes, err := r.runRemoteReview(ctx, rawDiff, opts); err == nil && remoteRes != nil {
			remoteRes.Duration = time.Since(startTime)
			r.evaluateBlockingThreshold(remoteRes, opts)
			return remoteRes, nil
		}
		// Remote failure falls through to high-fidelity local engine
	}

	// 2. Local Review Evaluation Pipeline
	patches, err := diff.ParseUnifiedDiff(strings.NewReader(rawDiff))
	if err != nil {
		return nil, fmt.Errorf("failed parsing diff: %w", err)
	}

	// Filter out files matching .scandrixignore or standard ignore patterns
	ignoreEngine := NewIgnoreEngine(opts.TargetDirectory)
	patches = ignoreEngine.FilterPatches(patches)
	result.FilesReviewed = len(patches)

	if len(patches) == 0 {
		result.Duration = time.Since(startTime)
		return result, nil
	}

	// Evaluate rules against diff hunks
	dummyID := uuid.New()
	findings := r.evaluator.EvaluatePatches(dummyID, dummyID, patches)

	// Filter findings if Focus or RulesOnly is specified
	if opts.Focus != "" {
		focusedFindings := make([]models.CodeFinding, 0)
		focusLower := strings.ToLower(opts.Focus)
		for _, f := range findings {
			if strings.Contains(strings.ToLower(f.Title), focusLower) ||
				strings.Contains(strings.ToLower(f.Description), focusLower) ||
				strings.Contains(strings.ToLower(f.Category), focusLower) {
				focusedFindings = append(focusedFindings, f)
			}
		}
		findings = focusedFindings
	}

	result.Findings = findings
	result.TotalFindings = len(findings)

	// 3. Aggregate severity statistics
	for _, f := range findings {
		switch f.Severity {
		case models.SeverityCritical:
			result.CriticalCount++
		case models.SeverityHigh:
			result.HighCount++
		case models.SeverityMedium:
			result.MediumCount++
		case models.SeverityLow:
			result.LowCount++
		}
	}

	// 4. Apply automatic remediations if --fix is requested
	if opts.Fix && len(findings) > 0 {
		applied := r.applyRemediations(opts.TargetDirectory, findings)
		result.FixesApplied = applied
	}

	// 5. Evaluate blocking threshold and exit codes
	r.evaluateBlockingThreshold(result, opts)
	result.Duration = time.Since(startTime)

	return result, nil
}

func (r *CLIRunner) evaluateBlockingThreshold(result *CLIResult, opts CLIOptions) {
	threshold := opts.SeverityThreshold
	if threshold == "" {
		threshold = models.SeverityHigh
	}

	isBlocking := false
	switch threshold {
	case models.SeverityCritical:
		if result.CriticalCount > 0 {
			isBlocking = true
		}
	case models.SeverityHigh:
		if result.CriticalCount > 0 || result.HighCount > 0 {
			isBlocking = true
		}
	case models.SeverityMedium:
		if result.CriticalCount > 0 || result.HighCount > 0 || result.MediumCount > 0 {
			isBlocking = true
		}
	case models.SeverityLow, models.SeverityInfo:
		if result.TotalFindings > 0 {
			isBlocking = true
		}
	}

	result.IsBlocking = isBlocking
	if isBlocking && !opts.DryRun {
		result.Status = "failed"
		result.ExitCode = 1
	} else {
		result.Status = "passed"
		result.ExitCode = 0
	}
}

func (r *CLIRunner) runRemoteReview(ctx context.Context, rawDiff string, opts CLIOptions) (*CLIResult, error) {
	endpoint := strings.TrimRight(opts.APIBaseURL, "/") + "/api/v1/reviews"
	payload := map[string]any{
		"raw_diff": rawDiff,
		"title":    "ScanDrix CLI Automated Review",
		"options": map[string]any{
			"fast":       opts.Fast,
			"heavy":      opts.Heavy,
			"rules_only": opts.RulesOnly,
			"focus":      opts.Focus,
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ScanDrix-CLI/v1.2.0")

	if opts.APIKey != "" {
		req.Header.Set("X-Team-Key", opts.APIKey)
	} else if opts.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+opts.AccessToken)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("remote API returned status %d", resp.StatusCode)
	}

	var remoteResp struct {
		ReviewID string               `json:"review_id"`
		Status   string               `json:"status"`
		Findings []models.CodeFinding `json:"findings,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&remoteResp); err != nil {
		return nil, err
	}

	res := &CLIResult{
		Status:        remoteResp.Status,
		Findings:      remoteResp.Findings,
		TotalFindings: len(remoteResp.Findings),
	}
	for _, f := range remoteResp.Findings {
		switch f.Severity {
		case models.SeverityCritical:
			res.CriticalCount++
		case models.SeverityHigh:
			res.HighCount++
		case models.SeverityMedium:
			res.MediumCount++
		case models.SeverityLow:
			res.LowCount++
		}
	}
	return res, nil
}

func (r *CLIRunner) applyRemediations(targetDir string, findings []models.CodeFinding) int {
	if targetDir == "" {
		targetDir = "."
	}
	appliedCount := 0

	for _, f := range findings {
		if f.FilePath == "" || (f.Remediation == "" && f.SuggestedDiff == "") {
			continue
		}
		if _, err := ApplyFindingFix(targetDir, f); err == nil {
			appliedCount++
		}
	}
	return appliedCount
}
