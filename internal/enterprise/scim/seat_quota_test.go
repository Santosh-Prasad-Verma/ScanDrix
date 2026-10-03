// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - SCIM seat-quota enforcement
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package scim_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/enterprise/license"
	"github.com/scandrix/backend/internal/enterprise/scim"
)

// quotaStore serves a fixed plan with a hard seat cap.
type quotaStore struct {
	tier     string
	maxSeats int
	features []string
	expires  time.Time
}

func (s quotaStore) GetActiveLicense(context.Context, uuid.UUID) (*license.StoredLicense, error) {
	return &license.StoredLicense{
		Tier:      s.tier,
		MaxSeats:  s.maxSeats,
		Features:  s.features,
		ExpiresAt: s.expires,
	}, nil
}

// seatCounter reports a fixed number of consumed seats.
type seatCounter struct{ used int }

func (c seatCounter) CountSeats(context.Context, uuid.UUID) (int, error) { return c.used, nil }

// The regression that matters: the quota check existed in handleCreateUser but
// nothing ever set the resolver, so SCIM provisioned past the licensed seat
// count. This asserts that a bound service refuses at the cap.
func TestCreateUserRefusesWhenSeatQuotaExhausted(t *testing.T) {
	wsID := uuid.New()
	svc := scim.NewSCIMService() // no repository: exercises the quota path only
	svc.SetBearerToken("test-bearer-token")

	// Plan allows 2 seats, 2 already consumed.
	resolver := license.NewResolver(nil, quotaStore{
		tier:     "TEAM",
		maxSeats: 2,
		features: []string{string(license.FeatureSCIM)},
		expires:  time.Now().UTC().Add(24 * time.Hour),
	}, seatCounter{used: 2})
	svc.SetEntitlement(wsID, resolver)

	body := `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"new hire@example.com","emails":[{"value":"new hire@example.com","primary":true}]}`
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/Users", strings.NewReader(body))
	svc.Routes().ServeHTTP(rr, req)

	// The routes require the bearer token; supply it so the quota path runs.
	req = httptest.NewRequest(http.MethodPost, "/Users", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-bearer-token")
	rr = httptest.NewRecorder()
	svc.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict at the seat cap, got %d: %s", rr.Code, rr.Body.String())
	}

	var scimErr struct {
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &scimErr); err != nil {
		t.Fatalf("decoding error body: %v", err)
	}
	if !strings.Contains(strings.ToLower(scimErr.Detail), "seat") {
		t.Fatalf("the refusal must name the seat quota, got: %q", scimErr.Detail)
	}
}

// Below the cap the request must pass the quota check. With no repository the
// create then reports a persistence failure rather than a quota refusal, which
// is how we prove the quota was not what blocked it.
func TestCreateUserPassesQuotaWhenSeatsRemain(t *testing.T) {
	wsID := uuid.New()
	svc := scim.NewSCIMService()
	svc.SetBearerToken("test-bearer-token")

	resolver := license.NewResolver(nil, quotaStore{
		tier:     "TEAM",
		maxSeats: 10,
		features: []string{string(license.FeatureSCIM)},
		expires:  time.Now().UTC().Add(24 * time.Hour),
	}, seatCounter{used: 1})
	svc.SetEntitlement(wsID, resolver)

	body := `{"userName":"new hire@example.com","emails":[{"value":"new hire@example.com","primary":true}]}`
	req := httptest.NewRequest(http.MethodPost, "/Users", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-bearer-token")
	rr := httptest.NewRecorder()
	svc.Routes().ServeHTTP(rr, req)

	if rr.Code == http.StatusConflict {
		t.Fatalf("a workspace with seats remaining must not be refused: %s", rr.Body.String())
	}
}

// An unlimited plan never refuses on seats.
func TestCreateUserUnlimitedPlanNeverRefused(t *testing.T) {
	svc := scim.NewSCIMService()
	svc.SetBearerToken("test-bearer-token")
	svc.SetEntitlement(uuid.New(), license.NewResolver(nil, quotaStore{
		tier:     "ENTERPRISE",
		maxSeats: 0, // unlimited
		expires:  time.Now().UTC().Add(24 * time.Hour),
	}, seatCounter{used: 9999}))

	body := `{"userName":"anyone@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/Users", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-bearer-token")
	rr := httptest.NewRecorder()
	svc.Routes().ServeHTTP(rr, req)

	if rr.Code == http.StatusConflict {
		t.Fatalf("an unlimited plan must never refuse on seats: %s", rr.Body.String())
	}
}

// A plan past its grace window is refused at the entitlement gate, before the
// seat check is ever reached: an invalid entitlement unlocks no feature, so
// SCIM itself is not available. 403 rather than 409, because the truthful answer
// is "your plan does not include this", not "buy a seat".
func TestCreateUserRefusesOnExpiredPlan(t *testing.T) {
	svc := scim.NewSCIMService()
	svc.SetBearerToken("test-bearer-token")
	svc.SetEntitlement(uuid.New(), license.NewResolver(nil, quotaStore{
		tier:     "TEAM",
		maxSeats: 0,
		features: []string{string(license.FeatureSCIM)},
		expires:  time.Now().UTC().Add(-license.LicenseGracePeriod - time.Hour),
	}, seatCounter{used: 0}))

	body := `{"userName":"late@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/Users", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-bearer-token")
	rr := httptest.NewRecorder()
	svc.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("a plan past the grace window must be refused, got %d: %s", rr.Code, rr.Body.String())
	}
}

// Without a binding there is no tenant, so provisioning must be refused rather
// than silently unmetered. This is the state both binaries were in before the
// wiring was added.
func TestCreateUserRefusesWhenUnbound(t *testing.T) {
	svc := scim.NewSCIMService()
	svc.SetBearerToken("test-bearer-token")
	// SetEntitlement deliberately not called.

	body := `{"userName":"unbound@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/Users", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-bearer-token")
	rr := httptest.NewRecorder()
	svc.Routes().ServeHTTP(rr, req)

	if rr.Code == http.StatusCreated {
		t.Fatal("an unbound SCIM service must not provision: seats would go unmetered")
	}
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("an unbound service should report unavailable, got %d", rr.Code)
	}
}
