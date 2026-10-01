package models

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// SSOProtocol identifies the identity federation protocol for a workspace.
type SSOProtocol string

const (
	SSOProtocolSAML SSOProtocol = "SAML"
	SSOProtocolOIDC SSOProtocol = "OIDC"
)

// Valid reports whether the protocol is one this build supports.
func (p SSOProtocol) Valid() bool {
	return p == SSOProtocolSAML || p == SSOProtocolOIDC
}

// SSOProviderConfig holds the IdP settings for a workspace.
//
// An IdP client secret (OIDC) is present in this struct in plaintext only
// in memory, between the request handler and the repository, which encrypts it
// at rest before writing. It is never logged and never returned by the API in
// a read response — ReadForResponse substitutes a presence marker instead.
type SSOProviderConfig struct {
	// Shared
	Issuer      string `json:"idpIssuer,omitempty"` // SAML issuer / OIDC issuer URL
	DisplayName string `json:"displayName,omitempty"`

	// SAML
	EntryPoint         string `json:"entryPoint,omitempty"`  // IdP SSO URL
	IdPCert            string `json:"cert,omitempty"`        // IdP X.509 signing certificate
	IssuerID           string `json:"idpIssuerId,omitempty"` // optional IdP entity id
	SignatureAlgorithm string `json:"signatureAlgorithm,omitempty"`
	IdentifierFormat   string `json:"identifierFormat,omitempty"` // email / persistent / transient
	AllowUnsolicited   bool   `json:"allowUnsolicitedResponse,omitempty"`

	// OIDC
	ClientID     string `json:"clientId,omitempty"`
	ClientSecret string `json:"clientSecret,omitempty"` // write-only; never returned
	AuthorizeURL string `json:"authorizeUrl,omitempty"`
	TokenURL     string `json:"tokenUrl,omitempty"`
	JWKSURL      string `json:"jwksUrl,omitempty"`
	Scopes       string `json:"scopes,omitempty"`
}

// NormalizeSSODomains lowercases, trims, de-duplicates and drops empty entries
// so domain routing cannot be bypassed with casing or padding tricks.
//
// Shared by the controller (validation boundary) and the repository (storage
// boundary) so the rule is enforced twice rather than assumed.
func NormalizeSSODomains(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, d := range in {
		d = strings.ToLower(strings.TrimSpace(d))
		if d == "" {
			continue
		}
		if _, dup := seen[d]; dup {
			continue
		}
		seen[d] = struct{}{}
		out = append(out, d)
	}
	return out
}

// SSOConfig is the per-workspace single sign-on configuration.
type SSOConfig struct {
	ID           uuid.UUID         `json:"uuid"`
	WorkspaceID  uuid.UUID         `json:"organizationId"`
	Protocol     SSOProtocol       `json:"protocol"`
	Provider     SSOProviderConfig `json:"providerConfig"`
	Active       bool              `json:"active"`
	Domains      []string          `json:"domains"`
	SAMLRequired bool              `json:"samlRequired"`
	VerifiedAt   *time.Time        `json:"verifiedAt,omitempty"`
	CreatedAt    time.Time         `json:"createdAt"`
	UpdatedAt    time.Time         `json:"updatedAt"`
}
