// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/scandrix/backend/internal/cli/engine"
	"github.com/scandrix/backend/internal/cli/services/review"
	"github.com/scandrix/backend/internal/cli/utils"
	"github.com/spf13/cobra"
)

// EXPORT COMMAND FLAGS

var (
	exportFormat string
	exportOutput string
	exportStaged bool
	exportBranch string
	exportCommit string
	exportFast   bool
)

// EXPORT COMMAND SPECIFICATION

var exportCmd = &cobra.Command{
	Use:   "export [target]",
	Short: "Export security & quality audit findings into SARIF, JSON, CSV, or Markdown",
	Long:  `Generates industry-standard reports (SARIF for GitHub Code Scanning, CSV for audits, JSON for CI pipelines, Markdown for PR summaries).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		targetDir := "."
		if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
			targetDir = args[0]
		}

		ctx := utils.NewCommandContext("export", exportFormat, exportOutput, verboseFlag, quietFlag, agentFlag)

		outFmt := engine.FormatSARIF
		switch strings.ToLower(exportFormat) {
		case "json":
			outFmt = engine.FormatJSON
		case "csv":
			outFmt = engine.FormatCSV
		case "markdown", "md":
			outFmt = engine.FormatMarkdown
		case "agent":
			outFmt = engine.FormatAgent
		default:
			outFmt = engine.FormatSARIF
		}

		var res *engine.CLIResult

		if exportStaged || exportBranch != "" || exportCommit != "" {
			// Analyze git diff
			opts := review.ReviewOptions{
				Staged: exportStaged,
				Branch: exportBranch,
				Commit: exportCommit,
				Fast:   exportFast,
			}
			reviewRes, err := review.DefaultService().Analyze(cmd.Context(), opts)
			if err != nil {
				return utils.NewCommandError("ANALYSIS_FAILED", fmt.Sprintf("Export analysis failed: %v", err))
			}
			res = &engine.CLIResult{
				Findings:   reviewRes.Findings,
				Summary:    reviewRes.Summary,
				IsBlocking: reviewRes.IsBlocking,
			}
		} else {
			// Scan target directory
			runner := engine.NewCLIRunner()
			scanOpts := engine.CLIOptions{
				TargetDirectory: targetDir,
				Offline:         true,
				Fast:            exportFast,
				Format:          outFmt,
			}
			var err error
			res, err = runner.RunScan(context.Background(), targetDir, scanOpts)
			if err != nil {
				return utils.NewCommandError("SCAN_FAILED", fmt.Sprintf("Export scan failed: %v", err))
			}
		}

		formatter := engine.NewOutputFormatter()
		var outWriter io.Writer = os.Stdout
		if ctx.OutputFile != "" {
			f, createErr := os.Create(ctx.OutputFile)
			if createErr != nil {
				return utils.NewCommandError("INTERNAL_ERROR", fmt.Sprintf("Failed creating export file %s: %v", ctx.OutputFile, createErr))
			}
			defer f.Close()
			outWriter = f
		}

		if err := formatter.Render(outWriter, res, outFmt); err != nil {
			return utils.NewCommandError("INTERNAL_ERROR", fmt.Sprintf("Failed rendering export: %v", err))
		}

		if ctx.OutputFile != "" && !ctx.Quiet && !ctx.IsAgent {
			utils.Success("Exported %d findings to %s (%s format)", len(res.Findings), ctx.OutputFile, strings.ToUpper(exportFormat))
		}

		return nil
	},
}

// COMMAND REGISTRATION & FLAG INITIALIZATION

func init() {
	exportCmd.Flags().StringVarP(&exportFormat, "format", "f", "sarif", "Output format: sarif, json, csv, markdown")
	exportCmd.Flags().StringVarP(&exportOutput, "output", "o", "", "Write exported report directly to a file")
	exportCmd.Flags().BoolVarP(&exportStaged, "staged", "s", false, "Export findings from staged git changes")
	exportCmd.Flags().StringVarP(&exportBranch, "branch", "b", "", "Export findings against target branch")
	exportCmd.Flags().StringVarP(&exportCommit, "commit", "c", "", "Export findings from a specific commit")
	exportCmd.Flags().BoolVar(&exportFast, "fast", false, "Export using fast AST/pattern rules only")
}
