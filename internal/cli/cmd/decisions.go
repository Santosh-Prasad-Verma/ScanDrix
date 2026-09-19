// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/cli/services/lifecycle"
	"github.com/scandrix/backend/internal/cli/utils"
	"github.com/spf13/cobra"
)

// DECISIONS COMMAND FLAGS

var (
	decisionsAgents string
	codexConfigPath string
	captureAgent    string
	legacyAgent     string
	captureEvent    string
	captureSummary  string
)

// DECISIONS PARENT COMMAND (Session Tracking & Memory)

var decisionsCmd = &cobra.Command{
	Use:     "decisions",
	Aliases: []string{"memory"},
	Short:   "Session tracking, architectural decision capture, and AI assistant hooks",
	Long: `Install, configure, and capture coding session events and architectural decisions
from Claude Code, Cursor, and Codex CLI into ScanDrix.`,
}

// DECISIONS ENABLE (Assistant Hook Scaffolding)

var decisionsEnableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Install session tracking and decision capture hooks for AI assistants",
	RunE: func(cmd *cobra.Command, args []string) error {
		srv := lifecycle.DefaultService()
		agentsList := strings.Split(decisionsAgents, ",")

		installed, err := srv.EnableHooks(".", agentsList, codexConfigPath)
		if err != nil {
			return err
		}

		if agentFlag {
			env := utils.BuildAgentSuccessEnvelope("decisions enable", map[string]any{
				"installed_agents": installed,
			}, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}

		utils.Success("✔ Decision capture hooks enabled for: %s", strings.Join(installed, ", "))
		return nil
	},
}

// DECISIONS DISABLE

var decisionsDisableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Remove all installed assistant decision capture hooks",
	RunE: func(cmd *cobra.Command, args []string) error {
		srv := lifecycle.DefaultService()
		if err := srv.DisableHooks("."); err != nil {
			return err
		}

		if agentFlag {
			env := utils.BuildAgentSuccessEnvelope("decisions disable", map[string]bool{"disabled": true}, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}

		utils.Success("✔ Assistant decision capture hooks removed.")
		return nil
	},
}

// DECISIONS CAPTURE

var decisionsCaptureCmd = &cobra.Command{
	Use:   "capture [payload]",
	Short: "Internal hook command to submit decision capture event to API",
	RunE: func(cmd *cobra.Command, args []string) error {
		agent := captureAgent
		if agent == "" {
			agent = legacyAgent
		}
		if agent == "" {
			return fmt.Errorf("required flag --capture-agent <agent> not specified")
		}
		if captureEvent == "" {
			return fmt.Errorf("required flag --event <event> not specified")
		}

		var payloadObj any
		if len(args) > 0 {
			_ = json.Unmarshal([]byte(args[0]), &payloadObj)
		}

		srv := lifecycle.DefaultService()
		evt := lifecycle.Event{
			Agent:     agent,
			HookName:  captureEvent,
			Summary:   captureSummary,
			Payload:   payloadObj,
			Timestamp: time.Now().UTC(),
		}

		if err := srv.RecordEvent(cmd.Context(), evt); err != nil {
			return err
		}

		if agentFlag {
			env := utils.BuildAgentSuccessEnvelope("decisions capture", map[string]string{"status": "recorded"}, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}

		utils.Success("✔ Decision event %s captured from %s.", captureEvent, agent)
		return nil
	},
}

// INTERNAL ASSISTANT HOOK ENDPOINTS

var decisionsHooksCmd = &cobra.Command{
	Use:   "hooks",
	Short: "Internal session lifecycle hook handlers for specific AI assistants",
}

var decisionsHookClaudeCmd = &cobra.Command{
	Use:   "claude-code <hook-name>",
	Short: "Handle Claude Code lifecycle hooks",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		hookName := args[0]
		srv := lifecycle.DefaultService()
		return srv.RecordEvent(cmd.Context(), lifecycle.Event{
			Agent:    "claude-code",
			HookName: hookName,
		})
	},
}

var decisionsHookCursorCmd = &cobra.Command{
	Use:   "cursor <hook-name>",
	Short: "Handle Cursor lifecycle hooks",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		hookName := args[0]
		srv := lifecycle.DefaultService()
		return srv.RecordEvent(cmd.Context(), lifecycle.Event{
			Agent:    "cursor",
			HookName: hookName,
		})
	},
}

var decisionsHookCodexCmd = &cobra.Command{
	Use:   "codex <hook-name>",
	Short: "Handle Codex CLI lifecycle hooks",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		hookName := args[0]
		srv := lifecycle.DefaultService()
		return srv.RecordEvent(cmd.Context(), lifecycle.Event{
			Agent:    "codex",
			HookName: hookName,
		})
	},
}

// COMMAND REGISTRATION & FLAG INITIALIZATION

func init() {
	decisionsEnableCmd.Flags().StringVar(&decisionsAgents, "agents", "claude,cursor,codex", "Comma-separated list of target assistants: claude,cursor,codex")
	decisionsEnableCmd.Flags().StringVar(&codexConfigPath, "codex-config", "", "Path to Codex config.toml (default: ~/.codex/config.toml)")

	decisionsCaptureCmd.Flags().StringVar(&captureAgent, "capture-agent", "", "Agent name: claude-code, cursor, codex")
	decisionsCaptureCmd.Flags().StringVar(&legacyAgent, "agent", "", "Legacy alias for --capture-agent")
	decisionsCaptureCmd.Flags().StringVar(&captureEvent, "event", "", "Hook event name (e.g. stop, session-start)")
	decisionsCaptureCmd.Flags().StringVar(&captureSummary, "summary", "", "Optional summary text")

	decisionsHooksCmd.AddCommand(decisionsHookClaudeCmd)
	decisionsHooksCmd.AddCommand(decisionsHookCursorCmd)
	decisionsHooksCmd.AddCommand(decisionsHookCodexCmd)

	decisionsCmd.AddCommand(decisionsEnableCmd)
	decisionsCmd.AddCommand(decisionsDisableCmd)
	decisionsCmd.AddCommand(decisionsCaptureCmd)
	decisionsCmd.AddCommand(decisionsHooksCmd)
}
