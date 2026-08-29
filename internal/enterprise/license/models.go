package license

import (
	"time"

	"github.com/google/uuid"
)

// LicenseTier defines the subscription plan.
type LicenseTier string

const (
	TierCommunity  LicenseTier = "COMMUNITY"
	TierTeam       LicenseTier = "TEAM"
	TierEnterprise LicenseTier = "ENTERPRISE"
)

// FeatureFlag specifies capabilities subject to enterprise license gating.
type FeatureFlag string

const (
	FeatureSSOSAML                FeatureFlag = "FEATURE_SSO_SAML"
	FeatureSCIM                   FeatureFlag = "FEATURE_SCIM_PROVISIONING"
	FeatureAuditWarehouse         FeatureFlag = "FEATURE_AUDIT_WAREHOUSE"
	FeatureMultiAgentDeliberation FeatureFlag = "FEATURE_MULTI_AGENT_DELIBERATION"
	FeatureBYOK                   FeatureFlag = "FEATURE_BYOK_ENCRYPTION"
	FeatureCustomRules            FeatureFlag = "FEATURE_CUSTOM_RULES"
	FeatureDORAMetrics            FeatureFlag = "FEATURE_DORA_METRICS"
	FeatureAirGapped              FeatureFlag = "FEATURE_AIR_GAPPED"
)

// LicensePayload contains the cryptographically signed entitlement data.
type LicensePayload struct {
	LicenseID       uuid.UUID   `json:"license_id"`
	CustomerName    string      `json:"customer_name"`
	CustomerID      string      `json:"customer_id"`
	Tier            LicenseTier `json:"tier"`
	IssuedAt        time.Time   `json:"issued_at"`
	ExpiresAt       time.Time   `json:"expires_at"`
	MaxSeats        int         `json:"max_seats"`        // 0 = unlimited
	MaxRepositories int         `json:"max_repositories"` // 0 = unlimited
	Features        []string    `json:"features"`
}

// SignedLicenseToken represents the serialized license file format.
type SignedLicenseToken struct {
	Payload   string `json:"payload"`   // Base64 encoded LicensePayload JSON
	Signature string `json:"signature"` // Base64 encoded Ed25519 signature
}
