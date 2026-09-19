// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Harness Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package contracts

import (
	"context"
)

// ConversationMessage represents a single turn in a persisted thread.
type ConversationMessage struct {
	Role    AgentRole `json:"role"`
	Content string    `json:"content"`
}

// ConversationAppendMeta attaches tenancy and correlation metadata to a thread turn.
type ConversationAppendMeta struct {
	TenantID       string `json:"tenant_id,omitempty"`
	OrganizationID string `json:"organization_id,omitempty"`
	TeamID         string `json:"team_id,omitempty"`
	RepositoryID   string `json:"repository_id,omitempty"`
	Channel        string `json:"channel,omitempty"`
	CorrelationID  string `json:"correlation_id,omitempty"`
}

// ConversationStore persists conversation turns keyed by threadID.
type ConversationStore interface {
	// Load retrieves prior turns for a given thread, oldest first.
	Load(ctx context.Context, threadID string) ([]ConversationMessage, error)

	// Append stores new turns for the thread alongside metadata.
	Append(ctx context.Context, threadID string, turns []ConversationMessage, meta *ConversationAppendMeta) error
}
