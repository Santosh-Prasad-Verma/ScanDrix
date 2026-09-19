// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Persistence
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package persistence

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/agentharness/contracts"
)

const (
	// DefaultTenant is the default tenant identifier for agent conversations.
	DefaultTenant = "scandrix-agent-conversation"

	// CollectionName is the collection/table name for agent sessions.
	CollectionName = "scandrix-agent-sessions"

	// MaxMessages is the maximum number of messages preserved per thread (sliding window).
	MaxMessages = 100
)

// Re-export or alias core harness contracts for clean boundary.
type (
	ConversationStore      = contracts.ConversationStore
	ConversationMessage    = contracts.ConversationMessage
	ConversationAppendMeta = contracts.ConversationAppendMeta
)

// PersistedMessage represents a single message in the agent session document.
type PersistedMessage struct {
	Role    string `json:"role" bson:"role"`
	Content string `json:"content" bson:"content"`
	Ts      int64  `json:"ts" bson:"ts"`
}

// PersistedSessionData represents the internal sessionData payload of a session document.
type PersistedSessionData struct {
	SessionID             string             `json:"sessionId" bson:"sessionId"`
	ThreadID              string             `json:"threadId" bson:"threadId"`
	TenantID              string             `json:"tenantId" bson:"tenantId"`
	Status                string             `json:"status" bson:"status"`
	Runtime               RuntimeData        `json:"runtime" bson:"runtime"`
	OrganizationID        string             `json:"organizationId,omitempty" bson:"organizationId,omitempty"`
	TeamID                string             `json:"teamId,omitempty" bson:"teamId,omitempty"`
	RepositoryID          string             `json:"repositoryId,omitempty" bson:"repositoryId,omitempty"`
	Channel               string             `json:"channel,omitempty" bson:"channel,omitempty"`
	CreatedAt             time.Time          `json:"createdAt" bson:"createdAt"`
	LastActivityAt        time.Time          `json:"lastActivityAt" bson:"lastActivityAt"`
	CreatedAtTimestamp    int64              `json:"createdAtTimestamp" bson:"createdAtTimestamp"`
	LastActivityTimestamp int64              `json:"lastActivityTimestamp" bson:"lastActivityTimestamp"`
	LastCorrelationID     string             `json:"lastCorrelationId,omitempty" bson:"lastCorrelationId,omitempty"`
	CorrelationIDHistory  []string           `json:"correlationIdHistory" bson:"correlationIdHistory"`
}

// RuntimeData wraps the messages list.
type RuntimeData struct {
	Messages []PersistedMessage `json:"messages" bson:"messages"`
}

// AgentSessionDocument mirrors the legacy and modern scandrix-agent-sessions document schema.
type AgentSessionDocument struct {
	ID          string               `json:"id" bson:"id"`
	ThreadID    string               `json:"threadId" bson:"threadId"`
	Timestamp   int64                `json:"timestamp" bson:"timestamp"`
	SessionData PersistedSessionData `json:"sessionData" bson:"sessionData"`
}

// AgentSessionStore provides an in-memory/thread-safe implementation of ConversationStore
// that produces documents conforming to the scandrix-agent-sessions schema with sliding-window caps.
type AgentSessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*AgentSessionDocument
	maxTurns int
}

// NewAgentSessionStore creates a new session store with optional max turn override.
func NewAgentSessionStore(maxTurns ...int) *AgentSessionStore {
	limit := MaxMessages
	if len(maxTurns) > 0 && maxTurns[0] > 0 {
		limit = maxTurns[0]
	}
	return &AgentSessionStore{
		sessions: make(map[string]*AgentSessionDocument),
		maxTurns: limit,
	}
}

// Load retrieves conversation history for a thread.
func (s *AgentSessionStore) Load(ctx context.Context, threadID string) ([]ConversationMessage, error) {
	if threadID == "" {
		return nil, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	doc, exists := s.sessions[threadID]
	if !exists || doc == nil {
		return nil, nil
	}

	messages := doc.SessionData.Runtime.Messages
	result := make([]ConversationMessage, 0, len(messages))
	for _, m := range messages {
		result = append(result, ConversationMessage{
			Role:    contracts.AgentRole(m.Role),
			Content: m.Content,
		})
	}

	return result, nil
}

// Append appends new turns to the thread with sliding window truncation.
// Best-effort: failures are logged and do not break the caller.
func (s *AgentSessionStore) Append(
	ctx context.Context,
	threadID string,
	turns []ConversationMessage,
	meta *ConversationAppendMeta,
) error {
	if threadID == "" || len(turns) == 0 {
		return nil
	}

	now := time.Now()
	nowUnix := now.UnixMilli()

	s.mu.Lock()
	defer s.mu.Unlock()

	doc, exists := s.sessions[threadID]
	if !exists {
		sessionID := uuid.NewString()
		tenantID := DefaultTenant
		if meta != nil && meta.TenantID != "" {
			tenantID = meta.TenantID
		}

		doc = &AgentSessionDocument{
			ID:        sessionID,
			ThreadID:  threadID,
			Timestamp: nowUnix,
			SessionData: PersistedSessionData{
				SessionID:             sessionID,
				ThreadID:              threadID,
				TenantID:              tenantID,
				Status:                "active",
				Runtime:               RuntimeData{Messages: []PersistedMessage{}},
				CreatedAt:             now,
				CreatedAtTimestamp:    nowUnix,
				LastActivityAt:        now,
				LastActivityTimestamp: nowUnix,
				CorrelationIDHistory:  []string{},
			},
		}
		s.sessions[threadID] = doc
	}

	// Update metadata
	doc.Timestamp = nowUnix
	doc.SessionData.Status = "active"
	doc.SessionData.LastActivityAt = now
	doc.SessionData.LastActivityTimestamp = nowUnix

	if meta != nil {
		if meta.OrganizationID != "" {
			doc.SessionData.OrganizationID = meta.OrganizationID
		}
		if meta.TeamID != "" {
			doc.SessionData.TeamID = meta.TeamID
		}
		if meta.RepositoryID != "" {
			doc.SessionData.RepositoryID = meta.RepositoryID
		}
		if meta.Channel != "" {
			doc.SessionData.Channel = meta.Channel
		}
		if meta.CorrelationID != "" {
			doc.SessionData.LastCorrelationID = meta.CorrelationID
			doc.SessionData.CorrelationIDHistory = append(doc.SessionData.CorrelationIDHistory, meta.CorrelationID)
		}
	}

	// Append new messages
	for _, t := range turns {
		doc.SessionData.Runtime.Messages = append(doc.SessionData.Runtime.Messages, PersistedMessage{
			Role:    string(t.Role),
			Content: t.Content,
			Ts:      nowUnix,
		})
	}

	// Enforce max turns cap (sliding window)
	if len(doc.SessionData.Runtime.Messages) > s.maxTurns {
		start := len(doc.SessionData.Runtime.Messages) - s.maxTurns
		doc.SessionData.Runtime.Messages = doc.SessionData.Runtime.Messages[start:]
	}

	return nil
}

// GetDocument returns the underlying document for inspection (e.g. for testing or BI reporting).
func (s *AgentSessionStore) GetDocument(threadID string) (*AgentSessionDocument, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	doc, exists := s.sessions[threadID]
	return doc, exists
}
