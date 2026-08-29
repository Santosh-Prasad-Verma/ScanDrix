package featuregate_test

import (
	"testing"

	"github.com/scandrix/backend/internal/featuregate"
)

func TestFeatureGateEngine(t *testing.T) {
	engine := featuregate.NewFeatureGateEngine()

	// 1. Stable feature on cloud
	stableCtx := featuregate.EvaluationContext{
		WorkspaceID: "ws-123",
		Track:       featuregate.TrackStable,
		Audience:    featuregate.AudienceCloud,
	}

	if !engine.IsEnabled("dora_executive_cockpit", stableCtx) {
		t.Fatal("expected stable cloud user to access dora_executive_cockpit")
	}

	// 2. Audience mismatch: self-hosted accessing cloud-only feature -> Denied
	selfHostedCtx := featuregate.EvaluationContext{
		WorkspaceID: "ws-123",
		Track:       featuregate.TrackStable,
		Audience:    featuregate.AudienceSelfHosted,
	}
	if engine.IsEnabled("dora_executive_cockpit", selfHostedCtx) {
		t.Fatal("expected self-hosted user to be denied cloud-only feature")
	}

	// 3. Track gating: Alpha user accessing Beta-minimum feature -> Denied
	alphaCtx := featuregate.EvaluationContext{
		WorkspaceID: "ws-123",
		Track:       featuregate.TrackAlpha,
		Audience:    featuregate.AudienceCloud,
	}
	if engine.IsEnabled("deep_ast_reasoning", alphaCtx) {
		t.Fatal("expected alpha user to be denied beta-minimum feature")
	}

	// 4. Percentage rollout bucketing
	engine.SetFlag(featuregate.FeatureFlag{
		Key:             "experimental_scanner",
		MinTrack:        featuregate.TrackAlpha,
		AllowedAudience: featuregate.AudienceCloud,
		RolloutPercent:  50,
		Enabled:         true,
	})

	// Run across 10 distinct workspaces, expect ~50% enablement
	enabledCount := 0
	for i := 0; i < 20; i++ {
		ctx := featuregate.EvaluationContext{
			WorkspaceID: string(rune('A' + i)),
			Track:       featuregate.TrackAlpha,
			Audience:    featuregate.AudienceCloud,
		}
		if engine.IsEnabled("experimental_scanner", ctx) {
			enabledCount++
		}
	}

	if enabledCount == 0 || enabledCount == 20 {
		t.Fatalf("expected gradual rollout split, got %d/20 enabled", enabledCount)
	}
}
