// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package testutil

import (
	"context"
	"testing"

	"github.com/scandrix/backend/internal/cli/services/api"
	"github.com/scandrix/backend/internal/cli/trace"
	"github.com/scandrix/backend/pkg/models"
)

func TestMockAPIServer_LifecycleIntegration(t *testing.T) {
	server := NewMockAPIServer()
	defer server.Close()

	client := api.NewClient(server.URL(), "test_token_123", "")
	sessionsAPI := client.Sessions()

	ctx := context.Background()
	tempDir := t.TempDir()

	event := api.SessionStartEvent{
		BaseSessionEvent: api.BaseSessionEvent{
			Type:      api.SessionEventTypeStart,
			SessionID: "sess_mock_test_42",
			Branch:    "feature/auth-hardening",
			Timestamp: "2026-09-12T10:00:00Z",
		},
		AgentType:  trace.AgentClaudeCode,
		GitRemote:  "https://github.com/scandrix/backend.git",
		BaseCommit: "a1b2c3d4e5f6",
		CLIVersion: "1.0.0",
	}

	if err := sessionsAPI.SendEvent(ctx, event, tempDir); err != nil {
		t.Fatalf("SendEvent failed: %v", err)
	}

	events := server.GetEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event posted to server, got %d", len(events))
	}

	evtMap, ok := events[0].(map[string]any)
	if !ok {
		t.Fatalf("expected event map, got %T", events[0])
	}

	if evtMap["sessionId"] != "sess_mock_test_42" {
		t.Errorf("expected sessionId sess_mock_test_42, got %v", evtMap["sessionId"])
	}
	if evtMap["type"] != "session_start" {
		t.Errorf("expected type session_start, got %v", evtMap["type"])
	}
}

func TestMockAPIServer_ReviewIntegration(t *testing.T) {
	server := NewMockAPIServer()
	defer server.Close()

	// Seed mock findings
	server.SetFindings([]models.CodeFinding{
		{
			FilePath:    "pkg/auth/token.go",
			StartLine:   42,
			EndLine:     45,
			Severity:    models.SeverityHigh,
			Category:    "security",
			Title:       "Hardcoded Credentials Risk",
			Description: "Secrets should be fetched from environment variables",
		},
	})

	client := api.NewClient(server.URL(), "test_token_123", "")
	ctx := context.Background()

	resp, err := client.SubmitReview(ctx, api.ReviewRequest{
		Diff:       "diff --git a/pkg/auth/token.go b/pkg/auth/token.go\n...",
		Branch:     "feature/security-hardening",
		Repository: "scandrix/backend",
	})
	if err != nil {
		t.Fatalf("SubmitReview failed: %v", err)
	}

	if resp == nil {
		t.Fatal("expected non-nil review response")
	}
	if resp.Status != "passed" {
		t.Errorf("expected status 'passed', got %s", resp.Status)
	}
	if len(resp.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(resp.Findings))
	}
	if resp.Findings[0].Category != "security" {
		t.Errorf("expected security category, got %s", resp.Findings[0].Category)
	}
}

func TestMockAPIServer_AuthIntegration(t *testing.T) {
	server := NewMockAPIServer()
	defer server.Close()

	client := api.NewClient(server.URL(), "test_token_123", "")
	ctx := context.Background()

	// 1. Whoami
	user, err := client.Whoami(ctx)
	if err != nil {
		t.Fatalf("Whoami failed: %v", err)
	}
	if user.Email != "engineer@scandrix.dev" {
		t.Errorf("expected user email engineer@scandrix.dev, got %s", user.Email)
	}

	// 2. Team Key Verification
	valid, teamName, err := client.VerifyTeamKey(ctx, "scandrix_team_testkey")
	if err != nil {
		t.Fatalf("VerifyTeamKey failed: %v", err)
	}
	if !valid {
		t.Error("expected team key to be valid")
	}
	if teamName != "ScanDrix Core Team" {
		t.Errorf("expected team name 'ScanDrix Core Team', got %s", teamName)
	}

	// 3. Device Flow Initiate & Poll
	initResp, err := client.StartDeviceAuth(ctx)
	if err != nil {
		t.Fatalf("StartDeviceAuth failed: %v", err)
	}
	if initResp.DeviceCode != "mock_device_code_123" {
		t.Errorf("expected device code mock_device_code_123, got %s", initResp.DeviceCode)
	}

	pollResp, err := client.PollDeviceToken(ctx, initResp.DeviceCode)
	if err != nil {
		t.Fatalf("PollDeviceToken failed: %v", err)
	}
	if pollResp.AccessToken != "mock_device_access_token_789" {
		t.Errorf("expected mock access token, got %s", pollResp.AccessToken)
	}
}

