// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/scandrix/backend/internal/cli/agents"
	"github.com/scandrix/backend/internal/cli/trace"
	"github.com/scandrix/backend/internal/cli/utils"
	"github.com/spf13/cobra"
)

// TRACE COMMAND FLAGS & RUNTIME OPTIONS

var (
	traceUIPort     int
	traceLimit      int
	traceRemote     string
	traceBranch     string
	traceAgents     string
	traceHead       string
	traceCodexConfig string
	traceNoPush     bool
	traceUnpin      bool
)

// TRACE ROOT COMMAND: RECALL DECISIONS FOR PATHS OR BRANCH

var traceCmd = &cobra.Command{
	Use:   "trace [paths...]",
	Short: "Read recorded architectural decisions, or manage session capture",
	Long: `Query decisions recorded for a file, module, or branch, and manage
continuous session capture across Claude Code, Cursor, and Codex.

Examples:
  scandrix trace
  scandrix trace src/auth/jwt.go
  scandrix trace internal/cli --limit 5
  scandrix trace enable
  scandrix trace status`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := trace.RecallOptions{
			Paths:  args,
			Limit:  traceLimit,
			Remote: traceRemote,
			Branch: traceBranch,
		}

		decisions, err := trace.RecallDecisions(cmd.Context(), ".", opts)
		if err != nil {
			return utils.NewCommandError("RECALL_FAILED", fmt.Sprintf("Failed recalling decisions: %v", err))
		}

		if len(decisions) == 0 {
			if len(args) > 0 {
				utils.Info("No decisions recorded for the specified paths: %s", strings.Join(args, ", "))
			} else {
				utils.Info("No decisions recorded for the current branch. Run 'scandrix trace enable' to start capturing.")
			}
			return nil
		}

		fmt.Printf("📋 Recalled Architectural Decisions (%d):\n\n", len(decisions))
		for i, d := range decisions {
			pinMarker := ""
			if d.Pinned {
				pinMarker = " 📌 [PINNED]"
			}
			fmt.Printf("  %d. [%s] %s%s\n", i+1, d.Type, d.Decision, pinMarker)
			fmt.Printf("     ID: %s\n", d.ID)
			if len(d.Scope) > 0 {
				fmt.Printf("     Scope: %s\n", strings.Join(d.Scope, ", "))
			}
			if d.Rationale != "" {
				lines := strings.Split(d.Rationale, "\n")
				firstLine := strings.TrimSpace(lines[0])
				if len(firstLine) > 100 {
					firstLine = firstLine[:100] + "..."
				}
				fmt.Printf("     Rationale: %s\n", firstLine)
			}
			fmt.Println()
		}
		return nil
	},
}

// TRACE ENABLE & DISABLE HOOKS

var traceEnableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Install session capture hooks for this repository",
	Long:  "Installs hooks into .claude/settings.json, .cursor/hooks.json, and git hooks (prepare-commit-msg, pre-push).",
	RunE: func(cmd *cobra.Command, args []string) error {
		agentList := strings.Split(traceAgents, ",")
		claudeStatus, cursorStatus, codexStatus := "skipped", "skipped", "skipped"

		for _, a := range agentList {
			switch strings.TrimSpace(strings.ToLower(a)) {
			case "claude", "claude-code":
				if changed, err := trace.InstallClaudeHooks("."); err == nil {
					if changed {
						claudeStatus = "installed (.claude/settings.json)"
					} else {
						claudeStatus = "already active"
					}
				}
			case "cursor":
				if changed, err := trace.InstallCursorHooks("."); err == nil {
					if changed {
						cursorStatus = "installed (.cursor/hooks.json)"
					} else {
						cursorStatus = "already active"
					}
				}
			case "codex":
				if changed, err := trace.InstallCodexHooks(traceCodexConfig); err == nil {
					if changed {
						codexStatus = "installed (~/.codex/config.toml)"
					} else {
						codexStatus = "already active"
					}
				} else {
					codexStatus = fmt.Sprintf("error: %v", err)
				}
			}
		}

		gitStatus := "skipped"
		if installed, err := trace.InstallGitTraceHooks(cmd.Context(), "."); err == nil {
			if len(installed) > 0 {
				gitStatus = fmt.Sprintf("installed (%s)", strings.Join(installed, ", "))
			} else {
				gitStatus = "already active"
			}
		}

		_ = trace.ConfigureTraceRefspec(cmd.Context(), ".", traceRemote)

		utils.Success("✔ ScanDrix Trace session capture enabled for this repository.")
		fmt.Printf("  • Claude Code hooks: %s\n", claudeStatus)
		fmt.Printf("  • Cursor IDE hooks:  %s\n", cursorStatus)
		fmt.Printf("  • Codex CLI hooks:   %s\n", codexStatus)
		fmt.Printf("  • Git hooks:         %s\n", gitStatus)
		fmt.Printf("  • Trace branch:      %s\n", trace.TraceBranch)
		fmt.Printf("  • Local store:       %s\n\n", trace.RepoStoreDir("."))
		fmt.Println("Nothing is committed to your source working tree. Run 'scandrix trace status' to verify.")
		return nil
	},
}

var traceDisableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Remove all session capture and git trace hooks",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, _ = trace.RemoveClaudeHooks(".")
		_, _ = trace.RemoveCursorHooks(".")
		removedCodex, _ := trace.RemoveCodexHooks(traceCodexConfig)
		removedGit, _ := trace.RemoveGitTraceHooks(cmd.Context(), ".")

		utils.Success("✔ Removed ScanDrix Trace hooks from this repository.")
		if removedCodex {
			fmt.Println("  • Codex CLI hooks:   removed (~/.codex/config.toml)")
		}
		if len(removedGit) > 0 {
			fmt.Printf("  • Cleaned git hooks: %s\n", strings.Join(removedGit, ", "))
		}
		return nil
	},
}

// TRACE STATUS

var traceStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Report what has been captured and which hooks are active",
	RunE: func(cmd *cobra.Command, args []string) error {
		rep, err := trace.GetTraceStatus(cmd.Context(), ".")
		if err != nil {
			return utils.NewCommandError("STATUS_FAILED", fmt.Sprintf("Failed reading trace status: %v", err))
		}
		fmt.Print(trace.FormatStatusReport(rep))

		if incidents, err := trace.ReadIncidents(".", 5); err == nil && len(incidents) > 0 {
			fmt.Printf("  Recent Operational Incidents (%d):\n", len(incidents))
			for _, inc := range incidents {
				fmt.Printf("    • [%s] %s: %s\n", inc.At, inc.Kind, inc.Message)
			}
			fmt.Println()
		}
		return nil
	},
}

// PIN & FORGET DECISION OVERRIDES

var tracePinCmd = &cobra.Command{
	Use:   "pin <decision-id>",
	Short: "Pin or unpin a critical decision to prioritize in reasoning context",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		decisionID := args[0]
		action := "pin"
		if traceUnpin {
			action = "unpin"
			if err := trace.RemovePin(".", decisionID); err != nil {
				return err
			}
			utils.Success("✔ Unpinned decision '%s'.", decisionID)
		} else {
			if err := trace.AddPin(".", decisionID); err != nil {
				return err
			}
			utils.Success("📌 Pinned decision '%s'.", decisionID)
		}

		shared, _ := trace.UpdateSharedDecisionCorrection(cmd.Context(), ".", decisionID, action, traceRemote)
		reportSharedOutcome(shared)
		return nil
	},
}

var traceForgetCmd = &cobra.Command{
	Use:   "forget <decision-id>",
	Short: "Tombstone a decision the assistant got wrong so it is excluded from future reasoning",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		decisionID := args[0]
		if err := trace.AddForget(".", decisionID); err != nil {
			return err
		}
		utils.Success("✔ Forgotten decision '%s'.", decisionID)

		shared, _ := trace.UpdateSharedDecisionCorrection(cmd.Context(), ".", decisionID, "forget", traceRemote)
		reportSharedOutcome(shared)
		return nil
	},
}

func reportSharedOutcome(outcome *trace.SharedCorrectionResult) {
	if outcome == nil || !outcome.Found {
		utils.Info("  Saved locally; decision was not yet published to shared trace branch.")
	} else if outcome.Pushed {
		utils.Info("  Published to %s on remote.", trace.TraceBranch)
	} else {
		utils.Info("  Saved to local %s ref; it will publish on the next push.", trace.TraceBranch)
	}
}

// BRANCH DISTILLATION & COMMIT TRAILER (CALLED BY GIT HOOKS)

var traceDistillCmd = &cobra.Command{
	Use:   "distill",
	Short: "Distill current or target branch into durable decisions (invoked by pre-push)",
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := trace.DistillOptions{
			Branch: traceBranch,
			Head:   traceHead,
			Remote: traceRemote,
			Push:   !traceNoPush,
		}

		res, err := trace.DistillBranch(cmd.Context(), ".", opts)
		if err != nil {
			return utils.NewCommandError("DISTILL_FAILED", fmt.Sprintf("Branch distillation failed: %v", err))
		}

		utils.Success("✔ Distilled %s: %d decisions across %d commits",
			res.Branch, res.DecisionsCount, res.CommitsProcessed)
		if res.Pushed {
			utils.Info("  Pushed decision record to %s on %s", trace.TraceBranch, traceRemote)
		}
		return nil
	},
}

var traceCommitTrailerCmd = &cobra.Command{
	Use:   "commit-trailer",
	Short: "Output the ScanDrix-Trace trailer for current commit (invoked by prepare-commit-msg)",
	RunE: func(cmd *cobra.Command, args []string) error {
		trailer := trace.ResolveCommitTrailer(cmd.Context(), ".")
		if trailer != "" {
			fmt.Println(trailer)
		}
		return nil
	},
}

// SESSION LIFECYCLE HOOKS RECEIVER

func runHookAction(cmd *cobra.Command, agentName, hookName string) error {
	payloadBytes, _ := io.ReadAll(os.Stdin)

	var adapter agents.AgentAdapter
	switch strings.ToLower(agentName) {
	case "claude", "claude-code":
		adapter = agents.NewClaudeCodeAdapter()
	case "cursor":
		adapter = agents.NewCursorAdapter()
	case "codex":
		adapter = agents.NewCodexAdapter()
	default:
		return nil // Fail open
	}

	evt, err := adapter.ParseHookEvent(hookName, payloadBytes)
	if err != nil || evt == nil {
		return nil // Fail open
	}

	coordinator := trace.NewLifecycleCoordinator(nil)
	_ = coordinator.Dispatch(cmd.Context(), ".", adapter.AgentType(), evt)
	return nil
}

var traceHooksCmd = &cobra.Command{
	Use:   "hooks [agent] [hook-name]",
	Short: "Internal session lifecycle event dispatcher (claude-code, cursor, codex)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return cmd.Help()
		}
		return runHookAction(cmd, args[0], args[1])
	},
}

var claudeCodeHookCmd = &cobra.Command{
	Use:   "claude-code <hook-name>",
	Short: "Handle Claude Code lifecycle hooks",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runHookAction(cmd, "claude-code", args[0])
	},
}

var cursorHookCmd = &cobra.Command{
	Use:   "cursor <hook-name>",
	Short: "Handle Cursor IDE lifecycle hooks",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runHookAction(cmd, "cursor", args[0])
	},
}

var codexHookCmd = &cobra.Command{
	Use:   "codex <hook-name>",
	Short: "Handle Codex CLI lifecycle hooks",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runHookAction(cmd, "codex", args[0])
	},
}

// LOCAL DASHBOARD COCKPIT UI

var traceUICmd = &cobra.Command{
	Use:   "ui",
	Short: "Launch interactive web viewer for recorded traces and decisions",
	RunE: func(cmd *cobra.Command, args []string) error {
		return trace.LaunchTraceUI(traceUIPort)
	},
}

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "List recent session traces and captured decisions",
	RunE: func(cmd *cobra.Command, args []string) error {
		return traceStatusCmd.RunE(cmd, args)
	},
}

// FLAG REGISTRATION & TREE INITIALIZATION

func init() {
	traceCmd.Flags().IntVarP(&traceLimit, "limit", "n", 10, "Maximum number of decisions to output")
	traceCmd.Flags().StringVar(&traceRemote, "remote", "origin", "Git remote holding the decision branch")
	traceCmd.Flags().StringVarP(&traceBranch, "branch", "b", "", "Specific branch to query (default: current branch)")

	traceEnableCmd.Flags().StringVar(&traceAgents, "agents", "claude,cursor,codex", "Comma-separated list of IDE agents: claude,cursor,codex")
	traceEnableCmd.Flags().StringVar(&traceRemote, "remote", "origin", "Git remote to synchronize decision branch with")
	traceEnableCmd.Flags().StringVar(&traceCodexConfig, "codex-config", "", "Path to Codex config.toml (default: ~/.codex/config.toml)")

	traceDisableCmd.Flags().StringVar(&traceCodexConfig, "codex-config", "", "Path to Codex config.toml (default: ~/.codex/config.toml)")

	tracePinCmd.Flags().BoolVar(&traceUnpin, "remove", false, "Unpin instead of pinning")

	traceDistillCmd.Flags().StringVarP(&traceBranch, "branch", "b", "", "Branch to distill (default: current branch)")
	traceDistillCmd.Flags().StringVar(&traceHead, "head", "", "Exact commit SHA supplied by pre-push")
	traceDistillCmd.Flags().StringVar(&traceRemote, "remote", "origin", "Git remote to push the decision branch to")
	traceDistillCmd.Flags().BoolVar(&traceNoPush, "no-push", false, "Write record locally without pushing to remote")

	traceUICmd.Flags().IntVarP(&traceUIPort, "port", "p", 4711, "Port for Trace dashboard server")

	traceHooksCmd.AddCommand(claudeCodeHookCmd)
	traceHooksCmd.AddCommand(cursorHookCmd)
	traceHooksCmd.AddCommand(codexHookCmd)

	traceCmd.AddCommand(traceEnableCmd)
	traceCmd.AddCommand(traceDisableCmd)
	traceCmd.AddCommand(traceStatusCmd)
	traceCmd.AddCommand(tracePinCmd)
	traceCmd.AddCommand(traceForgetCmd)
	traceCmd.AddCommand(traceDistillCmd)
	traceCmd.AddCommand(traceCommitTrailerCmd)
	traceCmd.AddCommand(traceHooksCmd)
	traceCmd.AddCommand(traceUICmd)
}

