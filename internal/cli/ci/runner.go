// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package ci

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/scandrix/backend/pkg/models"
)

// CIOptions defines parameters for running reviews in CI pipelines.
type CIOptions struct {
	FailOnSeverity string // critical, high, medium, low
	OutputFormat   string // annotations, gitlab, markdown, json
	ReportPath     string // optional file path to save artifact
}

// CIRunner orchestrates continuous integration review evaluations.
type CIRunner struct {
	context *CIContext
}

// NewCIRunner creates a new CI pipeline runner.
func NewCIRunner() *CIRunner {
	return &CIRunner{
		context: DetectCI(),
	}
}

// EvaluateFindings assesses findings against severity gates and emits pipeline artifacts.
func (r *CIRunner) EvaluateFindings(ctx context.Context, findings []models.CodeFinding, opts CIOptions, out io.Writer) (bool, error) {
	passGate := r.isGateSatisfied(findings, opts.FailOnSeverity)

	// Emit provider-specific annotations
	if r.context.Provider == ProviderGitHubActions || opts.OutputFormat == "annotations" {
		EmitGitHubAnnotations(out, findings)
	}

	// Always generate and persist Step Summary if in GitHub Actions
	summaryMD := GenerateMarkdownSummary(r.context, findings, passGate)
	if r.context.StepSummary != "" {
		_ = WriteStepSummaryFile(summaryMD)
	}

	// Generate specific artifacts if requested
	if opts.OutputFormat == "gitlab" || (r.context.Provider == ProviderGitLabCI && opts.ReportPath != "") {
		targetFile := opts.ReportPath
		if targetFile == "" {
			targetFile = "gl-code-quality-report.json"
		}
		data, err := FormatGitLabCodeQualityJSON(findings)
		if err == nil {
			_ = os.WriteFile(targetFile, data, 0600)
		}
	} else if opts.ReportPath != "" {
		_ = os.WriteFile(opts.ReportPath, []byte(summaryMD), 0600)
	}

	if !passGate {
		return false, fmt.Errorf("ScanDrix CI gate failed: issues detected at or above severity %s", opts.FailOnSeverity)
	}

	return true, nil
}

func (r *CIRunner) isGateSatisfied(findings []models.CodeFinding, failOn string) bool {
	if failOn == "" {
		return true
	}

	targetSev := strings.ToLower(strings.TrimSpace(failOn))
	targetRank := severityRank(targetSev)

	for _, f := range findings {
		rank := severityRank(string(f.Severity))
		if rank >= targetRank {
			return false
		}
	}

	return true
}

func severityRank(sev string) int {
	switch strings.ToLower(sev) {
	case "critical":
		return 4
	case "high", "error":
		return 3
	case "medium", "warning":
		return 2
	case "low", "info":
		return 1
	default:
		return 0
	}
}
