// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package trace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClaudeSessionHooksInstallAndRemove(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Initial install
	res, err := InstallSessionHooks(tempDir, "claude-code")
	if err != nil {
		t.Fatalf("InstallSessionHooks failed: %v", err)
	}
	if !res.Changed {
		t.Fatalf("expected changed = true on initial install")
	}

	data, err := os.ReadFile(res.SettingsPath)
	if err != nil {
		t.Fatalf("failed reading settings: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("failed parsing settings JSON: %v", err)
	}

	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		t.Fatalf("expected hooks object in settings")
	}

	// Verify SessionStart
	if matchers, ok := hooks["SessionStart"].([]any); !ok || len(matchers) == 0 {
		t.Fatalf("expected SessionStart matchers")
	} else {
		m := matchers[0].(map[string]any)
		hList := m["hooks"].([]any)
		h := hList[0].(map[string]any)
		if h["command"] != "scandrix trace hooks claude-code session-start" {
			t.Fatalf("unexpected command: %v", h["command"])
		}
	}

	// Verify PostToolUse with TodoWrite matcher
	if matchers, ok := hooks["PostToolUse"].([]any); !ok || len(matchers) == 0 {
		t.Fatalf("expected PostToolUse matchers")
	} else {
		m := matchers[0].(map[string]any)
		if m["matcher"] != "TodoWrite" {
			t.Fatalf("expected matcher TodoWrite, got: %v", m["matcher"])
		}
		hList := m["hooks"].([]any)
		h := hList[0].(map[string]any)
		if h["command"] != "scandrix trace hooks claude-code post-todo" {
			t.Fatalf("unexpected command: %v", h["command"])
		}
	}

	// 2. Idempotent install
	res2, err := InstallSessionHooks(tempDir, "claude-code")
	if err != nil {
		t.Fatalf("re-install failed: %v", err)
	}
	if res2.Changed {
		t.Fatalf("expected changed = false on idempotent install")
	}

	// 3. Upgrade legacy hook (e.g. scandrix decisions ...)
	legacySettings := map[string]any{
		"hooks": map[string]any{
			"SessionStart": []any{
				map[string]any{
					"matcher": "",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "scandrix decisions legacy-start",
						},
					},
				},
			},
		},
	}
	legacyBytes, _ := json.MarshalIndent(legacySettings, "", "  ")
	_ = os.WriteFile(res.SettingsPath, legacyBytes, 0644)

	res3, err := InstallSessionHooks(tempDir, "claude-code")
	if err != nil {
		t.Fatalf("upgrade install failed: %v", err)
	}
	if !res3.Changed {
		t.Fatalf("expected changed = true when upgrading legacy hook")
	}

	// 4. Remove hooks
	remRes, err := RemoveSessionHooks(tempDir)
	if err != nil {
		t.Fatalf("RemoveSessionHooks failed: %v", err)
	}
	if !remRes.Removed {
		t.Fatalf("expected removed = true")
	}

	dataAfterRemove, _ := os.ReadFile(res.SettingsPath)
	var afterSettings map[string]any
	_ = json.Unmarshal(dataAfterRemove, &afterSettings)
	if _, exists := afterSettings["hooks"]; exists {
		t.Fatalf("expected hooks to be deleted after removal, got: %v", afterSettings)
	}

	// 5. Second removal
	remRes2, err := RemoveSessionHooks(tempDir)
	if err != nil {
		t.Fatalf("second RemoveSessionHooks failed: %v", err)
	}
	if remRes2.Removed {
		t.Fatalf("expected removed = false when hooks already gone")
	}
}

func TestCursorSessionHooksInstallAndRemove(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Initial install
	res, err := InstallCursorSessionHooks(tempDir)
	if err != nil {
		t.Fatalf("InstallCursorSessionHooks failed: %v", err)
	}
	if !res.Changed {
		t.Fatalf("expected changed = true")
	}

	data, err := os.ReadFile(res.SettingsPath)
	if err != nil {
		t.Fatalf("failed reading cursor hooks: %v", err)
	}

	var conf CursorHooksConfig
	if err := json.Unmarshal(data, &conf); err != nil {
		t.Fatalf("failed parsing cursor hooks: %v", err)
	}

	if conf.Version != 1 {
		t.Fatalf("expected version 1, got %d", conf.Version)
	}

	if entries, ok := conf.Hooks["sessionStart"]; !ok || len(entries) == 0 {
		t.Fatalf("expected sessionStart entry")
	} else if entries[0].Command != "scandrix trace hooks cursor sessionStart" {
		t.Fatalf("unexpected cursor command: %s", entries[0].Command)
	}

	// 2. Idempotent install
	res2, err := InstallCursorSessionHooks(tempDir)
	if err != nil {
		t.Fatalf("re-install failed: %v", err)
	}
	if res2.Changed {
		t.Fatalf("expected changed = false on idempotent install")
	}

	// 3. Remove hooks
	remRes, err := RemoveCursorSessionHooks(tempDir)
	if err != nil {
		t.Fatalf("RemoveCursorSessionHooks failed: %v", err)
	}
	if !remRes.Removed {
		t.Fatalf("expected removed = true")
	}

	dataAfter, _ := os.ReadFile(res.SettingsPath)
	var confAfter CursorHooksConfig
	_ = json.Unmarshal(dataAfter, &confAfter)
	if len(confAfter.Hooks) != 0 {
		t.Fatalf("expected empty hooks after removal, got %v", confAfter.Hooks)
	}

	// 4. Second removal
	remRes2, err := RemoveCursorSessionHooks(tempDir)
	if err != nil {
		t.Fatalf("second RemoveCursorSessionHooks failed: %v", err)
	}
	if remRes2.Removed {
		t.Fatalf("expected removed = false on second removal")
	}
}

func TestCodexSessionHooksInstallAndRemove(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.toml")

	res, err := InstallCodexSessionHooks(configPath)
	if err != nil {
		t.Fatalf("InstallCodexSessionHooks failed: %v", err)
	}
	if !res.Changed {
		t.Fatalf("expected changed = true")
	}

	data, err := os.ReadFile(res.SettingsPath)
	if err != nil {
		t.Fatalf("failed reading config.toml: %v", err)
	}
	if !strings.Contains(string(data), CodexHookMarker) {
		t.Fatalf("expected marker in config")
	}

	// Idempotent
	res2, err := InstallCodexSessionHooks(configPath)
	if err != nil {
		t.Fatalf("re-install failed: %v", err)
	}
	if res2.Changed {
		t.Fatalf("expected changed = false")
	}

	// Remove
	remRes, err := RemoveCodexSessionHooks(configPath)
	if err != nil {
		t.Fatalf("RemoveCodexSessionHooks failed: %v", err)
	}
	if !remRes.Removed {
		t.Fatalf("expected removed = true")
	}
}
