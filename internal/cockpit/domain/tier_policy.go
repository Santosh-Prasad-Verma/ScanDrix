package domain

// OrganizationLicenseStatus models the subscription and license state of an organization.
type OrganizationLicenseStatus struct {
	SubscriptionStatus string `json:"subscription_status"` // "active", "trial", "canceled", "expired", "self-hosted"
	Plan               string `json:"plan"`                // "teams", "enterprise", "free_byok", "starter"
	IsSelfHosted       bool   `json:"is_self_hosted"`
	IsValid            bool   `json:"is_valid"`
}

// IsCockpitTierAllowed enforces the access policy for the ScanDrix cockpit.
// Allowed:
//   - cloud paid (active) on Teams or Enterprise plans
//   - licensed self-hosted on Enterprise plan
//   - trial (treated as Teams-cloud equivalent)
// Blocked:
//   - invalid / expired / canceled licenses
//   - unlicensed self-hosted
//   - free_byok
//   - licensed self-hosted on Teams plans (Teams is cloud-only)
func IsCockpitTierAllowed(license *OrganizationLicenseStatus) bool {
	if license == nil || !license.IsValid {
		return false
	}

	if license.SubscriptionStatus == "canceled" || license.SubscriptionStatus == "expired" {
		return false
	}

	if license.Plan == "free_byok" {
		return false
	}

	if license.IsSelfHosted {
		// Self-hosted is only permitted on Enterprise plans
		return license.Plan == "enterprise" && license.SubscriptionStatus != "self-hosted"
	}

	// Cloud: trial or active subscription on teams/enterprise
	if license.SubscriptionStatus == "trial" {
		return true
	}

	if license.SubscriptionStatus == "active" {
		return license.Plan == "teams" || license.Plan == "enterprise"
	}

	return false
}
