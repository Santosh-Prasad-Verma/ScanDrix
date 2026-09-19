// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package utils_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/cli/utils"
	"github.com/spf13/cobra"
)

func TestAgentEnvelopes(t *testing.T) {
	start := time.Now().UTC()

	// 1. Success Envelope
	payload := map[string]any{
		"repo":   "scandrix",
		"status": "ready",
	}
	successEnv := utils.BuildAgentSuccessEnvelope("test", payload, start)
	if !successEnv.OK {
		t.Fatalf("expected OK to be true")
	}
	if successEnv.Command != "test" {
		t.Fatalf("expected command 'test', got '%s'", successEnv.Command)
	}
	if successEnv.Meta.SchemaVersion != "1.0" {
		t.Fatalf("expected schema version '1.0', got '%s'", successEnv.Meta.SchemaVersion)
	}

	// 2. Error Envelope
	errPayload := utils.AgentErrorPayload{
		Code:    "AUTH_REQUIRED",
		Message: "Authentication required",
		Details: map[string]string{"hint": "login"},
	}
	errEnv := utils.BuildAgentErrorEnvelope("test", errPayload, start)
	if errEnv.OK {
		t.Fatalf("expected OK to be false")
	}
	if errEnv.Error.Code != "AUTH_REQUIRED" {
		t.Fatalf("expected code 'AUTH_REQUIRED', got '%s'", errEnv.Error.Code)
	}
}

func TestNormalizeCommandError(t *testing.T) {
	// Nil error
	norm := utils.NormalizeCommandError(nil)
	if norm.Code != "" || norm.ExitCode != 0 {
		t.Fatalf("expected zero error on nil, got %+v", norm)
	}

	// Typed CommandError
	typed := utils.NewCommandError("NOT_IN_GIT_REPO", "Must be inside a git repository", 1, nil)
	normTyped := utils.NormalizeCommandError(typed)
	if normTyped.Code != "NOT_IN_GIT_REPO" || normTyped.ExitCode != 1 {
		t.Fatalf("unexpected normalized error: %+v", normTyped)
	}

	// Standard Go error
	std := errors.New("unauthorized 401 token expired")
	normStd := utils.NormalizeCommandError(std)
	if normStd.Code != string(utils.ErrCodeAuthRequired) {
		t.Fatalf("expected AUTH_REQUIRED code from 401 error, got %s", normStd.Code)
	}
}

func TestFieldMask(t *testing.T) {
	data := map[string]any{
		"id":    "123",
		"title": "Secret rule",
		"nested": map[string]any{
			"deep": "value",
			"drop": "hidden",
		},
	}

	fields := utils.ParseFieldList("id,nested.deep")
	if len(fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(fields))
	}

	masked, err := utils.ApplyFieldMask(data, fields)
	if err != nil {
		t.Fatalf("ApplyFieldMask failed: %v", err)
	}

	m, ok := masked.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any result")
	}

	if m["id"] != "123" {
		t.Errorf("expected id '123', got %v", m["id"])
	}
	if _, exists := m["title"]; exists {
		t.Errorf("expected title to be omitted by mask")
	}
	nestedMap, ok := m["nested"].(map[string]any)
	if !ok || nestedMap["deep"] != "value" {
		t.Errorf("expected nested.deep 'value', got %v", m["nested"])
	}
	if _, exists := nestedMap["drop"]; exists {
		t.Errorf("expected nested.drop to be omitted by mask")
	}
}

func TestCredentialsManager(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", tmpDir)

	// Initial load should error (not exist)
	creds, err := utils.LoadCredentials()
	if err == nil && creds != nil {
		t.Fatalf("expected error or nil credentials on fresh dir")
	}

	// Save
	newCreds := &utils.StoredCredentials{
		AccessToken: "test-token",
		ExpiresAt:   time.Now().Add(1 * time.Hour).UnixMilli(),
	}
	if err := utils.SaveCredentials(newCreds); err != nil {
		t.Fatalf("save error: %v", err)
	}

	// Reload
	reloaded, err := utils.LoadCredentials()
	if err != nil {
		t.Fatalf("reload error: %v", err)
	}
	if reloaded.AccessToken != "test-token" {
		t.Fatalf("expected 'test-token', got '%s'", reloaded.AccessToken)
	}

	// Permissions check (0600 on posix)
	path := filepath.Join(tmpDir, ".scandrix", "credentials.json")
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("credentials file stat error: %v", err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Errorf("expected file mode 0600, got %o", fi.Mode().Perm())
	}

	// Clear
	if err := utils.ClearCredentials(); err != nil {
		t.Fatalf("clear error: %v", err)
	}
}

func TestSchemagen(t *testing.T) {
	root := &cobra.Command{
		Use:   "app",
		Short: "Test root command",
	}
	var flagVal string
	root.Flags().StringVarP(&flagVal, "test-flag", "t", "default", "A test flag")

	sub := &cobra.Command{
		Use:   "sub",
		Short: "Test sub command",
	}
	root.AddCommand(sub)

	schema := utils.BuildCommandSchema(root)
	if schema.Name != "app" {
		t.Fatalf("expected schema name 'app', got '%s'", schema.Name)
	}
	if len(schema.Options) != 1 {
		t.Fatalf("expected 1 option, got %d", len(schema.Options))
	}
	if len(schema.Subcommands) != 1 {
		t.Fatalf("expected 1 subcommand, got %d", len(schema.Subcommands))
	}

	found := utils.FindCommandByPath(root, "sub")
	if found == nil || found.Name() != "sub" {
		t.Fatalf("FindCommandByPath failed to find 'sub'")
	}
}

func TestActivityLog(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", tmpDir)

	if err := utils.RecordRecentActivity("review --staged"); err != nil {
		t.Fatalf("RecordRecentActivity failed: %v", err)
	}

	actPath := filepath.Join(tmpDir, ".scandrix", "activity.json")
	data, err := os.ReadFile(actPath)
	if err != nil {
		t.Fatalf("failed reading activity.json: %v", err)
	}

	var state struct {
		Recent []struct {
			Command string `json:"command"`
		} `json:"recent"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(state.Recent) != 1 || state.Recent[0].Command != "review --staged" {
		t.Fatalf("unexpected recent commands: %+v", state.Recent)
	}

	// Verify sensitive flag redaction
	if err := utils.RecordRecentActivity("review --key scandrix_team_secret123 --branch main"); err != nil {
		t.Fatalf("RecordRecentActivity failed: %v", err)
	}

	data, err = os.ReadFile(actPath)
	if err != nil {
		t.Fatalf("failed reading activity.json: %v", err)
	}
	_ = json.Unmarshal(data, &state)

	if len(state.Recent) < 2 {
		t.Fatalf("expected at least 2 entries, got %d", len(state.Recent))
	}

	lastCmd := state.Recent[0].Command
	if strings.Contains(lastCmd, "scandrix_team_secret123") {
		t.Errorf("expected secret to be redacted, got: %s", lastCmd)
	}
	if !strings.Contains(lastCmd, "[REDACTED]") {
		t.Errorf("expected [REDACTED] in command, got: %s", lastCmd)
	}
}

func TestClipboardCopy(t *testing.T) {
	// Must not panic on arbitrary strings
	_ = utils.CopyToClipboard("ScanDrix remediation text")
}
