// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package orgdomain

import (
	"context"

	"github.com/google/uuid"
)

// OrganizationFilter allows structured querying for organizations.
type OrganizationFilter struct {
	UUID       *uuid.UUID `json:"uuid,omitempty"`
	Name       *string    `json:"name,omitempty"`
	TenantName *string    `json:"tenant_name,omitempty"`
	Status     *bool      `json:"status,omitempty"`
}

// IOrganizationRepository defines the storage boundary for organizations.
type IOrganizationRepository interface {
	Find(ctx context.Context, filter OrganizationFilter) ([]*OrganizationEntity, error)
	FindOne(ctx context.Context, filter OrganizationFilter) (*OrganizationEntity, error)
	FindByID(ctx context.Context, id uuid.UUID) (*OrganizationEntity, error)
	FindByUserID(ctx context.Context, userID uuid.UUID) (*OrganizationEntity, error)
	Create(ctx context.Context, entity *OrganizationEntity) (*OrganizationEntity, error)
	DeleteOne(ctx context.Context, filter OrganizationFilter) error
	Delete(ctx context.Context, id uuid.UUID) error
	Update(ctx context.Context, filter OrganizationFilter, data *OrganizationEntity) (*OrganizationEntity, error)
}

// IOrganizationService defines the domain service contract for organizations.
type IOrganizationService interface {
	IOrganizationRepository
	CreateOrganizationWithTenant(ctx context.Context, name, tenantName string) (*OrganizationEntity, error)
	FindOneByUserID(ctx context.Context, userID uuid.UUID) (*OrganizationEntity, error)
	GetReleaseTrack(ctx context.Context, organizationID uuid.UUID) (ReleaseTrack, error)
}
