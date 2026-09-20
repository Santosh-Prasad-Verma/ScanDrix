// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package review

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestReviewConfigBuilder_Defaults(t *testing.T) {
	tmpDir := t.TempDir()
	builder := NewReviewConfigBuilder(tmpDir)

	cfg, err := builder.Build(context.Background(), "", nil)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if cfg.MinSeverity != "low" {
		t.Errorf("Expected default minSeverity 'low', got %q", cfg.MinSeverity)
	}
	if len(cfg.BaseBranches) != 3 {
		t.Errorf("Expected 3 base branches, got %d", len(cfg.BaseBranches))
	}
	if !cfg.ShouldIgnoreFile("node_modules/express/index.js") {
		t.Error("Expected node_modules to be ignored")
	}
	if !cfg.ShouldIgnoreFile("package-lock.json") {
		t.Error("Expected package-lock.json to be ignored")
	}
	if !cfg.ShouldIgnoreFile("assets/logo.png") {
		t.Error("Expected .png to be ignored")
	}
	if cfg.ShouldIgnoreFile("src/index.ts") {
		t.Error("Expected src/index.ts not to be ignored")
	}
}

func TestReviewConfigBuilder_LocalConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	yamlContent := `
review:
  enabled: true
  autoApprove: true
  minSeverity: high
  ignoreFiles:
    - "legacy/**"
    - "generated/*.go"
  ignoreTitles:
    - "[WIP]"
    - "[DRAFT]"
  baseBranches:
    - "release"
  maxFiles: 50
  maxDiffBytes: 1048576
  customRuleSets:
    - "security-hardened"
`
	err := os.WriteFile(filepath.Join(tmpDir, ".scandrix.yml"), []byte(yamlContent), 0644)
	if err != nil {
		t.Fatalf("Failed writing .scandrix.yml: %v", err)
	}

	builder := NewReviewConfigBuilder(tmpDir)
	cfg, err := builder.Build(context.Background(), "", []string{"custom-extra/**"})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if cfg.MinSeverity != "high" {
		t.Errorf("Expected minSeverity 'high', got %q", cfg.MinSeverity)
	}
	if !cfg.AutoApprove {
		t.Error("Expected autoApprove true")
	}
	if cfg.MaxFiles != 50 {
		t.Errorf("Expected maxFiles 50, got %d", cfg.MaxFiles)
	}
	if cfg.MaxDiffBytes != 1048576 {
		t.Errorf("Expected maxDiffBytes 1048576, got %d", cfg.MaxDiffBytes)
	}
	if !cfg.ShouldIgnoreFile("legacy/old_code.py") {
		t.Error("Expected legacy/old_code.py to be ignored")
	}
	if !cfg.ShouldIgnoreFile("custom-extra/something.txt") {
		t.Error("Expected custom-extra to be ignored")
	}
}

func TestReviewConfigBuilder_InvalidSeverity(t *testing.T) {
	tmpDir := t.TempDir()
	builder := NewReviewConfigBuilder(tmpDir)

	_, err := builder.Build(context.Background(), "invalid-severity", nil)
	if err == nil {
		t.Fatal("Expected error for invalid severity, got nil")
	}
}
