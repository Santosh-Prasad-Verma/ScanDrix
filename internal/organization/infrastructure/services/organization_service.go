// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	orgdomain "github.com/scandrix/backend/internal/organization/domain/organization"
)

// OrganizationService implements orgdomain.IOrganizationService.
type OrganizationService struct {
	repo orgdomain.IOrganizationRepository
}

// NewOrganizationService instantiates a new OrganizationService.
func NewOrganizationService(repo orgdomain.IOrganizationRepository) *OrganizationService {
	return &OrganizationService{repo: repo}
}

func (s *OrganizationService) Find(ctx context.Context, filter orgdomain.OrganizationFilter) ([]*orgdomain.OrganizationEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.Find(ctx, filter)
}

func (s *OrganizationService) FindOne(ctx context.Context, filter orgdomain.OrganizationFilter) (*orgdomain.OrganizationEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.FindOne(ctx, filter)
}

func (s *OrganizationService) FindByID(ctx context.Context, id uuid.UUID) (*orgdomain.OrganizationEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.FindByID(ctx, id)
}

func (s *OrganizationService) Create(ctx context.Context, entity *orgdomain.OrganizationEntity) (*orgdomain.OrganizationEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.Create(ctx, entity)
}

func (s *OrganizationService) DeleteOne(ctx context.Context, filter orgdomain.OrganizationFilter) error {
	if s.repo == nil {
		return errors.New("repository uninitialized")
	}
	return s.repo.DeleteOne(ctx, filter)
}

func (s *OrganizationService) Delete(ctx context.Context, id uuid.UUID) error {
	if s.repo == nil {
		return errors.New("repository uninitialized")
	}
	return s.repo.Delete(ctx, id)
}

func (s *OrganizationService) Update(ctx context.Context, filter orgdomain.OrganizationFilter, data *orgdomain.OrganizationEntity) (*orgdomain.OrganizationEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.Update(ctx, filter, data)
}

// CreateOrganizationWithTenant initializes an organization aggregate with a slug tenant name.
func (s *OrganizationService) CreateOrganizationWithTenant(ctx context.Context, name, tenantName string) (*orgdomain.OrganizationEntity, error) {
	if name == "" {
		return nil, errors.New("organization name is required")
	}
	id := uuid.New()
	if tenantName == "" {
		tenantName = fmt.Sprintf("%s-%s", name, id.String()[:8])
	}
	entity := orgdomain.NewOrganizationEntity(name, tenantName)
	entity.UUID = id
	return s.Create(ctx, entity)
}

// FindByUserID retrieves the organization associated with a given user.
func (s *OrganizationService) FindByUserID(ctx context.Context, userID uuid.UUID) (*orgdomain.OrganizationEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.FindByUserID(ctx, userID)
}

// FindOneByUserID retrieves the organization associated with a given user.
func (s *OrganizationService) FindOneByUserID(ctx context.Context, userID uuid.UUID) (*orgdomain.OrganizationEntity, error) {
	return s.FindByUserID(ctx, userID)
}

// GetReleaseTrack queries the release track for an organization, defaulting to DefaultReleaseTrack (beta).
func (s *OrganizationService) GetReleaseTrack(ctx context.Context, organizationID uuid.UUID) (orgdomain.ReleaseTrack, error) {
	org, err := s.FindByID(ctx, organizationID)
	if err != nil || org == nil {
		return orgdomain.DefaultReleaseTrack, nil
	}
	if org.ReleaseTrack == "" {
		return orgdomain.DefaultReleaseTrack, nil
	}
	return org.ReleaseTrack, nil
}
