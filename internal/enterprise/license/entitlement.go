package license

import (
	"fmt"
	"sort"
	"time"
)

// EntitlementSource identifies which authority produced an Entitlement.
type EntitlementSource string

const (
	// SourceCommunity is the default answer when no license and no plan exist.
	SourceCommunity EntitlementSource = "community"
	// SourceSigned is an Ed25519-signed license file (self-hosted / air-gapped).
	SourceSigned EntitlementSource = "signed"
	// SourcePlan is a database plan_configurations row (cloud / subscription).
	SourcePlan EntitlementSource = "plan"
)

// Entitlement is the single resolved answer to "what may this workspace do".
// Both the signed-license path and the database plan path produce one of these,
// so gating code never has to know which authority is in play.
type Entitlement struct {
	Tier            LicenseTier          `json:"tier"`
	Valid           bool                 `json:"valid"`
	ExpiresAt       time.Time            `json:"expires_at"`
	MaxSeats        int                  `json:"max_seats"`        // 0 = unlimited
	MaxRepositories int                  `json:"max_repositories"` // 0 = unlimited
	Quota           PlanQuota            `json:"quota"`
	Features        map[FeatureFlag]bool `json:"features"`
	Source          EntitlementSource    `json:"source"`
	Reason          string               `json:"reason,omitempty"`
}

// Community features are available on every plan, including the unlicensed
// Community plan. BYOK is deliberately not a paid capability: a customer
// supplying their own provider key funds that inference themselves.
var communityFeatures = []FeatureFlag{FeatureBYOK}

// CommunityEntitlement returns the default entitlement for a workspace with no
// license and no plan. It is valid and unlocks only the community features.
func CommunityEntitlement() *Entitlement {
	return &Entitlement{
		Tier:            TierCommunity,
		Valid:           true,
		MaxSeats:        QuotaCommunity.MaxSeats,
		MaxRepositories: QuotaCommunity.MaxRepositories,
		Quota:           QuotaCommunity,
		Features:        newFeatureSet(),
		Source:          SourceCommunity,
	}
}

// EntitlementFromLicense resolves an Entitlement from a signed license payload.
func EntitlementFromLicense(p *LicensePayload) *Entitlement {
	if p == nil {
		return CommunityEntitlement()
	}

	tier := NormalizeTier(p.Tier)
	ent := &Entitlement{
		Tier:            tier,
		ExpiresAt:       p.ExpiresAt,
		MaxSeats:        p.MaxSeats,
		MaxRepositories: p.MaxRepositories,
		Quota:           GetPlanQuota(tier),
		Features:        newFeatureSet(),
		Source:          SourceSigned,
	}

	// Grace period matches LoadLicense; an expired license keeps its tier for
	// display but unlocks nothing, because Allows denies an invalid entitlement.
	if time.Now().UTC().After(p.ExpiresAt.Add(LicenseGracePeriod)) {
		ent.Valid = false
		ent.Reason = fmt.Sprintf("license expired on %s", p.ExpiresAt.UTC().Format(time.RFC3339))
		return ent
	}

	ent.Valid = true
	for _, f := range p.Features {
		ent.Features[FeatureFlag(f)] = true
	}
	return ent
}

// EntitlementFromPlan resolves an Entitlement from a database plan row.
func EntitlementFromPlan(tier LicenseTier, features []string, maxSeats, maxRepositories int, expiresAt time.Time) *Entitlement {
	normalized := NormalizeTier(tier)
	ent := &Entitlement{
		Tier:            normalized,
		ExpiresAt:       expiresAt,
		MaxSeats:        maxSeats,
		MaxRepositories: maxRepositories,
		Quota:           GetPlanQuota(normalized),
		Features:        newFeatureSet(),
		Source:          SourcePlan,
	}

	// The plan row honours the same grace window as a signed license, so the two
	// sources cannot disagree about whether a lapsed subscription still works.
	// GetWorkspacePlanDetails independently reports a GRACE_PERIOD state over the
	// same constant; before this, EntitlementFromPlan cut off at the instant of
	// expiry while the signed path stayed valid for LicenseGracePeriod, which
	// meant the same customer was entitled on one path and not the other.
	if !expiresAt.IsZero() && time.Now().UTC().After(expiresAt.Add(LicenseGracePeriod)) {
		ent.Valid = false
		ent.Reason = fmt.Sprintf("plan expired on %s (grace period of %s exceeded)",
			expiresAt.UTC().Format(time.RFC3339), LicenseGracePeriod)
		return ent
	}

	ent.Valid = true
	for _, f := range features {
		ent.Features[FeatureFlag(f)] = true
	}
	return ent
}

// FeatureList returns the entitled feature flags as a sorted string slice, for
// serialization in API responses.
func (e *Entitlement) FeatureList() []string {
	out := make([]string, 0, len(e.Features))
	for flag, granted := range e.Features {
		if granted && e.Allows(flag) {
			out = append(out, string(flag))
		}
	}
	sort.Strings(out)
	return out
}

// IsCommunityFeature reports whether a feature is available on every plan.
func IsCommunityFeature(flag FeatureFlag) bool {
	for _, f := range communityFeatures {
		if f == flag {
			return true
		}
	}
	return false
}

// newFeatureSet returns a feature map seeded with the community features.
func newFeatureSet() map[FeatureFlag]bool {
	features := make(map[FeatureFlag]bool, len(communityFeatures)+4)
	for _, flag := range communityFeatures {
		features[flag] = true
	}
	return features
}

// Allows reports whether a gated feature is entitled. An invalid entitlement
// unlocks nothing. Community features are entitled on every valid plan.
func (e *Entitlement) Allows(flag FeatureFlag) bool {
	if e == nil || !e.Valid {
		return false
	}
	if IsCommunityFeature(flag) {
		return true
	}
	if e.Tier == TierEnterprise {
		return true
	}
	return e.Features[flag]
}

// AssertAllows returns a descriptive error when a gated feature is not entitled.
func (e *Entitlement) AssertAllows(flag FeatureFlag) error {
	if e.Allows(flag) {
		return nil
	}
	tier := TierCommunity
	if e != nil {
		tier = e.Tier
	}
	return fmt.Errorf("feature '%s' is not entitled under current %s license", flag, tier)
}

// SeatLimit returns the seat cap, where 0 means unlimited.
func (e *Entitlement) SeatLimit() int {
	if e == nil {
		return QuotaCommunity.MaxSeats
	}
	return e.MaxSeats
}

// RepoLimit returns the repository cap, where 0 means unlimited.
func (e *Entitlement) RepoLimit() int {
	if e == nil {
		return QuotaCommunity.MaxRepositories
	}
	return e.MaxRepositories
}

// CheckSeats validates an active seat count against the entitlement.
func (e *Entitlement) CheckSeats(activeSeats int) error {
	limit := e.SeatLimit()
	if limit > 0 && activeSeats > limit {
		return fmt.Errorf("license seat quota (%d) exceeded: %d active seats", limit, activeSeats)
	}
	return nil
}

// CheckRepositories validates an active repository count against the entitlement.
func (e *Entitlement) CheckRepositories(activeRepos int) error {
	limit := e.RepoLimit()
	if limit > 0 && activeRepos > limit {
		return fmt.Errorf("license repository quota (%d) exceeded: %d active repos", limit, activeRepos)
	}
	return nil
}

// Entitlement resolves the current active license into an Entitlement.
func (m *LicenseManager) Entitlement() *Entitlement {
	return EntitlementFromLicense(m.GetActiveLicense())
}
