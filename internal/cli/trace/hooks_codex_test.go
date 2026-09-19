// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package trace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexHooksInstallationAndRemoval(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.toml")

	// 1. Initial install on non-existent file
	changed, err := InstallCodexHooks(configPath)
	if err != nil {
		t.Fatalf("InstallCodexHooks failed: %v", err)
	}
	if !changed {
		t.Fatalf("expected changed = true on initial install")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed reading config: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, CodexHookMarker) {
		t.Fatalf("expected marker in config, got:\n%s", content)
	}
	if !strings.Contains(content, "event = \"AfterAgent\"") {
		t.Fatalf("expected AfterAgent event in config, got:\n%s", content)
	}

	// 2. Idempotent install
	changed, err = InstallCodexHooks(configPath)
	if err != nil {
		t.Fatalf("re-install failed: %v", err)
	}
	if changed {
		t.Fatalf("expected changed = false when already installed")
	}

	// 3. Remove hooks
	removed, err := RemoveCodexHooks(configPath)
	if err != nil {
		t.Fatalf("RemoveCodexHooks failed: %v", err)
	}
	if !removed {
		t.Fatalf("expected removed = true")
	}

	data, err = os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed reading config after remove: %v", err)
	}
	if strings.Contains(string(data), CodexHookMarker) {
		t.Fatalf("expected marker to be removed, got:\n%s", string(data))
	}

	// 4. Second removal returns false
	removed, err = RemoveCodexHooks(configPath)
	if err != nil {
		t.Fatalf("second removal failed: %v", err)
	}
	if removed {
		t.Fatalf("expected removed = false when marker absent")
	}
}
