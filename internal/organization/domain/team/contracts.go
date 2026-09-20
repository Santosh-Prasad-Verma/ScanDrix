// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package teamdomain

import (
	"context"

	"github.com/google/uuid"
)

// TeamFilter allows querying teams.
type TeamFilter struct {
	UUID        *uuid.UUID `json:"uuid,omitempty"`
	WorkspaceID *uuid.UUID `json:"workspace_id,omitempty"`
	Name        *string    `json:"name,omitempty"`
	Status      *bool      `json:"status,omitempty"`
}

// ITeamRepository defines data access for teams.
type ITeamRepository interface {
	Find(ctx context.Context, filter TeamFilter) ([]*TeamEntity, error)
	FindOne(ctx context.Context, filter TeamFilter) (*TeamEntity, error)
	FindByID(ctx context.Context, id uuid.UUID) (*TeamEntity, error)
	FindByWorkspaceID(ctx context.Context, workspaceID uuid.UUID) ([]*TeamEntity, error)
	GetTeamsByUserID(ctx context.Context, userID, workspaceID uuid.UUID) ([]*TeamEntity, error)
	FindFirstCreatedTeam(ctx context.Context, workspaceID uuid.UUID) (*TeamEntity, error)
	Create(ctx context.Context, entity *TeamEntity) (*TeamEntity, error)
	Update(ctx context.Context, filter TeamFilter, data *TeamEntity) (*TeamEntity, error)
	Delete(ctx context.Context, id uuid.UUID) error
	ListWithIntegrations(ctx context.Context, workspaceID uuid.UUID) ([]*TeamWithIntegrations, error)
}

// ITeamService defines business operations for teams.
type ITeamService interface {
	ITeamRepository
	CreateTeam(ctx context.Context, wsID uuid.UUID, name, description string) (*TeamEntity, error)
}
