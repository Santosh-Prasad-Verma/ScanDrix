// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/scandrix/backend/internal/cli/chat"
	"github.com/scandrix/backend/internal/cli/configcli"
	"github.com/scandrix/backend/internal/cli/utils"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// CHAT COMMAND SPECIFICATION (Interactive Autonomous Terminal)

var chatCmd = &cobra.Command{
	Use:     "chat [prompt...]",
	Aliases: []string{"ask", "interactive", "drixy"},
	Short:   "Launch live interactive AI agent terminal (CommandCode / Claude Code style)",
	Long:    `Launches a bidirectional, real-time autonomous AI code analysis & pairing session with full workspace context.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := configcli.Load(".")
		if cfg.AccessToken == "" && cfg.APIKey == "" {
			if !term.IsTerminal(int(os.Stdin.Fd())) {
				return utils.NewCommandError("AUTH_REQUIRED", "You must be authenticated to start an interactive review session. Run 'scandrix auth login' or 'scandrix auth team-key <key>'.")
			}
			fmt.Println("🔒 Authentication Required")
			fmt.Println("\nTo authenticate:")
			fmt.Println("  • Run 'scandrix auth login' to authenticate via your browser")
			fmt.Println("  • Run 'scandrix auth team-key <key>' to configure a team API key")
			fmt.Println("  • Or set SCANDRIX_API_KEY / SCANDRIX_ACCESS_TOKEN in your environment")
			return nil
		}

		initialPrompt := ""
		if len(args) > 0 {
			initialPrompt = strings.Join(args, " ")
		}

		chat.StartInteractiveSession(".", initialPrompt)
		return nil
	},
}
