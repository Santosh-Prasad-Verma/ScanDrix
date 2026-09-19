// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package review

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/cli/engine"
	"github.com/scandrix/backend/internal/cli/services/api"
	"github.com/scandrix/backend/internal/cli/services/auth"
	ctxService "github.com/scandrix/backend/internal/cli/services/context"
	"github.com/scandrix/backend/internal/cli/services/git"
	"github.com/scandrix/backend/internal/cli/utils"
	"github.com/scandrix/backend/pkg/models"
)

// REVIEW CONFIGURATION OPTIONS & RESULTS

// ReviewOptions specifies configuration flags for a code review execution.
type ReviewOptions struct {
	Files          []string
	Staged         bool
	Branch         string
	Commit         string
	File           string
	RulesOnly      bool
	Fast           bool
	Heavy          bool
	Focus          string
	Fix            bool
	PromptOnly     bool
	ContextFile    string
	FailOnSeverity string
	Fields         string
	Offline        bool
	DryRun         bool
	NoHunk         bool
	GithubPAT      string
}

// ReviewResult wraps review analysis outcome, findings, and metadata.
type ReviewResult struct {
	ReviewID      string               `json:"review_id"`
	Status        string               `json:"status"` // "passed" or "failed"
	Summary       string               `json:"summary"`
	FilesAnalyzed int                  `json:"files_analyzed"`
	TotalFindings int                  `json:"total_findings"`
	CriticalCount int                  `json:"critical_count"`
	HighCount     int                  `json:"high_count"`
	MediumCount   int                  `json:"medium_count"`
	LowCount      int                  `json:"low_count"`
	Findings      []models.CodeFinding `json:"findings"`
	Duration      time.Duration        `json:"duration"`
	DurationMs    int64                `json:"duration_ms"`
	IsBlocking    bool                 `json:"is_blocking"`
	ExitCode      int                  `json:"exit_code"`
	FixesApplied  int                  `json:"fixes_applied,omitempty"`
}

// REVIEW SERVICE SPECIFICATION & FACTORY

// Service coordinates git diff extraction, project context enrichment, rule evaluation, and fix application.
type Service struct {
	apiClient   *api.Client
	authService *auth.Service
	gitService  *git.Service
	ctxService  *ctxService.Service
	localRunner *engine.CLIRunner
}

var defaultReviewService = &Service{
	apiClient:   api.NewClient("", "", ""),
	authService: auth.DefaultService(),
	gitService:  git.DefaultService(),
	ctxService:  ctxService.DefaultService(),
	localRunner: engine.NewCLIRunner(),
}

// DefaultService returns the singleton ReviewService.
func DefaultService() *Service {
	return defaultReviewService
}

// NewService constructs a ReviewService instance.
func NewService(apiClient *api.Client, authService *auth.Service, gitService *git.Service, ctxService *ctxService.Service, localRunner ...*engine.CLIRunner) *Service {
	runner := engine.NewCLIRunner()
	if len(localRunner) > 0 && localRunner[0] != nil {
		runner = localRunner[0]
	}
	return &Service{
		apiClient:   apiClient,
		authService: authService,
		gitService:  gitService,
		ctxService:  ctxService,
		localRunner: runner,
	}
}

// PIPELINE EXECUTION (Diff Resolution, Context, AI/AST Review)

// ExtractChangedFiles parses a unified diff to return all unique modified file paths.
func ExtractChangedFiles(diff string) []string {
	seen := make(map[string]bool)
	var files []string
	lines := strings.Split(diff, "\n")
	for _, l := range lines {
		if strings.HasPrefix(l, "+++ b/") {
			f := strings.TrimPrefix(l, "+++ b/")
			if f != "/dev/null" && !seen[f] {
				seen[f] = true
				files = append(files, f)
			}
		}
	}
	return files
}

// BuildNoChangesMessages generates actionable hint messages when diff is empty.
func BuildNoChangesMessages(files []string, opts ReviewOptions, untracked []string) []string {
	var msgs []string
	if len(files) > 0 {
		msgs = append(msgs,
			"None of the requested files have diff content in the selected scope.",
			"Check the file paths or try running 'scandrix review' without explicit files.",
		)
		return msgs
	}

	if opts.Branch != "" {
		msgs = append(msgs,
			fmt.Sprintf("No diff was found against '%s'.", opts.Branch),
			"Confirm the branch name or try a different base branch.",
		)
		return msgs
	}

	if opts.Commit != "" {
		msgs = append(msgs,
			fmt.Sprintf("No diff was found for commit '%s'.", opts.Commit),
			"Confirm the commit SHA or review a different revision.",
		)
		return msgs
	}

	if opts.Staged {
		msgs = append(msgs,
			"There are no staged changes to review.",
			"Stage files first with 'git add' or run 'scandrix review' to inspect the full working tree.",
		)
		return msgs
	}

	if len(untracked) > 0 {
		msgs = append(msgs,
			fmt.Sprintf("Found %d untracked file(s) (run 'git add' to track and stage them).", len(untracked)),
		)
	}

	msgs = append(msgs,
		"Try 'scandrix review --staged' to review staged changes only.",
		"Or compare against a branch: 'scandrix review --branch main'.",
	)
	return msgs
}

// Analyze executes the complete review pipeline.
func (s *Service) Analyze(ctx context.Context, opts ReviewOptions) (*ReviewResult, error) {
	return s.AnalyzeWithProgress(ctx, opts, nil)
}

// AnalyzeWithProgress executes the review pipeline and reports progress status events.
func (s *Service) AnalyzeWithProgress(ctx context.Context, opts ReviewOptions, onProgress func(status string)) (*ReviewResult, error) {
	startTime := time.Now()

	// 1. Resolve Git Diff
	diff, err := s.resolveDiff(ctx, opts)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(diff) == "" {
		untracked, _ := s.gitService.GetUntrackedFiles(ctx)
		hints := BuildNoChangesMessages(opts.Files, opts, untracked)
		msg := "No changes detected to review."
		if len(hints) > 0 {
			msg += "\n" + strings.Join(hints, "\n")
		}
		return nil, utils.NewCommandError(utils.ErrCodeNoChanges, msg, 0, nil)
	}

	// 2. Extract changed files and inline file contents for rich LLM context
	changedFiles := ExtractChangedFiles(diff)
	inlinedFiles, _ := s.gitService.GetFullFileContents(ctx, changedFiles, 500*1024, 20*1024*1024)
	gitInfo, _ := s.gitService.GetGitInfo(ctx, opts.Branch)

	// 3. Read and Enrich Project Context (.scandrix/context.md, etc.)
	enrichedDiff, _ := s.ctxService.EnrichDiff(ctx, diff, opts.ContextFile)

	// 4. Execute Review: Remote API if authenticated/online, otherwise Local Engine Fallback
	var findings []models.CodeFinding
	summary := "Review completed successfully."
	filesAnalyzed := len(changedFiles)
	if filesAnalyzed == 0 {
		filesAnalyzed = 1
	}

	isAuthenticated := s.authService.IsAuthenticated()

	if isAuthenticated && !opts.Offline {
		req := api.ReviewRequest{
			Diff:         enrichedDiff,
			RulesOnly:    opts.RulesOnly,
			Fast:         opts.Fast,
			Heavy:        opts.Heavy,
			Focus:        opts.Focus,
			Files:        opts.Files,
			InlinedFiles: inlinedFiles,
			Staged:       opts.Staged,
			Branch:       opts.Branch,
			Commit:       opts.Commit,
			Metrics:      gitInfo,
			GithubPAT:    opts.GithubPAT,
		}

		resp, apiErr := s.apiClient.SubmitReviewWithProgress(ctx, req, onProgress)
		if apiErr == nil && resp != nil {
			findings = resp.Findings
			summary = resp.Summary
			if resp.FilesAnalyzed > 0 {
				filesAnalyzed = resp.FilesAnalyzed
			}
		} else {
			utils.Debug("Remote review API failed (%v); executing local AST rule evaluation fallback", apiErr)
			localRes, localErr := s.executeLocalReview(ctx, opts, diff)
			if localErr != nil {
				return nil, fmt.Errorf("review failed: %w", apiErr)
			}
			findings = localRes.Findings
			summary = localRes.Summary
			filesAnalyzed = localRes.FilesReviewed
		}
	} else {
		// Trial or Local Evaluation
		localRes, err := s.executeLocalReview(ctx, opts, diff)
		if err != nil {
			return nil, err
		}
		findings = localRes.Findings
		summary = localRes.Summary
		filesAnalyzed = localRes.FilesReviewed
	}

	// 5. Focus area filtering if specified
	if opts.Focus != "" {
		findings = filterFindingsByFocus(findings, opts.Focus)
	}

	// 6. Automatic batch fix application if --fix requested (sorted descending by StartLine)
	fixesApplied := 0
	if opts.Fix && len(findings) > 0 {
		batchRes, err := engine.ApplyBatchFixes(".", findings)
		if err == nil && batchRes != nil {
			fixesApplied = batchRes.Applied
		}
	}

	// 7. Aggregate severity counts and blocking thresholds
	crit, high, med, low := 0, 0, 0, 0
	for _, f := range findings {
		switch f.Severity {
		case models.SeverityCritical:
			crit++
		case models.SeverityHigh:
			high++
		case models.SeverityMedium:
			med++
		case models.SeverityLow:
			low++
		}
	}

	isBlocking, exitCode := evaluateBlocking(crit, high, med, low, opts.FailOnSeverity)
	if opts.DryRun {
		isBlocking = false
		exitCode = 0
	}

	duration := time.Since(startTime)
	res := &ReviewResult{
		Status:        "passed",
		Summary:       summary,
		FilesAnalyzed: filesAnalyzed,
		TotalFindings: len(findings),
		CriticalCount: crit,
		HighCount:     high,
		MediumCount:   med,
		LowCount:      low,
		Findings:      findings,
		Duration:      duration,
		DurationMs:    duration.Milliseconds(),
		IsBlocking:    isBlocking,
		ExitCode:      exitCode,
		FixesApplied:  fixesApplied,
	}

	if isBlocking {
		res.Status = "failed"
	}

	return res, nil
}

// INTERNAL HELPERS & FILTERING

func (s *Service) resolveDiff(ctx context.Context, opts ReviewOptions) (string, error) {
	if opts.File != "" {
		data, err := s.gitService.ReadFileContentsAtWorkingTree(opts.File)
		if err != nil {
			return "", fmt.Errorf("unable to read diff file %s: %w", opts.File, err)
		}
		return data, nil
	}

	if len(opts.Files) > 0 {
		return s.gitService.GetFilesDiff(ctx, opts.Files, opts.Staged)
	}

	if opts.Commit != "" {
		return s.gitService.GetCommitDiff(ctx, opts.Commit)
	}

	if opts.Branch != "" {
		return s.gitService.GetBranchDiff(ctx, opts.Branch)
	}

	if opts.Staged {
		return s.gitService.GetStagedDiff(ctx)
	}

	// Default: unstaged working tree diff. If empty, check staged.
	diff, err := s.gitService.GetWorkingTreeDiff(ctx)
	if err == nil && strings.TrimSpace(diff) != "" {
		return diff, nil
	}

	// Check if any staged changes exist
	stagedDiff, errStaged := s.gitService.GetStagedDiff(ctx)
	if errStaged == nil && strings.TrimSpace(stagedDiff) != "" {
		return stagedDiff, nil
	}

	return diff, err
}

func (s *Service) executeLocalReview(ctx context.Context, opts ReviewOptions, diff string) (*engine.CLIResult, error) {
	cliOpts := engine.CLIOptions{
		Staged:    opts.Staged,
		Branch:    opts.Branch,
		Commit:    opts.Commit,
		RulesOnly: opts.RulesOnly,
		Fast:      opts.Fast,
		Heavy:     opts.Heavy,
		Focus:     opts.Focus,
		Fix:       opts.Fix,
		Offline:   true,
		DryRun:    opts.DryRun,
	}

	return s.localRunner.RunReview(ctx, diff, cliOpts)
}

func filterFindingsByFocus(findings []models.CodeFinding, focus string) []models.CodeFinding {
	focusLower := strings.ToLower(focus)
	var filtered []models.CodeFinding
	for _, f := range findings {
		if strings.Contains(strings.ToLower(f.Title), focusLower) ||
			strings.Contains(strings.ToLower(f.Description), focusLower) ||
			strings.Contains(strings.ToLower(f.Category), focusLower) {
			filtered = append(filtered, f)
		}
	}
	return filtered
}

func evaluateBlocking(crit, high, med, low int, threshold string) (bool, int) {
	thresh := strings.ToUpper(strings.TrimSpace(threshold))
	if thresh == "" {
		return false, 0
	}

	isBlocking := false
	switch thresh {
	case "CRITICAL":
		isBlocking = crit > 0
	case "HIGH", "ERROR":
		isBlocking = crit > 0 || high > 0
	case "MEDIUM", "WARN", "WARNING":
		isBlocking = crit > 0 || high > 0 || med > 0
	case "LOW", "INFO":
		isBlocking = crit > 0 || high > 0 || med > 0 || low > 0
	}

	exitCode := 0
	if isBlocking {
		exitCode = 1
	}
	return isBlocking, exitCode
}
