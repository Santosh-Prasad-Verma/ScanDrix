// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/cli/cmd"
	"github.com/scandrix/backend/internal/cli/configcli"
	"github.com/scandrix/backend/internal/cli/formatters"
	"github.com/scandrix/backend/internal/cli/services/api"
)

// CONFIGURATION LOAD & FORMATTER TESTS

func TestConfig_LoadDefaultAndFile(t *testing.T) {
	t.Setenv("SCANDRIX_SERVER_URL", "")
	t.Setenv("SCANDRIX_API_URL", "")
	t.Setenv("APP_BASE_URL", "")
	t.Setenv("API_BASE_URL", "")

	tmpDir := t.TempDir()
	cfgDir := filepath.Join(tmpDir, ".scandrix")
	_ = os.MkdirAll(cfgDir, 0700)

	cfgPath := filepath.Join(cfgDir, "config.json")
	cfgData := `{
		"server_url": "https://api.scandrix.custom",
		"api_key": "scandrix_customkey_123",
		"default_format": "json",
		"fail_on_severity": "HIGH",
		"auto_review_staged": true,
		"timeout_minutes": 45
	}`
	if err := os.WriteFile(cfgPath, []byte(cfgData), 0600); err != nil {
		t.Fatalf("failed to write mock config: %v", err)
	}

	cfg := configcli.Load(tmpDir)
	if cfg == nil {
		t.Fatalf("expected loaded config, got nil")
	}
	if cfg.ServerURL != "https://api.scandrix.custom" {
		t.Errorf("expected custom server URL, got %s", cfg.ServerURL)
	}
	if cfg.APIKey != "scandrix_customkey_123" {
		t.Errorf("expected custom API key, got %s", cfg.APIKey)
	}
	if cfg.DefaultFormat != "json" {
		t.Errorf("expected json default format, got %s", cfg.DefaultFormat)
	}
	if cfg.FailOnSeverity != "HIGH" {
		t.Errorf("expected HIGH fail-on-severity, got %s", cfg.FailOnSeverity)
	}
	if !cfg.AutoReviewStaged {
		t.Errorf("expected AutoReviewStaged to be true")
	}
	if cfg.TimeoutMinutes != 45 {
		t.Errorf("expected 45 timeout minutes, got %d", cfg.TimeoutMinutes)
	}
}

func TestFormatters_RepoSettings(t *testing.T) {
	settings := &api.RepositorySettings{
		Namespace:     "acme/core-engine",
		DefaultBranch: "main",
		IgnoredPaths:  []string{"vendor/**", "*.gen.go"},
		FocusAreas:    []string{"security", "performance"},
		Reviewers:     []string{"alice", "bob"},
	}

	var buf bytes.Buffer
	formatters.PrintRepoSettings(&buf, settings)

	out := buf.String()
	if !strings.Contains(out, "Repository: acme/core-engine") {
		t.Errorf("expected repository in output")
	}
	if !strings.Contains(out, "Default Branch: main") {
		t.Errorf("expected default branch in output")
	}
	if !strings.Contains(out, "vendor/**") {
		t.Errorf("expected ignored path in output")
	}
	if !strings.Contains(out, "alice, bob") {
		t.Errorf("expected reviewers in output")
	}
}

func TestFormatters_RepoList(t *testing.T) {
	repos := []api.TrackedRepository{
		{Namespace: "org/repo-a", Provider: "github", DefaultBranch: "main"},
		{Namespace: "org/repo-b", Provider: "gitlab", DefaultBranch: "master"},
	}

	var buf bytes.Buffer
	formatters.PrintRepoList(&buf, repos)

	out := buf.String()
	if !strings.Contains(out, "Tracked Repositories (2)") {
		t.Errorf("expected header with count")
	}
	if !strings.Contains(out, "org/repo-a") || !strings.Contains(out, "org/repo-b") {
		t.Errorf("expected repos listed in output")
	}
}

// CONFIG COBRA COMMAND REGISTRATION & FLAG VERIFICATION

func TestConfigCommand_SubcommandsAndFlags(t *testing.T) {
	var found bool
	for _, c := range cmd.RootCmd.Commands() {
		if c.Name() == "config" {
			found = true

			// Verify subcommands: show, remote, centralized
			expectedSubs := []string{"show", "remote", "centralized"}
			subNames := make(map[string]bool)
			for _, sc := range c.Commands() {
				subNames[sc.Name()] = true
			}

			for _, exp := range expectedSubs {
				if !subNames[exp] {
					t.Errorf("expected subcommand '%s' under config command", exp)
				}
			}

			// Check remote subcommands: add, list, show, open
			for _, sc := range c.Commands() {
				if sc.Name() == "remote" {
					remoteSubs := make(map[string]bool)
					for _, rsc := range sc.Commands() {
						remoteSubs[rsc.Name()] = true
					}
					for _, rExp := range []string{"add", "list", "show", "open"} {
						if !remoteSubs[rExp] {
							t.Errorf("expected remote subcommand '%s'", rExp)
						}
					}
				}

				if sc.Name() == "centralized" {
					centSubs := make(map[string]bool)
					for _, csc := range sc.Commands() {
						centSubs[csc.Name()] = true
					}
					for _, cExp := range []string{"status", "init", "sync", "disable", "download"} {
						if !centSubs[cExp] {
							t.Errorf("expected centralized subcommand '%s'", cExp)
						}
					}
				}
			}
			break
		}
	}

	if !found {
		t.Fatalf("command 'config' not found in RootCmd")
	}
}
