// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Harness Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package persistence

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

func TestInMemoryConversationStore_Basic(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryConversationStore(3)

	threadID := "cmc-org-team-user-test1"

	// Initial load should be empty
	history, err := store.Load(ctx, threadID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(history) != 0 {
		t.Fatalf("expected 0 messages, got %d", len(history))
	}

	// Append turns
	turns1 := []contracts.ConversationMessage{
		{Role: contracts.RoleUser, Content: "Hello Drixy"},
		{Role: contracts.RoleAssistant, Content: "Hello, how can I help you?"},
	}
	meta := &contracts.ConversationAppendMeta{
		OrganizationID: "org-1",
		TeamID:         "team-1",
		RepositoryID:   "repo-1",
		CorrelationID:  "corr-1",
	}
	if err := store.Append(ctx, threadID, turns1, meta); err != nil {
		t.Fatalf("unexpected append error: %v", err)
	}

	loaded, err := store.Load(ctx, threadID)
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(loaded))
	}
	if loaded[0].Content != "Hello Drixy" {
		t.Errorf("unexpected content: %s", loaded[0].Content)
	}

	// Append more turns to test sliding window cap (capacity = 3)
	turns2 := []contracts.ConversationMessage{
		{Role: contracts.RoleUser, Content: "Explain the diff"},
		{Role: contracts.RoleAssistant, Content: "The diff touches 2 files"},
	}
	if err := store.Append(ctx, threadID, turns2, nil); err != nil {
		t.Fatalf("unexpected append error: %v", err)
	}

	capped, err := store.Load(ctx, threadID)
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}
	if len(capped) != 3 {
		t.Fatalf("expected sliding window cap 3, got %d", len(capped))
	}
	// Oldest ("Hello Drixy") should have rolled off
	if capped[0].Content != "Hello, how can I help you?" {
		t.Errorf("expected rolled off window, got oldest: %s", capped[0].Content)
	}
	if capped[2].Content != "The diff touches 2 files" {
		t.Errorf("expected newest message preserved, got: %s", capped[2].Content)
	}

	// Check metadata
	session, exists := store.GetSessionMetadata(threadID)
	if !exists {
		t.Fatalf("expected session metadata to exist")
	}
	if session.OrganizationID != "org-1" || session.TeamID != "team-1" {
		t.Errorf("expected org and team preserved, got org=%s team=%s", session.OrganizationID, session.TeamID)
	}
	if len(session.CorrelationIDHistory) != 1 || session.CorrelationIDHistory[0] != "corr-1" {
		t.Errorf("unexpected correlation history: %v", session.CorrelationIDHistory)
	}
}

func TestInMemoryConversationStore_Concurrency(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryConversationStore(50)

	var wg sync.WaitGroup
	workers := 10

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			th := fmt.Sprintf("thread-%d", workerID%3)
			for step := 0; step < 20; step++ {
				_ = store.Append(ctx, th, []contracts.ConversationMessage{
					{Role: contracts.RoleUser, Content: fmt.Sprintf("msg %d from %d", step, workerID)},
				}, nil)
				_, _ = store.Load(ctx, th)
			}
		}(i)
	}

	wg.Wait()
}
