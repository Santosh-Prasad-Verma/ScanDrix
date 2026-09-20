// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Feature Gate Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package featuregate

// FeatureKey is a canonical feature flag key.
type FeatureKey string

const (
	FeatureKeyGithubEnterpriseServerPAT FeatureKey = "github-enterprise-server-pat"
	FeatureKeyHeavyReview               FeatureKey = "heavy-review"
	FeatureKeyScanDrixTraceReviewContext FeatureKey = "scandrix-trace-review-context"
)

var allFeatureKeys = []FeatureKey{
	FeatureKeyGithubEnterpriseServerPAT,
	FeatureKeyHeavyReview,
	FeatureKeyScanDrixTraceReviewContext,
}

// AllFeatureKeys returns all recognized canonical feature keys.
func AllFeatureKeys() []FeatureKey {
	keys := make([]FeatureKey, len(allFeatureKeys))
	copy(keys, allFeatureKeys)
	return keys
}

// IsFeatureKey checks whether a string is a registered feature key.
func IsFeatureKey(val string) bool {
	for _, k := range allFeatureKeys {
		if string(k) == val {
			return true
		}
	}
	return false
}
