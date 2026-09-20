// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package repoconfig

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/scandrix/backend/internal/cli/services/api"
	"github.com/scandrix/backend/internal/cli/utils"
)

var (
	RecommendedIgnoredFiles = []string{
		"*.lock",
		"package-lock.json",
		"yarn.lock",
		"pnpm-lock.yaml",
		".env",
		"dist/**",
		"coverage/**",
		"vendor/**",
	}

	RecommendedBaseBranches = []string{
		"main",
		"master",
		"develop",
		"release/*",
	}

	RecommendedIgnoredTitles = []string{
		"wip*",
		"draft*",
		"chore(release)*",
		"release:*",
	}
)

// RepositorySettingsSummary provides structured comparison between existing and target configuration.
type RepositorySettingsSummary struct {
	RepositoryName string                  `json:"repositoryName"`
	Current        *api.RepositorySettings `json:"current"`
	Next           *api.RepositorySettings `json:"next"`
	Applied        bool                    `json:"applied"`
}

// ConfigRepoSetupAction orchestrates the repository setup wizard flow.
func ConfigRepoSetupAction(ctx context.Context, client *api.Client, repository string, opts ConfigRepoSetupOptions) error {
	if client == nil {
		client = api.NewClient("", "", "")
	}
	if repository == "" || repository == "." {
		repository = "."
	}

	current, err := client.GetRepositorySettings(ctx, repository)
	if err != nil {
		// Fallback defaults if repository is new or offline
		current = &api.RepositorySettings{
			Namespace:                 repository,
			ReviewEnabled:             true,
			AutoApproveEnabled:        false,
			RequestChangesMinSeverity: "high",
			IgnoredFilePatterns:       RecommendedIgnoredFiles,
			BaseBranchPatterns:        RecommendedBaseBranches,
			IgnoredTitlePatterns:      RecommendedIgnoredTitles,
		}
	}

	nextSettings := *current
	reader := bufio.NewReader(os.Stdin)

	if !opts.Yes && !opts.JSON {
		utils.Info("ScanDrix Repository Review Setup Wizard")
		utils.Info("Configuring review policies for repository: %s\n", repository)

		// 1. General settings
		nextSettings.ReviewEnabled = promptYesNo(reader, "Enable automated ScanDrix code reviews on PRs?", current.ReviewEnabled)
		nextSettings.AutoApproveEnabled = promptYesNo(reader, "Auto-approve PRs with 0 critical/high findings?", current.AutoApproveEnabled)

		defSev := current.RequestChangesMinSeverity
		if defSev == "" {
			defSev = "high"
		}
		nextSettings.RequestChangesMinSeverity = promptSelect(reader, "Minimum severity threshold to block PRs (critical/high/medium/low)", []string{"critical", "high", "medium", "low"}, defSev)

		// 2. Patterns settings
		if len(nextSettings.IgnoredFilePatterns) == 0 {
			nextSettings.IgnoredFilePatterns = RecommendedIgnoredFiles
		}
		if len(nextSettings.BaseBranchPatterns) == 0 {
			nextSettings.BaseBranchPatterns = RecommendedBaseBranches
		}
		if len(nextSettings.IgnoredTitlePatterns) == 0 {
			nextSettings.IgnoredTitlePatterns = RecommendedIgnoredTitles
		}

		utils.Info("\nConfigured Patterns:")
		utils.Info("  Ignore Files: %v", nextSettings.IgnoredFilePatterns)
		utils.Info("  Base Branches: %v", nextSettings.BaseBranchPatterns)
		utils.Info("  Ignore Titles: %v\n", nextSettings.IgnoredTitlePatterns)

		confirmApply := promptYesNo(reader, "Apply these settings to repository?", true)
		if !confirmApply {
			utils.Info("Setup cancelled. No changes were made.")
			return nil
		}
	}

	// Update settings via API
	updated, err := client.UpdateRepositorySettings(ctx, repository, nextSettings)
	if err != nil {
		return fmt.Errorf("failed updating repository settings: %w", err)
	}

	if opts.JSON {
		res := RepositorySettingsSummary{
			RepositoryName: repository,
			Current:        current,
			Next:           updated,
			Applied:        true,
		}
		data, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	utils.Success("✔ Successfully applied review configuration to %s", repository)
	return nil
}

func promptYesNo(r *bufio.Reader, prompt string, defaultVal bool) bool {
	hint := "[Y/n]"
	if !defaultVal {
		hint = "[y/N]"
	}
	fmt.Printf("%s %s: ", prompt, hint)
	line, err := r.ReadString('\n')
	if err != nil {
		return defaultVal
	}
	line = strings.TrimSpace(strings.ToLower(line))
	if line == "" {
		return defaultVal
	}
	return line == "y" || line == "yes"
}

func promptSelect(r *bufio.Reader, prompt string, options []string, defaultVal string) string {
	fmt.Printf("%s [default: %s]: ", prompt, defaultVal)
	line, err := r.ReadString('\n')
	if err != nil {
		return defaultVal
	}
	line = strings.TrimSpace(strings.ToLower(line))
	if line == "" {
		return defaultVal
	}
	for _, opt := range options {
		if strings.EqualFold(opt, line) {
			return opt
		}
	}
	return defaultVal
}
