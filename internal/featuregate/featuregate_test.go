// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Feature Gate Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package featuregate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/scandrix/backend/internal/telemetry/product"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockPostHogClient struct {
	enabledFlagMap map[string]bool
	fail           bool
}

func (m *mockPostHogClient) IsEnabled() bool { return true }
func (m *mockPostHogClient) Capture(ctx context.Context, distinctID string, event string, properties map[string]any, groups map[string]string) error {
	return nil
}
func (m *mockPostHogClient) Identify(ctx context.Context, distinctID string, properties map[string]any) error {
	return nil
}
func (m *mockPostHogClient) GroupIdentify(ctx context.Context, groupType string, groupKey string, properties map[string]any) error {
	return nil
}
func (m *mockPostHogClient) IsFeatureEnabled(ctx context.Context, featureName string, identifier string, orgTeamData any, evalCtx *product.FeatureEvaluationContext) (bool, error) {
	if m.fail {
		return false, errors.New("posthog unreachable")
	}
	val, ok := m.enabledFlagMap[featureName]
	if !ok {
		return false, nil
	}
	return val, nil
}

func TestEvaluateCatalogGate(t *testing.T) {
	trackAlpha := TrackAlpha
	trackBeta := TrackBeta
	trackStable := TrackStable

	gaFeature := &SnapshotFeature{
		Name:     "GA Feature",
		Stage:    StageGeneralAvailability,
		Audience: []FeatureAudience{FeatureAudienceCloud, FeatureAudienceSelfHosted},
	}
	betaFeature := &SnapshotFeature{
		Name:     "Beta Feature",
		Stage:    StageBeta,
		Audience: []FeatureAudience{FeatureAudienceCloud, FeatureAudienceSelfHosted},
	}
	alphaFeature := &SnapshotFeature{
		Name:     "Alpha Feature",
		Stage:    StageAlpha,
		Audience: []FeatureAudience{FeatureAudienceCloud, FeatureAudienceSelfHosted},
	}
	cloudOnlyFeature := &SnapshotFeature{
		Name:     "Cloud Only Feature",
		Stage:    StageGeneralAvailability,
		Audience: []FeatureAudience{FeatureAudienceCloud},
	}

	// 1. Missing entry -> compat-pass
	assert.Equal(t, GateDecisionCompatPass, EvaluateCatalogGate(GateInputs{
		Entry:    nil,
		Audience: FeatureAudienceCloud,
	}))

	// 2. Audience mismatch -> deny
	assert.Equal(t, GateDecisionDeny, EvaluateCatalogGate(GateInputs{
		Entry:    cloudOnlyFeature,
		Audience: FeatureAudienceSelfHosted,
	}))

	// 3. Self-hosted: GA passes, Alpha denied
	assert.Equal(t, GateDecisionPass, EvaluateCatalogGate(GateInputs{
		Entry:    gaFeature,
		Audience: FeatureAudienceSelfHosted,
	}))
	assert.Equal(t, GateDecisionDeny, EvaluateCatalogGate(GateInputs{
		Entry:    alphaFeature,
		Audience: FeatureAudienceSelfHosted,
	}))

	// 4. Self-hosted Beta: depends on SelfHostedBetaEnabled
	assert.Equal(t, GateDecisionDeny, EvaluateCatalogGate(GateInputs{
		Entry:                 betaFeature,
		Audience:              FeatureAudienceSelfHosted,
		SelfHostedBetaEnabled: false,
	}))
	assert.Equal(t, GateDecisionPass, EvaluateCatalogGate(GateInputs{
		Entry:                 betaFeature,
		Audience:              FeatureAudienceSelfHosted,
		SelfHostedBetaEnabled: true,
	}))

	// 5. Cloud: track checks
	// Alpha track: sees alpha, beta, ga
	assert.Equal(t, GateDecisionPass, EvaluateCatalogGate(GateInputs{
		Entry:    alphaFeature,
		Audience: FeatureAudienceCloud,
		Track:    &trackAlpha,
	}))
	assert.Equal(t, GateDecisionPass, EvaluateCatalogGate(GateInputs{
		Entry:    betaFeature,
		Audience: FeatureAudienceCloud,
		Track:    &trackAlpha,
	}))
	assert.Equal(t, GateDecisionPass, EvaluateCatalogGate(GateInputs{
		Entry:    gaFeature,
		Audience: FeatureAudienceCloud,
		Track:    &trackAlpha,
	}))

	// Beta track: sees beta, ga, but NOT alpha
	assert.Equal(t, GateDecisionDeny, EvaluateCatalogGate(GateInputs{
		Entry:    alphaFeature,
		Audience: FeatureAudienceCloud,
		Track:    &trackBeta,
	}))
	assert.Equal(t, GateDecisionPass, EvaluateCatalogGate(GateInputs{
		Entry:    betaFeature,
		Audience: FeatureAudienceCloud,
		Track:    &trackBeta,
	}))
	assert.Equal(t, GateDecisionPass, EvaluateCatalogGate(GateInputs{
		Entry:    gaFeature,
		Audience: FeatureAudienceCloud,
		Track:    &trackBeta,
	}))

	// Stable track: sees ONLY ga
	assert.Equal(t, GateDecisionDeny, EvaluateCatalogGate(GateInputs{
		Entry:    alphaFeature,
		Audience: FeatureAudienceCloud,
		Track:    &trackStable,
	}))
	assert.Equal(t, GateDecisionDeny, EvaluateCatalogGate(GateInputs{
		Entry:    betaFeature,
		Audience: FeatureAudienceCloud,
		Track:    &trackStable,
	}))
	assert.Equal(t, GateDecisionPass, EvaluateCatalogGate(GateInputs{
		Entry:    gaFeature,
		Audience: FeatureAudienceCloud,
		Track:    &trackStable,
	}))
}

func TestCloudFallbackOnPosthogError(t *testing.T) {
	gaEntry := &SnapshotFeature{Stage: StageGeneralAvailability}
	betaEntry := &SnapshotFeature{Stage: StageBeta}

	assert.True(t, CloudFallbackOnPosthogError(GateDecisionCompatPass, nil))
	assert.False(t, CloudFallbackOnPosthogError(GateDecisionDeny, gaEntry))
	assert.True(t, CloudFallbackOnPosthogError(GateDecisionPass, gaEntry))
	assert.False(t, CloudFallbackOnPosthogError(GateDecisionPass, betaEntry))
}

func TestFeatureGateService_Evaluation(t *testing.T) {
	ctx := context.Background()
	snapshot := DefaultFeaturesSnapshot()

	// 1. Self-hosted mode: beta feature without beta enabled -> denied
	svcSelfHosted := NewFeatureGateServiceWithConfig(nil, snapshot, false, false, nil)
	enabled, err := svcSelfHosted.IsEnabled(ctx, FeatureKeyGithubEnterpriseServerPAT, FeatureCheckContext{
		Identifier: "user-1",
	})
	require.NoError(t, err)
	assert.False(t, enabled)

	// 2. Self-hosted mode: beta feature WITH beta enabled -> permitted
	svcSelfHostedBeta := NewFeatureGateServiceWithConfig(nil, snapshot, false, true, nil)
	enabled, err = svcSelfHostedBeta.IsEnabled(ctx, FeatureKeyGithubEnterpriseServerPAT, FeatureCheckContext{
		Identifier: "user-1",
	})
	require.NoError(t, err)
	assert.True(t, enabled)

	// 3. Cloud mode with PostHog success
	posthogMock := &mockPostHogClient{
		enabledFlagMap: map[string]bool{
			string(FeatureKeyGithubEnterpriseServerPAT): true,
		},
	}
	trackBeta := TrackBeta
	svcCloud := NewFeatureGateServiceWithConfig(posthogMock, snapshot, true, false, nil)
	enabled, err = svcCloud.IsEnabled(ctx, FeatureKeyGithubEnterpriseServerPAT, FeatureCheckContext{
		Identifier:   "user-1",
		ReleaseTrack: &trackBeta,
	})
	require.NoError(t, err)
	assert.True(t, enabled)

	// 4. Cloud mode with PostHog failure -> falls back
	posthogFailing := &mockPostHogClient{fail: true}
	svcCloudFail := NewFeatureGateServiceWithConfig(posthogFailing, snapshot, true, false, nil)
	enabled, err = svcCloudFail.IsEnabled(ctx, FeatureKeyGithubEnterpriseServerPAT, FeatureCheckContext{
		Identifier:   "user-1",
		ReleaseTrack: &trackBeta,
	})
	require.NoError(t, err)
	// Beta feature in cloud on PostHog error should be false (only GA falls back to true)
	assert.False(t, enabled)
}

func TestSnapshotLoader_FileAndDefault(t *testing.T) {
	// 1. Default loader
	s := LoadSnapshot()
	require.NotNil(t, s)
	assert.Equal(t, 1, s.SchemaVersion)
	feat, exists := FindFeature(s, string(FeatureKeyGithubEnterpriseServerPAT))
	assert.True(t, exists)
	assert.Equal(t, StageBeta, feat.Stage)

	// 2. Custom file loader
	tmpDir := t.TempDir()
	customPath := filepath.Join(tmpDir, "custom-features.json")
	customContent := `{"schema_version":1,"generated_at":"2026-01-01T00:00:00Z","source":"test","features":{"custom-flag":{"name":"Custom","stage":"general-availability"}}}`
	err := os.WriteFile(customPath, []byte(customContent), 0600)
	require.NoError(t, err)

	sCustom := LoadSnapshot(WithSnapshotPath(customPath))
	require.NotNil(t, sCustom)
	featCustom, existsCustom := FindFeature(sCustom, "custom-flag")
	assert.True(t, existsCustom)
	assert.Equal(t, StageGeneralAvailability, featCustom.Stage)
}

func TestFeatureKeysAndReleaseTracks(t *testing.T) {
	assert.True(t, IsFeatureKey(string(FeatureKeyHeavyReview)))
	assert.False(t, IsFeatureKey("random-flag"))
	assert.Len(t, AllFeatureKeys(), 3)

	assert.True(t, IsReleaseTrack("alpha"))
	assert.True(t, IsReleaseTrack("beta"))
	assert.True(t, IsReleaseTrack("stable"))
	assert.False(t, IsReleaseTrack("experimental"))
}

func TestLegacyFeatureGateEngine(t *testing.T) {
	engine := NewFeatureGateEngine()
	ctx := EvaluationContext{
		WorkspaceID: "ws-1",
		UserID:      "u-1",
		Track:       TrackStable,
		Audience:    AudienceCloud,
	}

	assert.True(t, engine.IsEnabled("dora_executive_cockpit", ctx))
	// Deep AST reasoning requires TrackBeta, user is TrackStable (rank 3 >= 2) -> true
	assert.True(t, engine.IsEnabled("deep_ast_reasoning", ctx))

	// User on Alpha track (rank 1 < 3) cannot access dora_executive_cockpit
	alphaCtx := EvaluationContext{
		WorkspaceID: "ws-1",
		UserID:      "u-1",
		Track:       TrackAlpha,
		Audience:    AudienceCloud,
	}
	assert.False(t, engine.IsEnabled("dora_executive_cockpit", alphaCtx))
}
