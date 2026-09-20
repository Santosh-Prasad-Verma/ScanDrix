package domain

import (
	"github.com/google/uuid"
)

// Integration models connected third-party platforms (Slack, Jira, Linear, Sentry).
type Integration struct {
	TenantScopedEntity
	OrganizationID       uuid.UUID `json:"organization_id" db:"organization_id"`
	Provider             string    `json:"provider" db:"provider"`
	DisplayName          string    `json:"display_name" db:"display_name"`
	IsActive             bool      `json:"is_active" db:"is_active"`
	EncryptedAuthPayload string    `json:"-" db:"encrypted_auth_payload"`
	Settings             JSONBMap  `json:"settings" db:"settings"`
}

// IntegrationModel alias.
type IntegrationModel = Integration

// IntegrationConfig holds provider-specific webhook URLs, project keys, and channel mappings.
type IntegrationConfig struct {
	TenantScopedEntity
	IntegrationID uuid.UUID `json:"integration_id" db:"integration_id"`
	ConfigKey     string    `json:"config_key" db:"config_key"`
	ConfigValue   string    `json:"config_value" db:"config_value"`
	IsEncrypted   bool      `json:"is_encrypted" db:"is_encrypted"`
}

// IntegrationConfigModel alias.
type IntegrationConfigModel = IntegrationConfig

// Issue models tracked Jira/Linear tickets or internal ScanDrix security findings.
type Issue struct {
	TenantScopedEntity
	RepositoryID uuid.UUID `json:"repository_id" db:"repository_id"`
	ExternalID   string    `json:"external_id" db:"external_id"`
	Provider     string    `json:"provider" db:"provider"`
	Title        string    `json:"title" db:"title"`
	Description  *string   `json:"description,omitempty" db:"description"`
	Severity     string    `json:"severity" db:"severity"`
	Status       string    `json:"status" db:"status"`
	Assignee     *string   `json:"assignee,omitempty" db:"assignee"`
	URL          string    `json:"url" db:"url"`
	Metadata     JSONBMap  `json:"metadata" db:"metadata"`
}

// IssuesModel alias.
type IssuesModel = Issue
