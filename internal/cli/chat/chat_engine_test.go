// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestToolRegistry_BuiltinTools(t *testing.T) {
	tempDir := t.TempDir()

	testFile := filepath.Join(tempDir, "sample.go")
	_ = os.WriteFile(testFile, []byte("package sample\n\nfunc Hello() string {\n\treturn \"world\"\n}\n"), 0600)

	reg := NewToolRegistry(tempDir)

	// Verify all tools registered
	expectedTools := []string{"view_file", "search_files", "list_files", "run_command", "apply_patch", "pentest_scan"}
	for _, name := range expectedTools {
		if _, ok := reg.Get(name); !ok {
			t.Fatalf("expected tool %q to be registered", name)
		}
	}

	// Test ViewFileTool
	viewRes, err := reg.Execute(context.Background(), "view_file", json.RawMessage(`{"path": "sample.go", "start_line": 1, "end_line": 3}`))
	if err != nil || viewRes.IsError {
		t.Fatalf("view_file failed: %v, output: %s", err, viewRes.Output)
	}
	if !strings.Contains(viewRes.Output, "package sample") {
		t.Errorf("expected view output to contain 'package sample'")
	}

	// Test SearchFilesTool
	searchRes, err := reg.Execute(context.Background(), "search_files", json.RawMessage(`{"query": "Hello"}`))
	if err != nil || searchRes.IsError {
		t.Fatalf("search_files failed: %v, output: %s", err, searchRes.Output)
	}
	if !strings.Contains(searchRes.Output, "sample.go") {
		t.Errorf("expected search output to reference sample.go")
	}

	// Test ListFilesTool
	listRes, err := reg.Execute(context.Background(), "list_files", json.RawMessage(`{}`))
	if err != nil || listRes.IsError {
		t.Fatalf("list_files failed: %v", err)
	}
	if !strings.Contains(listRes.Output, "sample.go") {
		t.Errorf("expected list output to include sample.go")
	}

	// Test ApplyPatchTool
	patchRes, err := reg.Execute(context.Background(), "apply_patch", json.RawMessage(`{
		"path": "sample.go",
		"target_content": "return \"world\"",
		"replacement_content": "return \"scandrix\""
	}`))
	if err != nil || patchRes.IsError {
		t.Fatalf("apply_patch failed: %v, output: %s", err, patchRes.Output)
	}

	updatedBytes, _ := os.ReadFile(testFile)
	if !strings.Contains(string(updatedBytes), "return \"scandrix\"") {
		t.Fatalf("patched file content not updated: %s", string(updatedBytes))
	}
}

func TestInteractiveREPL_SlashCommands(t *testing.T) {
	tempDir := t.TempDir()
	reg := NewToolRegistry(tempDir)
	engine := NewAgentEngine(nil, reg, "local-model")
	repl := NewInteractiveREPL(tempDir, engine, reg)

	var buf bytes.Buffer
	// Test /help
	err := repl.HandleInput(context.Background(), "/help", &buf)
	if err != nil {
		t.Fatalf("/help failed: %v", err)
	}

	// Test /persona
	err = repl.HandleInput(context.Background(), "/persona security", &buf)
	if err != nil {
		t.Fatalf("/persona failed: %v", err)
	}
	if !strings.Contains(repl.activePersona, "Security") {
		t.Errorf("expected security persona, got %s", repl.activePersona)
	}

	// Test /tools
	err = repl.HandleInput(context.Background(), "/tools", &buf)
	if err != nil {
		t.Fatalf("/tools failed: %v", err)
	}

	// Test /clear
	err = repl.HandleInput(context.Background(), "/clear", &buf)
	if err != nil {
		t.Fatalf("/clear failed: %v", err)
	}
	if len(repl.history) != 0 {
		t.Errorf("expected history cleared")
	}
}
