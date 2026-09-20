package wizard

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepoSettingsWizard_PromptAndSave(t *testing.T) {
	input := "heavy\ncritical\ny\ny\ny\n"
	r := strings.NewReader(input)
	w := &bytes.Buffer{}

	wizard := NewRepoSettingsWizard(r, w)
	ctx := context.Background()

	opts, err := wizard.PromptConfig(ctx, WizardOptions{})
	if err != nil {
		t.Fatalf("PromptConfig failed: %v", err)
	}

	if opts.ReviewMode != "heavy" {
		t.Errorf("expected review mode heavy, got %s", opts.ReviewMode)
	}
	if opts.FailOnSeverity != "critical" {
		t.Errorf("expected severity critical, got %s", opts.FailOnSeverity)
	}
	if !opts.EnableHooks || !opts.AutoFix || !opts.OfflineFallback {
		t.Errorf("expected hooks, autofix, and offline fallback enabled")
	}

	tmpDir := t.TempDir()
	if err := wizard.SaveConfig(tmpDir, *opts); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	loaded, err := wizard.LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if loaded.ReviewMode != "heavy" || loaded.FailOnSeverity != "critical" {
		t.Errorf("loaded config does not match saved config: %+v", loaded)
	}
	if len(loaded.IgnoredPaths) == 0 {
		t.Errorf("expected default ignored paths")
	}

	// Verify file exists
	path := filepath.Join(tmpDir, ".scandrix.yml")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("config file was not created at %s", path)
	}
}
