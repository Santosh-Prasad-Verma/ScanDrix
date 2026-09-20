// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package context_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	contextsvc "github.com/scandrix/backend/internal/cli/services/context"
)

func TestReadProjectContext(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	_ = cmd.Run()

	// Create .cursorrules
	if err := os.WriteFile(filepath.Join(tmpDir, ".cursorrules"), []byte("use typescript strict\n"), 0644); err != nil {
		t.Fatalf("failed writing .cursorrules: %v", err)
	}

	// Create .scandrix.md
	if err := os.WriteFile(filepath.Join(tmpDir, ".scandrix.md"), []byte("# ScanDrix Rules\nRule 1: No plain secrets\n"), 0644); err != nil {
		t.Fatalf("failed writing .scandrix.md: %v", err)
	}

	// Create custom context file
	customFile := filepath.Join(tmpDir, "custom-spec.md")
	if err := os.WriteFile(customFile, []byte("Architecture spec v2\n"), 0644); err != nil {
		t.Fatalf("failed writing custom-spec: %v", err)
	}

	// Change dir to tmpDir
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	_ = os.Chdir(tmpDir)

	svc := contextsvc.DefaultService()
	pCtx, err := svc.ReadProjectContext(context.Background(), customFile)
	if err != nil {
		t.Fatalf("ReadProjectContext failed: %v", err)
	}

	if pCtx.CursorRules != "use typescript strict" {
		t.Errorf("expected cursor rules 'use typescript strict', got '%s'", pCtx.CursorRules)
	}
	if pCtx.ScanDrixRules != "# ScanDrix Rules\nRule 1: No plain secrets" {
		t.Errorf("unexpected scandrix rules: '%s'", pCtx.ScanDrixRules)
	}
	if pCtx.CustomContext != "Architecture spec v2" {
		t.Errorf("unexpected custom context: '%s'", pCtx.CustomContext)
	}
}
