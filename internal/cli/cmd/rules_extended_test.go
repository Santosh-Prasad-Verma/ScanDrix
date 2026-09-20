// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/scandrix/backend/internal/cli/services/api"
	"github.com/spf13/cobra"
)

func TestRulesCmd_PrintHelpers(t *testing.T) {
	rule := &api.RuleModel{
		UUID:     "rule-123",
		RepoID:   "repo-456",
		Title:    "No hardcoded credentials",
		Rule:     `(?i)(password|secret)\s*=\s*['"][^'"]+['"]`,
		Severity: "critical",
		Scope:    "file",
		Path:     "**/*.go",
	}

	// Should not panic on nil or populated rule
	printRule(nil, "")
	printRule(rule, "")

	list := []api.RuleModel{*rule}
	printRuleList(nil, "")
	printRuleList(list, "")
}

func TestRulesCmd_InitAndValidate(t *testing.T) {
	tempDir := t.TempDir()

	// Switch working directory temporarily to tempDir
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed getting current wd: %v", err)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("failed changing wd: %v", err)
	}

	root := &cobra.Command{Use: "scandrix"}
	root.AddCommand(rulesCmd)

	// 1. Run rules init
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"rules", "init"})

	if err := root.Execute(); err != nil {
		t.Fatalf("rules init failed: %v", err)
	}

	// Verify file was created in .drixy/rules.yaml or similar
	createdFile := filepath.Join(tempDir, ".drixy", "rules.yaml")
	if _, err := os.Stat(createdFile); os.IsNotExist(err) {
		// check if it's .scandrix/rules.yaml
		altFile := filepath.Join(tempDir, ".scandrix", "rules.yaml")
		if _, errAlt := os.Stat(altFile); os.IsNotExist(errAlt) {
			t.Fatalf("expected rules file to be created, neither %s nor %s found", createdFile, altFile)
		}
	}

	// 2. Run rules validate
	buf.Reset()
	root.SetArgs([]string{"rules", "validate"})
	if err := root.Execute(); err != nil {
		t.Fatalf("rules validate failed: %v", err)
	}
}
