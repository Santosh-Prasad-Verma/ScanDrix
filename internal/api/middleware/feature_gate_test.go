// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package middleware

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/enterprise/license"
)

type staticSource struct {
	ent *license.Entitlement
}

func (s staticSource) Resolve(ctx context.Context, wsID uuid.UUID) *license.Entitlement {
	return s.ent
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
}

func gated(source EntitlementSource, flag license.FeatureFlag) http.Handler {
	return RequireFeature(source, flag)(okHandler())
}

func requestWithWorkspace(withWorkspace bool) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	if withWorkspace {
		req = req.WithContext(auth.WithWorkspaceContext(req.Context(), uuid.New()))
	}
	return req
}

func TestRequireFeatureDeniesCommunityOnPremiumFlag(t *testing.T) {
	handler := gated(staticSource{ent: license.CommunityEntitlement()}, license.FeatureSSOSAML)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, requestWithWorkspace(true))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for community on SSO, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "COMMUNITY") {
		t.Fatalf("expected the denial to name the current plan, got %s", rec.Body.String())
	}
}

func TestRequireFeatureDeniesEveryPremiumFlagOnCommunity(t *testing.T) {
	premium := []license.FeatureFlag{
		license.FeatureSSOSAML,
		license.FeatureSCIM,
		license.FeatureAuditWarehouse,
		license.FeatureMultiAgentDeliberation,
		license.FeatureCustomRules,
		license.FeatureDORAMetrics,
		license.FeatureAirGapped,
	}
	for _, flag := range premium {
		handler := gated(staticSource{ent: license.CommunityEntitlement()}, flag)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, requestWithWorkspace(true))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 for community on %s, got %d", flag, rec.Code)
		}
	}
}

func TestRequireFeatureAllowsCommunityFeatureOnCommunity(t *testing.T) {
	handler := gated(staticSource{ent: license.CommunityEntitlement()}, license.FeatureBYOK)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, requestWithWorkspace(true))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for BYOK on community, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRequireFeatureAllowsEntitledFeature(t *testing.T) {
	ent := license.EntitlementFromPlan(
		license.TierScale,
		[]string{string(license.FeatureAuditWarehouse)},
		100, 0, time.Time{},
	)
	handler := gated(staticSource{ent: ent}, license.FeatureAuditWarehouse)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, requestWithWorkspace(true))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for an entitled feature, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRequireFeatureAllowsAnyFeatureOnEnterprise(t *testing.T) {
	handler := gated(
		staticSource{ent: license.EntitlementFromPlan(license.TierEnterprise, nil, 0, 0, time.Time{})},
		license.FeatureAirGapped,
	)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, requestWithWorkspace(true))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for enterprise on air-gapped, got %d", rec.Code)
	}
}

func TestRequireFeatureDeniesWhenWorkspaceContextMissing(t *testing.T) {
	handler := gated(staticSource{ent: license.CommunityEntitlement()}, license.FeatureBYOK)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, requestWithWorkspace(false))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without a workspace context, got %d", rec.Code)
	}
}

func TestRequireFeatureFailsClosedOnNilSource(t *testing.T) {
	handler := gated(nil, license.FeatureSSOSAML)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, requestWithWorkspace(true))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when entitlement resolution is unavailable, got %d", rec.Code)
	}
}

func TestRequireFeatureFailsClosedOnNilEntitlement(t *testing.T) {
	handler := gated(staticSource{ent: nil}, license.FeatureSSOSAML)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, requestWithWorkspace(true))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 on a nil entitlement, got %d", rec.Code)
	}
}

func TestRequireFeatureDeniesExpiredLicense(t *testing.T) {
	// Past the grace window. A plan row stays entitled for LicenseGracePeriod
	// after expiry, so an hour-old expiry would be allowed by design.
	ent := license.EntitlementFromPlan(
		license.TierEnterprise, nil, 0, 0,
		time.Now().UTC().Add(-license.LicenseGracePeriod-time.Hour),
	)
	handler := gated(staticSource{ent: ent}, license.FeatureSSOSAML)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, requestWithWorkspace(true))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for an expired license, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "active license") {
		t.Fatalf("expected the denial to mention the active license, got %s", rec.Body.String())
	}
}

func TestRequireFeatureEscapesQuotesInMessage(t *testing.T) {
	handler := gated(
		staticSource{ent: license.EntitlementFromPlan("weird\"tier", nil, 0, 0, time.Time{})},
		license.FeatureSCIM,
	)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, requestWithWorkspace(true))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
	if !strings.HasPrefix(rec.Body.String(), `{"error":"`) || !strings.HasSuffix(strings.TrimSpace(rec.Body.String()), `"}`) {
		t.Fatalf("denial body is not valid JSON: %s", rec.Body.String())
	}
}

func TestResolverResolvesCommunityWithNoManagerOrStore(t *testing.T) {
	r := license.NewResolver(nil, nil)
	ent := r.Resolve(context.Background(), uuid.New())

	if ent.Tier != license.TierCommunity {
		t.Fatalf("expected COMMUNITY, got %s", ent.Tier)
	}
	if !ent.Allows(license.FeatureBYOK) {
		t.Fatal("community should retain BYOK")
	}
	if ent.Allows(license.FeatureSSOSAML) {
		t.Fatal("community must not reach SSO")
	}
}

func TestNilResolverReturnsCommunity(t *testing.T) {
	var r *license.Resolver
	ent := r.Resolve(context.Background(), uuid.New())

	if ent == nil {
		t.Fatal("a nil resolver must still return an entitlement")
	}
	if ent.Tier != license.TierCommunity {
		t.Fatalf("expected COMMUNITY from a nil resolver, got %s", ent.Tier)
	}
}

func TestResolverPrefersSignedLicenseOverStoredRow(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keygen failed: %v", err)
	}
	token, err := license.IssueLicense(license.LicensePayload{
		Tier:      license.TierEnterprise,
		MaxSeats:  500,
		IssuedAt:  time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(365 * 24 * time.Hour),
	}, priv)
	if err != nil {
		t.Fatalf("issue failed: %v", err)
	}
	manager := license.NewLicenseManager(pub)
	if _, err := manager.LoadLicense(token); err != nil {
		t.Fatalf("load failed: %v", err)
	}

	store := fakeStore{lic: &license.StoredLicense{
		Tier:      string(license.TierDeveloper),
		MaxSeats:  10,
		ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
	}}
	r := license.NewResolver(manager, store)
	ent := r.Resolve(context.Background(), uuid.New())

	if ent.Tier != license.TierEnterprise {
		t.Fatalf("expected the signed license to win, got %s", ent.Tier)
	}
	if ent.Source != license.SourceSigned {
		t.Fatalf("expected signed source, got %q", ent.Source)
	}
}

func TestResolverFallsBackToStoredPlan(t *testing.T) {
	store := fakeStore{lic: &license.StoredLicense{
		Tier:      string(license.TierTeam),
		Features:  []string{string(license.FeatureSCIM)},
		MaxSeats:  25,
		ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
	}}
	r := license.NewResolver(nil, store)
	ent := r.Resolve(context.Background(), uuid.New())

	if ent.Source != license.SourcePlan {
		t.Fatalf("expected plan source, got %q", ent.Source)
	}
	if !ent.Allows(license.FeatureSCIM) {
		t.Fatal("expected the stored plan to grant SCIM")
	}
	if ent.Allows(license.FeatureAirGapped) {
		t.Fatal("the stored plan must not grant air-gapped")
	}
}

func TestResolverFallsBackToCommunityWhenStoreErrors(t *testing.T) {
	r := license.NewResolver(nil, fakeStore{err: context.DeadlineExceeded})
	ent := r.Resolve(context.Background(), uuid.New())

	if ent.Tier != license.TierCommunity {
		t.Fatalf("expected COMMUNITY on a store error, got %s", ent.Tier)
	}
	if ent.Allows(license.FeatureSSOSAML) {
		t.Fatal("a store error must not unlock premium features")
	}
}

type fakeStore struct {
	lic *license.StoredLicense
	err error
}

func (f fakeStore) GetActiveLicense(ctx context.Context, wsID uuid.UUID) (*license.StoredLicense, error) {
	return f.lic, f.err
}
