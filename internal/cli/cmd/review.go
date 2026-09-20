// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/scandrix/backend/internal/cli/engine"
	"github.com/scandrix/backend/internal/cli/formatters"
	"github.com/scandrix/backend/internal/cli/services/api"
	"github.com/scandrix/backend/internal/cli/services/auth"
	ctxService "github.com/scandrix/backend/internal/cli/services/context"
	"github.com/scandrix/backend/internal/cli/services/git"
	"github.com/scandrix/backend/internal/cli/services/review"
	"github.com/scandrix/backend/internal/cli/tui"
	"github.com/scandrix/backend/internal/cli/ui"
	"github.com/scandrix/backend/internal/cli/utils"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// COMMAND FLAGS & RUNTIME OPTIONS

var reviewOpts review.ReviewOptions
var (
	tuiFlag         bool
	interactiveFlag bool
	noHunkFlag      bool
	copyFlag        bool
	serverOverride  string
	keyOverride     string
	githubPatFlag   string
)

func resolveGithubPAT(flagVal string) string {
	if strings.TrimSpace(flagVal) != "" {
		return strings.TrimSpace(flagVal)
	}
	if v := os.Getenv("SCANDRIX_GITHUB_PAT"); strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	if v := os.Getenv("GITHUB_TOKEN"); strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	if v := os.Getenv("GH_TOKEN"); strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return ""
}

// REVIEW COMMAND SPECIFICATION

var reviewCmd = &cobra.Command{
	Use:   "review [files...]",
	Short: "Perform AI-powered code review on local git diffs, patches, or files",
	Long: `Analyze modified files or git diffs for security vulnerabilities, logic flaws,
and code quality smells, with AST and LLM multi-critic verification.

Examples:
  scandrix review
  scandrix review --staged
  scandrix review --branch main
  scandrix review --commit HEAD~1
  scandrix review src/auth.go src/config.go
  scandrix review --format markdown -o review.md
  scandrix review --fail-on critical
  scandrix review --staged --fix`,
	RunE: func(cmd *cobra.Command, args []string) error {
		startTime := time.Now().UTC()
		reviewOpts.Files = args
		reviewOpts.GithubPAT = resolveGithubPAT(githubPatFlag)
		reviewOpts.NoHunk = noHunkFlag

		if interactiveFlag && reviewOpts.PromptOnly {
			return fmt.Errorf("The `--interactive` and `--prompt-only` options cannot be used together.")
		}

		if interactiveFlag && reviewOpts.Fix {
			return fmt.Errorf("The `--interactive` and `--fix` options cannot be used together.")
		}

		if reviewOpts.FailOnSeverity != "" {
			valid := false
			norm := strings.ToLower(strings.TrimSpace(reviewOpts.FailOnSeverity))
			for _, s := range []string{"info", "warning", "error", "critical"} {
				if norm == s {
					valid = true
					break
				}
			}
			if !valid {
				return fmt.Errorf("Invalid value for `--fail-on`: `%s`. Use one of: info, warning, error, critical.", reviewOpts.FailOnSeverity)
			}
		}

		if reviewOpts.PromptOnly && !agentFlag {
			formatFlag = "prompt"
		}

		// 1. Interactive Bubbletea TUI Cockpit (Optional)
		if tuiFlag {
			gitSvc := git.DefaultService()
			var rawDiff string
			if reviewOpts.Staged {
				rawDiff, _ = gitSvc.GetStagedDiff(cmd.Context())
			} else if reviewOpts.Branch != "" {
				rawDiff, _ = gitSvc.GetBranchDiff(cmd.Context(), reviewOpts.Branch)
			} else if reviewOpts.Commit != "" {
				rawDiff, _ = gitSvc.GetCommitDiff(cmd.Context(), reviewOpts.Commit)
			} else {
				rawDiff, _ = gitSvc.GetWorkingTreeDiff(cmd.Context())
			}
			model := tui.NewModel(rawDiff, engine.NewCLIRunner())
			p := tea.NewProgram(model, tea.WithAltScreen())
			_, err := p.Run()
			return err
		}

		// 2. Service Resolution with Overrides
		srv := review.DefaultService()
		if serverOverride != "" || keyOverride != "" {
			srv = review.NewService(
				api.NewClient(serverOverride, "", keyOverride),
				auth.DefaultService(),
				git.DefaultService(),
				ctxService.DefaultService(),
				engine.NewCLIRunner(),
			)
		}

		res, err := srv.Analyze(cmd.Context(), reviewOpts)
		if err != nil {
			return err
		}

		// 2.5 Hunk TUI Viewer / Interactive Navigator Mode
		if !noHunkFlag && !interactiveFlag && !agentFlag && outputFlag == "" && (formatFlag == "" || formatFlag == "terminal") && !reviewOpts.PromptOnly {
			if hunkPath, err := exec.LookPath("hunk"); err == nil && hunkPath != "" && runtime.GOOS != "windows" {
				if term.IsTerminal(int(os.Stdout.Fd())) {
					if err := runHunkViewer(cmd.Context(), res, reviewOpts); err == nil {
						if res.ExitCode != 0 {
							os.Exit(res.ExitCode)
						}
						return nil
					}
				}
			}
		}

		if interactiveFlag {
			if reviewOpts.Fix {
				return ui.DefaultInteractiveUI.RunQuickFix(res)
			}
			return ui.DefaultInteractiveUI.Run(res)
		}

		// 3. Agent Mode Envelope Formatting
		if agentFlag || formatFlag == "agent" {
			data := any(res)
			if reviewOpts.Fields != "" {
				fields := utils.ParseFieldList(reviewOpts.Fields)
				masked, maskErr := utils.ApplyFieldMask(res, fields)
				if maskErr == nil {
					data = masked
				}
			}
			envelope := utils.BuildAgentSuccessEnvelope("review", data, startTime)
			return utils.EmitAgentEnvelope(envelope, outputFlag)
		}

		// 4. Output Rendering (terminal, json, markdown, sarif, prompt)
		var outBuf bytes.Buffer
		switch formatFlag {
		case "prompt":
			err = formatters.RenderPrompt(&outBuf, formatters.PromptFormatOptions{
				FilesAnalyzed: res.FilesAnalyzed,
				DurationMs:    res.DurationMs,
				Summary:       res.Summary,
				Findings:      res.Findings,
			})
		case "markdown":
			legacyRes := convertToEngineResult(res)
			formatter := engine.NewOutputFormatter()
			err = formatter.Render(&outBuf, legacyRes, engine.FormatMarkdown)
		case "json":
			legacyRes := convertToEngineResult(res)
			formatter := engine.NewOutputFormatter()
			err = formatter.RenderWithFields(&outBuf, legacyRes, engine.FormatJSON, reviewOpts.Fields)
		case "sarif":
			legacyRes := convertToEngineResult(res)
			formatter := engine.NewOutputFormatter()
			err = formatter.Render(&outBuf, legacyRes, engine.FormatSARIF)
		default: // "terminal"
			legacyRes := convertToEngineResult(res)
			formatter := engine.NewOutputFormatter()
			err = formatter.Render(&outBuf, legacyRes, engine.FormatTerminal)
		}

		if err != nil {
			return fmt.Errorf("failed rendering review report: %w", err)
		}

		if outputFlag != "" {
			if writeErr := os.WriteFile(outputFlag, outBuf.Bytes(), 0644); writeErr != nil {
				return fmt.Errorf("failed writing report to %s: %w", outputFlag, writeErr)
			}
			utils.Success("✔ Review report written directly to %s", outputFlag)
		} else {
			fmt.Print(outBuf.String())
		}

		if copyFlag {
			if utils.CopyToClipboard(outBuf.String()) {
				utils.Success("✔ Review output copied to clipboard (%d bytes)", outBuf.Len())
			} else {
				utils.Warn("⚠ Could not copy review output to system clipboard")
			}
		}

		// Non-zero exit code if blocking severity threshold was violated
		if res.ExitCode > 0 {
			if reviewOpts.FailOnSeverity != "" {
				blockingCount := 0
				thresh := strings.ToUpper(strings.TrimSpace(reviewOpts.FailOnSeverity))
				switch thresh {
				case "CRITICAL":
					blockingCount = res.CriticalCount
				case "HIGH", "ERROR":
					blockingCount = res.CriticalCount + res.HighCount
				case "MEDIUM", "WARN", "WARNING":
					blockingCount = res.CriticalCount + res.HighCount + res.MediumCount
				case "LOW", "INFO":
					blockingCount = res.TotalFindings
				}
				issueLabel := "issue"
				verbPhrase := "meets or exceeds"
				if blockingCount > 1 {
					issueLabel = "issues"
					verbPhrase = "meet or exceed"
				}
				utils.Warn("Exiting with code 1 because %d %s %s '--fail-on %s'.", blockingCount, issueLabel, verbPhrase, reviewOpts.FailOnSeverity)
			}
		}

		return nil
	},
}

// RESULT CONVERSION HELPER

func convertToEngineResult(res *review.ReviewResult) *engine.CLIResult {
	return &engine.CLIResult{
		Status:        res.Status,
		FilesReviewed: res.FilesAnalyzed,
		TotalFindings: res.TotalFindings,
		CriticalCount: res.CriticalCount,
		HighCount:     res.HighCount,
		MediumCount:   res.MediumCount,
		LowCount:      res.LowCount,
		Summary:       res.Summary,
		Findings:      res.Findings,
		ExitCode:      res.ExitCode,
		Duration:      res.Duration,
		IsBlocking:    res.IsBlocking,
		FixesApplied:  res.FixesApplied,
	}
}

func runHunkViewer(ctx context.Context, res *review.ReviewResult, opts review.ReviewOptions) error {
	contextPath, findingsPath, cleanup, err := review.ExportHunkSidecarFiles(res)
	if err != nil {
		return err
	}
	defer cleanup()

	var args []string
	if opts.Commit != "" {
		args = append(args, "show", opts.Commit)
	} else {
		args = append(args, "diff")
		if opts.Branch != "" {
			args = append(args, opts.Branch)
		} else if opts.Staged {
			args = append(args, "--staged")
		}
	}
	args = append(args, "--agent-context", contextPath, "--agent-notes", "--experimental")
	if len(opts.Files) > 0 {
		args = append(args, "--")
		args = append(args, opts.Files...)
	}

	hunkCmd := exec.CommandContext(ctx, "hunk", args...)
	hunkCmd.Env = append(os.Environ(), "SCANDRIX_HUNK_FINDINGS="+findingsPath)
	hunkCmd.Stdin = os.Stdin
	hunkCmd.Stdout = os.Stdout
	hunkCmd.Stderr = os.Stderr

	return hunkCmd.Run()
}

// COMMAND REGISTRATION & FLAG INITIALIZATION

func init() {
	reviewCmd.Flags().BoolVarP(&reviewOpts.Staged, "staged", "s", false, "Analyze only staged git changes (git diff --cached)")
	reviewCmd.Flags().StringVarP(&reviewOpts.Branch, "branch", "b", "", "Compare current changes against target base branch (e.g. main)")
	reviewCmd.Flags().StringVarP(&reviewOpts.Commit, "commit", "c", "", "Analyze diff introduced by a specific commit SHA")
	reviewCmd.Flags().StringVar(&reviewOpts.File, "file", "", "Review an external unified .diff or .patch file")
	reviewCmd.Flags().BoolVar(&reviewOpts.RulesOnly, "rules-only", false, "Review using only configured custom and catalog rules")
	reviewCmd.Flags().BoolVar(&reviewOpts.Fast, "fast", false, "Fast mode: lighter checks with minimal overhead")
	reviewCmd.Flags().BoolVar(&reviewOpts.Heavy, "heavy", false, "Heavy mode: multi-critic LLM verification pass")
	reviewCmd.Flags().StringVar(&reviewOpts.Focus, "focus", "", "Focus review on specific area (e.g. 'auth', 'sql', 'concurrency')")
	reviewCmd.Flags().BoolVarP(&tuiFlag, "tui", "t", false, "Launch interactive Bubbletea terminal cockpit dashboard")
	reviewCmd.Flags().BoolVarP(&interactiveFlag, "interactive", "i", false, "Launch interactive finding navigator and quick-fix prompt")
	reviewCmd.Flags().BoolVar(&noHunkFlag, "no-hunk", false, "Disable automatic launch of hunk diff viewer")
	reviewCmd.Flags().BoolVar(&reviewOpts.Fix, "fix", false, "Automatically apply all fixable issues")
	reviewCmd.Flags().BoolVar(&reviewOpts.PromptOnly, "prompt-only", false, "Output compact, structured prompt for AI agents")
	reviewCmd.Flags().StringVar(&reviewOpts.FailOnSeverity, "fail-on", "", "Exit code 1 if issues meet or exceed severity (info, warning, error, critical)")
	reviewCmd.Flags().StringVar(&reviewOpts.FailOnSeverity, "fail-on-severity", "", "Alias for --fail-on")
	reviewCmd.Flags().StringVar(&reviewOpts.ContextFile, "context", "", "Custom context file to include in review")
	reviewCmd.Flags().StringVar(&reviewOpts.Fields, "fields", "", "Select response fields (JSON/agent mode only), e.g. summary,findings.file_path")
	reviewCmd.Flags().BoolVarP(&copyFlag, "copy", "C", false, "Copy review output or prompt directly to system clipboard")
	reviewCmd.Flags().BoolVar(&reviewOpts.Offline, "offline", false, "Run review entirely offline using local rules engine")
	reviewCmd.Flags().BoolVar(&reviewOpts.DryRun, "dry-run", false, "Preview review rules without blocking or exiting with error")
	reviewCmd.Flags().StringVar(&serverOverride, "server", "", "Override ScanDrix API server URL")
	reviewCmd.Flags().StringVar(&keyOverride, "key", "", "Provide team API key (scandrix_*) or Bearer token directly")
	reviewCmd.Flags().StringVar(&githubPatFlag, "github-pat", "", "GitHub Personal Access Token for private repository sandbox clones")

	dryRunCmd.Flags().AddFlagSet(reviewCmd.Flags())
}
