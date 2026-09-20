// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package cmd

import (
	"fmt"
	"time"

	"github.com/scandrix/backend/internal/cli/status"
	"github.com/scandrix/backend/internal/cli/utils"
	"github.com/spf13/cobra"
)

// STATUS COMMAND SPECIFICATION

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show consolidated developer status, repository state, and hook health",
	RunE: func(cmd *cobra.Command, args []string) error {
		res, err := status.GetStatus(".")
		if err != nil {
			return err
		}

		if agentFlag || formatFlag == "json" {
			env := utils.BuildAgentSuccessEnvelope("status", res, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}

		fmt.Println("\n=== ScanDrix System Status ===")
		fmt.Printf("Version:        %s\n", res.Version)
		fmt.Printf("Server URL:     %s\n", res.ServerURL)
		fmt.Printf("Auth State:     %s\n", res.AuthMode)
		if res.UserEmail != "" {
			fmt.Printf("User:           %s\n", res.UserEmail)
		}
		fmt.Printf("Team API Key:   %s\n", res.TeamKeyStatus)
		fmt.Printf("Repository:     %s (%s)\n", res.Repository, res.CurrentBranch)
		fmt.Printf("Pre-Commit:     %s\n", res.PreCommitHook)
		fmt.Printf("Pre-Push:       %s\n", res.PrePushHook)
		fmt.Printf("Agent Hooks:    %s\n", res.AssistantHooks)
		fmt.Printf("Bundled Skills: %d skills active\n", res.BundledSkills)
		return nil
	},
}
