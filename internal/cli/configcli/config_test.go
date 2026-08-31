package configcli_test

import (
	"os"
	"testing"

	"github.com/scandrix/backend/internal/cli/configcli"
)

func TestHierarchicalConfigLoadAndSave(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "scandrix-config-test-*")
	if err != nil {
		t.Fatalf("failed creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 1. Save repo config
	repoCfg := &configcli.CLIConfig{
		ServerURL:      "https://custom-server.scandrix.dev",
		DefaultFormat:  "sarif",
		FailOnSeverity: "CRITICAL",
	}
	if err := configcli.SaveRepo(tempDir, repoCfg); err != nil {
		t.Fatalf("failed saving repo config: %v", err)
	}

	// 2. Load merged config
	loaded := configcli.Load(tempDir)
	if loaded.ServerURL != "https://custom-server.scandrix.dev" || loaded.DefaultFormat != "sarif" {
		t.Fatalf("unexpected loaded config: %+v", loaded)
	}
}
