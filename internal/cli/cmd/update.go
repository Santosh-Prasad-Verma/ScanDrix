// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package cmd

import (
	"fmt"
	"time"

	"github.com/scandrix/backend/internal/cli/updater"
	"github.com/scandrix/backend/internal/cli/utils"
	"github.com/spf13/cobra"
)

// UPDATE COMMAND FLAGS

var updateCheckOnly bool

// UPDATE COMMAND SPECIFICATION (Binary Self-Update)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Check for and apply self-updates to the ScanDrix CLI binary",
	RunE: func(cmd *cobra.Command, args []string) error {
		utils.Info("Checking for ScanDrix updates...")
		info, err := updater.CheckUpdate(utils.CLIVersion, "")
		if err != nil {
			return fmt.Errorf("update check failed: %w", err)
		}

		if agentFlag || formatFlag == "json" {
			env := utils.BuildAgentSuccessEnvelope("update", info, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}

		if !info.UpdateAvailable {
			utils.Success("✔ ScanDrix CLI is already up to date (%s).", utils.CLIVersion)
			return nil
		}

		fmt.Printf("⚡ New version available: %s → %s\n", info.CurrentVersion, info.LatestVersion)
		if updateCheckOnly {
			return nil
		}

		utils.Info("Applying update from %s...", info.DownloadURL)
		if err := updater.ApplyUpdate(info.DownloadURL); err != nil {
			return fmt.Errorf("failed applying update: %w", err)
		}

		utils.Success("✔ ScanDrix CLI successfully updated to %s!", info.LatestVersion)
		return nil
	},
}

// COMMAND REGISTRATION & FLAG INITIALIZATION

func init() {
	updateCmd.Flags().BoolVar(&updateCheckOnly, "check-only", false, "Only check if update is available without installing")
}
