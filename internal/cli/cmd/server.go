// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package cmd

import (
	"fmt"

	"github.com/scandrix/backend/internal/cli/devservercli"
	"github.com/scandrix/backend/internal/cli/utils"
	"github.com/spf13/cobra"
)

// DEV SERVER COMMAND FLAGS

var serverPort int

// DEV SERVER COMMAND SPECIFICATION

var serverCmd = &cobra.Command{
	Use:     "server",
	Aliases: []string{"dev-server", "dev"},
	Short:   "Start local ScanDrix HTTP API & Webhook development server",
	Long:    `Runs a lightweight local HTTP server for API mocking, webhook forwarding, and browser testing.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := devservercli.StartDevServer(serverPort); err != nil {
			return utils.NewCommandError("SERVER_ERROR", fmt.Sprintf("Error running ScanDrix server: %v", err))
		}
		return nil
	},
}

// COMMAND REGISTRATION & FLAG INITIALIZATION

func init() {
	serverCmd.Flags().IntVarP(&serverPort, "port", "p", 8080, "Port to listen on")
}
