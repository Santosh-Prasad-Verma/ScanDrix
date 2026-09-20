// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/cli/trace"
)

func TestSessionsAPI_SendEventAndFlush(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("SCANDRIX_TRACE_HOME", tempDir)
	defer os.Unsetenv("SCANDRIX_TRACE_HOME")

	var requestCount int32
	var lastReceivedBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		if r.URL.Path != SessionsEndpoint {
			http.NotFound(w, r)
			return
		}

		// Verify Authorization / Team-Key header
		teamKey := r.Header.Get("X-Team-Key")
		authHeader := r.Header.Get("Authorization")
		if teamKey == "" && authHeader == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		lastReceivedBody = body

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token-jwt", "")
	sessions := NewSessionsAPI(client)

	ctx := context.Background()
	event := SessionStartEvent{
		BaseSessionEvent: BaseSessionEvent{
			Type:      SessionEventTypeStart,
			SessionID: "sess-12345",
			Branch:    "feature/auth",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
		AgentType:  trace.AgentClaudeCode,
		GitRemote:  "https://github.com/scandrix/test.git",
		BaseCommit: "abc1234",
		CLIVersion: "1.0.0",
	}

	err := sessions.SendEvent(ctx, event, tempDir)
	if err != nil {
		t.Fatalf("expected SendEvent to succeed, got %v", err)
	}

	if atomic.LoadInt32(&requestCount) != 1 {
		t.Errorf("expected 1 request, got %d", requestCount)
	}

	if !strings.Contains(string(lastReceivedBody), "sess-12345") {
		t.Errorf("expected body to contain sess-12345, got %s", string(lastReceivedBody))
	}
}

func TestSessionsAPI_BufferOnFailureAndFlush(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("SCANDRIX_TRACE_HOME", tempDir)
	defer os.Unsetenv("SCANDRIX_TRACE_HOME")

	var serverFailing int32 = 1

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.LoadInt32(&serverFailing) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"server unavailable"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token-jwt", "")
	sessions := NewSessionsAPI(client)

	ctx := context.Background()
	event1 := TurnStartEvent{
		BaseSessionEvent: BaseSessionEvent{
			Type:      SessionEventTypeTurnStart,
			SessionID: "sess-fail-test",
			Branch:    "main",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
		TurnID:       "turn-1",
		Prompt:       "implement auth token check",
		CommitBefore: "sha-before",
	}

	// 1. SendEvent while server is failing -> should buffer to disk
	err := sessions.SendEvent(ctx, event1, tempDir)
	if err != nil {
		t.Fatalf("expected SendEvent to buffer without returning fatal error, got %v", err)
	}

	pendingPath := trace.PendingEventsPath(tempDir)
	if _, statErr := os.Stat(pendingPath); os.IsNotExist(statErr) {
		t.Fatalf("expected pending file to exist at %s", pendingPath)
	}

	lines, err := sessions.readPending(tempDir)
	if err != nil || len(lines) != 1 {
		t.Fatalf("expected 1 pending line, got %d (err: %v)", len(lines), err)
	}

	// 2. Server recovers, flush pending
	atomic.StoreInt32(&serverFailing, 0)
	flushErr := sessions.FlushPending(ctx, tempDir)
	if flushErr != nil {
		t.Fatalf("expected FlushPending to succeed, got %v", flushErr)
	}

	// Verify pending file is deleted after flush
	if _, statErr := os.Stat(pendingPath); !os.IsNotExist(statErr) {
		t.Errorf("expected pending file to be removed after flush, but it still exists")
	}
}

func TestSessionsAPI_RedactsSecretsBeforeSend(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("SCANDRIX_TRACE_HOME", tempDir)
	defer os.Unsetenv("SCANDRIX_TRACE_HOME")

	var lastReceivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		lastReceivedBody = body
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token-jwt", "")
	sessions := NewSessionsAPI(client)

	ctx := context.Background()
	secretKey := "sk-ant-api03-abcdef1234567890"
	event := TurnStartEvent{
		BaseSessionEvent: BaseSessionEvent{
			Type:      SessionEventTypeTurnStart,
			SessionID: "sess-redact",
			Branch:    "main",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
		TurnID: "turn-redact",
		Prompt: "Here is my key: " + secretKey,
	}

	err := sessions.SendEvent(ctx, event, tempDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(string(lastReceivedBody), secretKey) {
		t.Errorf("SECURITY LEAK: sent body contains plaintext secret key: %s", string(lastReceivedBody))
	}
	if !strings.Contains(string(lastReceivedBody), trace.RedactionPlaceholder) {
		t.Errorf("expected redacted placeholder in body, got: %s", string(lastReceivedBody))
	}
}
