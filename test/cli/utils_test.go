// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package cli_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/cli/utils"
)

// AGENT ENVELOPE CONTRACT TESTS

func TestEnvelope_SuccessEnvelope(t *testing.T) {
	startTime := time.Now().Add(-50 * time.Millisecond)
	payload := map[string]string{"result": "ok", "target": "auth"}

	env := utils.BuildAgentSuccessEnvelope("auth login", payload, startTime)
	if !env.OK {
		t.Errorf("expected OK to be true")
	}
	if env.Command != "auth login" {
		t.Errorf("expected command 'auth login', got %s", env.Command)
	}
	if env.Meta.Mode != "agent" {
		t.Errorf("expected mode 'agent', got %s", env.Meta.Mode)
	}
	if env.Meta.DurationMs <= 0 {
		t.Errorf("expected positive DurationMs, got %d", env.Meta.DurationMs)
	}
	if env.Error != nil {
		t.Errorf("expected nil error on success envelope")
	}
}

func TestEnvelope_ErrorEnvelope(t *testing.T) {
	startTime := time.Now().Add(-20 * time.Millisecond)
	payload := utils.AgentErrorPayload{
		Code:    "NOT_IN_GIT_REPO",
		Message: "Directory is not a git repository",
		Details: map[string]string{"dir": "/tmp"},
	}
	env := utils.BuildAgentErrorEnvelope("review", payload, startTime)

	if env.OK {
		t.Errorf("expected OK to be false")
	}
	if env.Error.Code != "NOT_IN_GIT_REPO" {
		t.Errorf("expected code NOT_IN_GIT_REPO, got %s", env.Error.Code)
	}
	if !strings.Contains(env.Error.Message, "not a git repository") {
		t.Errorf("expected error message in payload")
	}
}

// COMMAND ERROR & EXIT CODE TESTS

func TestErrors_CommandError(t *testing.T) {
	err := utils.NewCommandError(utils.ErrCodeAuthRequired, "Please authenticate using 'scandrix auth login'", 2)

	if err.Code != utils.ErrCodeAuthRequired {
		t.Errorf("expected code ErrCodeAuthRequired, got %s", err.Code)
	}
	if err.ExitCode != 2 {
		t.Errorf("expected exit code 2, got %d", err.ExitCode)
	}
	if err.Error() != "Please authenticate using 'scandrix auth login'" {
		t.Errorf("unexpected error string: %s", err.Error())
	}
}

// FIELD MASK TESTS

func TestFieldMask_ParseAndApply(t *testing.T) {
	fields := utils.ParseFieldList("id, user.email, user.role")
	if len(fields) != 3 {
		t.Fatalf("expected 3 fields parsed, got %d", len(fields))
	}

	input := map[string]any{
		"id":     "usr-1",
		"secret": "hidden-secret-key",
		"user": map[string]any{
			"email":    "dev@scandrix.dev",
			"role":     "admin",
			"password": "hashed_pass_secret",
		},
	}

	filtered, err := utils.ApplyFieldMask(input, fields)
	if err != nil {
		t.Fatalf("ApplyFieldMask failed: %v", err)
	}

	resMap, ok := filtered.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any result")
	}

	if resMap["id"] != "usr-1" {
		t.Errorf("expected id usr-1")
	}
	if _, hasSecret := resMap["secret"]; hasSecret {
		t.Errorf("secret field should have been masked out")
	}

	nested, ok := resMap["user"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested user map")
	}
	if nested["email"] != "dev@scandrix.dev" {
		t.Errorf("expected email in filtered user")
	}
	if _, hasPass := nested["password"]; hasPass {
		t.Errorf("password should have been masked out")
	}
}

// LOGGER STREAM & VERBOSITY TESTS

func TestLogger_VerbosityAndQuietModes(t *testing.T) {
	var outBuf, errBuf bytes.Buffer
	utils.SetWriters(&outBuf, &errBuf)
	defer utils.SetWriters(nil, nil)

	// Test normal mode
	utils.SetOutputMode(false, false)
	utils.Info("Hello Standard Output")
	if !strings.Contains(outBuf.String(), "Hello Standard Output") {
		t.Errorf("expected standard output to receive Info message")
	}

	// Test quiet mode
	outBuf.Reset()
	utils.SetOutputMode(true, false)
	utils.Info("Should not appear")
	if outBuf.Len() > 0 {
		t.Errorf("expected quiet mode to suppress Info message")
	}

	// Errors should still be written in quiet mode
	errBuf.Reset()
	utils.Error("Critical Failure")
	if !strings.Contains(errBuf.String(), "Critical Failure") {
		t.Errorf("expected Error to bypass quiet mode")
	}

	// Restore normal mode
	utils.SetOutputMode(false, false)
}

// RECENT ACTIVITY LOGGING TESTS

func TestActivity_RecordRecentActivity(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("USERPROFILE", tempDir)

	err := utils.RecordRecentActivity("scandrix review --staged")
	if err != nil {
		t.Fatalf("RecordRecentActivity failed: %v", err)
	}

	err = utils.RecordRecentActivity("scandrix rules view")
	if err != nil {
		t.Fatalf("second RecordRecentActivity failed: %v", err)
	}
}
