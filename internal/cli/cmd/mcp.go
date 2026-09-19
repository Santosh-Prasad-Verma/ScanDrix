// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/scandrix/backend/internal/cli/configcli"
	"github.com/scandrix/backend/internal/cli/utils"
	"github.com/scandrix/backend/internal/mcp/gateway"
	"github.com/spf13/cobra"
)

// MCP COMMAND FLAGS

var (
	mcpRole  string
	mcpToken string
)

// MCP SERVER COMMAND (Model Context Protocol stdio Server)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Run standard Model Context Protocol (MCP) server over stdio for IDE AI agents",
	Long: `Run standard Model Context Protocol (MCP) server over stdio for IDE AI agents (Claude Desktop, Cursor, Zed, Codex).

IDE Configuration (.cursor/mcp.json or claude_desktop_config.json):
  {
    "mcpServers": {
      "scandrix": {
        "command": "scandrix",
        "args": ["mcp"]
      }
    }
  }`,
	RunE: func(cmd *cobra.Command, args []string) error {
		role := gateway.RoleAdmin
		if mcpRole != "" {
			role = gateway.AgentRole(strings.ToLower(mcpRole))
		}

		token := mcpToken
		if token == "" {
			cfg := configcli.Load(".")
			if cfg.AccessToken != "" {
				token = cfg.AccessToken
			} else if cfg.APIKey != "" {
				token = cfg.APIKey
			}
		}

		server := gateway.NewMCPServer()
		ctx := gateway.WithCallerRole(context.Background(), role)
		if token != "" {
			ctx = gateway.WithCallerToken(ctx, token)
		}

		if err := server.ServeStdio(ctx, os.Stdin, os.Stdout); err != nil {
			return utils.NewCommandError("MCP_SERVER_ERROR", fmt.Sprintf("MCP server exited with error: %v", err))
		}
		return nil
	},
}

// COMMAND REGISTRATION & FLAG INITIALIZATION

func init() {
	mcpCmd.Flags().StringVar(&mcpRole, "role", "admin", "Caller authorization role: admin, reviewer, auditor, mutator")
	mcpCmd.Flags().StringVar(&mcpToken, "token", "", "ScanDrix access token or team API key for authenticated tool execution")
}
