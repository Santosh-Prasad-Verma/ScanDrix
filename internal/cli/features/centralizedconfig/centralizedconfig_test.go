// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package centralizedconfig

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestRegisterCentralizedConfigCommand(t *testing.T) {
	rootCmd := &cobra.Command{Use: "scandrix"}
	configCmd := &cobra.Command{Use: "config"}
	rootCmd.AddCommand(configCmd)

	RegisterCentralizedConfigCommand(configCmd)

	var centCmd *cobra.Command
	for _, sub := range configCmd.Commands() {
		if sub.Name() == "centralized" {
			centCmd = sub
			break
		}
	}

	if centCmd == nil {
		t.Fatalf("expected 'centralized' subcommand to be attached under 'config'")
	}

	expectedSubcommands := map[string]bool{
		"status":   false,
		"init":     false,
		"sync":     false,
		"disable":  false,
		"download": false,
	}

	for _, sub := range centCmd.Commands() {
		if _, ok := expectedSubcommands[sub.Name()]; ok {
			expectedSubcommands[sub.Name()] = true
		}
	}

	for name, found := range expectedSubcommands {
		if !found {
			t.Errorf("expected subcommand %q under 'config centralized', but was not found", name)
		}
	}
}

func TestResolveSyncOption(t *testing.T) {
	cases := []struct {
		input    string
		expected string
		err      bool
	}{
		{"", "pr", false},
		{"pr", "pr", false},
		{"manual", "manual", false},
		{"PR", "pr", false},
		{"MANUAL", "manual", false},
		{"invalid_mode", "", true},
	}

	for _, tc := range cases {
		out, err := resolveSyncOption(tc.input)
		if tc.err && err == nil {
			t.Errorf("input %q: expected error, got nil", tc.input)
		}
		if !tc.err && err != nil {
			t.Errorf("input %q: unexpected error %v", tc.input, err)
		}
		if !tc.err && out != tc.expected {
			t.Errorf("input %q: expected %q, got %q", tc.input, tc.expected, out)
		}
	}
}
