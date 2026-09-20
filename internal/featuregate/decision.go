// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Feature Gate Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package featuregate

// GateDecision represents the outcome of a deterministic catalog gate check.
type GateDecision string

const (
	GateDecisionDeny       GateDecision = "deny"
	GateDecisionPass       GateDecision = "pass"
	GateDecisionCompatPass GateDecision = "compat-pass"
)

// GateInputs provides the inputs required for catalog gate evaluation.
type GateInputs struct {
	Entry                 *SnapshotFeature
	Audience              FeatureAudience
	Track                 *ReleaseTrack
	SelfHostedBetaEnabled bool
}

// EvaluateCatalogGate computes the deterministic gating decision.
func EvaluateCatalogGate(inputs GateInputs) GateDecision {
	if inputs.Entry == nil {
		return GateDecisionCompatPass
	}

	if len(inputs.Entry.Audience) > 0 {
		allowed := false
		for _, aud := range inputs.Entry.Audience {
			if aud == inputs.Audience {
				allowed = true
				break
			}
		}
		if !allowed {
			return GateDecisionDeny
		}
	}

	if inputs.Audience == FeatureAudienceSelfHosted {
		switch inputs.Entry.Stage {
		case StageGeneralAvailability:
			return GateDecisionPass
		case StageBeta:
			if inputs.SelfHostedBetaEnabled {
				return GateDecisionPass
			}
			return GateDecisionDeny
		case StageAlpha:
			fallthrough
		default:
			return GateDecisionDeny
		}
	}

	effectiveTrack := DefaultReleaseTrack
	if inputs.Track != nil && IsReleaseTrack(string(*inputs.Track)) {
		effectiveTrack = *inputs.Track
	}

	if TrackPermitsStage(effectiveTrack, inputs.Entry.Stage) {
		return GateDecisionPass
	}
	return GateDecisionDeny
}

// CloudFallbackOnPosthogError provides historical permissive fallback when PostHog is unreachable in cloud mode.
func CloudFallbackOnPosthogError(decision GateDecision, entry *SnapshotFeature) bool {
	if decision == GateDecisionCompatPass {
		return true
	}
	if decision == GateDecisionDeny {
		return false
	}
	return entry != nil && entry.Stage == StageGeneralAvailability
}
