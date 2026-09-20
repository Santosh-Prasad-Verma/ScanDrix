// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package cmd

import (
	"fmt"
	"time"

	"github.com/scandrix/backend/internal/cli/hooks"
	"github.com/scandrix/backend/internal/cli/utils"
	"github.com/spf13/cobra"
)

// HOOK COMMAND FLAGS

var (
	hookPreCommit bool
	hookPrePush   bool
	hookFailOn    string
	hookDryRun    bool
	hookFast      bool
	hookForce     bool
)

// HOOK PARENT COMMAND

var hookCmd = &cobra.Command{
	Use:   "hook",
	Short: "Manage Git pre-commit and pre-push automated review guards",
	Long: `Install, inspect, or remove Git review hooks that automatically verify
changesets before committing or pushing to remotes.`,
}

// HOOK INSTALLATION

var hookInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install pre-push and pre-commit review guards in .git/hooks",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Default to pre-push if neither explicitly set
		if !hookPreCommit && !hookPrePush {
			hookPrePush = true
		}

		failOn := hookFailOn
		if failOn == "" {
			failOn = "CRITICAL"
		}

		if !hookDryRun {
			if err := hooks.Install(".", hookPreCommit, hookPrePush, failOn, hookFast); err != nil {
				return err
			}
		}

		if agentFlag {
			env := utils.BuildAgentSuccessEnvelope("hook install", map[string]any{
				"pre_commit": hookPreCommit,
				"pre_push":   hookPrePush,
				"fail_on":    failOn,
				"dry_run":    hookDryRun,
			}, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}

		mode := "installed"
		if hookDryRun {
			mode = "planned (dry-run)"
		}
		utils.Success("✔ ScanDrix Git hooks %s successfully!", mode)
		return nil
	},
}

// HOOK UNINSTALLATION

var hookUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove ScanDrix automated review guards from .git/hooks",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !hookDryRun {
			if err := hooks.Uninstall("."); err != nil {
				return err
			}
		}

		if agentFlag {
			env := utils.BuildAgentSuccessEnvelope("hook uninstall", map[string]any{
				"dry_run": hookDryRun,
			}, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}

		mode := "uninstalled"
		if hookDryRun {
			mode = "planned for removal (dry-run)"
		}
		utils.Success("✔ ScanDrix Git hooks %s cleanly.", mode)
		return nil
	},
}

// HOOK STATUS INSPECTION

var hookStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current installation state of Git review hooks",
	RunE: func(cmd *cobra.Command, args []string) error {
		status, err := hooks.Status(".")
		if err != nil {
			return err
		}

		if agentFlag || formatFlag == "json" {
			env := utils.BuildAgentSuccessEnvelope("hook status", status, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}

		fmt.Println("\n=== Git Review Hook Status ===")
		if !status.GitRepoDetected {
			fmt.Println("Git Repository: \033[90mNot detected in current directory\033[0m")
			return nil
		}

		fmt.Println("Git Repository: \033[32mDetected\033[0m")

		preCommitState := "\033[90mNot Installed\033[0m"
		if status.PreCommitActive {
			preCommitState = "\033[32mActive (ScanDrix Guard)\033[0m"
		} else if status.PreCommitIsCustom {
			preCommitState = "\033[33mCustom User Hook\033[0m"
		}
		fmt.Printf("Pre-Commit:     %s\n", preCommitState)

		prePushState := "\033[90mNot Installed\033[0m"
		if status.PrePushActive {
			prePushState = "\033[32mActive (ScanDrix Guard)\033[0m"
		} else if status.PrePushIsCustom {
			prePushState = "\033[33mCustom User Hook\033[0m"
		}
		fmt.Printf("Pre-Push:       %s\n", prePushState)

		return nil
	},
}

// COMMAND REGISTRATION & FLAG INITIALIZATION

func init() {
	hookInstallCmd.Flags().BoolVar(&hookPreCommit, "pre-commit", false, "Install pre-commit guard")
	hookInstallCmd.Flags().BoolVar(&hookPrePush, "pre-push", false, "Install pre-push guard")
	hookInstallCmd.Flags().StringVar(&hookFailOn, "fail-on", "CRITICAL", "Minimum severity to block (CRITICAL, HIGH, MEDIUM)")
	hookInstallCmd.Flags().StringVar(&hookFailOn, "fail-on-severity", "CRITICAL", "Alias for --fail-on")
	hookInstallCmd.Flags().BoolVar(&hookDryRun, "dry-run", false, "Preview planned hook actions without writing files")
	hookInstallCmd.Flags().BoolVar(&hookFast, "fast", true, "Use fast mode for review execution")
	hookInstallCmd.Flags().BoolVar(&hookForce, "force", false, "Force overwrite without prompt")

	hookUninstallCmd.Flags().BoolVar(&hookDryRun, "dry-run", false, "Preview planned uninstall without writing files")

	hookCmd.AddCommand(hookInstallCmd)
	hookCmd.AddCommand(hookUninstallCmd)
	hookCmd.AddCommand(hookStatusCmd)
}
