// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/scandrix/backend/internal/cli/services/git"
	"github.com/scandrix/backend/internal/cli/services/review"
	"github.com/scandrix/backend/internal/cli/utils"
	"github.com/spf13/cobra"
)

// FIX COMMAND FLAGS

var (
	fixStaged    bool
	fixBranch    string
	fixCommit    string
	fixFile      string
	fixRulesOnly bool
	fixFast      bool
)

// FIX COMMAND SPECIFICATION

var fixCmd = &cobra.Command{
	Use:   "fix [target] [files...]",
	Short: "Automatically remediate code security flaws and quality issues using AST patches",
	Long:  `Analyzes code changes and applies precise AST-guided automated remediations directly to local source files.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := utils.NewCommandContext("fix", formatFlag, outputFlag, verboseFlag, quietFlag, agentFlag)

		if !ctx.Quiet && !ctx.IsAgent {
			fmt.Println("🔧 Launching ScanDrix AST-Guided Automated Remediation Engine...")
		}

		var targetPath string
		var targetFiles []string
		for _, a := range args {
			if !strings.HasPrefix(a, "-") {
				if fi, err := os.Stat(a); err == nil {
					targetPath = a
					if fi.IsDir() {
						break
					}
				} else {
					targetFiles = append(targetFiles, a)
				}
			}
		}

		// If a directory was provided, delegate to scan with fix enabled
		if targetPath != "" {
			if fi, err := os.Stat(targetPath); err == nil && fi.IsDir() {
				scanFix = true
				scanFast = fixFast
				scanRulesOnly = fixRulesOnly
				return scanCmd.RunE(cmd, []string{targetPath})
			}
		}

		gitSvc := git.DefaultService()
		if !gitSvc.IsGitRepository(context.Background()) && fixFile == "" {
			// No git repo, run directory scan with fix
			scanFix = true
			scanFast = fixFast
			scanRulesOnly = fixRulesOnly
			return scanCmd.RunE(cmd, []string{"."})
		}

		// Execute review with Fix enabled
		opts := review.ReviewOptions{
			Staged:    fixStaged,
			Branch:    fixBranch,
			Commit:    fixCommit,
			File:      fixFile,
			RulesOnly: fixRulesOnly,
			Fast:      fixFast,
			Fix:       true,
			Files:     targetFiles,
		}

		srv := review.DefaultService()
		res, err := srv.Analyze(cmd.Context(), opts)
		if err != nil {
			return err
		}

		if ctx.IsAgent {
			env := utils.BuildAgentSuccessEnvelope(ctx.Command, res, ctx.StartedAt)
			return utils.EmitAgentEnvelope(env, ctx.OutputFile)
		}

		if res.FixesApplied > 0 {
			utils.Success("Successfully applied %d AST automated fixes.", res.FixesApplied)
		} else {
			utils.Info("No automated fixes could be applied.")
		}

		return nil
	},
}

// COMMAND REGISTRATION & FLAG INITIALIZATION

func init() {
	fixCmd.Flags().BoolVar(&fixStaged, "staged", false, "Fix staged git changes")
	fixCmd.Flags().StringVar(&fixBranch, "branch", "", "Fix changes against a target branch")
	fixCmd.Flags().StringVar(&fixCommit, "commit", "", "Fix changes in a specific commit")
	fixCmd.Flags().StringVar(&fixFile, "file", "", "Fix changes from a patch or diff file")
	fixCmd.Flags().BoolVar(&fixRulesOnly, "rules-only", false, "Apply fixes for custom/catalog rule violations only")
	fixCmd.Flags().BoolVar(&fixFast, "fast", false, "Fast fix mode using local AST patches")
}
