// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/scandrix/backend/internal/cli/services/git"
	"github.com/scandrix/backend/internal/cli/utils"
	"github.com/spf13/cobra"
)

// DIFF COMMAND FLAGS

var (
	diffStaged   bool
	diffBranch   string
	diffCommit   string
	diffFilePath string
)

// DIFF COMMAND SPECIFICATION

var diffCmd = &cobra.Command{
	Use:   "diff [files...]",
	Short: "Inspect unified git diff with repository metadata",
	Long:  `Inspects git diff for staged, working tree, branch, or specific commit changes.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := utils.NewCommandContext("diff", formatFlag, outputFlag, verboseFlag, quietFlag, agentFlag)
		gitSvc := git.DefaultService()

		var rawDiff string
		var err error

		if diffFilePath != "" {
			bytes, readErr := os.ReadFile(diffFilePath)
			if readErr != nil {
				return utils.NewCommandError("INVALID_INPUT", fmt.Sprintf("Failed to read diff file: %v", readErr))
			}
			rawDiff = string(bytes)
		} else {
			if !gitSvc.IsGitRepository(context.Background()) {
				return utils.NewCommandError(utils.ErrCodeNotInGitRepo, "Not a git repository. Run inside a git repo or initialize one with 'git init'.")
			}

			if diffStaged {
				if len(args) > 0 {
					rawDiff, err = gitSvc.GetFilesDiff(context.Background(), args, true)
				} else {
					rawDiff, err = gitSvc.GetStagedDiff(context.Background())
				}
			} else if diffBranch != "" {
				rawDiff, err = gitSvc.GetBranchDiff(context.Background(), diffBranch)
			} else if diffCommit != "" {
				rawDiff, err = gitSvc.GetCommitDiff(context.Background(), diffCommit)
			} else if len(args) > 0 {
				rawDiff, err = gitSvc.GetFilesDiff(context.Background(), args, false)
			} else {
				// Default to working tree diff, fallback to staged if empty
				rawDiff, err = gitSvc.GetWorkingTreeDiff(context.Background())
				if err == nil && strings.TrimSpace(rawDiff) == "" {
					rawDiff, err = gitSvc.GetStagedDiff(context.Background())
				}
			}

			if err != nil {
				return utils.NewCommandError("GIT_ERROR", fmt.Sprintf("Failed extracting diff: %v", err))
			}
		}

		if strings.TrimSpace(rawDiff) == "" {
			if ctx.IsAgent {
				env := utils.BuildAgentSuccessEnvelope(ctx.Command, map[string]any{
					"has_changes": false,
					"diff":        "",
				}, ctx.StartedAt)
				return utils.EmitAgentEnvelope(env, ctx.OutputFile)
			}
			utils.Info("✨ No git changes detected.")
			return nil
		}

		if ctx.IsAgent {
			env := utils.BuildAgentSuccessEnvelope(ctx.Command, map[string]any{
				"has_changes": true,
				"diff":        rawDiff,
			}, ctx.StartedAt)
			return utils.EmitAgentEnvelope(env, ctx.OutputFile)
		}

		if ctx.OutputFile != "" {
			if writeErr := os.WriteFile(ctx.OutputFile, []byte(rawDiff), 0644); writeErr != nil {
				return utils.NewCommandError("INTERNAL_ERROR", fmt.Sprintf("Failed writing diff to %s: %v", ctx.OutputFile, writeErr))
			}
			utils.Success("Diff saved to %s", ctx.OutputFile)
			return nil
		}

		fmt.Print(rawDiff)
		return nil
	},
}

// COMMAND REGISTRATION & FLAG INITIALIZATION

func init() {
	diffCmd.Flags().BoolVarP(&diffStaged, "staged", "s", false, "Inspect staged git changes")
	diffCmd.Flags().StringVarP(&diffBranch, "branch", "b", "", "Inspect changes against a target branch")
	diffCmd.Flags().StringVarP(&diffCommit, "commit", "c", "", "Inspect changes in a specific commit")
	diffCmd.Flags().StringVar(&diffFilePath, "file", "", "Inspect a unified .diff or .patch file")
}
