// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package orgdomain

import (
	"time"

	"github.com/google/uuid"
)

// ReleaseTrack represents release channels for feature gates and updates.
type ReleaseTrack string

const (
	ReleaseTrackStable   ReleaseTrack = "stable"
	ReleaseTrackBeta     ReleaseTrack = "beta"
	ReleaseTrackPreview  ReleaseTrack = "preview"
	ReleaseTrackInternal ReleaseTrack = "internal"

	DefaultReleaseTrack = ReleaseTrackBeta
)

// OrganizationEntity represents the core enterprise organization aggregate.
type OrganizationEntity struct {
	UUID                         uuid.UUID    `json:"uuid"`
	Name                         string       `json:"name"`
	TenantName                   string       `json:"tenant_name"`
	Status                       bool         `json:"status"`
	CodeHostMemberCount          int          `json:"code_host_member_count"`
	CodeHostMemberCountUpdatedAt *time.Time   `json:"code_host_member_count_updated_at,omitempty"`
	ReleaseTrack                 ReleaseTrack `json:"release_track"`
	Language                     string       `json:"language,omitempty"`
	Domain                       []string     `json:"domain,omitempty"`
	CreatedAt                    time.Time    `json:"created_at"`
	UpdatedAt                    time.Time    `json:"updated_at"`
}

// NewOrganizationEntity creates a new OrganizationEntity with default values.
func NewOrganizationEntity(name string, tenantName string) *OrganizationEntity {
	now := time.Now().UTC()
	id := uuid.New()
	if tenantName == "" {
		tenantName = name + "-" + id.String()
	}
	return &OrganizationEntity{
		UUID:         id,
		Name:         name,
		TenantName:   tenantName,
		Status:       true,
		ReleaseTrack: DefaultReleaseTrack,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}
