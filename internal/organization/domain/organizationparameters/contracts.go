// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package orgparams

import (
	"context"

	"github.com/google/uuid"
)

// OrganizationParametersFilter allows structured querying for parameters.
type OrganizationParametersFilter struct {
	UUID        *uuid.UUID    `json:"uuid,omitempty"`
	WorkspaceID *uuid.UUID    `json:"workspace_id,omitempty"`
	ConfigKey   *ParameterKey `json:"config_key,omitempty"`
	IsActive    *bool         `json:"is_active,omitempty"`
}

// IOrganizationParametersRepository defines the storage boundary for organization parameters.
type IOrganizationParametersRepository interface {
	Find(ctx context.Context, filter OrganizationParametersFilter) ([]*OrganizationParametersEntity, error)
	FindOne(ctx context.Context, filter OrganizationParametersFilter) (*OrganizationParametersEntity, error)
	FindByID(ctx context.Context, id uuid.UUID) (*OrganizationParametersEntity, error)
	FindByOrganizationName(ctx context.Context, orgName string) (*OrganizationParametersEntity, error)
	FindByKey(ctx context.Context, wsID uuid.UUID, key ParameterKey) (*OrganizationParametersEntity, error)
	FindByKeyAndValue(ctx context.Context, key ParameterKey, matchJSON map[string]any) ([]*OrganizationParametersEntity, error)
	FindByKeyAndValueFuzzy(ctx context.Context, key ParameterKey, matchJSON map[string]any, fuzzy bool) ([]*OrganizationParametersEntity, error)
	Create(ctx context.Context, entity *OrganizationParametersEntity) (*OrganizationParametersEntity, error)
	Update(ctx context.Context, filter OrganizationParametersFilter, data *OrganizationParametersEntity) (*OrganizationParametersEntity, error)
	Delete(ctx context.Context, wsID uuid.UUID, key ParameterKey) error
}

// IOrganizationParametersService defines the domain service contract for organization parameters.
type IOrganizationParametersService interface {
	IOrganizationParametersRepository
	CreateOrUpdateConfig(ctx context.Context, wsID uuid.UUID, key ParameterKey, val any, desc string) (*OrganizationParametersEntity, error)
	DeleteBYOKConfig(ctx context.Context, wsID uuid.UUID, configType string) error
	DeleteBYOKModel(ctx context.Context, wsID uuid.UUID, modelID string) error
}
