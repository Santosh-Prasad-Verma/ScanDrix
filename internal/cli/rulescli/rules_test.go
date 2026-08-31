package rulescli_test

import (
	"os"
	"testing"

	"github.com/scandrix/backend/internal/cli/rulescli"
)

func TestRulesInitAndValidate(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "scandrix-rules-test-*")
	if err != nil {
		t.Fatalf("failed creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 1. Initialize starter rules
	path, err := rulescli.Init(tempDir)
	if err != nil || path == "" {
		t.Fatalf("rules init failed: %v", err)
	}

	// 2. Validate rules
	count, err := rulescli.Validate(tempDir)
	if err != nil || count == 0 {
		t.Fatalf("rules validate failed: %v", err)
	}

	// 3. Double init should fail cleanly
	if _, err := rulescli.Init(tempDir); err == nil {
		t.Fatal("expected error on re-initializing existing rules file")
	}
}
