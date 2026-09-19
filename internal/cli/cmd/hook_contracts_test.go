package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHookContracts_InstallStatusUninstall(t *testing.T) {
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	hooksDir := filepath.Join(gitDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatalf("failed to create hooks dir: %v", err)
	}

	origDir, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	buf := &bytes.Buffer{}
	rootCmd := RootCmd
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)

	// 1. Install hooks
	rootCmd.SetArgs([]string{"hook", "install"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("hook install failed: %v", err)
	}

	prePushPath := filepath.Join(hooksDir, "pre-push")
	if _, err := os.Stat(prePushPath); os.IsNotExist(err) {
		t.Errorf("pre-push hook was not created at %s", prePushPath)
	}

	// 2. Check hook status
	buf.Reset()
	rootCmd = RootCmd
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"hook", "status"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("hook status failed: %v", err)
	}
	if !strings.Contains(strings.ToLower(buf.String()), "installed") && !strings.Contains(strings.ToLower(buf.String()), "active") {
		t.Logf("hook status output: %s", buf.String())
	}

	// 3. Uninstall hooks
	buf.Reset()
	rootCmd = RootCmd
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"hook", "uninstall"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("hook uninstall failed: %v", err)
	}

	if _, err := os.Stat(prePushPath); !os.IsNotExist(err) {
		// Hook should either be deleted or ScanDrix section removed
		content, _ := os.ReadFile(prePushPath)
		if strings.Contains(string(content), "scandrix") {
			t.Errorf("scandrix hook command still found in pre-push after uninstall")
		}
	}
}
