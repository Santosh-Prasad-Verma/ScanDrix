// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package centralizedconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/scandrix/backend/internal/cli/services/api"
	"github.com/scandrix/backend/internal/cli/utils"
)

// ConfigCentralizedStatusOptions holds options for querying centralized config status.
type ConfigCentralizedStatusOptions struct {
	JSON bool
}

// ConfigCentralizedInitOptions holds options for initializing centralized config.
type ConfigCentralizedInitOptions struct {
	SyncOption string
	JSON       bool
}

// ConfigCentralizedActionOptions holds generic options for centralized config actions.
type ConfigCentralizedActionOptions struct {
	JSON bool
}

// ConfigCentralizedDownloadOptions holds options for downloading centralized config zip.
type ConfigCentralizedDownloadOptions struct {
	Out  string
	JSON bool
}

func resolveSyncOption(value string) (string, error) {
	norm := strings.ToLower(strings.TrimSpace(value))
	if norm == "" {
		return "pr", nil
	}
	if norm == "pr" || norm == "manual" {
		return norm, nil
	}
	return "", fmt.Errorf("invalid value for `--sync-option`: '%s'. Use one of: pr, manual", value)
}

// ConfigCentralizedStatusAction checks and renders organization centralized config state.
func ConfigCentralizedStatusAction(ctx context.Context, client *api.Client, opts ConfigCentralizedStatusOptions) error {
	if client == nil {
		client = api.NewClient("", "", "")
	}

	status, err := client.GetCentralizedStatus(ctx)
	if err != nil {
		return fmt.Errorf("failed fetching centralized config status: %w", err)
	}

	if opts.JSON {
		data, _ := json.MarshalIndent(status, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	if !status.Enabled || (status.Repository == nil && status.SelectedRepository == nil) {
		utils.Warn("Centralized config is disabled.")
		return nil
	}

	utils.Success("✔ Centralized config is enabled.")
	if status.Repository != nil {
		fmt.Printf("   Repository: %s (%s)\n", status.Repository.Name, status.Repository.ID)
	} else if status.SelectedRepository != nil {
		fmt.Printf("   Repository: %s (%s)\n", status.SelectedRepository.Namespace, status.SelectedRepository.ID)
	}
	if status.SyncMode != "" {
		fmt.Printf("   Sync mode: %s\n", status.SyncMode)
	}
	if status.LastSyncedAt != "" {
		fmt.Printf("   Last synced: %s\n", status.LastSyncedAt)
	}
	return nil
}

// ConfigCentralizedInitAction enables centralized config for the organization.
func ConfigCentralizedInitAction(ctx context.Context, client *api.Client, repository string, opts ConfigCentralizedInitOptions) error {
	if client == nil {
		client = api.NewClient("", "", "")
	}
	if repository == "" {
		repository = "."
	}

	syncOpt, err := resolveSyncOption(opts.SyncOption)
	if err != nil {
		return err
	}

	resp, err := client.InitCentralizedConfig(ctx, repository, syncOpt)
	if err != nil {
		return fmt.Errorf("failed initializing centralized config: %w", err)
	}

	if opts.JSON {
		data, _ := json.MarshalIndent(resp, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	if resp.Message != "" {
		utils.Success("✔ %s", resp.Message)
	} else {
		utils.Success("✔ Centralized configuration successfully initialized.")
	}

	if resp.Repository != nil {
		fmt.Printf("   Repository: %s\n", resp.Repository.Namespace)
	}
	if prURL := resp.GetPRURL(); prURL != "" {
		fmt.Printf("   Pull Request: %s\n", prURL)
	}
	return nil
}

// ConfigCentralizedSyncAction propagates centralized settings across tracked repositories.
func ConfigCentralizedSyncAction(ctx context.Context, client *api.Client, opts ConfigCentralizedActionOptions) error {
	if client == nil {
		client = api.NewClient("", "", "")
	}

	resp, err := client.SyncCentralizedConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed syncing centralized config: %w", err)
	}

	if opts.JSON {
		data, _ := json.MarshalIndent(resp, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	if resp.Message != "" {
		utils.Success("✔ %s", resp.Message)
	} else {
		utils.Success("✔ Centralized configuration synced across all repositories.")
	}
	if prURL := resp.GetPRURL(); prURL != "" {
		fmt.Printf("   Pull Request: %s\n", prURL)
	}
	return nil
}

// ConfigCentralizedDisableAction turns off centralized configuration.
func ConfigCentralizedDisableAction(ctx context.Context, client *api.Client, opts ConfigCentralizedActionOptions) error {
	if client == nil {
		client = api.NewClient("", "", "")
	}

	resp, err := client.DisableCentralizedConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed disabling centralized config: %w", err)
	}

	if opts.JSON {
		data, _ := json.MarshalIndent(resp, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	if resp.Message != "" {
		utils.Success("✔ %s", resp.Message)
	} else {
		utils.Success("✔ Centralized configuration has been disabled.")
	}
	return nil
}

// ConfigCentralizedDownloadAction downloads the centralized configuration archive.
func ConfigCentralizedDownloadAction(ctx context.Context, client *api.Client, opts ConfigCentralizedDownloadOptions) error {
	if client == nil {
		client = api.NewClient("", "", "")
	}

	if strings.TrimSpace(opts.Out) == "" {
		return fmt.Errorf("`--out` flag is required specifying destination archive path")
	}

	if err := client.DownloadCentralizedZip(ctx, opts.Out); err != nil {
		return fmt.Errorf("failed downloading centralized config archive: %w", err)
	}

	fi, err := os.Stat(opts.Out)
	size := int64(0)
	if err == nil {
		size = fi.Size()
	}

	if opts.JSON {
		fmt.Printf(`{"out": "%s", "bytes": %d}`+"\n", opts.Out, size)
		return nil
	}

	utils.Success("✔ Centralized configuration archive saved to %s (%d bytes)", opts.Out, size)
	return nil
}
