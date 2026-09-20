// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package cli_test

import (
	"testing"

	"github.com/scandrix/backend/internal/cli/cmd"
	"github.com/scandrix/backend/internal/cli/utils"
)

// SCANDRIX EXTENDED CAPABILITIES REGISTRATION TESTS

func TestSuperpowers_CommandsRegisteredInRoot(t *testing.T) {
	expectedCommands := []string{
		"diff",
		"scan",
		"fix",
		"export",
		"pentest",
		"mcp",
		"server",
		"chat",
		"tui",
		"trace",
		"skills",
		"status",
		"schema",
		"subscribe",
		"update",
		"dry-run",
		"decisions",
		"auth",
		"review",
		"rules",
		"config",
	}

	commandMap := make(map[string]bool)
	for _, c := range cmd.RootCmd.Commands() {
		commandMap[c.Name()] = true
	}

	for _, name := range expectedCommands {
		if !commandMap[name] {
			t.Errorf("expected command '%s' to be registered in RootCmd", name)
		}
	}
}

// SCHEMA GENERATOR INTROSPECTION TESTS

func TestSuperpowers_BuildCommandSchema(t *testing.T) {
	schema := utils.BuildCommandSchema(cmd.RootCmd)

	if schema.Name != "scandrix" {
		t.Errorf("expected root schema name 'scandrix', got %s", schema.Name)
	}

	if len(schema.Subcommands) == 0 {
		t.Fatalf("expected subcommands in root schema, got 0")
	}

	// Verify review command schema is introspected
	var reviewSchema *utils.CommandSchema
	for i := range schema.Subcommands {
		if schema.Subcommands[i].Name == "review" {
			reviewSchema = &schema.Subcommands[i]
			break
		}
	}

	if reviewSchema == nil {
		t.Fatalf("expected review command schema inside root schema")
	}

	// Check flags on review
	flagFound := false
	for _, opt := range reviewSchema.Options {
		if opt.Flags != "" && (opt.Flags == "--staged" || opt.Flags == "--branch <string>") {
			flagFound = true
			break
		}
	}
	if !flagFound && len(reviewSchema.Options) == 0 {
		t.Errorf("expected flags on review command schema")
	}
}

// MCP AND EXTENDED COMMAND FLAG TESTS

func TestSuperpowers_MCPCommandFlags(t *testing.T) {
	for _, c := range cmd.RootCmd.Commands() {
		if c.Name() == "mcp" {
			if c.Flags().Lookup("role") == nil {
				t.Errorf("expected --role flag on mcp command")
			}
			if c.Flags().Lookup("token") == nil {
				t.Errorf("expected --token flag on mcp command")
			}
			return
		}
	}
	t.Fatalf("command 'mcp' not found in RootCmd")
}
