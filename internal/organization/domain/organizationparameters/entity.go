// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package orgparams

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// OrganizationParametersEntity represents a persisted configuration setting.
type OrganizationParametersEntity struct {
	UUID         uuid.UUID       `json:"uuid"`
	WorkspaceID  uuid.UUID       `json:"workspace_id"`
	ConfigKey    ParameterKey    `json:"config_key"`
	ConfigValue  json.RawMessage `json:"config_value"`
	Description  string          `json:"description"`
	IsActive     bool            `json:"is_active"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// NewOrganizationParametersEntity constructs a parameter entity.
func NewOrganizationParametersEntity(wsID uuid.UUID, key ParameterKey, val any, desc string) (*OrganizationParametersEntity, error) {
	bytes, err := json.Marshal(val)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	return &OrganizationParametersEntity{
		UUID:        uuid.New(),
		WorkspaceID: wsID,
		ConfigKey:   key,
		ConfigValue: bytes,
		Description: desc,
		IsActive:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}
