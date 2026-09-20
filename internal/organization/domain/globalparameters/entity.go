// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package globalparams

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// GlobalParametersEntity represents platform-wide configuration settings.
type GlobalParametersEntity struct {
	UUID        uuid.UUID       `json:"uuid"`
	ConfigKey   string          `json:"config_key"`
	ConfigValue json.RawMessage `json:"config_value"`
	Description string          `json:"description"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// NewGlobalParametersEntity creates a new GlobalParametersEntity.
func NewGlobalParametersEntity(key string, val any, desc string) (*GlobalParametersEntity, error) {
	bytes, err := json.Marshal(val)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	return &GlobalParametersEntity{
		UUID:        uuid.New(),
		ConfigKey:   key,
		ConfigValue: bytes,
		Description: desc,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}
