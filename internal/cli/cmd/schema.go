// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/scandrix/backend/internal/cli/utils"
	"github.com/spf13/cobra"
)

// SCHEMA COMMAND FLAGS

var schemaCommandPath string

// SCHEMA COMMAND SPECIFICATION (Agent Tool Introspection)

var schemaCmd = &cobra.Command{
	Use:   "schema",
	Short: "Inspect command schema for agent/tool introspection",
	Long:  `Outputs complete machine-readable command schemas with flags and options for AI agents.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := utils.NewCommandContext("schema", formatFlag, outputFlag, verboseFlag, quietFlag, agentFlag)

		target := RootCmd
		if schemaCommandPath != "" {
			target = utils.FindCommandByPath(RootCmd, schemaCommandPath)
			if target == nil {
				return utils.NewCommandError("INVALID_INPUT", fmt.Sprintf("Unknown command path: %s", schemaCommandPath))
			}
		}

		schema := utils.BuildCommandSchema(target)

		if ctx.IsAgent {
			env := utils.BuildAgentSuccessEnvelope(ctx.Command, schema, ctx.StartedAt)
			return utils.EmitAgentEnvelope(env, ctx.OutputFile)
		}

		data, err := json.MarshalIndent(schema, "", "  ")
		if err != nil {
			return utils.NewCommandError("INTERNAL_ERROR", fmt.Sprintf("Failed to marshal schema: %v", err))
		}

		if ctx.OutputFile != "" {
			if err := os.WriteFile(ctx.OutputFile, data, 0644); err != nil {
				return utils.NewCommandError("INTERNAL_ERROR", fmt.Sprintf("Failed to write output file: %v", err))
			}
			utils.Success("Schema saved to %s", ctx.OutputFile)
			return nil
		}

		fmt.Println(string(data))
		return nil
	},
}

// COMMAND REGISTRATION & FLAG INITIALIZATION

func init() {
	schemaCmd.Flags().StringVar(&schemaCommandPath, "command", "", "Command path (example: 'review' or 'pr suggestions')")
}
