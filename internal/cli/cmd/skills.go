// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package cmd

import (
	"fmt"
	"time"

	"github.com/scandrix/backend/internal/cli/services/skills"
	"github.com/scandrix/backend/internal/cli/utils"
	"github.com/spf13/cobra"
)

// SKILLS COMMAND FLAGS

var skillsDryRun bool

// SKILLS PARENT COMMAND

var skillsCmd = &cobra.Command{
	Use:   "skills",
	Short: "Inspect, install, and synchronize bundled AI assistant skills",
	Long: `Manage embedded skills for Cursor, Claude Code, Codex, and Antigravity agents,
enabling AI coding assistants to automatically invoke ScanDrix review and rule enforcement.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSkillsList(cmd)
	},
}

// SKILLS LIST COMMAND

var skillsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all bundled assistant skills available in ScanDrix",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSkillsList(cmd)
	},
}

func runSkillsList(cmd *cobra.Command) error {
	srv := skills.DefaultService()
	catalog := srv.Catalog()

	if agentFlag || formatFlag == "json" {
		env := utils.BuildAgentSuccessEnvelope("skills list", catalog, time.Now())
		return utils.EmitAgentEnvelope(env, outputFlag)
	}

	fmt.Printf("\nBundled ScanDrix Assistant Skills (%d):\n\n", len(catalog))
	for i, sk := range catalog {
		fmt.Printf("  %d. %s\n     %s\n", i+1, sk.Name, sk.Description)
	}
	return nil
}

// SKILLS SYNC & RESYNC

var skillsSyncCmd = &cobra.Command{
	Use:     "sync",
	Aliases: []string{"resync"},
	Short:   "Sync bundled skills to detected local agent directories",
	RunE: func(cmd *cobra.Command, args []string) error {
		srv := skills.DefaultService()
		res, err := srv.Sync(".", skillsDryRun, false)
		if err != nil {
			return err
		}

		if agentFlag || formatFlag == "json" {
			env := utils.BuildAgentSuccessEnvelope("skills sync", res, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}

		mode := "synchronized"
		if skillsDryRun {
			mode = "planned (dry-run)"
		}
		utils.Success("✔ Skills %s across %d targets (%d created, %d updated, %d unchanged)",
			mode, len(res.Targets), res.Created, res.Updated, res.Unchanged)
		return nil
	},
}

// SKILLS INSTALLATION

var skillsInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install bundled skills into agent directories (creating folders if needed)",
	RunE: func(cmd *cobra.Command, args []string) error {
		srv := skills.DefaultService()
		res, err := srv.Sync(".", skillsDryRun, true)
		if err != nil {
			return err
		}

		if agentFlag || formatFlag == "json" {
			env := utils.BuildAgentSuccessEnvelope("skills install", res, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}

		utils.Success("✔ Skills installed across %d targets (%d created, %d updated)",
			len(res.Targets), res.Created, res.Updated)
		return nil
	},
}

// SKILLS UNINSTALLATION

var skillsUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove managed ScanDrix skills from local agent directories",
	RunE: func(cmd *cobra.Command, args []string) error {
		srv := skills.DefaultService()
		res, err := srv.Uninstall(".", skillsDryRun)
		if err != nil {
			return err
		}

		if agentFlag || formatFlag == "json" {
			env := utils.BuildAgentSuccessEnvelope("skills uninstall", res, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}

		utils.Success("✔ Managed skills removed from %d targets (%d files deleted)",
			len(res.Targets), res.Removed)
		return nil
	},
}

// SKILLS AUDIT & CHECK

var skillsCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Audit installed skills across detected agent directories",
	RunE: func(cmd *cobra.Command, args []string) error {
		srv := skills.DefaultService()
		report, err := srv.Check(".")
		if err != nil {
			return err
		}

		if agentFlag || formatFlag == "json" {
			env := utils.BuildAgentSuccessEnvelope("skills check", report, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}

		fmt.Printf("\nScanDrix Agent Skills Audit (%d active / %d scanned targets):\n\n",
			report.ActiveTargets, report.ScannedTargets)

		for _, t := range report.Targets {
			statusIcon := "\033[32m✔\033[0m"
			if t.Status == "outdated" {
				statusIcon = "\033[33m▲\033[0m"
			} else if t.Status == "missing" {
				statusIcon = "\033[31m✖\033[0m"
			}

			fmt.Printf("  %s %-32s [%s]\n", statusIcon, t.Target.Label, t.Status)
			fmt.Printf("    Path:      %s\n", t.Target.BaseDir)
			fmt.Printf("    Installed: %d up-to-date, %d outdated, %d missing\n",
				t.UpToDate, t.Outdated, t.Missing)
			if t.Stale > 0 {
				fmt.Printf("    Stale:     %d uncataloged managed entries\n", t.Stale)
			}
			fmt.Println()
		}

		if report.ActiveTargets == 0 {
			utils.Warn("No active agent configuration roots detected in workspace or home.")
			fmt.Println("Run 'scandrix skills install' to set up agent skills.")
		} else if report.HealthyTargets == report.ActiveTargets {
			utils.Success("All active agent targets are up-to-date with current ScanDrix skills.")
		} else {
			utils.Warn("Some agent skills are missing or outdated. Run 'scandrix skills sync' to synchronize.")
		}
		return nil
	},
}

// SKILLS PROMPT EXPORT COMMAND

var skillsPromptFormat string

var skillsPromptCmd = &cobra.Command{
	Use:   "prompt [skills...]",
	Short: "Export skills prompt definition for AI agent context injection",
	Long: `Format bundled skills as XML, JSON, or Markdown for injection into
system prompts, agent contexts, or custom LLM workflows.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		srv := skills.DefaultService()
		format := skillsPromptFormat
		if format == "" {
			format = "xml"
		}

		out, err := srv.FormatPrompt(format, args)
		if err != nil {
			return err
		}

		if agentFlag {
			env := utils.BuildAgentSuccessEnvelope("skills prompt", map[string]any{
				"format":  format,
				"content": out,
				"skills":  args,
			}, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}

		fmt.Println(out)
		return nil
	},
}

// COMMAND REGISTRATION & FLAG INITIALIZATION

func init() {
	skillsSyncCmd.Flags().BoolVar(&skillsDryRun, "dry-run", false, "Preview planned changes without writing files")
	skillsInstallCmd.Flags().BoolVar(&skillsDryRun, "dry-run", false, "Preview planned changes without writing files")
	skillsUninstallCmd.Flags().BoolVar(&skillsDryRun, "dry-run", false, "Preview planned changes without writing files")

	skillsPromptCmd.Flags().StringVarP(&skillsPromptFormat, "format", "f", "xml", "Output format: xml, json, or markdown")

	skillsCmd.AddCommand(skillsListCmd)
	skillsCmd.AddCommand(skillsSyncCmd)
	skillsCmd.AddCommand(skillsInstallCmd)
	skillsCmd.AddCommand(skillsUninstallCmd)
	skillsCmd.AddCommand(skillsCheckCmd)
	skillsCmd.AddCommand(skillsPromptCmd)
}
