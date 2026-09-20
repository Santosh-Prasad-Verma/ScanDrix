// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Harness Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package persistence

import (
	"context"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

const defaultMaxMessages = 100

// StoredSession holds conversational history and tenant metadata in memory.
type StoredSession struct {
	ThreadID             string
	TenantID             string
	OrganizationID       string
	TeamID               string
	RepositoryID         string
	Channel              string
	Messages             []contracts.ConversationMessage
	LastActivity         time.Time
	CorrelationIDHistory []string
}

// InMemoryConversationStore provides a thread-safe in-memory implementation
// of contracts.ConversationStore. Ideal for unit tests, CLI runs, and local dev.
type InMemoryConversationStore struct {
	mu          sync.RWMutex
	sessions    map[string]*StoredSession
	maxMessages int
}

// NewInMemoryConversationStore creates an in-memory conversation store with
// message capacity driven by SCANDRIX_AGENT_MAX_HISTORY (defaults to 100).
func NewInMemoryConversationStore(maxMsgs ...int) *InMemoryConversationStore {
	capacity := defaultMaxMessages
	if envVal := os.Getenv("SCANDRIX_AGENT_MAX_HISTORY"); envVal != "" {
		if parsed, err := strconv.Atoi(envVal); err == nil && parsed > 0 {
			capacity = parsed
		}
	}
	if len(maxMsgs) > 0 && maxMsgs[0] > 0 {
		capacity = maxMsgs[0]
	}

	return &InMemoryConversationStore{
		sessions:    make(map[string]*StoredSession),
		maxMessages: capacity,
	}
}

// Load retrieves messages for a threadID, oldest first.
func (s *InMemoryConversationStore) Load(ctx context.Context, threadID string) ([]contracts.ConversationMessage, error) {
	if threadID == "" {
		return []contracts.ConversationMessage{}, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	session, exists := s.sessions[threadID]
	if !exists || len(session.Messages) == 0 {
		return []contracts.ConversationMessage{}, nil
	}

	// Return a copy to avoid race conditions
	copied := make([]contracts.ConversationMessage, len(session.Messages))
	copy(copied, session.Messages)
	return copied, nil
}

// Append adds new turns to the thread, trimming to maxMessages sliding window.
func (s *InMemoryConversationStore) Append(
	ctx context.Context,
	threadID string,
	turns []contracts.ConversationMessage,
	meta *contracts.ConversationAppendMeta,
) error {
	if threadID == "" || len(turns) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	session, exists := s.sessions[threadID]
	if !exists {
		session = &StoredSession{
			ThreadID:     threadID,
			LastActivity: time.Now(),
		}
		s.sessions[threadID] = session
	}

	if meta != nil {
		if meta.TenantID != "" {
			session.TenantID = meta.TenantID
		}
		if meta.OrganizationID != "" {
			session.OrganizationID = meta.OrganizationID
		}
		if meta.TeamID != "" {
			session.TeamID = meta.TeamID
		}
		if meta.RepositoryID != "" {
			session.RepositoryID = meta.RepositoryID
		}
		if meta.Channel != "" {
			session.Channel = meta.Channel
		}
		if meta.CorrelationID != "" {
			session.CorrelationIDHistory = append(session.CorrelationIDHistory, meta.CorrelationID)
		}
	}

	session.Messages = append(session.Messages, turns...)
	session.LastActivity = time.Now()

	// Apply sliding window cap
	if len(session.Messages) > s.maxMessages {
		session.Messages = session.Messages[len(session.Messages)-s.maxMessages:]
	}

	return nil
}

// GetSessionMetadata returns the stored metadata for a thread (useful for testing/inspections).
func (s *InMemoryConversationStore) GetSessionMetadata(threadID string) (*StoredSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, exists := s.sessions[threadID]
	if !exists {
		return nil, false
	}
	cp := *session
	return &cp, true
}
