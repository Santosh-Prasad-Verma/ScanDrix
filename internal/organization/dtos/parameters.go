// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package dtos

import (
	"github.com/google/uuid"
)

// CreateOrUpdateParameterDTO holds payload for generic parameter updates.
type CreateOrUpdateParameterDTO struct {
	Key         string     `json:"key"`
	Value       any        `json:"value"`
	TeamID      *uuid.UUID `json:"teamId,omitempty"`
	Description string     `json:"description,omitempty"`
}

// FindByKeyQueryDTO parses key query.
type FindByKeyQueryDTO struct {
	Key    string     `json:"key"`
	TeamID *uuid.UUID `json:"teamId,omitempty"`
}
