package engine

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/pkg/models"
)

// CLIRunner orchestrates the review execution for CLI workflows.
type CLIRunner struct {
	evaluator *rules.Evaluator
}

// NewCLIRunner initializes the runner with the default OWASP rule engine.
func NewCLIRunner() *CLIRunner {
	ev, _ := rules.NewEvaluator(rules.DefaultCatalog())
	return &CLIRunner{
		evaluator: ev,
	}
}

// RunReview processes the provided unified diff and evaluates findings.
func (r *CLIRunner) RunReview(ctx context.Context, rawDiff string, opts CLIOptions) (*CLIResult, error) {
	startTime := time.Now()

	result := &CLIResult{
		Findings: make([]models.CodeFinding, 0),
	}

	if strings.TrimSpace(rawDiff) == "" {
		result.Duration = time.Since(startTime)
		return result, nil
	}

	// 1. Parse unified diff directly into []*diff.FilePatch
	files, err := diff.ParseUnifiedDiff(strings.NewReader(rawDiff))
	if err != nil {
		return nil, err
	}

	result.FilesReviewed = len(files)

	// 2. Evaluate rules against diff hunks
	dummyID := uuid.New()
	findings := r.evaluator.EvaluatePatches(dummyID, dummyID, files)
	result.Findings = findings
	result.TotalFindings = len(findings)

	// 3. Aggregate severity counts
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

	// 4. Check if blocking based on severity threshold
	threshold := opts.SeverityThreshold
	if threshold == "" {
		threshold = models.SeverityHigh // Default: block on HIGH and CRITICAL
	}

	isBlocking := false
	if threshold == models.SeverityCritical && result.CriticalCount > 0 {
		isBlocking = true
	} else if threshold == models.SeverityHigh && (result.CriticalCount > 0 || result.HighCount > 0) {
		isBlocking = true
	} else if threshold == models.SeverityMedium && (result.CriticalCount > 0 || result.HighCount > 0 || result.MediumCount > 0) {
		isBlocking = true
	} else if threshold == models.SeverityLow && result.TotalFindings > 0 {
		isBlocking = true
	}

	result.IsBlocking = isBlocking
	if isBlocking && !opts.DryRun {
		result.ExitCode = 1
	} else {
		result.ExitCode = 0
	}

	result.Duration = time.Since(startTime)
	return result, nil
}
