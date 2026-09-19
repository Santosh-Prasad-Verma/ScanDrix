// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Feature Gate Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package featuregate

import (
	"context"
	"log/slog"
	"os"

	"github.com/scandrix/backend/internal/telemetry/product"
)

// FeatureCheckContext encapsulates tenant, identity, and group metadata for flag checks.
type FeatureCheckContext struct {
	Identifier              string
	OrganizationAndTeamData any
	Groups                  map[string]string
	ReleaseTrack            *ReleaseTrack
}

// FeatureGateService evaluates feature access using catalog snapshots, release tracks, and PostHog.
type FeatureGateService struct {
	logger                *slog.Logger
	snapshot              *FeaturesSnapshot
	posthog               product.PostHogClient
	cloudMode             bool
	selfHostedBetaEnabled bool
}

// NewFeatureGateService constructs an operational FeatureGateService.
func NewFeatureGateService(posthog product.PostHogClient, snapshot *FeaturesSnapshot, logger *slog.Logger) *FeatureGateService {
	if logger == nil {
		logger = slog.Default()
	}
	if snapshot == nil {
		snapshot = LoadSnapshot()
	}
	cloudMode := os.Getenv("API_CLOUD_MODE") == "true"
	betaEnabled := os.Getenv("BETA_FEATURES") == "true"

	return &FeatureGateService{
		logger:                logger.With("component", "feature_gate_service"),
		snapshot:              snapshot,
		posthog:               posthog,
		cloudMode:             cloudMode,
		selfHostedBetaEnabled: betaEnabled,
	}
}

// NewFeatureGateServiceWithConfig constructs a service with explicit environment parameters.
func NewFeatureGateServiceWithConfig(
	posthog product.PostHogClient,
	snapshot *FeaturesSnapshot,
	cloudMode bool,
	selfHostedBetaEnabled bool,
	logger *slog.Logger,
) *FeatureGateService {
	if logger == nil {
		logger = slog.Default()
	}
	if snapshot == nil {
		snapshot = LoadSnapshot()
	}
	return &FeatureGateService{
		logger:                logger.With("component", "feature_gate_service"),
		snapshot:              snapshot,
		posthog:               posthog,
		cloudMode:             cloudMode,
		selfHostedBetaEnabled: selfHostedBetaEnabled,
	}
}

// IsEnabled checks whether a feature is permitted for the given context.
func (s *FeatureGateService) IsEnabled(ctx context.Context, feature FeatureKey, checkCtx FeatureCheckContext) (bool, error) {
	entry, _ := FindFeature(s.snapshot, string(feature))

	audience := FeatureAudienceSelfHosted
	if s.cloudMode {
		audience = FeatureAudienceCloud
	}

	decision := EvaluateCatalogGate(GateInputs{
		Entry:                 entry,
		Audience:              audience,
		Track:                 checkCtx.ReleaseTrack,
		SelfHostedBetaEnabled: s.selfHostedBetaEnabled,
	})

	if decision == GateDecisionDeny {
		return false, nil
	}

	if audience == FeatureAudienceSelfHosted {
		return true, nil
	}

	// Cloud audience: query PostHog with fallback
	if s.posthog == nil {
		return CloudFallbackOnPosthogError(decision, entry), nil
	}

	var evalCtx *product.FeatureEvaluationContext
	if len(checkCtx.Groups) > 0 {
		evalCtx = &product.FeatureEvaluationContext{
			Groups: checkCtx.Groups,
		}
	}

	enabled, err := s.posthog.IsFeatureEnabled(ctx, string(feature), checkCtx.Identifier, checkCtx.OrganizationAndTeamData, evalCtx)
	if err != nil {
		s.logger.Warn("PostHog feature flag check failed, falling back to snapshot",
			"feature", feature,
			"error", err,
		)
		return CloudFallbackOnPosthogError(decision, entry), nil
	}

	return enabled, nil
}
