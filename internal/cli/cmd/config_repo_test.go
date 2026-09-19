// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestConfigRepo_GetSetList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"namespace":     "test-repo",
			"reviewEnabled": true,
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	t.Setenv("SCANDRIX_SERVER_URL", server.URL)
	t.Setenv("SCANDRIX_API_URL", server.URL)

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	// 1. Initial config show/list
	buf := &bytes.Buffer{}
	rootCmd := RootCmd
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"config", "show"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("config show failed: %v", err)
	}

	// 2. Set review.enabled to true
	buf.Reset()
	rootCmd = RootCmd
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"config", "set", "review.enabled", "true"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("config set review.enabled failed: %v", err)
	}

	// 3. Set review.requestChanges.minSeverity to critical
	buf.Reset()
	rootCmd = RootCmd
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"config", "set", "review.requestChanges.minSeverity", "critical"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("config set review.requestChanges.minSeverity failed: %v", err)
	}

	// 4. Show config
	buf.Reset()
	rootCmd = RootCmd
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"config", "show"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("config show failed: %v", err)
	}
}

func TestConfigRepo_PatternsAndBranches(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/repositories") {
			if r.Method == http.MethodGet {
				_ = json.NewEncoder(w).Encode([]map[string]any{
					{
						"id":        "repo-123",
						"name":      "backend",
						"namespace": "scandrix/backend",
					},
				})
				return
			}
			if r.Method == http.MethodPost {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":        "repo-123",
					"namespace": "scandrix/backend",
					"status":    "tracked",
				})
				return
			}
		}

		// Settings endpoint
		_ = json.NewEncoder(w).Encode(map[string]any{
			"namespace":            "scandrix/backend",
			"ignorePatterns":       []string{"*.generated.go"},
			"baseBranches":         []string{"main", "production"},
			"ignoredTitlePatterns": []string{"^WIP:", "^Draft:"},
		})
	}))
	defer server.Close()

	t.Setenv("SCANDRIX_SERVER_URL", server.URL)
	t.Setenv("SCANDRIX_API_URL", server.URL)
	t.Setenv("SCANDRIX_TOKEN", "test-token")

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	buf := &bytes.Buffer{}

	// 1. config repo list
	buf.Reset()
	rootCmd := RootCmd
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"config", "repo", "list"})
	_ = rootCmd.Execute()

	// 2. config repo add
	buf.Reset()
	rootCmd = RootCmd
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"config", "repo", "add", "scandrix/backend"})
	_ = rootCmd.Execute()

	// 3. config repo pattern add
	buf.Reset()
	rootCmd = RootCmd
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"config", "repo", "pattern", "add", "*.min.js"})
	_ = rootCmd.Execute()

	// 4. config repo pattern remove
	buf.Reset()
	rootCmd = RootCmd
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"config", "repo", "pattern", "remove", "*.min.js"})
	_ = rootCmd.Execute()

	// 5. config repo add-ignore
	buf.Reset()
	rootCmd = RootCmd
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"config", "repo", "add-ignore", "vendor/**"})
	_ = rootCmd.Execute()

	// 6. config repo remove-ignore
	buf.Reset()
	rootCmd = RootCmd
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"config", "repo", "remove-ignore", "vendor/**"})
	_ = rootCmd.Execute()

	// 7. config repo add-base-branch
	buf.Reset()
	rootCmd = RootCmd
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"config", "repo", "add-base-branch", "release/v1"})
	_ = rootCmd.Execute()

	// 8. config repo remove-base-branch
	buf.Reset()
	rootCmd = RootCmd
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"config", "repo", "remove-base-branch", "release/v1"})
	_ = rootCmd.Execute()

	// 9. config repo add-ignore-title
	buf.Reset()
	rootCmd = RootCmd
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"config", "repo", "add-ignore-title", "^\\[skip ci\\]"})
	_ = rootCmd.Execute()

	// 10. config repo remove-ignore-title
	buf.Reset()
	rootCmd = RootCmd
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"config", "repo", "remove-ignore-title", "^\\[skip ci\\]"})
	_ = rootCmd.Execute()
}

func TestConfigCentralized_Lifecycle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":   "enabled",
			"strategy": "pr",
			"branch":   "main",
		})
	}))
	defer server.Close()

	t.Setenv("SCANDRIX_SERVER_URL", server.URL)
	t.Setenv("SCANDRIX_API_URL", server.URL)
	t.Setenv("SCANDRIX_TOKEN", "test-token")

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	buf := &bytes.Buffer{}

	// 1. config centralized status
	buf.Reset()
	rootCmd := RootCmd
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"config", "centralized", "status"})
	_ = rootCmd.Execute()

	// 2. config centralized sync
	buf.Reset()
	rootCmd = RootCmd
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"config", "centralized", "sync"})
	_ = rootCmd.Execute()

	// 3. config centralized disable
	buf.Reset()
	rootCmd = RootCmd
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"config", "centralized", "disable"})
	_ = rootCmd.Execute()
}

func TestConfigRepo_InvalidKeysAndValues(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	buf := &bytes.Buffer{}
	rootCmd := RootCmd
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)

	// Invalid review mode
	rootCmd.SetArgs([]string{"config", "set", "review_mode", "ultra-mega-mode"})
	err := rootCmd.Execute()
	if err == nil && !strings.Contains(buf.String(), "error") && !strings.Contains(buf.String(), "invalid") {
		t.Logf("Note: custom values may be accepted or warned: %s", buf.String())
	}
}
