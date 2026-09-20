// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/cli/engine"
	"github.com/scandrix/backend/internal/cli/utils"
	"github.com/scandrix/backend/pkg/models"
	"github.com/spf13/cobra"
)

// SCAN COMMAND FLAGS

var (
	scanFast           bool
	scanRulesOnly      bool
	scanOffline        bool
	scanHeavy          bool
	scanFocus          string
	scanFix            bool
	scanReportFile     string
	scanFailOnSeverity string
)

// SCAN COMMAND SPECIFICATION

var scanCmd = &cobra.Command{
	Use:     "scan [target]",
	Aliases: []string{"deep-scan"},
	Short:   "Deep scan repository or directory for security flaws, bugs, and code smells",
	Long:    `Performs comprehensive multi-language AST security audits, semantic vulnerability checks, and invariant rule evaluations.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := utils.NewCommandContext("scan", formatFlag, outputFlag, verboseFlag, quietFlag, agentFlag)

		targetDir := "."
		if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
			targetDir = args[0]
		}

		outFmt := engine.FormatTable
		if ctx.IsAgent {
			outFmt = engine.FormatAgent
		} else if formatFlag != "" {
			switch strings.ToLower(formatFlag) {
			case "json":
				outFmt = engine.FormatJSON
			case "sarif":
				outFmt = engine.FormatSARIF
			case "markdown", "md":
				outFmt = engine.FormatMarkdown
			case "agent":
				outFmt = engine.FormatAgent
			default:
				outFmt = engine.FormatTable
			}
		}

		threshold := models.SeverityHigh
		if scanFailOnSeverity != "" {
			threshold = models.FindingSeverity(strings.ToUpper(scanFailOnSeverity))
		}

		runner := engine.NewCLIRunner()
		var totalLinesScanned int
		opts := engine.CLIOptions{
			TargetDirectory:   targetDir,
			Format:            outFmt,
			AgentMode:         ctx.IsAgent,
			SeverityThreshold: threshold,
			Focus:             scanFocus,
			Fix:               scanFix,
			Fast:              scanFast,
			RulesOnly:         scanRulesOnly,
			Offline:           scanOffline,
			Heavy:             scanHeavy,
		}

		if !ctx.IsAgent && !ctx.Quiet {
			fmt.Printf("\nExploring %s workspace files ⌄\n\n", targetDir)
			startTime := time.Now()
			lastUpdate := time.Now()
			analyzedCount := 0
			analyzedMap := make(map[string]bool)

			opts.OnProgress = func(relPath string, current int, lineCount int) {
				totalLinesScanned += lineCount
				now := time.Now()
				if now.Sub(lastUpdate) < 90*time.Millisecond && current%30 != 0 {
					return
				}
				lastUpdate = now

				lower := strings.ToLower(relPath)
				isKey := strings.Contains(lower, "auth") || strings.Contains(lower, "login") ||
					strings.Contains(lower, "token") || strings.Contains(lower, "secret") ||
					strings.Contains(lower, "controller") || strings.Contains(lower, "gateway") ||
					strings.Contains(lower, "chat.go") || strings.Contains(lower, "main.go") ||
					strings.Contains(lower, "runner.go")

				if isKey && !analyzedMap[relPath] && analyzedCount < 5 {
					analyzedMap[relPath] = true
					analyzedCount++
					fmt.Print("\r\033[K")
					fmt.Printf("  Analyzed %s #L1-%d\n", filepath.Base(relPath), lineCount)
				}

				status := fmt.Sprintf("  Scanning... %d files (%d lines)", current, totalLinesScanned)
				if len(status) > 65 {
					status = status[:65]
				}
				fmt.Printf("\r\033[K%s", status)
			}

			opts.OnStatus = func(statusMsg string) {
				fmt.Print("\r\033[K")
				if strings.Contains(statusMsg, "AST") || strings.Contains(statusMsg, "rule") {
					fmt.Println("  Searched security AST patterns & invariant rules")
				} else if strings.Contains(statusMsg, "synthesized") {
					fmt.Printf("  ✦ %s\n", statusMsg)
				} else if strings.Contains(statusMsg, "skipped") || strings.Contains(statusMsg, "failed") {
					fmt.Printf("  ⚠️ %s\n", statusMsg)
				} else if strings.Contains(statusMsg, "AI") || strings.Contains(statusMsg, "Synthesizing") {
					elapsed := int(time.Since(startTime).Seconds())
					if elapsed < 1 {
						elapsed = 1
					}
					fmt.Printf("  Thought for %ds ›\n", elapsed)
					fmt.Println("  Working..")
				}
			}
		}

		res, err := runner.RunScan(context.Background(), targetDir, opts)
		if !ctx.IsAgent && !ctx.Quiet {
			fmt.Print("\r\033[K\n")
		}
		if err != nil {
			return utils.NewCommandError("SCAN_EXECUTION_ERROR", fmt.Sprintf("Scan failed: %v", err))
		}

		var outWriter io.Writer = os.Stdout
		if ctx.OutputFile != "" {
			f, createErr := os.Create(ctx.OutputFile)
			if createErr != nil {
				return utils.NewCommandError("INTERNAL_ERROR", fmt.Sprintf("Failed creating output file %s: %v", ctx.OutputFile, createErr))
			}
			defer f.Close()
			outWriter = f
		}

		formatter := engine.NewOutputFormatter()
		if err := formatter.RenderWithFields(outWriter, res, outFmt, ""); err != nil {
			return utils.NewCommandError("INTERNAL_ERROR", fmt.Sprintf("Failed rendering scan output: %v", err))
		}

		if ctx.OutputFile != "" && !ctx.IsAgent {
			utils.Success("Scan report saved to: %s", ctx.OutputFile)
		} else if scanReportFile != "" && !ctx.IsAgent {
			reportPath := scanReportFile
			if targetDir != "" && targetDir != "." {
				if fi, err := os.Stat(targetDir); err == nil && fi.IsDir() {
					reportPath = filepath.Join(targetDir, filepath.Base(scanReportFile))
				}
			}
			f, err := os.Create(reportPath)
			if err == nil {
				_ = formatter.Render(f, res, engine.FormatMarkdown)
				_ = f.Close()
				utils.Success("Full Security Audit Markdown Report saved to: %s", reportPath)
			}
		}

		if res.IsBlocking && !ctx.IsAgent {
			return utils.NewCommandError("SECURITY_POLICY_VIOLATION", "Scan identified blocking security vulnerabilities.", 1, nil)
		}

		return nil
	},
}

// COMMAND REGISTRATION & FLAG INITIALIZATION

func init() {
	scanCmd.Flags().BoolVar(&scanFast, "fast", false, "Fast scan mode (skip deep AI synthesis)")
	scanCmd.Flags().BoolVar(&scanRulesOnly, "rules-only", false, "Scan using only local/catalog invariant rules")
	scanCmd.Flags().BoolVar(&scanOffline, "offline", false, "Run entirely offline without cloud/LLM calls")
	scanCmd.Flags().BoolVar(&scanHeavy, "heavy", false, "Deep scan with multi-critic verification")
	scanCmd.Flags().StringVar(&scanFocus, "focus", "", "Focus scan on specific area (e.g. 'auth', 'sql')")
	scanCmd.Flags().BoolVar(&scanFix, "fix", false, "Automatically apply AST-guided fixes")
	scanCmd.Flags().StringVar(&scanReportFile, "report", "", "Generate standalone Markdown report (e.g. scandrix-security-scan-report.md)")
	scanCmd.Flags().StringVar(&scanFailOnSeverity, "fail-on-severity", "", "Fail if findings meet or exceed severity (CRITICAL, HIGH, MEDIUM)")
}
