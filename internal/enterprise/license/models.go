package license

import (
	"time"

	"github.com/google/uuid"
)

// LicenseTier defines the subscription plan.
type LicenseTier string

const (
	TierCommunity  LicenseTier = "COMMUNITY"
	TierDeveloper  LicenseTier = "DEVELOPER"
	TierTeam       LicenseTier = "TEAM"
	TierScale      LicenseTier = "SCALE"
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

// LicenseGracePeriod is how long an expired license keeps working before it
// stops unlocking gated features. This is the single definition: LoadLicense,
// EntitlementFromLicense and the plan-row reader all reference it, so the
// grace window can never drift between enforcement paths.
const LicenseGracePeriod = 7 * 24 * time.Hour

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

	// KeyID names the authority keypair that signed this license. Empty means
	// the primary key configured on the server. During a rotation overlap both
	// forms verify: licenses minted before KeyID existed carry no value, and
	// licenses minted after it name the new key. Omitempty keeps tokens issued
	// by earlier builds byte-identical after a round trip.
	KeyID string `json:"key_id,omitempty"`

	// HardwareFingerprint optionally binds a license to one server or cluster
	// (a machine-id, a cluster UUID, a customer-chosen opaque string). Empty
	// means unbound. Enforcement is opt-in on the server side: the fingerprint
	// is only compared when the deployment declares the value it expects, so
	// existing unbound licenses keep loading unchanged.
	HardwareFingerprint string `json:"hardware_fingerprint,omitempty"`
}

// SignedLicenseToken represents the serialized license file format.
type SignedLicenseToken struct {
	Payload   string `json:"payload"`   // Base64 encoded LicensePayload JSON
	Signature string `json:"signature"` // Base64 encoded Ed25519 signature
}
