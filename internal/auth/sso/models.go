package sso

import (
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// SSOProviderType identifies the federation protocol.
type SSOProviderType string

const (
	ProviderTypeSAML2 SSOProviderType = "SAML2"
	ProviderTypeOIDC  SSOProviderType = "OIDC"
)

// IdPConfiguration represents an enterprise Identity Provider registration.
type IdPConfiguration struct {
	ID             uuid.UUID         `json:"id"`
	WorkspaceID    uuid.UUID         `json:"workspace_id"`
	ProviderType   SSOProviderType   `json:"provider_type"`
	Issuer         string            `json:"issuer"`
	SSOURL         string            `json:"sso_url"`
	CertPEM        string            `json:"cert_pem,omitempty"`
	ClientID       string            `json:"client_id,omitempty"`
	ClientSecret   string            `json:"-"` // Never exposed
	AllowedDomains []string          `json:"allowed_domains"`
	RoleMapping    map[string]string `json:"role_mapping"` // e.g. "Admins" -> "ADMIN"
	Active         bool              `json:"active"`
	CreatedAt      time.Time         `json:"created_at"`
}

// FederatedIdentity holds the authenticated identity attributes extracted from IdP assertion/token.
type FederatedIdentity struct {
	ExternalID string            `json:"external_id"`
	Email      string            `json:"email"`
	FirstName  string            `json:"first_name"`
	LastName   string            `json:"last_name"`
	Groups     []string          `json:"groups"`
	MappedRole models.UserRole   `json:"mapped_role"`
	Provider   SSOProviderType   `json:"provider"`
	RawClaims  map[string]string `json:"raw_claims,omitempty"`
}

// SAMLAssertion represents parsed SAML XML attributes.
type SAMLAssertion struct {
	Issuer          string
	NameID          string
	Recipient       string
	Audience        string
	NotBefore       time.Time
	NotOnOrAfter    time.Time
	Attributes      map[string]string
	GroupMembership []string
}
