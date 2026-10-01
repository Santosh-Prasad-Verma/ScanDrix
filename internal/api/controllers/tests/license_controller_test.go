// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package controllers_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/enterprise/license"
	"github.com/scandrix/backend/pkg/models"
)

type mockLicenseRepo struct {
	license    *models.OrganizationLicense
	activated  bool
	activatedT string
	activatedS int
	activatedE time.Time
	activatedF []string
	activatedO string
}

func (m *mockLicenseRepo) GetActiveLicense(ctx context.Context, wsID uuid.UUID) (*models.OrganizationLicense, error) {
	return m.license, nil
}

func (m *mockLicenseRepo) ActivateLicense(ctx context.Context, wsID uuid.UUID, licenseKey, orgName, planTier string, totalSeats int, expiresAt time.Time, features []string) error {
	m.activated = true
	m.activatedT = planTier
	m.activatedS = totalSeats
	m.activatedE = expiresAt
	m.activatedF = features
	m.activatedO = orgName
	return nil
}

func issueTestLicense(t *testing.T, payload license.LicensePayload) (token string, pub ed25519.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed generating keypair: %v", err)
	}
	token, err = license.IssueLicense(payload, priv)
	if err != nil {
		t.Fatalf("failed issuing license: %v", err)
	}
	return token, pub
}

func doLicenseRequest(t *testing.T, ctrl *controllers.LicenseController, method, path string, body string, wsID uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req = req.WithContext(auth.WithWorkspaceContext(req.Context(), wsID))
	rec := httptest.NewRecorder()
	ctrl.Routes().ServeHTTP(rec, req)
	return rec
}

func TestLicenseActivateRejectsGarbageKey(t *testing.T) {
	wsID := uuid.New()
	repo := &mockLicenseRepo{}
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keygen failed: %v", err)
	}
	ctrl := controllers.NewLicenseController(repo).
		WithVerifier(license.NewLicenseManager(pub))

	rec := doLicenseRequest(t, ctrl, http.MethodPost, "/activate",
		`{"license_key":"not-a-real-license"}`, wsID)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for an invalid key, got %d", rec.Code)
	}
	if repo.activated {
		t.Fatal("an invalid key must not be persisted")
	}
}

func TestLicenseActivateRejectsEmptyKey(t *testing.T) {
	wsID := uuid.New()
	repo := &mockLicenseRepo{}
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	ctrl := controllers.NewLicenseController(repo).
		WithVerifier(license.NewLicenseManager(pub))

	rec := doLicenseRequest(t, ctrl, http.MethodPost, "/activate", `{"license_key":"  "}`, wsID)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a blank key, got %d", rec.Code)
	}
	if repo.activated {
		t.Fatal("a blank key must not be persisted")
	}
}

func TestLicenseActivatePersistsSignedPayloadNotHardcodedValues(t *testing.T) {
	wsID := uuid.New()
	repo := &mockLicenseRepo{}
	expires := time.Now().UTC().Add(120 * 24 * time.Hour)
	token, pub := issueTestLicense(t, license.LicensePayload{
		LicenseID:    uuid.New(),
		CustomerName: "Globex Industrial",
		Tier:         license.TierScale,
		MaxSeats:     77,
		Features:     []string{string(license.FeatureSCIM)},
		IssuedAt:     time.Now().UTC().Add(-time.Hour),
		ExpiresAt:    expires,
	})
	ctrl := controllers.NewLicenseController(repo).
		WithVerifier(license.NewLicenseManager(pub))

	body, _ := json.Marshal(map[string]string{"license_key": token})
	rec := doLicenseRequest(t, ctrl, http.MethodPost, "/activate", string(body), wsID)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !repo.activated {
		t.Fatal("a valid signed key must be persisted")
	}
	if repo.activatedT != string(license.TierScale) {
		t.Fatalf("expected tier SCALE from the signed payload, got %q", repo.activatedT)
	}
	if repo.activatedS != 77 {
		t.Fatalf("expected 77 seats from the signed payload, got %d", repo.activatedS)
	}
	if repo.activatedO != "Globex Industrial" {
		t.Fatalf("expected customer name from the signed payload, got %q", repo.activatedO)
	}
	if len(repo.activatedF) != 1 || repo.activatedF[0] != string(license.FeatureSCIM) {
		t.Fatalf("expected the signed feature list, got %v", repo.activatedF)
	}
}

func TestLicenseActivateRefusesWhenVerifierAbsent(t *testing.T) {
	wsID := uuid.New()
	repo := &mockLicenseRepo{}
	ctrl := controllers.NewLicenseController(repo)

	rec := doLicenseRequest(t, ctrl, http.MethodPost, "/activate",
		`{"license_key":"anything"}`, wsID)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when verification is unconfigured, got %d", rec.Code)
	}
	if repo.activated {
		t.Fatal("activation must be refused without a verifier")
	}
}

func TestLicenseStatusDoesNotFabricateAnExpiry(t *testing.T) {
	wsID := uuid.New()
	repo := &mockLicenseRepo{}
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	ctrl := controllers.NewLicenseController(repo).
		WithVerifier(license.NewLicenseManager(pub))

	rec := doLicenseRequest(t, ctrl, http.MethodGet, "/status", "", wsID)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed decoding response: %v", err)
	}

	if body["expiresAt"] != nil {
		t.Fatalf("an unlicensed workspace must report a null expiry, got %v", body["expiresAt"])
	}
	if body["plan"] != string(license.TierCommunity) {
		t.Fatalf("expected COMMUNITY, got %v", body["plan"])
	}
	if body["seats"] != float64(license.QuotaCommunity.MaxSeats) {
		t.Fatalf("expected community seat quota %d, got %v", license.QuotaCommunity.MaxSeats, body["seats"])
	}
	features, _ := body["features"].([]any)
	granted := make(map[string]bool, len(features))
	for _, f := range features {
		granted[fmt.Sprint(f)] = true
	}
	if !granted[string(license.FeatureBYOK)] {
		t.Fatalf("community must report BYOK as available, got %v", features)
	}
	for _, premium := range []license.FeatureFlag{
		license.FeatureSSOSAML, license.FeatureSCIM, license.FeatureAuditWarehouse,
		license.FeatureMultiAgentDeliberation, license.FeatureCustomRules,
		license.FeatureDORAMetrics, license.FeatureAirGapped,
	} {
		if granted[string(premium)] {
			t.Fatalf("community must not report premium feature %s", premium)
		}
	}
}

func TestLicenseStatusReportsExpiredLicense(t *testing.T) {
	wsID := uuid.New()
	repo := &mockLicenseRepo{
		license: &models.OrganizationLicense{
			PlanTier:        string(license.TierTeam),
			TotalSeats:      25,
			FeaturesEnabled: []string{string(license.FeatureSCIM)},
			// Beyond the grace window: a plan row stays entitled for
			// LicenseGracePeriod after expiry, so 24 hours is still valid and
			// would not exercise the expired path.
			ExpiresAt: time.Now().UTC().Add(-license.LicenseGracePeriod - 24*time.Hour),
		},
	}
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	ctrl := controllers.NewLicenseController(repo).
		WithVerifier(license.NewLicenseManager(pub))

	rec := doLicenseRequest(t, ctrl, http.MethodGet, "/status", "", wsID)

	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)

	if body["valid"] != false {
		t.Fatalf("expected valid=false for an expired license, got %v", body["valid"])
	}
	if body["subscriptionStatus"] != "expired" {
		t.Fatalf("expected subscriptionStatus=expired, got %v", body["subscriptionStatus"])
	}
}

func TestLicenseSignedLicenseTakesPrecedenceOverStoredRow(t *testing.T) {
	wsID := uuid.New()
	repo := &mockLicenseRepo{
		license: &models.OrganizationLicense{
			PlanTier:   string(license.TierDeveloper),
			TotalSeats: 10,
			ExpiresAt:  time.Now().UTC().Add(24 * time.Hour),
		},
	}
	token, pub := issueTestLicense(t, license.LicensePayload{
		Tier:      license.TierScale,
		MaxSeats:  100,
		Features:  []string{string(license.FeatureAirGapped)},
		IssuedAt:  time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(365 * 24 * time.Hour),
	})
	mgr := license.NewLicenseManager(pub)
	if _, err := mgr.LoadLicense(token); err != nil {
		t.Fatalf("failed loading license: %v", err)
	}
	ctrl := controllers.NewLicenseController(repo).WithVerifier(mgr)

	rec := doLicenseRequest(t, ctrl, http.MethodGet, "/status", "", wsID)

	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)

	if body["plan"] != string(license.TierScale) {
		t.Fatalf("expected the signed license to win with SCALE, got %v", body["plan"])
	}
	if body["source"] != string(license.SourceSigned) {
		t.Fatalf("expected source=signed, got %v", body["source"])
	}
	if body["seats"] != float64(100) {
		t.Fatalf("expected 100 seats from the signed license, got %v", body["seats"])
	}
}

func TestLicenseSeatsReportsUnlimited(t *testing.T) {
	wsID := uuid.New()
	token, pub := issueTestLicense(t, license.LicensePayload{
		Tier:      license.TierEnterprise,
		MaxSeats:  0,
		IssuedAt:  time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(365 * 24 * time.Hour),
	})
	mgr := license.NewLicenseManager(pub)
	if _, err := mgr.LoadLicense(token); err != nil {
		t.Fatalf("failed loading license: %v", err)
	}
	ctrl := controllers.NewLicenseController(&mockLicenseRepo{}).WithVerifier(mgr)

	rec := doLicenseRequest(t, ctrl, http.MethodGet, "/seats", "", wsID)

	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)

	if body["unlimited"] != true {
		t.Fatalf("expected unlimited=true for a 0 seat cap, got %v", body["unlimited"])
	}
	if body["available_seats"] != float64(-1) {
		t.Fatalf("expected available_seats=-1 when unlimited, got %v", body["available_seats"])
	}
}

func TestPublicKeyFromBase64AcceptsPEMArmouredKey(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keygen failed: %v", err)
	}
	encoded := base64.StdEncoding.EncodeToString(pub)
	pem := "-----BEGIN PUBLIC KEY-----\n" + encoded + "\n-----END PUBLIC KEY-----\n"

	decoded, err := license.PublicKeyFromBase64(pem)
	if err != nil {
		t.Fatalf("expected PEM-armoured key to parse: %v", err)
	}
	if !decoded.Equal(pub) {
		t.Fatal("decoded key does not match the original")
	}
}

func TestPublicKeyFromBase64RejectsGarbage(t *testing.T) {
	if _, err := license.PublicKeyFromBase64("not-base64!!"); err == nil {
		t.Fatal("expected an error for a non-base64 public key")
	}
	if _, err := license.PublicKeyFromBase64(""); err == nil {
		t.Fatal("expected an error for an empty public key")
	}
	if _, err := license.PublicKeyFromBase64(base64.StdEncoding.EncodeToString([]byte("too-short"))); err == nil {
		t.Fatal("expected an error for a wrong-length public key")
	}
}
