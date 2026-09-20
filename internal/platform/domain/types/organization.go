// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package types

import "github.com/google/uuid"

// Organization encapsulates tenant organization properties in VCS connectors.
type Organization struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Login     string `json:"login,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
}

// OrganizationAndTeamData passes tenant, team, and integration context across use cases.
type OrganizationAndTeamData struct {
	OrganizationID string     `json:"organizationId"`
	TeamID         string     `json:"teamId,omitempty"`
	WorkspaceID    uuid.UUID  `json:"workspaceId,omitempty"`
	TeamUUID       *uuid.UUID `json:"teamUuid,omitempty"`
	TeamMemberID   *uuid.UUID `json:"teamMemberId,omitempty"`
	Provider       string     `json:"provider,omitempty"`
	ProviderID     string     `json:"providerId,omitempty"`
	InstallationID *int64     `json:"installationId,omitempty"`
	ConfigKey              string         `json:"configKey,omitempty"`
	AuthToken              string         `json:"authToken,omitempty"`
	IntegrationCredentials map[string]any `json:"integrationCredentials,omitempty"`
}

// IntegrationCategory describes platform tool purpose.
type IntegrationCategory string

const (
	IntegrationCategoryCodeManagement    IntegrationCategory = "code_management"
	IntegrationCategoryProjectManagement IntegrationCategory = "project_management"
	IntegrationCategoryChat              IntegrationCategory = "chat"
	IntegrationCategoryCI                IntegrationCategory = "ci"
)
