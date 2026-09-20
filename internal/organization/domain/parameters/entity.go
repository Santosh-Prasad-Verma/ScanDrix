// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package paramdomain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ParametersEntity represents team- or workspace-scoped parameters.
type ParametersEntity struct {
	UUID        uuid.UUID       `json:"uuid"`
	WorkspaceID uuid.UUID       `json:"workspace_id"`
	TeamID      *uuid.UUID      `json:"team_id,omitempty"`
	ConfigKey   ParameterKey    `json:"config_key"`
	ConfigValue json.RawMessage `json:"config_value"`
	Active      bool            `json:"active"`
	Description string          `json:"description"`
	Version     int             `json:"version"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// NewParametersEntity constructs a new ParametersEntity.
func NewParametersEntity(wsID uuid.UUID, teamID *uuid.UUID, key ParameterKey, val any, desc string) (*ParametersEntity, error) {
	bytes, err := json.Marshal(val)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	return &ParametersEntity{
		UUID:        uuid.New(),
		WorkspaceID: wsID,
		TeamID:      teamID,
		ConfigKey:   key,
		ConfigValue: bytes,
		Active:      true,
		Description: desc,
		Version:     1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}
