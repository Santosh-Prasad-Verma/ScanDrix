// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/cli/configcli"
	"github.com/scandrix/backend/internal/cli/formatters"
	"github.com/scandrix/backend/internal/cli/git"
	"github.com/scandrix/backend/internal/cli/services/api"
	"github.com/scandrix/backend/internal/cli/utils"
	"github.com/spf13/cobra"
)

// CONFIGURATION COMMAND FLAGS

var (
	remoteShortcut  string
	centralizedSync string
	downloadOutPath string
	noPromptFlag    bool
	setupYesFlag    bool
)

// CONFIG PARENT COMMAND

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Inspect and manage global, repo-level, and centralized settings",
	Long: `Manage tracked remote repositories, centralized organization rules,
and local .scandrix/config.json configuration.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if remoteShortcut != "" {
			return runRemoteAdd(cmd, remoteShortcut)
		}
		return cmd.Help()
	},
}

// CONFIG SHOW

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show merged global, repository, and environment configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := configcli.Load(".")
		if agentFlag || formatFlag == "json" {
			env := utils.BuildAgentSuccessEnvelope("config show", cfg, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}

		data, _ := json.MarshalIndent(cfg, "", "  ")
		fmt.Println(string(data))
		return nil
	},
}

// REMOTE REPOSITORY MANAGEMENT

var configRemoteCmd = &cobra.Command{
	Use:   "remote",
	Short: "Manage remote repositories tracked in ScanDrix",
}

var configRemoteAddCmd = &cobra.Command{
	Use:   "add [repository]",
	Short: "Register a repository with ScanDrix (use '.' for current repo)",
	RunE: func(cmd *cobra.Command, args []string) error {
		repo := "."
		if len(args) > 0 {
			repo = args[0]
		}
		return runRemoteAdd(cmd, repo)
	},
}

func runRemoteAdd(cmd *cobra.Command, repo string) error {
	client := api.NewClient("", "", "")
	namespace := repo
	provider := "github"
	defaultBranch := "main"

	if repo == "." || repo == "" {
		if info, err := git.DetectRemote(cmd.Context(), "."); err == nil {
			namespace = info.NamespacePath
			provider = string(info.Provider)
			defaultBranch = info.DefaultBranch
		}
	}

	tracked, err := client.TrackRepository(cmd.Context(), namespace, provider, defaultBranch)
	if err != nil {
		return err
	}

	if agentFlag || formatFlag == "json" {
		env := utils.BuildAgentSuccessEnvelope("config remote add", tracked, time.Now())
		return utils.EmitAgentEnvelope(env, outputFlag)
	}

	utils.Success("✔ Repository %s (%s) tracked successfully in ScanDrix!", tracked.Namespace, tracked.Provider)
	return nil
}

var configRemoteListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all repositories tracked by active organization",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := api.NewClient("", "", "")
		repos, err := client.ListRepositories(cmd.Context())
		if err != nil {
			return err
		}

		if agentFlag || formatFlag == "json" {
			env := utils.BuildAgentSuccessEnvelope("config remote list", repos, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}

		formatters.PrintRepoList(cmd.OutOrStdout(), repos)
		return nil
	},
}

var configRemoteShowCmd = &cobra.Command{
	Use:   "show [repository]",
	Short: "Show settings for a tracked repository",
	RunE: func(cmd *cobra.Command, args []string) error {
		repo := "."
		if len(args) > 0 {
			repo = args[0]
		}
		if repo == "." {
			if info, err := git.DetectRemote(cmd.Context(), "."); err == nil {
				repo = info.NamespacePath
			}
		}

		client := api.NewClient("", "", "")
		settings, err := client.GetRepositorySettings(cmd.Context(), repo)
		if err != nil {
			return err
		}

		if agentFlag || formatFlag == "json" {
			env := utils.BuildAgentSuccessEnvelope("config remote show", settings, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}

		formatters.PrintRepoSettings(cmd.OutOrStdout(), settings)
		return nil
	},
}

var configRemoteOpenCmd = &cobra.Command{
	Use:   "open [repository]",
	Short: "Open repository dashboard in default web browser",
	RunE: func(cmd *cobra.Command, args []string) error {
		repo := "."
		if len(args) > 0 {
			repo = args[0]
		}
		if repo == "." {
			if info, err := git.DetectRemote(cmd.Context(), "."); err == nil {
				repo = info.NamespacePath
			}
		}

		cfg := configcli.Load(".")
		targetURL := fmt.Sprintf("%s/repos/%s", strings.TrimRight(cfg.ServerURL, "/"), repo)

		utils.Info("Opening %s in default browser...", targetURL)
		utils.OpenBrowser(targetURL)
		return nil
	},
}

var configRemoteSetupCmd = &cobra.Command{
	Use:   "setup [repository]",
	Short: "Interactive setup wizard for repository code review settings",
	RunE: func(cmd *cobra.Command, args []string) error {
		repo := "."
		if len(args) > 0 {
			repo = args[0]
		}
		if repo == "." {
			if info, err := git.DetectRemote(cmd.Context(), "."); err == nil {
				repo = info.NamespacePath
			}
		}

		client := api.NewClient("", "", "")
		current, err := client.GetRepositorySettings(cmd.Context(), repo)
		if err != nil {
			current = &api.RepositorySettings{
				Namespace: repo,
			}
		}

		currentSettings := configcli.RepositoryReviewSettings{
			RepositoryID:              current.RepositoryID,
			Namespace:                 current.Namespace,
			DefaultBranch:             current.DefaultBranch,
			ReviewEnabled:             current.ReviewEnabled,
			AutoApproveEnabled:        current.AutoApproveEnabled,
			RequestChangesMinSeverity: current.RequestChangesMinSeverity,
			IgnoredPaths:              current.IgnoredPaths,
			IgnoredFilePatterns:       current.IgnoredFilePatterns,
			BaseBranchPatterns:        current.BaseBranchPatterns,
			IgnoredTitlePatterns:      current.IgnoredTitlePatterns,
			FocusAreas:                current.FocusAreas,
			Reviewers:                 current.Reviewers,
			CustomSettings:            current.CustomSettings,
		}

		next, err := configcli.RunWizard(currentSettings, configcli.WizardOptions{
			Yes:            setupYesFlag,
			NonInteractive: agentFlag || formatFlag == "json",
			In:             cmd.InOrStdin(),
			Out:            cmd.OutOrStdout(),
		})
		if err != nil {
			return err
		}

		updatePayload := api.RepositorySettings{
			RepositoryID:              next.RepositoryID,
			Namespace:                 next.Namespace,
			DefaultBranch:             next.DefaultBranch,
			ReviewEnabled:             next.ReviewEnabled,
			AutoApproveEnabled:        next.AutoApproveEnabled,
			RequestChangesMinSeverity: next.RequestChangesMinSeverity,
			IgnoredPaths:              next.IgnoredPaths,
			IgnoredFilePatterns:       next.IgnoredFilePatterns,
			BaseBranchPatterns:        next.BaseBranchPatterns,
			IgnoredTitlePatterns:      next.IgnoredTitlePatterns,
			FocusAreas:                next.FocusAreas,
			Reviewers:                 next.Reviewers,
			CustomSettings:            next.CustomSettings,
		}

		updated, err := client.UpdateRepositorySettings(cmd.Context(), repo, updatePayload)
		if err != nil {
			return err
		}

		if agentFlag || formatFlag == "json" {
			env := utils.BuildAgentSuccessEnvelope("config remote setup", updated, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}

		if updated.PRURL != "" {
			utils.Success("✔ Repository settings change proposed through pull request: %s", updated.PRURL)
			return nil
		}

		utils.Success("✔ Repository settings successfully saved for %s!", repo)
		return nil
	},
}

// REMOTE PATTERN MANAGEMENT (ignore-files, review-files)

var configRemotePatternCmd = &cobra.Command{
	Use:   "pattern",
	Short: "Add or remove path glob patterns for repository settings",
}

func resolveRepoAndArgs(ctx context.Context, args []string, expectedTrailing int) (string, []string) {
	repo := "."
	remaining := args
	if len(args) > expectedTrailing {
		repo = args[0]
		remaining = args[1:]
	}
	if repo == "." {
		if info, err := git.DetectRemote(ctx, "."); err == nil {
			repo = info.NamespacePath
		}
	}
	return repo, remaining
}

var configRemotePatternAddCmd = &cobra.Command{
	Use:   "add [repository] <field> <pattern>",
	Short: "Add a pattern to a repository configuration field (e.g. ignore-files, review-files)",
	Args:  cobra.RangeArgs(2, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		repo, trailing := resolveRepoAndArgs(cmd.Context(), args, 2)
		field := trailing[0]
		pattern := trailing[1]

		client := api.NewClient("", "", "")
		settings, err := client.AddRepositoryPattern(cmd.Context(), repo, field, pattern)
		if err != nil {
			return err
		}

		if agentFlag || formatFlag == "json" {
			env := utils.BuildAgentSuccessEnvelope("config remote pattern add", settings, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}

		utils.Success("✔ Added pattern %q to %s for repository %s", pattern, field, repo)
		return nil
	},
}

var configRemotePatternRemoveCmd = &cobra.Command{
	Use:   "remove [repository] <field> <pattern>",
	Short: "Remove a pattern from a repository configuration field",
	Args:  cobra.RangeArgs(2, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		repo, trailing := resolveRepoAndArgs(cmd.Context(), args, 2)
		field := trailing[0]
		pattern := trailing[1]

		client := api.NewClient("", "", "")
		settings, err := client.RemoveRepositoryPattern(cmd.Context(), repo, field, pattern)
		if err != nil {
			return err
		}

		if agentFlag || formatFlag == "json" {
			env := utils.BuildAgentSuccessEnvelope("config remote pattern remove", settings, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}

		utils.Success("✔ Removed pattern %q from %s for repository %s", pattern, field, repo)
		return nil
	},
}

var configRemoteAddIgnoreCmd = &cobra.Command{
	Use:   "add-ignore-file [repository] <pattern>",
	Short: "Add a pattern to ignored file patterns for the repository",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		repo, trailing := resolveRepoAndArgs(cmd.Context(), args, 1)
		pattern := trailing[0]

		client := api.NewClient("", "", "")
		settings, err := client.AddRepositoryPattern(cmd.Context(), repo, "ignore-files", pattern)
		if err != nil {
			return err
		}

		if agentFlag || formatFlag == "json" {
			env := utils.BuildAgentSuccessEnvelope("config remote add-ignore-file", settings, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}

		utils.Success("✔ Added ignore pattern %q for repository %s", pattern, repo)
		return nil
	},
}

var configRemoteRemoveIgnoreCmd = &cobra.Command{
	Use:   "remove-ignore-file [repository] <pattern>",
	Short: "Remove a pattern from ignored file patterns for the repository",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		repo, trailing := resolveRepoAndArgs(cmd.Context(), args, 1)
		pattern := trailing[0]

		client := api.NewClient("", "", "")
		settings, err := client.RemoveRepositoryPattern(cmd.Context(), repo, "ignore-files", pattern)
		if err != nil {
			return err
		}

		if agentFlag || formatFlag == "json" {
			env := utils.BuildAgentSuccessEnvelope("config remote remove-ignore-file", settings, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}

		utils.Success("✔ Removed ignore pattern %q for repository %s", pattern, repo)
		return nil
	},
}

var configRemoteAddBaseBranchCmd = &cobra.Command{
	Use:   "add-base-branch [repository] <pattern>",
	Short: "Add a pattern to base branch patterns for the repository",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		repo, trailing := resolveRepoAndArgs(cmd.Context(), args, 1)
		pattern := trailing[0]

		client := api.NewClient("", "", "")
		settings, err := client.AddRepositoryPattern(cmd.Context(), repo, "base-branches", pattern)
		if err != nil {
			return err
		}

		if agentFlag || formatFlag == "json" {
			env := utils.BuildAgentSuccessEnvelope("config remote add-base-branch", settings, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}

		utils.Success("✔ Added base branch pattern %q for repository %s", pattern, repo)
		return nil
	},
}

var configRemoteRemoveBaseBranchCmd = &cobra.Command{
	Use:   "remove-base-branch [repository] <pattern>",
	Short: "Remove a pattern from base branch patterns for the repository",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		repo, trailing := resolveRepoAndArgs(cmd.Context(), args, 1)
		pattern := trailing[0]

		client := api.NewClient("", "", "")
		settings, err := client.RemoveRepositoryPattern(cmd.Context(), repo, "base-branches", pattern)
		if err != nil {
			return err
		}

		if agentFlag || formatFlag == "json" {
			env := utils.BuildAgentSuccessEnvelope("config remote remove-base-branch", settings, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}

		utils.Success("✔ Removed base branch pattern %q for repository %s", pattern, repo)
		return nil
	},
}

var configRemoteAddIgnoreTitleCmd = &cobra.Command{
	Use:   "add-ignore-title [repository] <pattern>",
	Short: "Add a pattern to ignored PR title patterns for the repository",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		repo, trailing := resolveRepoAndArgs(cmd.Context(), args, 1)
		pattern := trailing[0]

		client := api.NewClient("", "", "")
		settings, err := client.AddRepositoryPattern(cmd.Context(), repo, "ignore-titles", pattern)
		if err != nil {
			return err
		}

		if agentFlag || formatFlag == "json" {
			env := utils.BuildAgentSuccessEnvelope("config remote add-ignore-title", settings, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}

		utils.Success("✔ Added ignore title pattern %q for repository %s", pattern, repo)
		return nil
	},
}

var configRemoteRemoveIgnoreTitleCmd = &cobra.Command{
	Use:   "remove-ignore-title [repository] <pattern>",
	Short: "Remove a pattern from ignored PR title patterns for the repository",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		repo, trailing := resolveRepoAndArgs(cmd.Context(), args, 1)
		pattern := trailing[0]

		client := api.NewClient("", "", "")
		settings, err := client.RemoveRepositoryPattern(cmd.Context(), repo, "ignore-titles", pattern)
		if err != nil {
			return err
		}

		if agentFlag || formatFlag == "json" {
			env := utils.BuildAgentSuccessEnvelope("config remote remove-ignore-title", settings, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}

		utils.Success("✔ Removed ignore title pattern %q for repository %s", pattern, repo)
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set [repository] <key> <value>",
	Short: "Set a repository setting directly",
	Long:  fmt.Sprintf("Supported setting keys: %s", strings.Join(configcli.SupportedRepoSettingKeys, ", ")),
	Args:  cobra.RangeArgs(2, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		repo, trailing := resolveRepoAndArgs(cmd.Context(), args, 2)
		key := trailing[0]
		val := trailing[1]

		if err := configcli.ValidateRepositorySettingKey(key); err != nil {
			return err
		}

		client := api.NewClient("", "", "")
		current, err := client.GetRepositorySettings(cmd.Context(), repo)
		if err != nil {
			current = &api.RepositorySettings{
				Namespace: repo,
			}
		}

		reviewSettings := configcli.RepositoryReviewSettings{
			RepositoryID:              current.RepositoryID,
			Namespace:                 current.Namespace,
			DefaultBranch:             current.DefaultBranch,
			ReviewEnabled:             current.ReviewEnabled,
			AutoApproveEnabled:        current.AutoApproveEnabled,
			RequestChangesMinSeverity: current.RequestChangesMinSeverity,
			IgnoredPaths:              current.IgnoredPaths,
			IgnoredFilePatterns:       current.IgnoredFilePatterns,
			BaseBranchPatterns:        current.BaseBranchPatterns,
			IgnoredTitlePatterns:      current.IgnoredTitlePatterns,
			FocusAreas:                current.FocusAreas,
			Reviewers:                 current.Reviewers,
			CustomSettings:            current.CustomSettings,
		}

		if err := configcli.ApplyRepositorySetting(&reviewSettings, key, val); err != nil {
			return err
		}

		updatePayload := api.RepositorySettings{
			RepositoryID:              reviewSettings.RepositoryID,
			Namespace:                 reviewSettings.Namespace,
			DefaultBranch:             reviewSettings.DefaultBranch,
			ReviewEnabled:             reviewSettings.ReviewEnabled,
			AutoApproveEnabled:        reviewSettings.AutoApproveEnabled,
			RequestChangesMinSeverity: reviewSettings.RequestChangesMinSeverity,
			IgnoredPaths:              reviewSettings.IgnoredPaths,
			IgnoredFilePatterns:       reviewSettings.IgnoredFilePatterns,
			BaseBranchPatterns:        reviewSettings.BaseBranchPatterns,
			IgnoredTitlePatterns:      reviewSettings.IgnoredTitlePatterns,
			FocusAreas:                reviewSettings.FocusAreas,
			Reviewers:                 reviewSettings.Reviewers,
			CustomSettings:            reviewSettings.CustomSettings,
		}

		updated, err := client.UpdateRepositorySettings(cmd.Context(), repo, updatePayload)
		if err != nil {
			return err
		}

		if agentFlag || formatFlag == "json" {
			env := utils.BuildAgentSuccessEnvelope("config set", updated, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}

		if updated.PRURL != "" {
			utils.Success("✔ Repository setting change proposed through pull request: %s", updated.PRURL)
			return nil
		}

		utils.Success("✔ Repository setting %s set to %s for %s", key, val, repo)
		return nil
	},
}

// CENTRALIZED CONFIGURATION MANAGEMENT

var configCentralizedCmd = &cobra.Command{
	Use:   "centralized",
	Short: "Manage centralized repository configurations across organization",
}

var configCentralizedStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show whether centralized config is active and its source repo",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := api.NewClient("", "", "")
		status, err := client.GetCentralizedStatus(cmd.Context())
		if err != nil {
			return err
		}

		if agentFlag || formatFlag == "json" {
			env := utils.BuildAgentSuccessEnvelope("config centralized status", status, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}

		if !status.Enabled || (status.Repository == nil && status.SelectedRepository == nil) {
			utils.Warn("Centralized config is disabled.")
			return nil
		}

		utils.Success("Centralized config is enabled.")
		if status.Repository != nil {
			fmt.Printf("Repository: %s (%s)\n", status.Repository.Name, status.Repository.ID)
		} else if status.SelectedRepository != nil {
			fmt.Printf("Repository: %s (%s)\n", status.SelectedRepository.Namespace, status.SelectedRepository.Provider)
		}
		if status.LastSyncedAt != "" {
			fmt.Printf("Last Synced:       %s\n", status.LastSyncedAt)
		}
		return nil
	},
}

var configCentralizedInitCmd = &cobra.Command{
	Use:   "init [repository]",
	Short: "Enable centralized config and assign source repository",
	RunE: func(cmd *cobra.Command, args []string) error {
		repo := "."
		if len(args) > 0 {
			repo = args[0]
		}
		if repo == "." {
			if info, err := git.DetectRemote(cmd.Context(), "."); err == nil {
				repo = info.NamespacePath
			}
		}

		syncOpt := "pr"
		if centralizedSync == "manual" {
			syncOpt = "manual"
		}

		client := api.NewClient("", "", "")
		resp, err := client.InitCentralizedConfig(cmd.Context(), repo, syncOpt)
		if err != nil {
			return err
		}

		if agentFlag || formatFlag == "json" {
			env := utils.BuildAgentSuccessEnvelope("config centralized init", resp, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}

		if resp.Message != "" {
			utils.Success("%s", resp.Message)
		} else {
			utils.Success("✔ Centralized config enabled for %s", repo)
		}
		if resp.GetPRURL() != "" {
			fmt.Printf("Pull request: %s\n", resp.GetPRURL())
		}
		return nil
	},
}

var configCentralizedSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Propagate centralized rules to all repositories in workspace",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := api.NewClient("", "", "")
		resp, err := client.SyncCentralizedConfig(cmd.Context())
		if err != nil {
			return err
		}

		if agentFlag || formatFlag == "json" {
			env := utils.BuildAgentSuccessEnvelope("config centralized sync", resp, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}

		if resp.Message != "" {
			utils.Success("%s", resp.Message)
		} else {
			utils.Success("✔ Centralized configuration synchronized to repositories!")
		}
		return nil
	},
}

var configCentralizedDisableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Disable centralized configuration mode",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := api.NewClient("", "", "")
		resp, err := client.DisableCentralizedConfig(cmd.Context())
		if err != nil {
			return err
		}

		if agentFlag || formatFlag == "json" {
			env := utils.BuildAgentSuccessEnvelope("config centralized disable", resp, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}

		if resp.Message != "" {
			utils.Success("%s", resp.Message)
		} else {
			utils.Success("✔ Centralized configuration disabled.")
		}
		return nil
	},
}

var configCentralizedDownloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download the centralized configuration zip package",
	RunE: func(cmd *cobra.Command, args []string) error {
		if downloadOutPath == "" {
			return fmt.Errorf("required flag --out <path> not specified")
		}

		client := api.NewClient("", "", "")
		if err := client.DownloadCentralizedZip(cmd.Context(), downloadOutPath); err != nil {
			return err
		}

		utils.Success("✔ Centralized configuration zip downloaded to %s", downloadOutPath)
		return nil
	},
}

// COMMAND REGISTRATION & FLAG INITIALIZATION

func init() {
	configCmd.Flags().StringVarP(&remoteShortcut, "remote", "r", "", "Shortcut to add a repository (equivalent to 'config remote add [repo]')")
	configCmd.Flags().BoolVar(&noPromptFlag, "no-prompt", false, "Skip interactive setup prompt")

	configRemoteCmd.AddCommand(configRemoteAddCmd)
	configRemoteCmd.AddCommand(configRemoteSetupCmd)
	configRemoteCmd.AddCommand(configRemoteListCmd)
	configRemoteCmd.AddCommand(configRemoteShowCmd)
	configRemoteCmd.AddCommand(configRemoteOpenCmd)
	configRemotePatternCmd.AddCommand(configRemotePatternAddCmd)
	configRemotePatternCmd.AddCommand(configRemotePatternRemoveCmd)
	configRemoteCmd.AddCommand(configRemotePatternCmd)
	configRemoteCmd.AddCommand(configRemoteAddIgnoreCmd)
	configRemoteCmd.AddCommand(configRemoteRemoveIgnoreCmd)
	configRemoteCmd.AddCommand(configRemoteAddBaseBranchCmd)
	configRemoteCmd.AddCommand(configRemoteRemoveBaseBranchCmd)
	configRemoteCmd.AddCommand(configRemoteAddIgnoreTitleCmd)
	configRemoteCmd.AddCommand(configRemoteRemoveIgnoreTitleCmd)
	configRemoteCmd.AddCommand(cloneConfigCmd(configSetCmd))

	configRemoteSetupCmd.Flags().BoolVarP(&setupYesFlag, "yes", "y", false, "Accept all default recommended settings without prompts")

	// Repository configuration alias ('scandrix config repo')
	configRepoCmd := &cobra.Command{
		Use:    "repo",
		Hidden: true,
		Short:  configRemoteCmd.Short,
	}
	configRepoCmd.AddCommand(cloneConfigCmd(configRemoteAddCmd))
	configRepoCmd.AddCommand(cloneConfigCmd(configRemoteSetupCmd))
	configRepoCmd.AddCommand(cloneConfigCmd(configRemoteListCmd))
	configRepoCmd.AddCommand(cloneConfigCmd(configRemoteShowCmd))
	configRepoCmd.AddCommand(cloneConfigCmd(configRemoteOpenCmd))
	repoPatternCmd := cloneConfigCmd(configRemotePatternCmd)
	repoPatternCmd.AddCommand(cloneConfigCmd(configRemotePatternAddCmd))
	repoPatternCmd.AddCommand(cloneConfigCmd(configRemotePatternRemoveCmd))
	configRepoCmd.AddCommand(repoPatternCmd)
	configRepoCmd.AddCommand(cloneConfigCmd(configRemoteAddIgnoreCmd))
	configRepoCmd.AddCommand(cloneConfigCmd(configRemoteRemoveIgnoreCmd))
	configRepoCmd.AddCommand(cloneConfigCmd(configRemoteAddBaseBranchCmd))
	configRepoCmd.AddCommand(cloneConfigCmd(configRemoteRemoveBaseBranchCmd))
	configRepoCmd.AddCommand(cloneConfigCmd(configRemoteAddIgnoreTitleCmd))
	configRepoCmd.AddCommand(cloneConfigCmd(configRemoteRemoveIgnoreTitleCmd))
	configRepoCmd.AddCommand(cloneConfigCmd(configSetCmd))

	configCentralizedInitCmd.Flags().StringVar(&centralizedSync, "sync-option", "pr", "Sync strategy after initialization: pr or manual")
	configCentralizedDownloadCmd.Flags().StringVar(&downloadOutPath, "out", "", "Output path for downloaded zip file")
	_ = configCentralizedDownloadCmd.MarkFlagRequired("out")

	configCentralizedCmd.AddCommand(configCentralizedStatusCmd)
	configCentralizedCmd.AddCommand(configCentralizedInitCmd)
	configCentralizedCmd.AddCommand(configCentralizedSyncCmd)
	configCentralizedCmd.AddCommand(configCentralizedDisableCmd)
	configCentralizedCmd.AddCommand(configCentralizedDownloadCmd)

	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configRemoteCmd)
	configCmd.AddCommand(configRepoCmd)
	configCmd.AddCommand(configCentralizedCmd)
	configCmd.AddCommand(cloneConfigCmd(configRemoteSetupCmd))
	configCmd.AddCommand(cloneConfigCmd(configRemoteOpenCmd))
}

func cloneConfigCmd(src *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:   src.Use,
		Short: src.Short,
		Long:  src.Long,
		Args:  src.Args,
		RunE:  src.RunE,
	}
}
