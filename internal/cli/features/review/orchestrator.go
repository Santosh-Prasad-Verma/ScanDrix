// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package review

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/cli/features/review/quickfix"
	"github.com/scandrix/backend/internal/cli/formatters"
	"github.com/scandrix/backend/internal/cli/git"
	"github.com/scandrix/backend/internal/cli/services/api"
	"github.com/scandrix/backend/internal/cli/services/auth"
	ctxService "github.com/scandrix/backend/internal/cli/services/context"
	reviewSvc "github.com/scandrix/backend/internal/cli/services/review"
	"github.com/scandrix/backend/internal/cli/types"
	"github.com/scandrix/backend/internal/cli/utils"
	"golang.org/x/term"
)

// ReviewOrchestrator coordinates the end-to-end code review flow from diff gathering to output display.
type ReviewOrchestrator struct {
	gitService     *git.GitService
	authService    *auth.Service
	reviewService  *reviewSvc.Service
	contextService *ctxService.Service
	apiClient      *api.Client
}

// NewReviewOrchestrator creates an initialized review orchestrator.
func NewReviewOrchestrator(
	gitSvc *git.GitService,
	authSvc *auth.Service,
	revSvc *reviewSvc.Service,
	ctxSvc *ctxService.Service,
	apiCli *api.Client,
) *ReviewOrchestrator {
	if gitSvc == nil {
		gitSvc = git.NewGitService(".")
	}
	if authSvc == nil {
		authSvc = auth.DefaultService()
	}
	if revSvc == nil {
		revSvc = reviewSvc.DefaultService()
	}
	if ctxSvc == nil {
		ctxSvc = ctxService.DefaultService()
	}
	return &ReviewOrchestrator{
		gitService:     gitSvc,
		authService:    authSvc,
		reviewService:  revSvc,
		contextService: ctxSvc,
		apiClient:      apiCli,
	}
}

// ExecuteReview runs the complete review pipeline.
func (o *ReviewOrchestrator) ExecuteReview(ctx context.Context, opts ReviewOptions) (*types.ReviewResult, int, error) {
	// 1. Validate review options and mutually exclusive flags
	if err := ValidateReviewOptions(opts); err != nil {
		return nil, 1, err
	}

	if opts.PromptOnly && !opts.IsAgent && opts.Format == "" {
		opts.Format = "prompt"
	}

	// 2. Resolve authentication state
	isAuthed := o.authService != nil && o.authService.IsAuthenticated()

	// 3. Resolve git diff from specified files or branches
	diffResult, err := ResolveReviewDiff(ctx, ResolveReviewDiffParams{
		Files:   opts.Files,
		Options: opts,
		Verbose: opts.Verbose,
		Git:     o.gitService,
	})
	if err != nil {
		return nil, 1, fmt.Errorf("failed resolving diff: %w", err)
	}

	if opts.Verbose {
		for _, msg := range diffResult.VerboseMessages {
			utils.Debug("%s", msg)
		}
	}

	rawDiff := strings.TrimSpace(diffResult.Diff)
	if rawDiff == "" {
		o.handleNoChanges(opts)
		return nil, 0, nil
	}

	// 4. Enrich diff with repository context if requested or available
	enrichedDiff := rawDiff
	if o.contextService != nil {
		enriched, ctxErr := o.contextService.EnrichDiff(ctx, rawDiff, opts.ContextFile)
		if ctxErr == nil && enriched != "" {
			enrichedDiff = enriched
		}
	}

	// 5. Execute review via service
	serviceOpts := reviewSvc.ReviewOptions{
		Files:          opts.Files,
		Staged:         opts.Staged,
		Branch:         opts.Branch,
		Commit:         opts.Commit,
		RulesOnly:      opts.RulesOnly,
		Fast:           opts.Fast,
		Heavy:          opts.Heavy,
		Focus:          opts.Focus,
		Fix:            opts.Fix,
		PromptOnly:     opts.PromptOnly,
		ContextFile:    opts.ContextFile,
		FailOnSeverity: opts.FailOn,
		Fields:         opts.Fields,
		NoHunk:         opts.NoHunk,
		GithubPAT:      opts.GithubPAT,
	}

	serviceResult, err := o.reviewService.Analyze(ctx, serviceOpts)
	if err != nil {
		// If authenticated review failed with 401, fallback to trial
		if isAuthed && (strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "Unauthorized")) {
			if !opts.Quiet {
				utils.Warn("Authenticated review failed (invalid credentials). Retrying in trial mode...")
			}
			serviceResult, err = o.reviewService.Analyze(ctx, serviceOpts)
		}
		if err != nil {
			return nil, 1, err
		}
	}

	result := o.convertServiceResult(serviceResult, enrichedDiff)

	// 6. Route to Hunk Viewer if supported and in interactive terminal
	ttyOut := term.IsTerminal(int(os.Stdout.Fd()))
	platformSupported := IsHunkPlatformSupported()
	hunkScope := BuildHunkViewerScope(opts)
	scopeSupported := hunkScope != nil

	useHunkViewer := ShouldUseHunkViewer(
		opts.IsAgent,
		opts.Interactive,
		opts.NoHunk,
		opts.Output,
		opts.Format,
		ttyOut,
		scopeSupported,
		platformSupported,
	)

	if useHunkViewer && len(result.Issues) > 0 {
		hunkCtx := ConvertReviewToHunkAgentContext(result)
		annotationCount := CountHunkAnnotations(hunkCtx)
		if annotationCount > 0 {
			err := OpenReviewInHunk(ctx, result, *hunkScope, opts.Verbose)
			if err == nil {
				if ShouldFailReview(result, opts.FailOn) {
					failMsg := FormatFailOnExitMessage(result, opts.FailOn)
					if failMsg != "" && !opts.Quiet {
						utils.Info(failMsg)
					}
					return result, 1, nil
				}
				return result, 0, nil
			}
		}
	}

	// 7. Interactive Review & Quick-Fix
	if opts.Interactive && serviceResult != nil && len(serviceResult.Findings) > 0 {
		fixSession := quickfix.NewInteractiveFixSession(".", os.Stdin, os.Stdout)
		_, _, _ = fixSession.ReviewAndApply(serviceResult.Findings)
	}

	// 8. Format and output review report
	formattedOutput, err := o.formatOutput(result, opts.Format)
	if err != nil {
		return result, 1, fmt.Errorf("failed formatting review output: %w", err)
	}

	if opts.Output != "" {
		if writeErr := os.WriteFile(opts.Output, []byte(formattedOutput), 0644); writeErr != nil {
			return result, 1, fmt.Errorf("failed saving review output to %s: %w", opts.Output, writeErr)
		}
		if !opts.Quiet {
			utils.Info("Output saved to %s", opts.Output)
		}
	} else {
		fmt.Println(formattedOutput)
	}

	// 8. Check --fail-on severity threshold
	if ShouldFailReview(result, opts.FailOn) {
		failMsg := FormatFailOnExitMessage(result, opts.FailOn)
		if failMsg != "" && !opts.Quiet {
			utils.Warn("%s", failMsg)
		}
		return result, 1, nil
	}

	return result, 0, nil
}

func (o *ReviewOrchestrator) handleNoChanges(opts ReviewOptions) {
	if opts.Quiet {
		return
	}
	utils.Warn("No changes to review")
	messages := BuildNoChangesMessages(opts.Files, opts.Staged, opts.Branch, opts.Commit)
	for _, msg := range messages {
		fmt.Println(msg)
	}
}

func (o *ReviewOrchestrator) convertServiceResult(res *reviewSvc.ReviewResult, rawDiff string) *types.ReviewResult {
	if res == nil {
		return &types.ReviewResult{
			Issues:  []types.ReviewIssue{},
			Summary: "No issues identified.",
		}
	}

	issues := make([]types.ReviewIssue, 0, len(res.Findings))
	for _, f := range res.Findings {
		issue := types.ReviewIssue{
			ID:             f.ID.String(),
			File:           f.FilePath,
			Line:           f.StartLine,
			EndLine:        f.EndLine,
			Severity:       types.Severity(strings.ToLower(string(f.Severity))),
			Category:       f.Category,
			Message:        f.Title,
			Recommendation: f.Description,
			Suggestion:     f.Remediation,
			Fixable:        f.SuggestedDiff != "",
		}
		if f.SuggestedDiff != "" {
			issue.Fix = &types.CodeFix{
				Explanation: f.Remediation,
				NewCode:     f.SuggestedDiff,
				StartLine:   f.StartLine,
				EndLine:     f.EndLine,
			}
		}
		issues = append(issues, issue)
	}

	var stats types.ReviewStats
	stats.TotalIssues = len(issues)
	for _, issue := range issues {
		switch issue.Severity {
		case types.SeverityCritical:
			stats.CriticalCount++
		case types.SeverityError:
			stats.ErrorCount++
		case types.SeverityWarning:
			stats.WarningCount++
		case types.SeverityInfo:
			stats.InfoCount++
		}
		if issue.Fixable {
			stats.FixableCount++
		}
	}

	return &types.ReviewResult{
		ReviewID:      res.ReviewID,
		Status:        res.Status,
		Summary:       res.Summary,
		FilesAnalyzed: res.FilesAnalyzed,
		Issues:        issues,
		Stats:         stats,
		Duration:      time.Duration(res.DurationMs) * time.Millisecond,
		DurationMs:    res.DurationMs,
		ExitCode:      res.ExitCode,
		FixesApplied:  res.FixesApplied,
	}
}

func (o *ReviewOrchestrator) formatOutput(result *types.ReviewResult, format string) (string, error) {
	var buf strings.Builder
	var err error

	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		formatter := formatters.NewJSONFormatter(true)
		err = formatter.Format(&buf, result)
	case "markdown", "md":
		formatter := formatters.NewMarkdownFormatter(true)
		err = formatter.Format(&buf, result)
	case "sarif":
		formatter := formatters.NewSarifFormatter()
		err = formatter.Format(&buf, result)
	case "prompt":
		findings := make([]types.ReviewIssue, len(result.Issues))
		copy(findings, result.Issues)
		err = formatters.RenderPrompt(&buf, formatters.PromptFormatOptions{
			FilesAnalyzed: result.FilesAnalyzed,
			DurationMs:    result.DurationMs,
			Summary:       result.Summary,
			Findings:      nil,
		})
	default:
		formatter := formatters.NewTerminalFormatter(false, false)
		err = formatter.Format(&buf, result)
	}

	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

