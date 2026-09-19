// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package repoconfig

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestRegisterRepositoryConfigCommand(t *testing.T) {
	rootCmd := &cobra.Command{Use: "scandrix"}
	configCmd := &cobra.Command{Use: "config"}
	rootCmd.AddCommand(configCmd)

	RegisterRepositoryConfigCommand(configCmd)

	var remoteCmd *cobra.Command
	for _, sub := range configCmd.Commands() {
		if sub.Name() == "remote" {
			remoteCmd = sub
			break
		}
	}

	if remoteCmd == nil {
		t.Fatalf("expected 'remote' command to be registered under 'config'")
	}

	expectedSubcommands := map[string]bool{
		"add":            false,
		"list":           false,
		"show":           false,
		"set":            false,
		"pattern-add":    false,
		"pattern-remove": false,
		"open":           false,
	}

	for _, sub := range remoteCmd.Commands() {
		if _, ok := expectedSubcommands[sub.Name()]; ok {
			expectedSubcommands[sub.Name()] = true
		}
	}

	for name, found := range expectedSubcommands {
		if !found {
			t.Errorf("expected subcommand %q under 'config remote', but was not found", name)
		}
	}
}

func TestConfigRepoOptionsDefaults(t *testing.T) {
	opts := ConfigRepoAddOptions{Prompt: true}
	if !opts.Prompt {
		t.Errorf("expected prompt to default to true")
	}
}
