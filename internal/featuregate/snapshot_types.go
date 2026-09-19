// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Feature Gate Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package featuregate

// FeatureStage denotes feature lifecycle maturity.
type FeatureStage string

const (
	StageAlpha               FeatureStage = "alpha"
	StageBeta                FeatureStage = "beta"
	StageGeneralAvailability FeatureStage = "general-availability"
)

// FeatureAudience specifies target deployment environment.
type FeatureAudience string

const (
	FeatureAudienceCloud      FeatureAudience = "cloud"
	FeatureAudienceSelfHosted FeatureAudience = "self-hosted"
)

// SnapshotFeature details an individual feature flag entry in the catalog.
type SnapshotFeature struct {
	Name             string            `json:"name"`
	Stage            FeatureStage      `json:"stage"`
	Description      string            `json:"description,omitempty"`
	DocumentationURL string            `json:"documentation_url,omitempty"`
	Audience         []FeatureAudience `json:"audience,omitempty"`
	PromotedAt       map[string]string `json:"promoted_at,omitempty"`
	PRRefs           []int             `json:"pr_refs,omitempty"`
	FeatureFlagID    *int              `json:"feature_flag_id,omitempty"`
}

// FeaturesSnapshot represents the catalog snapshot file format.
type FeaturesSnapshot struct {
	SchemaVersion int                        `json:"schema_version"`
	GeneratedAt   string                     `json:"generated_at"`
	Source        string                     `json:"source"`
	Features      map[string]SnapshotFeature `json:"features"`
}
