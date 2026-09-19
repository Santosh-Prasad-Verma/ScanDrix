package domain

import (
	"time"

	"github.com/google/uuid"
)

// Team represents an engineering squad or project group within an organization.
type Team struct {
	TenantScopedEntity
	OrganizationID uuid.UUID `json:"organization_id" db:"organization_id"`
	Slug           string    `json:"slug" db:"slug"`
	Name           string    `json:"name" db:"name"`
	Description    *string   `json:"description,omitempty" db:"description"`
	IsActive       bool      `json:"is_active" db:"is_active"`
	Settings       JSONBMap  `json:"settings" db:"settings"`
}

// TeamModel alias.
type TeamModel = Team

// TeamMember maps users to teams with squad-level roles (Leader, Maintainer, Contributor).
type TeamMember struct {
	TenantScopedEntity
	TeamID            uuid.UUID `json:"team_id" db:"team_id"`
	UserID            uuid.UUID `json:"user_id" db:"user_id"`
	Role              string    `json:"role" db:"role"`
	NotificationOptIn bool      `json:"notification_opt_in" db:"notification_opt_in"`
}

// TeamMemberModel alias.
type TeamMemberModel = TeamMember

// TeamCliKey stores cryptographically hashed API keys for local CLI reviews (`scandrix_*`).
type TeamCliKey struct {
	TenantScopedEntity
	TeamID          uuid.UUID   `json:"team_id" db:"team_id"`
	CreatedByUserID uuid.UUID   `json:"created_by_user_id" db:"created_by_user_id"`
	KeyPrefix       string      `json:"key_prefix" db:"key_prefix"`
	KeyHash         string      `json:"-" db:"key_hash"`
	Name            string      `json:"name" db:"name"`
	Scopes          StringSlice `json:"scopes" db:"scopes"`
	LastUsedAt      *time.Time  `json:"last_used_at,omitempty" db:"last_used_at"`
	ExpiresAt       *time.Time  `json:"expires_at,omitempty" db:"expires_at"`
	RevokedAt       *time.Time  `json:"revoked_at,omitempty" db:"revoked_at"`
}

// TeamCliKeyModel alias.
type TeamCliKeyModel = TeamCliKey

// CliDevice tracks registered developer workstations running local ScanDrix engines.
type CliDevice struct {
	TenantScopedEntity
	UserID           uuid.UUID `json:"user_id" db:"user_id"`
	DeviceIdentifier string    `json:"device_identifier" db:"device_identifier"`
	Hostname         string    `json:"hostname" db:"hostname"`
	OS               string    `json:"os" db:"os"`
	Arch             string    `json:"arch" db:"arch"`
	ClientVersion    string    `json:"client_version" db:"client_version"`
	LastSeenAt       time.Time `json:"last_seen_at" db:"last_seen_at"`
	IsRevoked        bool      `json:"is_revoked" db:"is_revoked"`
}

// CliDeviceModel alias.
type CliDeviceModel = CliDevice

// CliAuthSession models browser-based login exchanges for the ScanDrix CLI (`scandrix login`).
type CliAuthSession struct {
	BaseEntity
	SessionCode  string     `json:"session_code" db:"session_code"`
	UserCode     string     `json:"user_code" db:"user_code"`
	Status       string     `json:"status" db:"status"`
	UserID       *uuid.UUID `json:"user_id,omitempty" db:"user_id"`
	WorkspaceID  *uuid.UUID `json:"workspace_id,omitempty" db:"workspace_id"`
	TokenPayload *string    `json:"-" db:"token_payload"`
	ClientIP     string     `json:"client_ip" db:"client_ip"`
	UserAgent    string     `json:"user_agent" db:"user_agent"`
	ExpiresAt    time.Time  `json:"expires_at" db:"expires_at"`
}

// SessionEvent records developer interactions within a CLI review session.
type SessionEvent struct {
	TenantScopedEntity
	SessionID uuid.UUID `json:"session_id" db:"session_id"`
	EventType string    `json:"event_type" db:"event_type"`
	Payload   JSONBMap  `json:"payload" db:"payload"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// SessionEventModel alias.
type SessionEventModel = SessionEvent
