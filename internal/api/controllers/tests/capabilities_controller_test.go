// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package controllers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/enterprise/license"
)

type nilCounter struct{}

func (nilCounter) CountSeats(ctx context.Context, wsID uuid.UUID) (int, error) {
	return 0, nil
}

func getCapabilities(t *testing.T, resolver *license.Resolver, withWorkspace bool) *httptest.ResponseRecorder {
	t.Helper()
	ctrl := controllers.NewCapabilitiesController(resolver)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if withWorkspace {
		req = req.WithContext(auth.WithWorkspaceContext(req.Context(), uuid.New()))
	}
	rec := httptest.NewRecorder()
	ctrl.Routes().ServeHTTP(rec, req)
	return rec
}

func decodeCaps(t *testing.T, rec *httptest.ResponseRecorder) license.Capabilities {
	t.Helper()
	var caps license.Capabilities
	if err := json.Unmarshal(rec.Body.Bytes(), &caps); err != nil {
		t.Fatalf("failed decoding capabilities: %v (body=%s)", err, rec.Body.String())
	}
	return caps
}

func TestCapabilitiesEndpointRequiresAWorkspace(t *testing.T) {
	r := license.NewResolver(nil, nil, nilCounter{})

	rec := getCapabilities(t, r, false)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without a workspace, got %d", rec.Code)
	}
}

func TestCapabilitiesEndpointReportsCommunityForUnlicensedWorkspace(t *testing.T) {
	r := license.NewResolver(nil, nil, nilCounter{})

	rec := getCapabilities(t, r, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	caps := decodeCaps(t, rec)
	if caps.Tier != string(license.TierCommunity) {
		t.Fatalf("expected COMMUNITY, got %s", caps.Tier)
	}
	if caps.Features[string(license.FeatureSSOSAML)] {
		t.Fatal("an unlicensed workspace must not report SSO available")
	}
	if !caps.Features[string(license.FeatureBYOK)] {
		t.Fatal("an unlicensed workspace must report BYOK available")
	}
	if caps.ExpiresAt != nil {
		t.Fatal("an unlicensed workspace must report a null expiry")
	}
}

func TestCapabilitiesEndpointFallsBackToCommunityWithoutAResolver(t *testing.T) {
	ctrl := controllers.NewCapabilitiesController(nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(auth.WithWorkspaceContext(req.Context(), uuid.New()))
	rec := httptest.NewRecorder()
	ctrl.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with no resolver, got %d", rec.Code)
	}
	caps := decodeCaps(t, rec)
	if caps.Tier != string(license.TierCommunity) {
		t.Fatalf("expected COMMUNITY fallback, got %s", caps.Tier)
	}
}

func TestCapabilitiesEndpointExposesEveryFlagKey(t *testing.T) {
	r := license.NewResolver(nil, nil, nilCounter{})

	rec := getCapabilities(t, r, true)

	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("failed decoding: %v", err)
	}
	features, ok := raw["features"].(map[string]any)
	if !ok {
		t.Fatalf("features must be an object, got %T", raw["features"])
	}
	for _, flag := range license.AllFlags() {
		if _, present := features[string(flag)]; !present {
			t.Fatalf("feature key %s missing from the response", flag)
		}
	}
	if len(features) != len(license.AllFlags()) {
		t.Fatalf("expected exactly %d feature keys, got %d", len(license.AllFlags()), len(features))
	}
}

func TestCapabilitiesEndpointReportsSeatUsage(t *testing.T) {
	counter := &seatStubCounter{count: 3}
	store := seatPlanStore{tier: license.TierTeam, maxSeats: 25}
	r := license.NewResolver(nil, store, counter)

	caps := decodeCaps(t, getCapabilities(t, r, true))

	if caps.Seats.Limit != 25 {
		t.Fatalf("expected seats.limit=25, got %d", caps.Seats.Limit)
	}
	if caps.Seats.Remaining != 22 {
		t.Fatalf("expected seats.remaining=22, got %d", caps.Seats.Remaining)
	}
	if caps.Seats.Unlimited {
		t.Fatal("a capped plan must report unlimited=false")
	}
}

func TestCapabilitiesEndpointReportsUnlimitedSeats(t *testing.T) {
	store := seatPlanStore{tier: license.TierEnterprise, maxSeats: 0}
	r := license.NewResolver(nil, store, &seatStubCounter{count: 99})

	caps := decodeCaps(t, getCapabilities(t, r, true))

	if !caps.Seats.Unlimited || caps.Seats.Remaining != -1 {
		t.Fatalf("expected unlimited seats, got %+v", caps.Seats)
	}
}

func TestCapabilitiesEndpointReportsExpiredPlan(t *testing.T) {
	// Past the grace window, not merely past the expiry instant. A plan row honours
	// the same LicenseGracePeriod as a signed license, matching the GRACE_PERIOD
	// state GetWorkspacePlanDetails already reports, so an hour-old expiry is
	// still entitled.
	store := expiredPlanStore{expires: time.Now().UTC().Add(-license.LicenseGracePeriod - time.Hour)}
	r := license.NewResolver(nil, store, &seatStubCounter{count: 1})

	caps := decodeCaps(t, getCapabilities(t, r, true))

	if caps.Valid {
		t.Fatal("an expired plan must report valid=false")
	}
	if caps.SubscriptionStatus != "expired" {
		t.Fatalf("expected subscriptionStatus=expired, got %s", caps.SubscriptionStatus)
	}
	if caps.Tier != string(license.TierScale) {
		t.Fatalf("an expired plan must retain its tier for display, got %s", caps.Tier)
	}
	if caps.Features[string(license.FeatureSSOSAML)] {
		t.Fatal("an expired plan must report SSO unavailable")
	}
}

type expiredPlanStore struct {
	expires time.Time
}

func (e expiredPlanStore) GetActiveLicense(ctx context.Context, wsID uuid.UUID) (*license.StoredLicense, error) {
	return &license.StoredLicense{
		Tier:      string(license.TierScale),
		MaxSeats:  100,
		ExpiresAt: e.expires,
	}, nil
}
