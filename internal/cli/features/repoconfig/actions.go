// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package repoconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/scandrix/backend/internal/cli/formatters"
	"github.com/scandrix/backend/internal/cli/services/api"
	"github.com/scandrix/backend/internal/cli/utils"
)

// ConfigRepoAddOptions holds options for adding a repository.
type ConfigRepoAddOptions struct {
	Prompt bool
	JSON   bool
}

// ConfigRepoListOptions holds options for listing repositories.
type ConfigRepoListOptions struct {
	JSON bool
}

// ConfigRepoShowOptions holds options for inspecting repository settings.
type ConfigRepoShowOptions struct {
	JSON bool
}

// ConfigRepoSetupOptions holds options for configuring repository review parameters.
type ConfigRepoSetupOptions struct {
	Yes  bool
	JSON bool
}

// ConfigRepoMutationOptions holds options for mutating repository settings.
type ConfigRepoMutationOptions struct {
	JSON bool
}

// ConfigRepoOpenOptions holds options for opening the repository dashboard.
type ConfigRepoOpenOptions struct {
	Section string
	JSON    bool
}

// ConfigRemoteAddAction registers a repository with ScanDrix.
func ConfigRemoteAddAction(ctx context.Context, client *api.Client, repository string, opts ConfigRepoAddOptions) error {
	if client == nil {
		client = api.NewClient("", "", "")
	}
	if repository == "" || repository == "." {
		repository = "."
	}

	res, err := client.TrackRepository(ctx, repository, "git", "main")
	if err != nil {
		return fmt.Errorf("failed adding repository to ScanDrix: %w", err)
	}

	if opts.JSON {
		data, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	utils.Success("✔ Repository '%s' successfully connected to ScanDrix.", res.Namespace)
	if res.DefaultBranch != "" {
		fmt.Printf("   Default branch: %s\n", res.DefaultBranch)
	}
	if res.Provider != "" {
		fmt.Printf("   Provider: %s\n", res.Provider)
	}
	return nil
}

// ConfigRepoListAction retrieves and renders all tracked repositories.
func ConfigRepoListAction(ctx context.Context, client *api.Client, opts ConfigRepoListOptions) error {
	if client == nil {
		client = api.NewClient("", "", "")
	}

	repos, err := client.ListRepositories(ctx)
	if err != nil {
		return fmt.Errorf("failed listing repositories: %w", err)
	}

	if opts.JSON {
		data, _ := json.MarshalIndent(repos, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	if len(repos) == 0 {
		utils.Info("No repositories currently configured. Use 'scandrix config remote add .' to add one.")
		return nil
	}

	formatters.PrintRepoList(os.Stdout, repos)
	return nil
}

// ConfigRepoShowAction displays review settings for a repository.
func ConfigRepoShowAction(ctx context.Context, client *api.Client, repository string, opts ConfigRepoShowOptions) error {
	if client == nil {
		client = api.NewClient("", "", "")
	}
	if repository == "" {
		repository = "."
	}

	settings, err := client.GetRepositorySettings(ctx, repository)
	if err != nil {
		return fmt.Errorf("failed retrieving repository settings: %w", err)
	}

	if opts.JSON {
		data, _ := json.MarshalIndent(settings, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	formatters.PrintRepoSettings(os.Stdout, settings)
	return nil
}

// ConfigRepoSetAction updates a specific setting key-value pair.
func ConfigRepoSetAction(ctx context.Context, client *api.Client, repository, key, value string, opts ConfigRepoMutationOptions) error {
	if client == nil {
		client = api.NewClient("", "", "")
	}
	if repository == "" {
		repository = "."
	}

	current, err := client.GetRepositorySettings(ctx, repository)
	if err != nil {
		return fmt.Errorf("failed fetching current settings: %w", err)
	}

	normKey := strings.ToLower(strings.TrimSpace(key))
	switch normKey {
	case "review_enabled", "review-enabled", "review":
		current.ReviewEnabled = strings.EqualFold(value, "true") || value == "1"
	case "auto_approve_enabled", "auto-approve", "auto_approve":
		current.AutoApproveEnabled = strings.EqualFold(value, "true") || value == "1"
	case "request_changes_min_severity", "min_severity", "min-severity":
		current.RequestChangesMinSeverity = strings.ToLower(value)
	case "default_branch", "default-branch", "branch":
		current.DefaultBranch = value
	default:
		if current.CustomSettings == nil {
			current.CustomSettings = make(map[string]any)
		}
		current.CustomSettings[key] = value
	}

	updated, err := client.UpdateRepositorySettings(ctx, repository, *current)
	if err != nil {
		return fmt.Errorf("failed updating repository settings: %w", err)
	}

	if opts.JSON {
		data, _ := json.MarshalIndent(updated, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	utils.Success("✔ Repository settings updated for '%s'", repository)
	if updated.PRURL != "" {
		utils.Info("Centralized change queued in pull request: %s", updated.PRURL)
	}
	return nil
}

// ConfigRepoPatternAddAction appends a pattern to an array field.
func ConfigRepoPatternAddAction(ctx context.Context, client *api.Client, repository, field, pattern string, opts ConfigRepoMutationOptions) error {
	if client == nil {
		client = api.NewClient("", "", "")
	}
	if repository == "" {
		repository = "."
	}

	res, err := client.AddRepositoryPattern(ctx, repository, field, pattern)
	if err != nil {
		return fmt.Errorf("failed adding pattern to %s: %w", field, err)
	}

	if opts.JSON {
		data, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	utils.Success("✔ Added pattern '%s' to '%s' for repository '%s'", pattern, field, repository)
	return nil
}

// ConfigRepoPatternRemoveAction removes a pattern from an array field.
func ConfigRepoPatternRemoveAction(ctx context.Context, client *api.Client, repository, field, pattern string, opts ConfigRepoMutationOptions) error {
	if client == nil {
		client = api.NewClient("", "", "")
	}
	if repository == "" {
		repository = "."
	}

	res, err := client.RemoveRepositoryPattern(ctx, repository, field, pattern)
	if err != nil {
		return fmt.Errorf("failed removing pattern from %s: %w", field, err)
	}

	if opts.JSON {
		data, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	utils.Success("✔ Removed pattern '%s' from '%s' for repository '%s'", pattern, field, repository)
	return nil
}

// ConfigRepoOpenAction opens the ScanDrix web dashboard for this repository.
func ConfigRepoOpenAction(ctx context.Context, client *api.Client, repository string, opts ConfigRepoOpenOptions) error {
	if client == nil {
		client = api.NewClient("", "", "")
	}
	if repository == "" {
		repository = "."
	}

	baseURL := client.ServerURL()
	if baseURL == "" {
		baseURL = "https://app.scandrix.dev"
	}

	targetURL := fmt.Sprintf("%s/repositories/%s", strings.TrimSuffix(baseURL, "/"), repository)
	if opts.Section != "" {
		targetURL += fmt.Sprintf("?tab=%s", opts.Section)
	}

	if opts.JSON {
		fmt.Printf(`{"url": "%s"}`+"\n", targetURL)
		return nil
	}

	utils.Info("Opening repository dashboard: %s", targetURL)
	utils.OpenBrowser(targetURL)
	return nil
}
