// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package teamdomain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// TeamEntity represents an engineering squad or team within a workspace.
type TeamEntity struct {
	UUID           uuid.UUID       `json:"uuid"`
	WorkspaceID    uuid.UUID       `json:"workspace_id"`
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	RepositoryIDs  []uuid.UUID     `json:"repository_ids"`
	AutoAssignMode string          `json:"auto_assign_mode"` // round_robin, least_busy, none
	Status         bool            `json:"status"`
	CLIConfig      json.RawMessage `json:"cli_config,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// TeamWithIntegrations decorates a team with connected tools.
type TeamWithIntegrations struct {
	Team               *TeamEntity `json:"team"`
	MemberCount        int         `json:"member_count"`
	RepositoryCount    int         `json:"repository_count"`
	GitIntegrations    []string    `json:"git_integrations"`
	PMIntegrations     []string    `json:"pm_integrations"`
	ChatIntegrations   []string    `json:"chat_integrations"`
}

// NewTeamEntity creates a new TeamEntity.
func NewTeamEntity(wsID uuid.UUID, name, description, autoAssignMode string) *TeamEntity {
	now := time.Now().UTC()
	if autoAssignMode == "" {
		autoAssignMode = "round_robin"
	}
	return &TeamEntity{
		UUID:           uuid.New(),
		WorkspaceID:    wsID,
		Name:           name,
		Description:    description,
		RepositoryIDs:  make([]uuid.UUID, 0),
		AutoAssignMode: autoAssignMode,
		Status:         true,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}
