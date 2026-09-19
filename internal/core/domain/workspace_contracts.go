package domain

import (
	"context"

	"github.com/google/uuid"
)

// WorkspaceCreateDTO payload for provisioning a new enterprise workspace.
type WorkspaceCreateDTO struct {
	Slug                string      `json:"slug"`
	Name                string      `json:"name"`
	Tier                string      `json:"tier"`
	SpendLimitUSD       float64     `json:"spend_limit_usd"`
	EnforceSpendLimit   bool        `json:"enforce_spend_limit"`
	AllowedEmailDomains StringSlice `json:"allowed_email_domains"`
	Metadata            JSONBMap    `json:"metadata"`
}

// WorkspaceUpdateDTO payload for modifying workspace attributes.
type WorkspaceUpdateDTO struct {
	Name                *string      `json:"name,omitempty"`
	Status              *string      `json:"status,omitempty"`
	Tier                *string      `json:"tier,omitempty"`
	SpendLimitUSD       *float64     `json:"spend_limit_usd,omitempty"`
	EnforceSpendLimit   *bool        `json:"enforce_spend_limit,omitempty"`
	CustomDomain        *string      `json:"custom_domain,omitempty"`
	SAMLRequired        *bool        `json:"saml_required,omitempty"`
	AllowedEmailDomains *StringSlice `json:"allowed_email_domains,omitempty"`
	Metadata            JSONBMap     `json:"metadata,omitempty"`
}

// OrganizationCreateDTO creates a linked VCS organization under a workspace.
type OrganizationCreateDTO struct {
	WorkspaceID    uuid.UUID `json:"workspace_id"`
	ExternalID     string    `json:"external_id"`
	Name           string    `json:"name"`
	AvatarURL      *string   `json:"avatar_url,omitempty"`
	BillingEmail   string    `json:"billing_email"`
	SCMProvider    string    `json:"scm_provider"`
	InstallationID *string   `json:"installation_id,omitempty"`
	Settings       JSONBMap  `json:"settings"`
}

// WorkspaceRepository contract.
type WorkspaceRepository interface {
	FindByID(ctx context.Context, id uuid.UUID) (*Workspace, error)
	FindBySlug(ctx context.Context, slug string) (*Workspace, error)
	Create(ctx context.Context, ws *Workspace) error
	Update(ctx context.Context, ws *Workspace) error
	List(ctx context.Context, pq PaginationQuery) (*PaginatedResult[*Workspace], error)
}

// OrganizationRepository contract.
type OrganizationRepository interface {
	FindByID(ctx context.Context, wsID, orgID uuid.UUID) (*Organization, error)
	FindByExternalID(ctx context.Context, provider, externalID string) (*Organization, error)
	Create(ctx context.Context, org *Organization) error
	Update(ctx context.Context, org *Organization) error
	ListByWorkspace(ctx context.Context, wsID uuid.UUID) ([]*Organization, error)
}
