package trace_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/cli/trace"
)

func TestTraceStoreAndHooks(t *testing.T) {
	store := trace.NewTraceStore()

	// 1. Record event
	evt := trace.TraceEvent{
		SessionID: "test-sess-001",
		Tool:      "cursor",
		Action:    "prompt",
		Prompt:    "Refactor auth handler",
		Timestamp: time.Now().UTC(),
	}

	if err := store.Record(evt); err != nil {
		t.Fatalf("failed recording trace event: %v", err)
	}

	// 2. Commit Trailer format
	trailer := trace.FormatCommitTrailer("sess-abc-123")
	if !strings.Contains(trailer, "ScanDrix-Trace: sess-abc-123") {
		t.Fatalf("invalid trailer format: %s", trailer)
	}

	// 3. Install Cursor Hook
	tempDir, err := os.MkdirTemp("", "scandrix-trace-hook-*")
	if err != nil {
		t.Fatalf("failed creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	path, err := trace.InstallSessionHook(tempDir, "cursor")
	if err != nil || path == "" {
		t.Fatalf("failed installing cursor hook: %v", err)
	}
}
