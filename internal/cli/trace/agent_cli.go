// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package trace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// AgentCliSpec defines the invocation contract for an external coding agent CLI.
type AgentCliSpec struct {
	Name string   `json:"name"`
	Bin  string   `json:"bin"`
	Args []string `json:"args"`
}

// AgentCliPreference establishes the fallback resolution order for CLI agents on $PATH.
var AgentCliPreference = []AgentCliSpec{
	{Name: "claude", Bin: "claude", Args: []string{"-p"}},
	{Name: "codex", Bin: "codex", Args: []string{"exec", "-"}},
	{Name: "gemini", Bin: "gemini", Args: []string{"-p"}},
	{Name: "cursor", Bin: "cursor-agent", Args: []string{"-p"}},
}

// TraceConfig models stored user preferences in ~/.scandrix/trace-config.json.
type TraceConfig struct {
	AgentCli string `json:"agentCli,omitempty"`
}

// ResolvedAgentCli bundles the found CLI specification and provenance.
type ResolvedAgentCli struct {
	Spec       AgentCliSpec
	FromConfig bool
}

// readTraceConfig reads trace configuration from disk.
func readTraceConfig() (*TraceConfig, error) {
	p := TraceConfigPath()
	data, err := os.ReadFile(p)
	if err != nil {
		return &TraceConfig{}, nil
	}
	var cfg TraceConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return &TraceConfig{}, nil
	}
	return &cfg, nil
}

// writeTraceConfig writes trace configuration to disk with 0600 permissions.
func writeTraceConfig(cfg *TraceConfig) error {
	p := TraceConfigPath()
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(data, '\n'), 0600)
}

// ResolveAgentCli locates the appropriate coding agent CLI installed on the system.
func ResolveAgentCli(override string) (*ResolvedAgentCli, error) {
	// 1. Environment variable override
	if envCmd := strings.TrimSpace(os.Getenv("SCANDRIX_TRACE_AGENT_CMD")); envCmd != "" {
		parts := strings.Fields(envCmd)
		if len(parts) > 0 {
			return &ResolvedAgentCli{
				Spec: AgentCliSpec{
					Name: parts[0],
					Bin:  parts[0],
					Args: parts[1:],
				},
				FromConfig: false,
			}, nil
		}
	}

	// 2. Explicit flag override
	if override != "" {
		trimmed := strings.TrimSpace(override)
		for _, p := range AgentCliPreference {
			if p.Name == trimmed || p.Bin == trimmed {
				if _, err := exec.LookPath(p.Bin); err == nil {
					return &ResolvedAgentCli{Spec: p, FromConfig: false}, nil
				}
				return nil, fmt.Errorf("requested agent CLI '%s' not found on PATH", trimmed)
			}
		}
		// Custom binary name
		if _, err := exec.LookPath(trimmed); err == nil {
			return &ResolvedAgentCli{
				Spec: AgentCliSpec{Name: trimmed, Bin: trimmed, Args: []string{"-p"}},
			}, nil
		}
		return nil, fmt.Errorf("agent CLI '%s' not found on PATH", trimmed)
	}

	// 3. Cached preference in configuration
	cfg, _ := readTraceConfig()
	if cfg != nil && cfg.AgentCli != "" {
		for _, p := range AgentCliPreference {
			if p.Name == cfg.AgentCli {
				if _, err := exec.LookPath(p.Bin); err == nil {
					return &ResolvedAgentCli{Spec: p, FromConfig: true}, nil
				}
			}
		}
	}

	// 4. Probe fixed preference order on PATH
	for _, p := range AgentCliPreference {
		if _, err := exec.LookPath(p.Bin); err == nil {
			_ = writeTraceConfig(&TraceConfig{AgentCli: p.Name})
			return &ResolvedAgentCli{Spec: p, FromConfig: false}, nil
		}
	}

	return nil, errors.New("no supported agent CLI found on PATH (checked: claude, codex, gemini, cursor-agent)")
}

// RunAgentCli executes the resolved agent CLI with the given prompt piped on stdin.
// Injects SCANDRIX_TRACE_SKIP=1 to prevent recursive hook interception.
func RunAgentCli(ctx context.Context, spec AgentCliSpec, prompt string, cwd string, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		timeout = 3 * time.Minute
	}

	ctxTimeout, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctxTimeout, spec.Bin, spec.Args...)
	if cwd != "" {
		cmd.Dir = cwd
	}

	// Set environment and inject anti-recursion sentinel
	env := os.Environ()
	env = append(env, "SCANDRIX_TRACE_SKIP=1")
	cmd.Env = env

	cmd.Stdin = strings.NewReader(prompt)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		return "", fmt.Errorf("agent CLI '%s' failed: %s", spec.Name, errMsg)
	}

	return stdout.String(), nil
}
