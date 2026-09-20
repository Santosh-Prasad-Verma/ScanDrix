// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/cli/status"
	"github.com/scandrix/backend/internal/cli/updater"
	"github.com/scandrix/backend/internal/cli/utils"
	"github.com/spf13/cobra"
)

// GLOBAL CLI FLAGS

var (
	formatFlag  string
	outputFlag  string
	verboseFlag bool
	quietFlag   bool
	agentFlag   bool
	jsonFlag    bool
)

// ROOT COMMAND CONFIGURATION

// RootCmd is the central entrypoint command for ScanDrix CLI.
var RootCmd = &cobra.Command{
	Use:   "scandrix",
	Short: "ScanDrix CLI — Autonomous AI Code Review & Security Assurance Platform",
	Long: `ScanDrix CLI provides enterprise-grade AI-powered code review, AST vulnerability detection,
dynamic penetration testing, and coding session telemetry directly from your terminal.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	Version:        utils.CLIVersion,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Bare invocation: print banner and check for updates
		status.PrintBanner(".")
		fmt.Println("\nRun 'scandrix --help' for available commands and usage instructions.")

		// Non-blocking update check
		if info, err := updater.CheckUpdate(utils.CLIVersion, ""); err == nil && info.UpdateAvailable {
			fmt.Printf("\n⚡ Update available: %s → %s. Run 'scandrix update' to upgrade.\n", info.CurrentVersion, info.LatestVersion)
		}
		return nil
	},
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if jsonFlag {
			formatFlag = "json"
		}
		utils.SetOutputMode(quietFlag, verboseFlag)
		return nil
	},
	PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
		fullCmd := strings.Join(os.Args[1:], " ")
		if fullCmd != "" {
			_ = utils.RecordRecentActivity(fullCmd)
		}
		return nil
	},
}

// COMMAND TREE REGISTRATION

func init() {
	RootCmd.PersistentFlags().StringVarP(&formatFlag, "format", "f", "terminal", "Output format: terminal, json, markdown, sarif, agent, prompt")
	RootCmd.PersistentFlags().BoolVar(&jsonFlag, "json", false, "Output result as JSON")
	RootCmd.PersistentFlags().StringVarP(&outputFlag, "output", "o", "", "Output file path (for json, markdown, sarif)")
	RootCmd.PersistentFlags().BoolVarP(&verboseFlag, "verbose", "v", false, "Verbose debug logging")
	RootCmd.PersistentFlags().BoolVarP(&quietFlag, "quiet", "q", false, "Quiet mode (errors only)")
	RootCmd.PersistentFlags().BoolVar(&agentFlag, "agent", false, "Agent mode: deterministic machine-readable JSON output")

	// Core subcommands
	RootCmd.AddCommand(reviewCmd)
	RootCmd.AddCommand(authCmd)
	RootCmd.AddCommand(configCmd)
	RootCmd.AddCommand(hookCmd)
	RootCmd.AddCommand(decisionsCmd)
	RootCmd.AddCommand(prCmd)
	RootCmd.AddCommand(rulesCmd)
	RootCmd.AddCommand(skillsCmd)
	RootCmd.AddCommand(statusCmd)
	RootCmd.AddCommand(subscribeCmd)
	RootCmd.AddCommand(updateCmd)
	RootCmd.AddCommand(schemaCmd)

	// ScanDrix Superpower subcommands
	RootCmd.AddCommand(chatCmd)
	RootCmd.AddCommand(tuiCmd)
	RootCmd.AddCommand(scanCmd)
	RootCmd.AddCommand(diffCmd)
	RootCmd.AddCommand(fixCmd)
	RootCmd.AddCommand(exportCmd)
	RootCmd.AddCommand(pentestCmd)
	RootCmd.AddCommand(mcpCmd)
	RootCmd.AddCommand(serverCmd)
	RootCmd.AddCommand(traceCmd)
	RootCmd.AddCommand(historyCmd)
	RootCmd.AddCommand(dryRunCmd)
	RootCmd.AddCommand(ciCmd)

	// Root-level aliases for common auth operations
	RootCmd.AddCommand(loginAliasCmd)
	RootCmd.AddCommand(logoutAliasCmd)
	RootCmd.AddCommand(whoamiAliasCmd)

	RootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print ScanDrix CLI version and build metadata",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprintf(cmd.OutOrStdout(), "ScanDrix CLI %s (schema v%s)\n", utils.CLIVersion, utils.SchemaVersion)
	},
}

// EXECUTION & NORMALIZED ERROR HANDLING

// Execute runs the Cobra root command and handles normalized error exits.
func Execute() {
	startTime := time.Now().UTC()
	if err := RootCmd.Execute(); err != nil {
		normErr := utils.NormalizeCommandError(err)

		if agentFlag {
			cmdName := "scandrix"
			if len(os.Args) > 1 {
				cmdName = os.Args[1]
			}
			env := utils.BuildAgentErrorEnvelope(
				cmdName,
				utils.AgentErrorPayload{
					Code:    normErr.Code,
					Message: normErr.Message,
					Details: normErr.Details,
				},
				startTime,
			)
			_ = utils.EmitAgentEnvelope(env, outputFlag)
		} else {
			utils.Error("%s", normErr.Message)
		}

		os.Exit(normErr.ExitCode)
	}
}
