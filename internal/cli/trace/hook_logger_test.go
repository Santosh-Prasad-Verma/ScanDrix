// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package trace

import (
	"os"
	"strings"
	"testing"
)

func TestHookLogger(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("SCANDRIX_TRACE_HOME", tempDir)

	logger := &HookLogger{}
	if err := logger.Init(tempDir); err != nil {
		t.Fatalf("logger Init failed: %v", err)
	}

	logger.Info("Starting turn", "claude-hook", map[string]any{
		"prompt": "Here is my secret: sk-ant-api03-abcdef1234567890",
		"meta":   "normal-data",
	})

	logFile := HookLogPath(tempDir)
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed reading hook log file: %v", err)
	}

	logContent := string(data)
	if !strings.Contains(logContent, "claude-hook") {
		t.Errorf("expected component claude-hook in log")
	}
	if strings.Contains(logContent, "sk-ant-api03-abcdef1234567890") {
		t.Errorf("expected sensitive API key to be redacted in log")
	}
	if !strings.Contains(logContent, RedactionPlaceholder) {
		t.Errorf("expected %s in log content", RedactionPlaceholder)
	}
}
