package license_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/enterprise/license"
)

func TestLicenseVerificationAndFeatureGating(t *testing.T) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed generating keypair: %v", err)
	}

	mgr := license.NewLicenseManager(pubKey)

	// 1. Initial State: Unlicensed Community Tier
	if mgr.GetTier() != license.TierCommunity {
		t.Fatalf("expected community tier by default, got %s", mgr.GetTier())
	}
	if mgr.HasFeature(license.FeatureSSOSAML) {
		t.Fatal("community tier must not have SSO SAML")
	}
	if err := mgr.CheckSeatLimit(3); err != nil {
		t.Fatalf("expected 3 seats allowed on community, got %v", err)
	}
	if err := mgr.CheckSeatLimit(10); err == nil {
		t.Fatal("expected 10 seats rejected on community (limit 5)")
	}

	// 2. Issue and Load Valid Team License
	now := time.Now().UTC()
	teamPayload := license.LicensePayload{
		LicenseID:       uuid.New(),
		CustomerName:    "Acme Corp",
		CustomerID:      "cust_acme_123",
		Tier:            license.TierTeam,
		IssuedAt:        now,
		ExpiresAt:       now.Add(365 * 24 * time.Hour),
		MaxSeats:        25,
		MaxRepositories: 10,
		Features: []string{
			string(license.FeatureCustomRules),
			string(license.FeatureBYOK),
		},
	}

	teamToken, err := license.IssueLicense(teamPayload, privKey)
	if err != nil {
		t.Fatalf("failed issuing team license: %v", err)
	}

	loaded, err := mgr.LoadLicense(teamToken)
	if err != nil {
		t.Fatalf("failed loading team license: %v", err)
	}
	if loaded.CustomerName != "Acme Corp" {
		t.Fatalf("loaded customer mismatch: %s", loaded.CustomerName)
	}
	if mgr.GetTier() != license.TierTeam {
		t.Fatalf("expected tier TEAM, got %s", mgr.GetTier())
	}

	// Feature Gating Checks for Team License
	if !mgr.HasFeature(license.FeatureCustomRules) {
		t.Fatal("expected CustomRules enabled for Team license")
	}
	if !mgr.HasFeature(license.FeatureBYOK) {
		t.Fatal("expected BYOK enabled for Team license")
	}
	if mgr.HasFeature(license.FeatureSSOSAML) {
		t.Fatal("expected SSO SAML disabled for Team license")
	}
	if err := mgr.AssertFeature(license.FeatureSSOSAML); err == nil {
		t.Fatal("expected AssertFeature error for gated SSO SAML")
	}

	// Quota Checks
	if err := mgr.CheckSeatLimit(20); err != nil {
		t.Fatalf("expected 20 seats allowed (limit 25): %v", err)
	}
	if err := mgr.CheckSeatLimit(26); err == nil {
		t.Fatal("expected 26 seats rejected (limit 25)")
	}

	// 3. Issue and Load Enterprise License (Unlocks All)
	entPayload := license.LicensePayload{
		LicenseID:       uuid.New(),
		CustomerName:    "Global Megacorp",
		CustomerID:      "cust_mega_999",
		Tier:            license.TierEnterprise,
		IssuedAt:        now,
		ExpiresAt:       now.Add(365 * 24 * time.Hour),
		MaxSeats:        0, // unlimited
		MaxRepositories: 0, // unlimited
	}

	entToken, err := license.IssueLicense(entPayload, privKey)
	if err != nil {
		t.Fatalf("failed issuing enterprise license: %v", err)
	}

	_, err = mgr.LoadLicense(entToken)
	if err != nil {
		t.Fatalf("failed loading enterprise license: %v", err)
	}
	if mgr.GetTier() != license.TierEnterprise {
		t.Fatalf("expected tier ENTERPRISE, got %s", mgr.GetTier())
	}

	// Enterprise tier unlocks all features unconditionally
	allFeatures := []license.FeatureFlag{
		license.FeatureSSOSAML,
		license.FeatureSCIM,
		license.FeatureAuditWarehouse,
		license.FeatureMultiAgentDeliberation,
		license.FeatureBYOK,
		license.FeatureCustomRules,
		license.FeatureDORAMetrics,
		license.FeatureAirGapped,
	}
	for _, f := range allFeatures {
		if !mgr.HasFeature(f) {
			t.Fatalf("expected %s unlocked on Enterprise", f)
		}
	}
	if err := mgr.CheckSeatLimit(5000); err != nil {
		t.Fatalf("expected unlimited seats allowed on Enterprise: %v", err)
	}

	// 4. Tamper Resistance Test: Tampering with payload fails validation
	rawTokenBytes, _ := base64.StdEncoding.DecodeString(teamToken)
	var tok license.SignedLicenseToken
	_ = json.Unmarshal(rawTokenBytes, &tok)

	// Tamper: unpack payload, modify MaxSeats to 99999, re-encode without signature
	rawPayloadBytes, _ := base64.StdEncoding.DecodeString(tok.Payload)
	var tamperedPayload license.LicensePayload
	_ = json.Unmarshal(rawPayloadBytes, &tamperedPayload)
	tamperedPayload.MaxSeats = 99999
	newPayloadBytes, _ := json.Marshal(tamperedPayload)
	tok.Payload = base64.StdEncoding.EncodeToString(newPayloadBytes)

	tamperedTokBytes, _ := json.Marshal(tok)
	tamperedTokenStr := base64.StdEncoding.EncodeToString(tamperedTokBytes)

	_, err = mgr.LoadLicense(tamperedTokenStr)
	if err == nil {
		t.Fatal("tamper resistance failure: expected error on tampered license token, but load succeeded")
	}

	// 5. Expiration Test (Beyond 72-hour grace period)
	expiredPayload := license.LicensePayload{
		LicenseID:    uuid.New(),
		CustomerName: "Expired Corp",
		Tier:         license.TierTeam,
		IssuedAt:     now.Add(-30 * 24 * time.Hour),
		ExpiresAt:    now.Add(-10 * 24 * time.Hour), // Expired 10 days ago
	}
	expiredToken, _ := license.IssueLicense(expiredPayload, privKey)
	_, err = mgr.LoadLicense(expiredToken)
	if err == nil {
		t.Fatal("expected expired license to be rejected")
	}
}
