package stages

import (
	"context"
	"errors"
	"strings"

	"github.com/scandrix/backend/internal/review/pipeline"
)

// PrerequisitesStage validates review readiness, size thresholds, and skip flags.
type PrerequisitesStage struct{}

func NewPrerequisitesStage() *PrerequisitesStage {
	return &PrerequisitesStage{}
}

func (s *PrerequisitesStage) Name() string {
	return "prerequisites_validator"
}

func (s *PrerequisitesStage) Execute(ctx context.Context, pCtx *pipeline.PipelineContext) error {
	// 1. Check for manual bypass tags in PR title
	lowerTitle := strings.ToLower(pCtx.Title)
	if strings.Contains(lowerTitle, "[scandrix-skip]") || strings.Contains(lowerTitle, "[skip-review]") || strings.Contains(lowerTitle, "[skip-ci]") {
		return errors.New("review skipped by PR title bypass flag")
	}

	// 2. Lifecycle Checks: Drop closed, merged, or locked pull requests
	if pCtx.PRState == "closed" || pCtx.PRState == "merged" {
		return errors.New("review skipped: pull request is closed or merged")
	}
	if pCtx.IsLocked {
		return errors.New("review skipped: pull request conversation is locked")
	}

	// 3. Automated Bot Author Filter
	authorLower := strings.ToLower(pCtx.Author)
	if strings.HasSuffix(authorLower, "[bot]") || authorLower == "dependabot" || authorLower == "renovate" || authorLower == "snyk-bot" {
		return errors.New("review skipped: automated bot pull request author (" + pCtx.Author + ")")
	}

	// 4. Circular Review Protection for Config & Governance Repositories
	repoLower := strings.ToLower(pCtx.RepoNamespace)
	if strings.HasSuffix(repoLower, "/.github") || strings.HasSuffix(repoLower, "/scandrix-rules") || strings.HasSuffix(repoLower, "/centralized-config") {
		return errors.New("review skipped: governance and rules definition repository")
	}

	// 5. Validate raw diff presence
	if strings.TrimSpace(pCtx.RawDiff) == "" {
		return errors.New("review diff is empty; nothing to inspect")
	}

	// 6. Check line limit threshold
	maxLines := pCtx.ReviewParams.MaxDiffLines
	if maxLines <= 0 {
		maxLines = 3000 // default upper bound
	}

	lineCount := strings.Count(pCtx.RawDiff, "\n")
	if lineCount > maxLines {
		// Flag diff as oversized in telemetry
		pCtx.AddMetric("diff_size_warning", 0, true, nil, lineCount)
	}

	return nil
}
