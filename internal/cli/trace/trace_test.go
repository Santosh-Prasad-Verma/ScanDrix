// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package trace

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRedaction(t *testing.T) {
	cases := []struct {
		input    string
		contains string
		redacted string
	}{
		{
			input:    "My API key is sk-ant-api03-abcdef1234567890 and it is confidential.",
			contains: "sk-ant-api03",
			redacted: "My API key is [REDACTED] and it is confidential.",
		},
		{
			input:    "Authorization: Bearer mySecretToken1234567890_value",
			contains: "mySecretToken1234567890",
			redacted: "Authorization: Bearer [REDACTED]",
		},
		{
			input:    "api_key: 'superSecretKeyValue123'",
			contains: "superSecretKeyValue123",
			redacted: "api_key: '[REDACTED]'",
		},
		{
			input:    "export GITHUB_TOKEN=ghp_1234567890abcdef123456",
			contains: "ghp_",
			redacted: "export GITHUB_TOKEN=[REDACTED]",
		},
	}

	for _, c := range cases {
		if !ContainsSecret(c.input) {
			t.Errorf("expected ContainsSecret(%q) to be true", c.input)
		}
		got := Redact(c.input)
		if strings.Contains(got, c.contains) {
			t.Errorf("Redact(%q) failed to scrub %q; got %q", c.input, c.contains, got)
		}
	}
}

func TestTranscriptParser(t *testing.T) {
	tmpDir := t.TempDir()
	transcriptPath := filepath.Join(tmpDir, "transcript.jsonl")

	lines := []string{
		`{"role":"user","content":"Implement JWT authentication"}`,
		`{"role":"assistant","content":[{"type":"tool_use","name":"Write","input":{"path":"internal/auth/jwt.go","content":"package auth"}}]}`,
		`{"role":"assistant","content":[{"type":"text","text":"I implemented JWT auth."}],"usage":{"input_tokens":120,"output_tokens":45}}`,
	}

	if err := os.WriteFile(transcriptPath, []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	res, err := ParseTranscript(transcriptPath, 0)
	if err != nil {
		t.Fatalf("ParseTranscript failed: %v", err)
	}

	if len(res.Prompts) != 1 || res.Prompts[0] != "Implement JWT authentication" {
		t.Errorf("unexpected prompts: %v", res.Prompts)
	}
	if len(res.ModifiedFiles) != 1 || res.ModifiedFiles[0] != "internal/auth/jwt.go" {
		t.Errorf("unexpected modified files: %v", res.ModifiedFiles)
	}
	if res.TokenUsage.InputTokens != 120 || res.TokenUsage.OutputTokens != 45 {
		t.Errorf("unexpected token usage: %+v", res.TokenUsage)
	}
	if res.Summary != "I implemented JWT auth." {
		t.Errorf("unexpected summary: %s", res.Summary)
	}
}

func TestOverrides(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("SCANDRIX_TRACE_HOME", tmpDir)

	if err := AddPin(tmpDir, "dec_123"); err != nil {
		t.Fatal(err)
	}
	if err := AddForget(tmpDir, "dec_456"); err != nil {
		t.Fatal(err)
	}

	ov, err := ReadOverrides(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ov.Pins) != 1 || ov.Pins[0] != "dec_123" {
		t.Errorf("unexpected pins: %v", ov.Pins)
	}
	if len(ov.Forgets) != 1 || ov.Forgets[0] != "dec_456" {
		t.Errorf("unexpected forgets: %v", ov.Forgets)
	}

	decisions := []Decision{
		{ID: "dec_123", Decision: "Use Postgres"},
		{ID: "dec_456", Decision: "Old choice"},
		{ID: "dec_789", Decision: "Keep this"},
	}

	applied := ApplyOverrides(decisions, ov)
	if len(applied) != 2 {
		t.Fatalf("expected 2 decisions after applying overrides, got %d", len(applied))
	}
	if !applied[0].Pinned {
		t.Errorf("expected dec_123 to be pinned")
	}
	if applied[1].ID != "dec_789" {
		t.Errorf("expected dec_789, got %s", applied[1].ID)
	}
}

func TestLifecycleCoordinator(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("SCANDRIX_TRACE_HOME", tmpDir)

	lc := NewLifecycleCoordinator(nil)
	sessID := "test_sess_001"

	// 1. SessionStart
	err := lc.Dispatch(context.Background(), tmpDir, AgentClaudeCode, &LifecycleEvent{
		Type:      "SessionStart",
		SessionID: sessID,
	})
	if err != nil {
		t.Fatal(err)
	}

	// 2. TurnStart
	err = lc.Dispatch(context.Background(), tmpDir, AgentClaudeCode, &LifecycleEvent{
		Type:      "TurnStart",
		SessionID: sessID,
		Prompt:    "Add user model",
	})
	if err != nil {
		t.Fatal(err)
	}

	// 3. TurnEnd
	err = lc.Dispatch(context.Background(), tmpDir, AgentClaudeCode, &LifecycleEvent{
		Type:      "TurnEnd",
		SessionID: sessID,
	})
	if err != nil {
		t.Fatal(err)
	}

	// 4. SessionEnd
	err = lc.Dispatch(context.Background(), tmpDir, AgentClaudeCode, &LifecycleEvent{
		Type:      "SessionEnd",
		SessionID: sessID,
	})
	if err != nil {
		t.Fatal(err)
	}

	sess, err := ReadSession(tmpDir, sessID)
	if err != nil {
		t.Fatalf("ReadSession failed: %v", err)
	}
	if sess.SessionID != sessID {
		t.Errorf("expected session ID %s, got %s", sessID, sess.SessionID)
	}
	if len(sess.Turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(sess.Turns))
	}
	if sess.Turns[0].Prompt != "Add user model" {
		t.Errorf("unexpected prompt: %s", sess.Turns[0].Prompt)
	}
}

func TestIncidents(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("SCANDRIX_TRACE_HOME", tmpDir)

	inc1 := TraceIncident{
		Kind:    "PUSH_COLLISION",
		Message: "Failed pushing trace branch to remote",
		Context: "scandrix/trace/v1 rejected non-fast-forward",
	}
	inc2 := TraceIncident{
		Kind:    "DISTILL_TIMEOUT",
		Message: "Distillation timed out after 30s",
	}

	if err := RecordIncident(tmpDir, inc1); err != nil {
		t.Fatalf("RecordIncident failed: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := RecordIncident(tmpDir, inc2); err != nil {
		t.Fatalf("RecordIncident failed: %v", err)
	}

	incidents, err := ReadIncidents(tmpDir, 10)
	if err != nil {
		t.Fatalf("ReadIncidents failed: %v", err)
	}
	if len(incidents) != 2 {
		t.Fatalf("expected 2 incidents, got %d", len(incidents))
	}
	// Verify most recent first
	if incidents[0].Kind != "DISTILL_TIMEOUT" {
		t.Errorf("expected first incident to be DISTILL_TIMEOUT, got %s", incidents[0].Kind)
	}
	if incidents[1].Kind != "PUSH_COLLISION" {
		t.Errorf("expected second incident to be PUSH_COLLISION, got %s", incidents[1].Kind)
	}

	// Test limit
	limited, err := ReadIncidents(tmpDir, 1)
	if err != nil || len(limited) != 1 {
		t.Fatalf("expected 1 limited incident, got %d (err: %v)", len(limited), err)
	}
	if limited[0].Kind != "DISTILL_TIMEOUT" {
		t.Errorf("expected DISTILL_TIMEOUT, got %s", limited[0].Kind)
	}

	// Test clear
	if err := ClearIncidents(tmpDir); err != nil {
		t.Fatalf("ClearIncidents failed: %v", err)
	}
	afterClear, err := ReadIncidents(tmpDir, 10)
	if err != nil || len(afterClear) != 0 {
		t.Fatalf("expected 0 incidents after clear, got %d", len(afterClear))
	}
}

func TestTraceUIServerDNSRebindingAndRoutes(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("SCANDRIX_TRACE_HOME", tmpDir)

	srv, err := StartTraceUIServer(tmpDir, 0, "127.0.0.1")
	if err != nil {
		t.Fatalf("StartTraceUIServer failed: %v", err)
	}
	defer srv.Shutdown(context.Background())

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", srv.Port)

	// 1. Valid request to / with 127.0.0.1 host
	req, _ := http.NewRequest("GET", baseURL+"/", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET / failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for valid host, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "ScanDrix Trace") {
		t.Errorf("HTML missing brand title; body preview: %s", string(body[:min(100, len(body))]))
	}

	// 2. DNS Rebinding Attack Protection: Malicious Host Header
	reqRebind, _ := http.NewRequest("GET", baseURL+"/", nil)
	reqRebind.Host = "attacker-domain.com"
	respRebind, err := http.DefaultClient.Do(reqRebind)
	if err != nil {
		t.Fatalf("Rebinding request failed: %v", err)
	}
	defer respRebind.Body.Close()
	if respRebind.StatusCode != http.StatusMisdirectedRequest {
		t.Errorf("expected 421 Misdirected Request for attacker host header, got %d", respRebind.StatusCode)
	}

	// 3. API endpoint /api/sessions
	reqAPI, _ := http.NewRequest("GET", baseURL+"/api/sessions", nil)
	respAPI, err := http.DefaultClient.Do(reqAPI)
	if err != nil {
		t.Fatalf("GET /api/sessions failed: %v", err)
	}
	defer respAPI.Body.Close()
	if respAPI.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for /api/sessions, got %d", respAPI.StatusCode)
	}
	apiBody, _ := io.ReadAll(respAPI.Body)
	if !strings.Contains(string(apiBody), `"sessions"`) {
		t.Errorf("API response missing sessions field: %s", string(apiBody))
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestSharedCorrections(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("SCANDRIX_TRACE_HOME", tmpDir)

	// Initialize git repo in tmpDir
	runGit(context.Background(), tmpDir, nil, "init")
	runGit(context.Background(), tmpDir, nil, "config", "user.name", "TestUser")
	runGit(context.Background(), tmpDir, nil, "config", "user.email", "test@scandrix.local")

	rec := &TraceBranchRecord{
		Version:   1,
		Branch:    "feature-auth",
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		Decisions: []Decision{
			{
				ID:       "dec-jwt-auth",
				Type:     DecisionTypeArchitectural,
				Decision: "Use RS256 for JWT signing",
				Scope:    []string{"internal/auth"},
			},
		},
	}

	// Save record to local decision path
	localPath := LocalBranchDecisionPath(tmpDir, "feature-auth")
	_ = os.MkdirAll(filepath.Dir(localPath), 0755)
	data, _ := json.MarshalIndent(rec, "", "  ")
	_ = os.WriteFile(localPath, data, 0600)

	ctx := context.Background()

	// 1. Pin
	res, err := UpdateSharedDecisionCorrection(ctx, tmpDir, "dec-jwt-auth", "pin", "")
	if err != nil {
		t.Fatalf("UpdateSharedDecisionCorrection(pin) failed: %v", err)
	}
	if !res.Found {
		t.Errorf("expected decision to be found")
	}

	updatedRec, err := ReadBranchRecord(ctx, tmpDir, "feature-auth", "")
	if err != nil {
		t.Fatalf("ReadBranchRecord failed: %v", err)
	}
	if len(updatedRec.Decisions) != 1 || !updatedRec.Decisions[0].Pinned {
		t.Errorf("expected decision to be pinned, got %+v", updatedRec.Decisions)
	}
	if updatedRec.Corrections == nil || len(updatedRec.Corrections.Pins) != 1 || updatedRec.Corrections.Pins[0] != "dec-jwt-auth" {
		t.Errorf("expected Corrections.Pins to contain dec-jwt-auth, got %+v", updatedRec.Corrections)
	}

	// 2. Unpin
	res, err = UpdateSharedDecisionCorrection(ctx, tmpDir, "dec-jwt-auth", "unpin", "")
	if err != nil {
		t.Fatalf("UpdateSharedDecisionCorrection(unpin) failed: %v", err)
	}
	if !res.Found {
		t.Errorf("expected decision to be found")
	}

	updatedRec, err = ReadBranchRecord(ctx, tmpDir, "feature-auth", "")
	if err != nil {
		t.Fatalf("ReadBranchRecord failed: %v", err)
	}
	if len(updatedRec.Decisions) != 1 || updatedRec.Decisions[0].Pinned {
		t.Errorf("expected decision to be unpinned, got %+v", updatedRec.Decisions)
	}

	// 3. Forget
	res, err = UpdateSharedDecisionCorrection(ctx, tmpDir, "dec-jwt-auth", "forget", "")
	if err != nil {
		t.Fatalf("UpdateSharedDecisionCorrection(forget) failed: %v", err)
	}
	if !res.Found {
		t.Errorf("expected decision to be found")
	}

	updatedRec, err = ReadBranchRecord(ctx, tmpDir, "feature-auth", "")
	if err != nil {
		t.Fatalf("ReadBranchRecord failed: %v", err)
	}
	if len(updatedRec.Decisions) != 0 {
		t.Errorf("expected decision to be removed from active list, got %d items", len(updatedRec.Decisions))
	}
	if updatedRec.Corrections == nil || len(updatedRec.Corrections.Forgets) != 1 || updatedRec.Corrections.Forgets[0] != "dec-jwt-auth" {
		t.Errorf("expected Corrections.Forgets to contain dec-jwt-auth, got %+v", updatedRec.Corrections)
	}
}
