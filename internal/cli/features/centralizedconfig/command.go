// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package centralizedconfig

import (
	"github.com/scandrix/backend/internal/cli/services/api"
	"github.com/spf13/cobra"
)

const CentralizedConfigDescription = "Manage centralized repository configuration from the ScanDrix CLI. Team API key or session auth required."

// RegisterCentralizedConfigCommand attaches centralized configuration management subcommands.
func RegisterCentralizedConfigCommand(parent *cobra.Command) {
	var (
		serverURL string
		authToken string
	)

	centralizedCmd := &cobra.Command{
		Use:   "centralized",
		Short: "Manage organization-wide centralized repository configuration",
		Long:  CentralizedConfigDescription,
	}

	centralizedCmd.PersistentFlags().StringVar(&serverURL, "server", "", "ScanDrix API server URL override")
	centralizedCmd.PersistentFlags().StringVar(&authToken, "token", "", "Authentication token or team key override")

	// 1. `centralized status`
	var statusJSON bool
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show whether centralized config is enabled and its selected repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := api.NewClient(serverURL, authToken, "")
			return ConfigCentralizedStatusAction(cmd.Context(), client, ConfigCentralizedStatusOptions{JSON: statusJSON})
		},
	}
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "Output centralized config status as JSON")

	// 2. `centralized init [repository]`
	var (
		initSyncOpt string
		initJSON    bool
	)
	initCmd := &cobra.Command{
		Use:   "init [repository]",
		Short: "Enable centralized config and assign a configuration repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			repo := "."
			if len(args) > 0 {
				repo = args[0]
			}
			client := api.NewClient(serverURL, authToken, "")
			return ConfigCentralizedInitAction(cmd.Context(), client, repo, ConfigCentralizedInitOptions{
				SyncOption: initSyncOpt,
				JSON:       initJSON,
			})
		},
	}
	initCmd.Flags().StringVar(&initSyncOpt, "sync-option", "pr", "Sync strategy after initialization: pr or manual")
	initCmd.Flags().BoolVar(&initJSON, "json", false, "Output initialization response as JSON")

	// 3. `centralized sync`
	var syncJSON bool
	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync centralized configuration rules across tracked repositories",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := api.NewClient(serverURL, authToken, "")
			return ConfigCentralizedSyncAction(cmd.Context(), client, ConfigCentralizedActionOptions{JSON: syncJSON})
		},
	}
	syncCmd.Flags().BoolVar(&syncJSON, "json", false, "Output sync response as JSON")

	// 4. `centralized disable`
	var disableJSON bool
	disableCmd := &cobra.Command{
		Use:   "disable",
		Short: "Disable centralized config and detach the active configuration repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := api.NewClient(serverURL, authToken, "")
			return ConfigCentralizedDisableAction(cmd.Context(), client, ConfigCentralizedActionOptions{JSON: disableJSON})
		},
	}
	disableCmd.Flags().BoolVar(&disableJSON, "json", false, "Output disable response as JSON")

	// 5. `centralized download --out <path>`
	var (
		downloadOut  string
		downloadJSON bool
	)
	downloadCmd := &cobra.Command{
		Use:   "download",
		Short: "Download the centralized configuration zip archive",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := api.NewClient(serverURL, authToken, "")
			return ConfigCentralizedDownloadAction(cmd.Context(), client, ConfigCentralizedDownloadOptions{
				Out:  downloadOut,
				JSON: downloadJSON,
			})
		},
	}
	downloadCmd.Flags().StringVar(&downloadOut, "out", "scandrix-centralized-config.zip", "Output path for downloaded zip archive")
	downloadCmd.Flags().BoolVar(&downloadJSON, "json", false, "Output download metadata as JSON")

	centralizedCmd.AddCommand(statusCmd)
	centralizedCmd.AddCommand(initCmd)
	centralizedCmd.AddCommand(syncCmd)
	centralizedCmd.AddCommand(disableCmd)
	centralizedCmd.AddCommand(downloadCmd)

	parent.AddCommand(centralizedCmd)
}
