// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Tools Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

func TestNativeTools_Execution(t *testing.T) {
	tempDir := t.TempDir()

	// Create dummy test files
	file1 := filepath.Join(tempDir, "sample.go")
	content1 := `package sample

// ImportantFunction does calculation
func ImportantFunction(a int) int {
	return a * 42
}
`
	if err := os.WriteFile(file1, []byte(content1), 0644); err != nil {
		t.Fatalf("failed to write sample file: %v", err)
	}

	subDir := filepath.Join(tempDir, "pkg")
	_ = os.Mkdir(subDir, 0755)
	file2 := filepath.Join(subDir, "readme.txt")
	_ = os.WriteFile(file2, []byte("Documentation goes here"), 0644)

	reg := BuildNativeToolsRegistry(tempDir)
	toolCtx := contracts.ToolContext{
		RunID:   "test-run",
		Context: context.Background(),
	}

	// 1. Test Grep Tool
	grepTool, ok := reg.Get("grep")
	if !ok {
		t.Fatalf("grep tool not registered")
	}
	grepRes, err := grepTool.Execute(toolCtx, map[string]any{"pattern": "ImportantFunction"})
	if err != nil || grepRes.IsError {
		t.Fatalf("grep failed: %v, res=%v", err, grepRes)
	}
	if !strings.Contains(grepRes.Output, "sample.go") {
		t.Errorf("expected grep output to contain sample.go, got: %s", grepRes.Output)
	}

	// 2. Test ReadFile Tool
	readTool, ok := reg.Get("readFile")
	if !ok {
		t.Fatalf("readFile tool not registered")
	}
	readRes, err := readTool.Execute(toolCtx, map[string]any{
		"path":  "sample.go",
		"start": 3,
		"end":   5,
	})
	if err != nil || readRes.IsError {
		t.Fatalf("readFile failed: %v, res=%v", err, readRes)
	}
	if !strings.Contains(readRes.Output, "ImportantFunction") {
		t.Errorf("expected lines 3-5 to contain ImportantFunction, got: %s", readRes.Output)
	}

	// 3. Test ListDir Tool
	listTool, ok := reg.Get("listDir")
	if !ok {
		t.Fatalf("listDir tool not registered")
	}
	listRes, err := listTool.Execute(toolCtx, map[string]any{
		"path":     ".",
		"maxDepth": 2,
	})
	if err != nil || listRes.IsError {
		t.Fatalf("listDir failed: %v, res=%v", err, listRes)
	}
	if !strings.Contains(listRes.Output, "sample.go") || !strings.Contains(listRes.Output, "pkg/") {
		t.Errorf("expected listDir to list sample.go and pkg/, got: %s", listRes.Output)
	}

	// 4. Test Exec Tool (Read-only guard)
	execTool, ok := reg.Get("exec")
	if !ok {
		t.Fatalf("exec tool not registered")
	}
	// Safe command
	safeRes, err := execTool.Execute(toolCtx, map[string]any{"command": "echo 'hello world'"})
	if err != nil || safeRes.IsError {
		t.Fatalf("exec safe failed: %v, res=%v", err, safeRes)
	}
	if !strings.Contains(safeRes.Output, "hello world") {
		t.Errorf("expected hello world, got: %s", safeRes.Output)
	}

	// Disallowed destructive command
	badRes, err := execTool.Execute(toolCtx, map[string]any{"command": "rm -rf /tmp/xyz"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !badRes.IsError || !strings.Contains(badRes.Output, "disallowed") {
		t.Errorf("expected disallowed error for rm command, got: %v", badRes)
	}
}
