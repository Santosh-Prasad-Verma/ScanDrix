package cmd_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/cli/cmd"
)

func TestBotCommand_Help(t *testing.T) {
	root := cmd.RootCmd
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"bot", "--help"})

	err := root.Execute()
	if err != nil {
		t.Fatalf("expected no error running bot --help, got: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "interactive bot daemon") || !strings.Contains(out, "daemon") {
		t.Errorf("unexpected output from bot --help: %s", out)
	}
}

func TestBotDaemonCommand_Help(t *testing.T) {
	root := cmd.RootCmd
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"bot", "daemon", "--help"})

	err := root.Execute()
	if err != nil {
		t.Fatalf("expected no error running bot daemon --help, got: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Start the interactive bot webhook daemon listener") {
		t.Errorf("unexpected output from bot daemon --help: %s", out)
	}
}
