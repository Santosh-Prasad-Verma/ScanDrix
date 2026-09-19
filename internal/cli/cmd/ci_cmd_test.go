// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package cmd_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/cli/cmd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCISubcommands(t *testing.T) {
	root := cmd.RootCmd

	// Verify 'ci' subcommand exists
	ciCmd, _, err := root.Find([]string{"ci"})
	require.NoError(t, err)
	require.NotNil(t, ciCmd)
	assert.Equal(t, "ci", ciCmd.Name())

	// Verify subcommands exist under 'ci'
	expectedChildren := []string{"run", "status", "format", "token"}
	for _, child := range expectedChildren {
		sub, _, err := ciCmd.Find([]string{child})
		require.NoError(t, err)
		require.NotNil(t, sub)
		assert.Equal(t, child, sub.Name())
	}
}

func TestCIStatusExecution(t *testing.T) {
	root := cmd.RootCmd

	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"ci", "status"})

	err := root.Execute()
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "ScanDrix CI Environment Detection")
	assert.Contains(t, out, "Platform:")
}

func TestCITokenExecution(t *testing.T) {
	root := cmd.RootCmd

	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"ci", "token", "--repo", "scandrix/test-repo", "--ttl", "1h"})

	err := root.Execute()
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "ScanDrix Scoped CI Pipeline Token Generated:")
	assert.Contains(t, out, "scandrix_ci_")
	assert.Contains(t, out, "scandrix/test-repo")
}

func TestCIFormatExecution(t *testing.T) {
	tmpDir := t.TempDir()
	findingsFile := filepath.Join(tmpDir, "findings.json")
	sarifOut := filepath.Join(tmpDir, "out.sarif")

	findingsJSON := `[
		{
			"id": "f1",
			"rule_id": "SEC-01",
			"rule_title": "SQL Injection",
			"category": "SECURITY",
			"severity": "critical",
			"file_path": "internal/db.go",
			"start_line": 15,
			"end_line": 15,
			"message": "Do not format SQL"
		}
	]`
	err := os.WriteFile(findingsFile, []byte(findingsJSON), 0644)
	require.NoError(t, err)

	root := cmd.RootCmd
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"ci", "format", "--input", findingsFile, "--to", "sarif", "--output", sarifOut})

	err = root.Execute()
	require.NoError(t, err)

	assert.FileExists(t, sarifOut)
	data, err := os.ReadFile(sarifOut)
	require.NoError(t, err)
	assert.True(t, strings.Contains(string(data), "2.1.0"))
	assert.True(t, strings.Contains(string(data), "SEC-01"))
}
