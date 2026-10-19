package cmd_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/cli/cmd"
)

func TestDatasetCommand_Help(t *testing.T) {
	root := cmd.RootCmd
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"dataset", "--help"})

	err := root.Execute()
	if err != nil {
		t.Fatalf("expected no error running dataset --help, got: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "training datasets (JSONL)") {
		t.Errorf("unexpected output from dataset --help: %s", out)
	}
}

func TestDatasetExportCommand_Help(t *testing.T) {
	root := cmd.RootCmd
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"dataset", "export", "--help"})

	err := root.Execute()
	if err != nil {
		t.Fatalf("expected no error running dataset export --help, got: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Export instruction-tuning or DPO dataset in JSONL format") {
		t.Errorf("unexpected output from dataset export --help: %s", out)
	}
	if !strings.Contains(out, "--format") || !strings.Contains(out, "--output") {
		t.Errorf("expected format and output flags in help: %s", out)
	}
}
