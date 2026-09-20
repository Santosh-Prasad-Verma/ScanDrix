// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Feature Gate Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package featuregate

// DefaultFeaturesSnapshot returns the compiled baseline snapshot of platform features.
func DefaultFeaturesSnapshot() *FeaturesSnapshot {
	return &FeaturesSnapshot{
		SchemaVersion: 1,
		GeneratedAt:   "2026-08-11T19:06:02.620Z",
		Source:        "manual",
		Features: map[string]SnapshotFeature{
			string(FeatureKeyGithubEnterpriseServerPAT): {
				Name:        "GitHub Enterprise Server (PAT auth)",
				Stage:       StageBeta,
				Description: "Connect a GitHub Enterprise Server installation using a personal access token, for environments where the GitHub App flow isn't available.",
				Audience: []FeatureAudience{
					FeatureAudienceCloud,
					FeatureAudienceSelfHosted,
				},
			},
			string(FeatureKeyHeavyReview): {
				Name:        "Heavy review (deeper pass)",
				Stage:       StageAlpha,
				Description: "An opt-in deeper review that re-runs the finder multiple times to surface more real bugs, for the changes where catching everything matters more than speed.",
				Audience: []FeatureAudience{
					FeatureAudienceCloud,
					FeatureAudienceSelfHosted,
				},
			},
			string(FeatureKeyScanDrixTraceReviewContext): {
				Name:        "ScanDrix Trace review context",
				Stage:       StageAlpha,
				Description: "Use sanitized decisions distilled from local coding-agent sessions as repository-scoped context for Drixy code reviews.",
				Audience: []FeatureAudience{
					FeatureAudienceCloud,
				},
			},
		},
	}
}
