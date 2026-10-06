// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package paramdomain

import (
	"context"

	"github.com/google/uuid"
)

// ParametersFilter allows querying team/workspace parameters.
type ParametersFilter struct {
	UUID        *uuid.UUID    `json:"uuid,omitempty"`
	WorkspaceID *uuid.UUID    `json:"workspace_id,omitempty"`
	TeamID      *uuid.UUID    `json:"team_id,omitempty"`
	ConfigKey   *ParameterKey `json:"config_key,omitempty"`
	Active      *bool         `json:"active,omitempty"`
}

// IParametersRepository defines data persistence for parameters.
type IParametersRepository interface {
	Find(ctx context.Context, filter ParametersFilter) ([]*ParametersEntity, error)
	FindOne(ctx context.Context, filter ParametersFilter) (*ParametersEntity, error)
	FindByID(ctx context.Context, wsID, id uuid.UUID) (*ParametersEntity, error)
	FindByKey(ctx context.Context, wsID uuid.UUID, teamID *uuid.UUID, key ParameterKey) (*ParametersEntity, error)
	Create(ctx context.Context, entity *ParametersEntity) (*ParametersEntity, error)
	Update(ctx context.Context, filter ParametersFilter, data *ParametersEntity) (*ParametersEntity, error)
	CreateNewActiveVersion(ctx context.Context, wsID uuid.UUID, teamID *uuid.UUID, key ParameterKey, val any, nextVersion int) (*ParametersEntity, error)
	CreateActiveVersionIfAbsent(ctx context.Context, wsID uuid.UUID, teamID *uuid.UUID, key ParameterKey, val any) (*ParametersEntity, error)
	Delete(ctx context.Context, wsID uuid.UUID, teamID *uuid.UUID, key ParameterKey) error
	// DeleteByTeamID takes the owning workspace explicitly. Resolving the
	// workspace from team_id alone would leave the update without a tenant
	// context, and RLS would then match zero rows and report a clean purge.
	DeleteByTeamID(ctx context.Context, wsID, teamID uuid.UUID) error
}

// IParametersService defines the business logic contract for parameters.
type IParametersService interface {
	IParametersRepository
	CreateOrUpdateConfig(ctx context.Context, wsID uuid.UUID, teamID *uuid.UUID, key ParameterKey, val any, desc string) (*ParametersEntity, error)
	FindByKeyCached(ctx context.Context, wsID uuid.UUID, teamID *uuid.UUID, key ParameterKey) (*ParametersEntity, error)
}
