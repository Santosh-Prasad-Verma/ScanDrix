// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package cmd_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/cli/cmd"
	"github.com/scandrix/backend/internal/cli/utils"
)

func TestRootCommandHierarchy(t *testing.T) {
	root := cmd.RootCmd

	expectedSubcommands := []string{
		"review",
		"auth",
		"config",
		"hook",
		"decisions",
		"pr",
		"rules",
		"skills",
		"status",
		"subscribe",
		"update",
		"schema",
		"chat",
		"tui",
		"scan",
		"diff",
		"fix",
		"export",
		"pentest",
		"mcp",
		"server",
		"trace",
		"history",
		"dry-run",
		"ci",
		"login",
		"logout",
		"whoami",
	}

	for _, expected := range expectedSubcommands {
		sub, _, err := root.Find([]string{expected})
		if err != nil || sub == nil || (sub.Name() != expected && !sub.HasAlias(expected)) {
			t.Errorf("expected subcommand or alias '%s' to exist on RootCmd", expected)
		}
	}

	// Persistent flags
	expectedFlags := []string{"format", "output", "verbose", "quiet", "agent"}
	for _, f := range expectedFlags {
		if root.PersistentFlags().Lookup(f) == nil {
			t.Errorf("expected persistent flag '--%s' on RootCmd", f)
		}
	}
}

func TestRulesSubcommands(t *testing.T) {
	root := cmd.RootCmd
	rulesCmd, _, err := root.Find([]string{"rules"})
	if err != nil || rulesCmd == nil {
		t.Fatalf("rules command not found")
	}

	subnames := []string{"create", "update", "view", "init", "validate", "generate"}
	for _, expected := range subnames {
		sub, _, err := rulesCmd.Find([]string{expected})
		if err != nil || sub == nil || sub.Name() != expected {
			t.Errorf("expected rules subcommand '%s' to exist", expected)
		}
	}
}

func TestConfigSubcommands(t *testing.T) {
	root := cmd.RootCmd
	configCmd, _, err := root.Find([]string{"config"})
	if err != nil || configCmd == nil {
		t.Fatalf("config command not found")
	}

	subnames := []string{"remote", "repo", "centralized", "show", "set"}
	for _, expected := range subnames {
		sub, _, err := configCmd.Find([]string{expected})
		if err != nil || sub == nil || sub.Name() != expected {
			t.Errorf("expected config subcommand '%s' to exist", expected)
		}
	}

	remoteSubnames := []string{
		"add", "list", "show", "open", "pattern", "set",
		"add-ignore-file", "remove-ignore-file",
		"add-base-branch", "remove-base-branch",
		"add-ignore-title", "remove-ignore-title",
	}
	for _, expected := range remoteSubnames {
		sub, _, err := root.Find([]string{"config", "remote", expected})
		if err != nil || sub == nil || !strings.HasPrefix(sub.Use, expected) {
			t.Errorf("expected config remote subcommand '%s' to exist", expected)
		}
	}

	for _, expected := range remoteSubnames {
		sub, _, err := root.Find([]string{"config", "repo", expected})
		if err != nil || sub == nil || !strings.HasPrefix(sub.Use, expected) {
			t.Errorf("expected config repo subcommand '%s' to exist", expected)
		}
	}

	for _, expected := range []string{"add", "remove"} {
		sub, _, err := root.Find([]string{"config", "remote", "pattern", expected})
		if err != nil || sub == nil || !strings.HasPrefix(sub.Use, expected) {
			t.Errorf("expected config remote pattern subcommand '%s' to exist", expected)
		}
	}
}

func TestSchemaCommandExecution(t *testing.T) {
	var buf bytes.Buffer
	root := cmd.RootCmd
	root.SetOut(&buf)
	root.SetArgs([]string{"schema", "--command", "review"})

	err := root.Execute()
	if err != nil {
		t.Fatalf("schema command execution failed: %v", err)
	}

	// Verify schema reflection
	schema := utils.BuildCommandSchema(root)
	if schema.Name != "scandrix" {
		t.Errorf("expected schema name 'scandrix', got '%s'", schema.Name)
	}

	data, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("failed to marshal schema: %v", err)
	}
	if len(data) == 0 {
		t.Fatalf("expected non-empty schema JSON")
	}
}

func TestTraceSubcommands(t *testing.T) {
	root := cmd.RootCmd
	subnames := []string{"enable", "disable", "status", "pin", "forget", "distill", "commit-trailer", "hooks", "ui"}
	for _, expected := range subnames {
		sub, _, err := root.Find([]string{"trace", expected})
		if err != nil || sub == nil || !strings.HasPrefix(sub.Use, expected) {
			t.Errorf("expected trace subcommand '%s' to exist", expected)
		}
	}
}

func TestSkillsSubcommands(t *testing.T) {
	root := cmd.RootCmd
	subnames := []string{"list", "sync", "install", "uninstall", "check"}
	for _, expected := range subnames {
		sub, _, err := root.Find([]string{"skills", expected})
		if err != nil || sub == nil || !strings.HasPrefix(sub.Use, expected) {
			t.Errorf("expected skills subcommand '%s' to exist", expected)
		}
	}
}

func TestVersionCommandExecution(t *testing.T) {
	var buf bytes.Buffer
	root := cmd.RootCmd
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"version"})

	err := root.Execute()
	if err != nil {
		t.Fatalf("version command execution failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "ScanDrix CLI") && !strings.Contains(out, "scandrix") {
		t.Errorf("expected ScanDrix in version output, got: %s", out)
	}
}


