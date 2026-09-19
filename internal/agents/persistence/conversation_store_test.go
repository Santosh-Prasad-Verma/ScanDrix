// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Persistence Tests
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package persistence

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

func TestAgentSessionStore_BasicLoadAppend(t *testing.T) {
	ctx := context.Background()
	store := NewAgentSessionStore(10)

	threadID := "thread-test-1"

	// Initial load should be empty
	history, err := store.Load(ctx, threadID)
	if err != nil {
		t.Fatalf("unexpected error on empty load: %v", err)
	}
	if len(history) != 0 {
		t.Fatalf("expected empty history, got %d", len(history))
	}

	// Append user and assistant turns
	turns := []contracts.ConversationMessage{
		{Role: contracts.RoleUser, Content: "Hello ScanDrix"},
		{Role: contracts.RoleAssistant, Content: "Hello! How can I assist you with code review?"},
	}

	meta := &contracts.ConversationAppendMeta{
		OrganizationID: "org-1",
		TeamID:         "team-1",
		RepositoryID:   "repo-1",
		Channel:        "web",
		CorrelationID:  "corr-1",
	}

	err = store.Append(ctx, threadID, turns, meta)
	if err != nil {
		t.Fatalf("unexpected error on append: %v", err)
	}

	// Load back
	loaded, err := store.Load(ctx, threadID)
	if err != nil {
		t.Fatalf("unexpected error on reload: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(loaded))
	}
	if loaded[0].Content != "Hello ScanDrix" || loaded[0].Role != contracts.RoleUser {
		t.Errorf("unexpected message 0: %+v", loaded[0])
	}
	if loaded[1].Content != "Hello! How can I assist you with code review?" || loaded[1].Role != contracts.RoleAssistant {
		t.Errorf("unexpected message 1: %+v", loaded[1])
	}

	// Inspect document
	doc, found := store.GetDocument(threadID)
	if !found || doc == nil {
		t.Fatalf("expected document to exist")
	}
	if doc.SessionData.OrganizationID != "org-1" || doc.SessionData.TeamID != "team-1" {
		t.Errorf("metadata mismatch: %+v", doc.SessionData)
	}
	if doc.SessionData.TenantID != DefaultTenant {
		t.Errorf("expected tenant %s, got %s", DefaultTenant, doc.SessionData.TenantID)
	}
	if strings.Contains(doc.SessionData.TenantID, "kodus") {
		t.Errorf("brand leak in tenant: %s", doc.SessionData.TenantID)
	}
}

func TestAgentSessionStore_SlidingWindowCap(t *testing.T) {
	ctx := context.Background()
	store := NewAgentSessionStore(3) // Cap at 3 messages

	threadID := "thread-sliding-cap"

	// Append 5 messages sequentially
	for i := 1; i <= 5; i++ {
		msg := contracts.ConversationMessage{
			Role:    contracts.RoleUser,
			Content: fmt.Sprintf("Message %d", i),
		}
		_ = store.Append(ctx, threadID, []contracts.ConversationMessage{msg}, nil)
	}

	loaded, err := store.Load(ctx, threadID)
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}

	if len(loaded) != 3 {
		t.Fatalf("expected 3 messages after sliding window, got %d", len(loaded))
	}

	// Should contain messages 3, 4, 5
	expectedContents := []string{"Message 3", "Message 4", "Message 5"}
	for i, exp := range expectedContents {
		if loaded[i].Content != exp {
			t.Errorf("expected loaded[%d] to be %q, got %q", i, exp, loaded[i].Content)
		}
	}
}

func TestAgentSessionStore_Concurrency(t *testing.T) {
	ctx := context.Background()
	store := NewAgentSessionStore(100)

	var wg sync.WaitGroup
	workers := 10
	messagesPerWorker := 10

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for m := 0; m < messagesPerWorker; m++ {
				threadID := fmt.Sprintf("thread-concurrent-%d", workerID%3)
				turn := contracts.ConversationMessage{
					Role:    contracts.RoleUser,
					Content: fmt.Sprintf("w%d-m%d", workerID, m),
				}
				_ = store.Append(ctx, threadID, []contracts.ConversationMessage{turn}, nil)
				_, _ = store.Load(ctx, threadID)
			}
		}(w)
	}

	wg.Wait()

	for i := 0; i < 3; i++ {
		threadID := fmt.Sprintf("thread-concurrent-%d", i)
		loaded, err := store.Load(ctx, threadID)
		if err != nil {
			t.Fatalf("error loading %s: %v", threadID, err)
		}
		if len(loaded) == 0 {
			t.Fatalf("expected messages in %s, got 0", threadID)
		}
	}
}
