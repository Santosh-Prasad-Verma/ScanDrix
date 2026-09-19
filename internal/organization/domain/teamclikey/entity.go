// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package clikey

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// TeamCliKeyEntity represents an API key for CLI and CI/CD pipelines.
type TeamCliKeyEntity struct {
	UUID        uuid.UUID       `json:"uuid"`
	WorkspaceID uuid.UUID       `json:"workspace_id"`
	TeamID      *uuid.UUID      `json:"team_id,omitempty"`
	Name        string          `json:"name"`
	KeyHash     string          `json:"key_hash"`
	KeyPrefix   string          `json:"key_prefix"`
	Active      bool            `json:"active"`
	Config      json.RawMessage `json:"config"`
	LastUsedAt  *time.Time      `json:"last_used_at,omitempty"`
	ExpiresAt   *time.Time      `json:"expires_at,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// ValidateKeyResult contains metadata of an authenticated CLI key.
type ValidateKeyResult struct {
	KeyID       uuid.UUID `json:"key_id"`
	WorkspaceID uuid.UUID `json:"workspace_id"`
	TeamID      *uuid.UUID `json:"team_id,omitempty"`
	Name        string    `json:"name"`
	Config      json.RawMessage `json:"config"`
}
