// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package dtos

import (
	"time"

	"github.com/google/uuid"
)

// CreateTeamDTO represents payload to create a new team.
type CreateTeamDTO struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	AutoAssignMode string `json:"autoAssignMode,omitempty"`
}

// TeamResponseDTO represents the serialized team payload.
type TeamResponseDTO struct {
	ID             uuid.UUID `json:"id"`
	WorkspaceID    uuid.UUID `json:"workspaceId"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	AutoAssignMode string    `json:"autoAssignMode"`
	MemberCount    int       `json:"memberCount"`
	CreatedAt      time.Time `json:"createdAt"`
}
