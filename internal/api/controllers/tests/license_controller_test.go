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
	// These routes are owner/admin only (auth.RoleGuard), so the test acts as an
	// owner. Without a profile the guard denies before the handler runs.
	ctx := auth.WithWorkspaceContext(req.Context(), wsID)
	ctx = auth.WithAccountContext(ctx, &models.AccountProfile{WorkspaceID: wsID, Role: models.RoleOwner})
	req = req.WithContext(ctx)
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

// GET / must surface the stored license row's own fields. It previously seeded
// OrganizationName from the tier, which made an unlicensed workspace report its
// plan as its company name.
func TestGetLicenseSurfacesStoredRowAndNeverSeedsOrgNameFromTier(t *testing.T) {
	wsID := uuid.New()
	pub, _, _ := ed25519.GenerateKey(rand.Reader)

	t.Run("licensed workspace reports the stored organization name", func(t *testing.T) {
		repo := &mockLicenseRepo{
			license: &models.OrganizationLicense{
				LicenseKey:       "lic_live_key",
				OrganizationName: "Acme Corp",
				PlanTier:         string(license.TierTeam),
				TotalSeats:       25,
				AllocatedSeats:   11,
				IsAirGapped:      true,
				ExpiresAt:        time.Now().UTC().Add(24 * time.Hour),
			},
		}
		ctrl := controllers.NewLicenseController(repo).WithVerifier(license.NewLicenseManager(pub))

		rec := doLicenseRequest(t, ctrl, http.MethodGet, "/", "", wsID)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
		}

		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed decoding response: %v", err)
		}

		if body["organization_name"] != "Acme Corp" {
			t.Errorf("expected the stored organization name, got %v", body["organization_name"])
		}
		if body["plan_tier"] != string(license.TierTeam) {
			t.Errorf("expected TEAM, got %v", body["plan_tier"])
		}
		if body["allocated_seats"] != float64(11) {
			t.Errorf("expected allocated_seats=11, got %v", body["allocated_seats"])
		}
		if body["total_seats"] != float64(25) {
			t.Errorf("expected total_seats=25, got %v", body["total_seats"])
		}
		if body["is_air_gapped"] != true {
			t.Errorf("expected is_air_gapped=true, got %v", body["is_air_gapped"])
		}
		if body["license_key"] != "lic_live_key" {
			t.Errorf("expected the stored license key, got %v", body["license_key"])
		}
	})

	t.Run("unlicensed workspace reports an empty organization name", func(t *testing.T) {
		ctrl := controllers.NewLicenseController(&mockLicenseRepo{}).WithVerifier(license.NewLicenseManager(pub))

		rec := doLicenseRequest(t, ctrl, http.MethodGet, "/", "", wsID)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
		}

		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed decoding response: %v", err)
		}

		if body["organization_name"] != "" {
			t.Fatalf("an unlicensed workspace must not report an organization name, got %v", body["organization_name"])
		}
		if body["license_key"] != "" {
			t.Fatalf("an unlicensed workspace must not report a license key, got %v", body["license_key"])
		}
		if body["allocated_seats"] != float64(0) {
			t.Fatalf("expected allocated_seats=0, got %v", body["allocated_seats"])
		}
		if body["plan_tier"] != string(license.TierCommunity) {
			t.Fatalf("expected COMMUNITY for an unlicensed workspace, got %v", body["plan_tier"])
		}
	})
}

// A signed license must win over the stored row for the entitlement fields, and
// the stored row must still supply the purely descriptive ones.
func TestGetLicensePrefersSignedLicenseForEntitlementFields(t *testing.T) {
	wsID := uuid.New()
	repo := &mockLicenseRepo{
		license: &models.OrganizationLicense{
			OrganizationName: "Acme Corp",
			PlanTier:         string(license.TierTeam),
			TotalSeats:       25,
			ExpiresAt:        time.Now().UTC().Add(24 * time.Hour),
		},
	}
	token, pub := issueTestLicense(t, license.LicensePayload{
		Tier:      license.TierEnterprise,
		MaxSeats:  500,
		Features:  []string{string(license.FeatureSCIM)},
		IssuedAt:  time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(365 * 24 * time.Hour),
	})
	mgr := license.NewLicenseManager(pub)
	if _, err := mgr.LoadLicense(token); err != nil {
		t.Fatalf("failed loading license: %v", err)
	}
	ctrl := controllers.NewLicenseController(repo).WithVerifier(mgr)

	rec := doLicenseRequest(t, ctrl, http.MethodGet, "/", "", wsID)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed decoding response: %v", err)
	}

	if body["plan_tier"] != string(license.TierEnterprise) {
		t.Errorf("expected the signed ENTERPRISE tier, got %v", body["plan_tier"])
	}
	if body["total_seats"] != float64(500) {
		t.Errorf("expected 500 seats from the signed license, got %v", body["total_seats"])
	}
	if body["organization_name"] != "Acme Corp" {
		t.Errorf("expected the stored organization name, got %v", body["organization_name"])
	}
}

// Every workspace-scoped licensing route must fail closed without a workspace
// rather than reporting a tier derived from nothing.
func TestLicenseRoutesRejectMissingWorkspaceContext(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	ctrl := controllers.NewLicenseController(&mockLicenseRepo{}).WithVerifier(license.NewLicenseManager(pub))

	cases := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/", ""},
		{http.MethodPost, "/activate", `{"license_key":"x"}`},
		{http.MethodGet, "/seats", ""},
		{http.MethodGet, "/status", ""},
		{http.MethodGet, "/org-status", ""},
		{http.MethodPost, "/assign", `{"users":[]}`},
		{http.MethodGet, "/removable-seats", ""},
		{http.MethodPost, "/prune-seats", `{"keepUserIds":[]}`},
		{http.MethodPost, "/trial-extension-request", `{"reason":"growth"}`},
	}

	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			rec := httptest.NewRecorder()
			ctrl.Routes().ServeHTTP(rec, req)

			// The owner/admin guard is middleware, so it runs before the handler's
			// own workspace check and answers 403. It cannot distinguish "no
			// credentials" from "no role" -- both present as a missing profile --
			// and these routes sit behind JWT middleware in production, where an
			// unauthenticated request is rejected upstream with 401 before it
			// reaches this router. Denying here is fail-closed, not a weakening.
			if rec.Code != http.StatusForbidden && rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected the request to be rejected without a workspace context, got %d (%s)", rec.Code, rec.Body.String())
			}
		})
	}
}

// GET /users exists so the UI can render the licensed-user table. It returns an
// empty list rather than inventing rows, and must do so as a JSON array.
func TestGetUsersWithLicenseReturnsEmptyArrayNotNil(t *testing.T) {
	wsID := uuid.New()
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	ctrl := controllers.NewLicenseController(&mockLicenseRepo{}).WithVerifier(license.NewLicenseManager(pub))

	rec := doLicenseRequest(t, ctrl, http.MethodGet, "/users", "", wsID)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
		t.Fatalf("expected a JSON array, got %q", got)
	}
}

// WithResolver must be honoured, so a prebuilt resolver injected by the router
// is the one that answers -- the controller must not silently rebuild its own.
func TestWithResolverIsHonouredOverTheStoreAdapter(t *testing.T) {
	wsID := uuid.New()
	token, pub := issueTestLicense(t, license.LicensePayload{
		Tier:      license.TierScale,
		MaxSeats:  100,
		IssuedAt:  time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(365 * 24 * time.Hour),
	})
	mgr := license.NewLicenseManager(pub)
	if _, err := mgr.LoadLicense(token); err != nil {
		t.Fatalf("failed loading license: %v", err)
	}

	// A store that would report COMMUNITY, paired with a resolver that reports
	// SCALE. The response proves which one was consulted.
	injected := controllers.NewLicenseController(&mockLicenseRepo{}).WithResolver(
		license.NewResolver(mgr, nil),
	)

	rec := doLicenseRequest(t, injected, http.MethodGet, "/", "", wsID)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed decoding response: %v", err)
	}
	if body["plan_tier"] != string(license.TierScale) {
		t.Fatalf("expected the injected resolver's SCALE tier, got %v", body["plan_tier"])
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
