// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package trace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAgentCliPreferences(t *testing.T) {
	if len(AgentCliPreference) < 4 {
		t.Fatalf("expected at least 4 default agent CLIs, got %d", len(AgentCliPreference))
	}

	names := make([]string, len(AgentCliPreference))
	for i, spec := range AgentCliPreference {
		names[i] = spec.Name
	}

	expected := []string{"claude", "codex", "gemini", "cursor"}
	for _, exp := range expected {
		found := false
		for _, n := range names {
			if n == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %s in AgentCliPreference, got %v", exp, names)
		}
	}
}

func TestResolveAgentCli_EnvOverride(t *testing.T) {
	t.Setenv("SCANDRIX_TRACE_AGENT_CMD", "cat -")

	resolved, err := ResolveAgentCli("")
	if err != nil {
		t.Fatalf("ResolveAgentCli failed: %v", err)
	}

	if resolved.Spec.Bin != "cat" {
		t.Errorf("expected bin 'cat', got %s", resolved.Spec.Bin)
	}
	if len(resolved.Spec.Args) != 1 || resolved.Spec.Args[0] != "-" {
		t.Errorf("expected args ['-'], got %v", resolved.Spec.Args)
	}
}

func TestRunAgentCli_Execution(t *testing.T) {
	spec := AgentCliSpec{
		Name: "cat",
		Bin:  "cat",
		Args: []string{},
	}

	prompt := "Architectural Decision: Adopt Hexagonal Architecture"
	ctx := context.Background()

	out, err := RunAgentCli(ctx, spec, prompt, t.TempDir(), 5*time.Second)
	if err != nil {
		t.Fatalf("RunAgentCli failed: %v", err)
	}

	if strings.TrimSpace(out) != prompt {
		t.Errorf("expected stdout '%s', got '%s'", prompt, strings.TrimSpace(out))
	}
}

func TestTraceConfig_RoundTrip(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("SCANDRIX_TRACE_HOME", tempHome)

	cfg := &TraceConfig{AgentCli: "gemini"}
	if err := writeTraceConfig(cfg); err != nil {
		t.Fatalf("writeTraceConfig failed: %v", err)
	}

	read, err := readTraceConfig()
	if err != nil {
		t.Fatalf("readTraceConfig failed: %v", err)
	}

	if read.AgentCli != "gemini" {
		t.Errorf("expected AgentCli 'gemini', got '%s'", read.AgentCli)
	}

	// Verify file permissions 0600
	fi, err := os.Stat(filepath.Join(tempHome, "trace-config.json"))
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Errorf("expected mode 0600, got %o", fi.Mode().Perm())
	}
}
