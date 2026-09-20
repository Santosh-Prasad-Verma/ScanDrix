// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package repoconfig

import (
	"github.com/scandrix/backend/internal/cli/services/api"
	"github.com/spf13/cobra"
)

const RepositoryConfigDescription = "Inspect and update repository review settings in ScanDrix. Team API key or session auth required."

// RegisterRepositoryConfigCommand attaches remote repository management commands.
func RegisterRepositoryConfigCommand(parent *cobra.Command) {
	var (
		serverURL string
		authToken string
	)

	remoteCmd := &cobra.Command{
		Use:   "remote [repository]",
		Short: "Manage remote repository review settings in ScanDrix",
		Long:  RepositoryConfigDescription,
		RunE: func(cmd *cobra.Command, args []string) error {
			repo := "."
			if len(args) > 0 {
				repo = args[0]
			}
			client := api.NewClient(serverURL, authToken, "")
			return ConfigRemoteAddAction(cmd.Context(), client, repo, ConfigRepoAddOptions{})
		},
	}

	remoteCmd.PersistentFlags().StringVar(&serverURL, "server", "", "ScanDrix API server URL override")
	remoteCmd.PersistentFlags().StringVar(&authToken, "token", "", "Authentication token or team key override")

	// 1. `remote add [repository]`
	var addNoPrompt, addJSON bool
	addCmd := &cobra.Command{
		Use:   "add [repository]",
		Short: "Connect a repository to ScanDrix for automated code reviews",
		RunE: func(cmd *cobra.Command, args []string) error {
			repo := "."
			if len(args) > 0 {
				repo = args[0]
			}
			client := api.NewClient(serverURL, authToken, "")
			return ConfigRemoteAddAction(cmd.Context(), client, repo, ConfigRepoAddOptions{
				Prompt: !addNoPrompt,
				JSON:   addJSON,
			})
		},
	}
	addCmd.Flags().BoolVar(&addNoPrompt, "no-prompt", false, "Skip post-add setup prompt")
	addCmd.Flags().BoolVar(&addJSON, "json", false, "Output connection result as JSON")

	// 2. `remote list`
	var listJSON bool
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all repositories configured in ScanDrix",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := api.NewClient(serverURL, authToken, "")
			return ConfigRepoListAction(cmd.Context(), client, ConfigRepoListOptions{JSON: listJSON})
		},
	}
	listCmd.Flags().BoolVar(&listJSON, "json", false, "Output configured repositories as JSON")

	// 3. `remote show [repository]`
	var showJSON bool
	showCmd := &cobra.Command{
		Use:   "show [repository]",
		Short: "Show current review settings for a repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			repo := "."
			if len(args) > 0 {
				repo = args[0]
			}
			client := api.NewClient(serverURL, authToken, "")
			return ConfigRepoShowAction(cmd.Context(), client, repo, ConfigRepoShowOptions{JSON: showJSON})
		},
	}
	showCmd.Flags().BoolVar(&showJSON, "json", false, "Output settings as JSON")

	// 4. `remote set <key> <value> [repository]`
	var setJSON bool
	setCmd := &cobra.Command{
		Use:   "set <key> <value> [repository]",
		Short: "Update a specific repository setting (e.g. review_enabled, default_branch)",
		Args:  cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			val := args[1]
			repo := "."
			if len(args) >= 3 {
				repo = args[2]
			}
			client := api.NewClient(serverURL, authToken, "")
			return ConfigRepoSetAction(cmd.Context(), client, repo, key, val, ConfigRepoMutationOptions{JSON: setJSON})
		},
	}
	setCmd.Flags().BoolVar(&setJSON, "json", false, "Output updated settings as JSON")

	// 5. `remote pattern-add <field> <pattern> [repository]`
	var patAddJSON bool
	patAddCmd := &cobra.Command{
		Use:   "pattern-add <field> <pattern> [repository]",
		Short: "Add a glob pattern to ignore-files, base-branches, or review-files",
		Args:  cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			field := args[0]
			pat := args[1]
			repo := "."
			if len(args) >= 3 {
				repo = args[2]
			}
			client := api.NewClient(serverURL, authToken, "")
			return ConfigRepoPatternAddAction(cmd.Context(), client, repo, field, pat, ConfigRepoMutationOptions{JSON: patAddJSON})
		},
	}
	patAddCmd.Flags().BoolVar(&patAddJSON, "json", false, "Output as JSON")

	// 6. `remote pattern-remove <field> <pattern> [repository]`
	var patRemJSON bool
	patRemCmd := &cobra.Command{
		Use:   "pattern-remove <field> <pattern> [repository]",
		Short: "Remove a glob pattern from ignore-files, base-branches, or review-files",
		Args:  cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			field := args[0]
			pat := args[1]
			repo := "."
			if len(args) >= 3 {
				repo = args[2]
			}
			client := api.NewClient(serverURL, authToken, "")
			return ConfigRepoPatternRemoveAction(cmd.Context(), client, repo, field, pat, ConfigRepoMutationOptions{JSON: patRemJSON})
		},
	}
	patRemCmd.Flags().BoolVar(&patRemJSON, "json", false, "Output as JSON")

	// 7. `remote open [repository]`
	var openSection string
	var openJSON bool
	openCmd := &cobra.Command{
		Use:   "open [repository]",
		Short: "Open the ScanDrix web dashboard for this repository in your browser",
		RunE: func(cmd *cobra.Command, args []string) error {
			repo := "."
			if len(args) > 0 {
				repo = args[0]
			}
			client := api.NewClient(serverURL, authToken, "")
			return ConfigRepoOpenAction(cmd.Context(), client, repo, ConfigRepoOpenOptions{
				Section: openSection,
				JSON:    openJSON,
			})
		},
	}
	openCmd.Flags().StringVar(&openSection, "section", "", "Dashboard subtab to open (e.g. settings, rules, integrations)")
	openCmd.Flags().BoolVar(&openJSON, "json", false, "Output dashboard URL as JSON instead of launching browser")

	remoteCmd.AddCommand(addCmd)
	remoteCmd.AddCommand(listCmd)
	remoteCmd.AddCommand(showCmd)
	remoteCmd.AddCommand(setCmd)
	remoteCmd.AddCommand(patAddCmd)
	remoteCmd.AddCommand(patRemCmd)
	remoteCmd.AddCommand(openCmd)

	parent.AddCommand(remoteCmd)
}
