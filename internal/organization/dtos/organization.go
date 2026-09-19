// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package dtos

import (
	"github.com/google/uuid"
)

// UpdateOrganizationInfoDTO represents org metadata updates.
type UpdateOrganizationInfoDTO struct {
	Name  string  `json:"name"`
	Phone *string `json:"phone,omitempty"`
}

// JoinOrganizationDTO represents a request for a user to join an organization.
type JoinOrganizationDTO struct {
	UserID         uuid.UUID  `json:"userId"`
	OrganizationID uuid.UUID  `json:"organizationId"`
	InviteToken    *string    `json:"inviteToken,omitempty"`
}

// OrganizationResponseDTO returns formatted organization details.
type OrganizationResponseDTO struct {
	ID                  uuid.UUID `json:"id"`
	Slug                string    `json:"slug"`
	Name                string    `json:"name"`
	ReleaseTrack        string    `json:"releaseTrack"`
	CodeHostMemberCount int       `json:"codeHostMemberCount"`
	Status              bool      `json:"status"`
}
