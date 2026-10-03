package license

import (
	"testing"
	"time"
)

func TestCommunityEntitlementUnlocksOnlyCommunityFeatures(t *testing.T) {
	ent := CommunityEntitlement()

	if !ent.Valid {
		t.Fatal("community entitlement should be valid")
	}
	if ent.Tier != TierCommunity {
		t.Fatalf("expected COMMUNITY tier, got %s", ent.Tier)
	}
	if ent.Source != SourceCommunity {
		t.Fatalf("expected community source, got %q", ent.Source)
	}
	if !ent.Allows(FeatureBYOK) {
		t.Fatal("BYOK is a community feature and must be available")
	}
	for _, flag := range []FeatureFlag{
		FeatureSSOSAML, FeatureSCIM, FeatureAuditWarehouse,
		FeatureMultiAgentDeliberation,
		FeatureCustomRules, FeatureDORAMetrics, FeatureAirGapped,
	} {
		if ent.Allows(flag) {
			t.Fatalf("community must not unlock premium feature %s", flag)
		}
		if err := ent.AssertAllows(flag); err == nil {
			t.Fatalf("community must be denied %s", flag)
		}
	}
	if ent.Quota.MonthlyTokens != QuotaCommunity.MonthlyTokens {
		t.Fatalf("expected community quota, got %d", ent.Quota.MonthlyTokens)
	}
}

func TestCommunityFeaturesAvailableOnEveryPaidPlan(t *testing.T) {
	for _, tier := range []LicenseTier{TierDeveloper, TierTeam, TierScale, TierEnterprise} {
		ent := EntitlementFromPlan(tier, nil, 0, 0, time.Time{})
		if !ent.Allows(FeatureBYOK) {
			t.Fatalf("tier %s must retain the community BYOK feature", tier)
		}
	}

	signed := EntitlementFromLicense(&LicensePayload{
		Tier:      TierTeam,
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	})
	if !signed.Allows(FeatureBYOK) {
		t.Fatal("a signed license must retain the community BYOK feature")
	}
}

func TestExpiredEntitlementDeniesEvenCommunityFeatures(t *testing.T) {
	ent := EntitlementFromLicense(&LicensePayload{
		Tier:      TierEnterprise,
		ExpiresAt: time.Now().UTC().Add(-LicenseGracePeriod - time.Hour),
	})

	if ent.Valid {
		t.Fatal("expected an invalid entitlement")
	}
	if ent.Allows(FeatureBYOK) {
		t.Fatal("an invalid entitlement must deny every feature, including BYOK")
	}
	if ent.Allows(FeatureSSOSAML) {
		t.Fatal("an invalid entitlement must deny every premium feature")
	}
}

func TestNilEntitlementFailsClosed(t *testing.T) {
	var ent *Entitlement

	if ent.Allows(FeatureSSOSAML) {
		t.Fatal("nil entitlement must not unlock any feature")
	}
	if err := ent.AssertAllows(FeatureSSOSAML); err == nil {
		t.Fatal("nil entitlement must deny with an error")
	}
	if ent.SeatLimit() != QuotaCommunity.MaxSeats {
		t.Fatalf("nil entitlement should fall back to community seats, got %d", ent.SeatLimit())
	}
	if ent.RepoLimit() != QuotaCommunity.MaxRepositories {
		t.Fatalf("nil entitlement should fall back to community repos, got %d", ent.RepoLimit())
	}
}

func TestEntitlementFromLicenseGrantsListedFeatures(t *testing.T) {
	payload := &LicensePayload{
		Tier:      TierTeam,
		ExpiresAt: time.Now().UTC().Add(30 * 24 * time.Hour),
		MaxSeats:  25,
		Features:  []string{string(FeatureSSOSAML), string(FeatureDORAMetrics)},
	}

	ent := EntitlementFromLicense(payload)

	if !ent.Valid {
		t.Fatal("expected valid entitlement")
	}
	if ent.Source != SourceSigned {
		t.Fatalf("expected signed source, got %q", ent.Source)
	}
	if !ent.Allows(FeatureSSOSAML) || !ent.Allows(FeatureDORAMetrics) {
		t.Fatal("expected listed features to be granted")
	}
	if ent.Allows(FeatureSCIM) {
		t.Fatal("unlisted feature must not be granted")
	}
	if ent.SeatLimit() != 25 {
		t.Fatalf("expected 25 seats, got %d", ent.SeatLimit())
	}
}

func TestEnterpriseTierUnlocksEverything(t *testing.T) {
	ent := EntitlementFromLicense(&LicensePayload{
		Tier:      TierEnterprise,
		ExpiresAt: time.Now().UTC().Add(365 * 24 * time.Hour),
		Features:  []string{},
	})

	if !ent.Allows(FeatureAirGapped) {
		t.Fatal("enterprise tier must unlock all features")
	}
	if !ent.Allows(FeatureSCIM) {
		t.Fatal("enterprise tier must unlock all features")
	}
}

func TestEntitlementNormalizesTierAliases(t *testing.T) {
	ent := EntitlementFromLicense(&LicensePayload{
		Tier:      "ent",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	})
	if ent.Tier != TierEnterprise {
		t.Fatalf("expected ENT to normalize to ENTERPRISE, got %s", ent.Tier)
	}
	if ent.Quota.MonthlyTokens != QuotaEnterprise.MonthlyTokens {
		t.Fatalf("expected enterprise quota, got %d", ent.Quota.MonthlyTokens)
	}
}

func TestExpiredLicenseKeepsTierButUnlocksNothing(t *testing.T) {
	ent := EntitlementFromLicense(&LicensePayload{
		Tier:      TierTeam,
		ExpiresAt: time.Now().UTC().Add(-LicenseGracePeriod - time.Hour),
		Features:  []string{string(FeatureSSOSAML)},
	})

	if ent.Valid {
		t.Fatal("license past the grace period must be invalid")
	}
	if ent.Tier != TierTeam {
		t.Fatalf("expired license should retain tier for display, got %s", ent.Tier)
	}
	if ent.Reason == "" {
		t.Fatal("expired entitlement should carry a reason")
	}
	if ent.Allows(FeatureSSOSAML) {
		t.Fatal("expired license must unlock nothing")
	}
}

func TestLicenseWithinGracePeriodStaysValid(t *testing.T) {
	ent := EntitlementFromLicense(&LicensePayload{
		Tier:      TierTeam,
		ExpiresAt: time.Now().UTC().Add(-time.Hour),
		Features:  []string{string(FeatureSSOSAML)},
	})

	if !ent.Valid {
		t.Fatal("license inside the grace period must stay valid")
	}
	if !ent.Allows(FeatureSSOSAML) {
		t.Fatal("license inside the grace period must keep its features")
	}
}

func TestEntitlementFromPlan(t *testing.T) {
	ent := EntitlementFromPlan(
		TierScale,
		[]string{string(FeatureAuditWarehouse)},
		100,
		0,
		time.Now().UTC().Add(30*24*time.Hour),
	)

	if !ent.Valid {
		t.Fatal("expected valid plan entitlement")
	}
	if ent.Source != SourcePlan {
		t.Fatalf("expected plan source, got %q", ent.Source)
	}
	if ent.Tier != TierScale {
		t.Fatalf("expected SCALE, got %s", ent.Tier)
	}
	if !ent.Allows(FeatureAuditWarehouse) {
		t.Fatal("expected audit warehouse to be granted")
	}
	if ent.Allows(FeatureSSOSAML) {
		t.Fatal("unlisted feature must not be granted")
	}
	if ent.SeatLimit() != 100 {
		t.Fatalf("expected 100 seats, got %d", ent.SeatLimit())
	}
	if ent.RepoLimit() != 0 {
		t.Fatalf("0 must mean unlimited repositories, got %d", ent.RepoLimit())
	}
}

func TestExpiredPlanUnlocksNothing(t *testing.T) {
	// Past the grace window, not merely past the expiry instant: a plan row
	// honours the same LicenseGracePeriod as a signed license, which is what
	// GetWorkspacePlanDetails reports as GRACE_PERIOD. Expiring one hour ago
	// must therefore still be entitled.
	ent := EntitlementFromPlan(
		TierTeam,
		[]string{string(FeatureAuditWarehouse)},
		25,
		0,
		time.Now().UTC().Add(-LicenseGracePeriod-time.Hour),
	)

	if ent.Valid {
		t.Fatal("a plan past the grace window must be invalid")
	}
	if ent.Allows(FeatureAuditWarehouse) {
		t.Fatal("expired plan must unlock nothing")
	}
}

func TestPlanWithZeroExpiryNeverExpires(t *testing.T) {
	ent := EntitlementFromPlan(TierDeveloper, nil, 10, 0, time.Time{})

	if !ent.Valid {
		t.Fatal("a plan with no expiry set must remain valid")
	}
}

func TestEntitlementQuotaChecks(t *testing.T) {
	ent := EntitlementFromPlan(TierTeam, nil, 25, 10, time.Time{})

	if err := ent.CheckSeats(25); err != nil {
		t.Fatalf("25 seats should be allowed: %v", err)
	}
	if err := ent.CheckSeats(26); err == nil {
		t.Fatal("26 seats should exceed the quota")
	}
	if err := ent.CheckRepositories(10); err != nil {
		t.Fatalf("10 repos should be allowed: %v", err)
	}
	if err := ent.CheckRepositories(11); err == nil {
		t.Fatal("11 repos should exceed the quota")
	}
}

func TestUnlimitedSeatAndRepoQuotas(t *testing.T) {
	ent := EntitlementFromPlan(TierEnterprise, nil, 0, 0, time.Time{})

	if err := ent.CheckSeats(10_000); err != nil {
		t.Fatalf("unlimited seats should allow 10000: %v", err)
	}
	if err := ent.CheckRepositories(10_000); err != nil {
		t.Fatalf("unlimited repos should allow 10000: %v", err)
	}
}

func TestNilPayloadFallsBackToCommunity(t *testing.T) {
	ent := EntitlementFromLicense(nil)

	if ent.Tier != TierCommunity {
		t.Fatalf("nil payload should yield community, got %s", ent.Tier)
	}
	if ent.Source != SourceCommunity {
		t.Fatalf("nil payload should yield community source, got %q", ent.Source)
	}
}
