// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package controllers_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/enterprise/license"
)

type seatStubCounter struct {
	count int
	err   error
}

func (s *seatStubCounter) CountSeats(ctx context.Context, wsID uuid.UUID) (int, error) {
	return s.count, s.err
}

type seatPlanStore struct {
	tier     license.LicenseTier
	maxSeats int
}

func (s seatPlanStore) GetActiveLicense(ctx context.Context, wsID uuid.UUID) (*license.StoredLicense, error) {
	return &license.StoredLicense{
		Tier:      string(s.tier),
		MaxSeats:  s.maxSeats,
		ExpiresAt: time.Time{},
	}, nil
}

func inviteBody(teamID string, emails ...string) string {
	members := make([]map[string]string, 0, len(emails))
	for _, e := range emails {
		members = append(members, map[string]string{"email": e})
	}
	payload, _ := json.Marshal(map[string]any{
		"teamId":  teamID,
		"members": members,
	})
	return string(payload)
}

func postInvite(t *testing.T, ctrl *controllers.TeamController, body string, wsID uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req = req.WithContext(auth.WithWorkspaceContext(req.Context(), wsID))
	rec := httptest.NewRecorder()
	ctrl.TeamMembersRoutes().ServeHTTP(rec, req)
	return rec
}

func TestInviteRefusedWhenSeatQuotaIsFull(t *testing.T) {
	wsID := uuid.New()
	teamID := uuid.New()
	counter := &seatStubCounter{count: 5}
	resolver := license.NewResolver(nil, seatPlanStore{tier: license.TierTeam, maxSeats: 5}, counter)
	ctrl := controllers.NewTeamController(nil).WithEntitlements(resolver)

	rec := postInvite(t, ctrl, inviteBody(teamID.String(), "new@example.com"), wsID)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 when the seat quota is full, got %d: %s", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed decoding response: %v", err)
	}
	if body["code"] != "seat_quota_exceeded" {
		t.Fatalf("expected code=seat_quota_exceeded, got %v", body["code"])
	}
	if body["seatsLimit"] != float64(5) {
		t.Fatalf("expected seatsLimit=5, got %v", body["seatsLimit"])
	}
	if body["seatsUsed"] != float64(5) {
		t.Fatalf("expected seatsUsed=5, got %v", body["seatsUsed"])
	}
}

func TestInviteRefusedWhenBulkExceedsRemainingSeats(t *testing.T) {
	wsID := uuid.New()
	teamID := uuid.New()
	counter := &seatStubCounter{count: 4}
	resolver := license.NewResolver(nil, seatPlanStore{tier: license.TierTeam, maxSeats: 5}, counter)
	ctrl := controllers.NewTeamController(nil).WithEntitlements(resolver)

	rec := postInvite(t, ctrl, inviteBody(teamID.String(), "a@example.com", "b@example.com"), wsID)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for a batch that overflows, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestInviteAllowedWhenSeatsRemain(t *testing.T) {
	wsID := uuid.New()
	teamID := uuid.New()
	counter := &seatStubCounter{count: 3}
	resolver := license.NewResolver(nil, seatPlanStore{tier: license.TierTeam, maxSeats: 5}, counter)
	ctrl := controllers.NewTeamController(nil).WithEntitlements(resolver)

	rec := postInvite(t, ctrl, inviteBody(teamID.String(), "new@example.com"), wsID)

	if rec.Code == http.StatusConflict {
		t.Fatalf("expected the invite to pass the seat check, got 409: %s", rec.Body.String())
	}
}

func TestInviteFailsClosedWhenSeatCountUnreadable(t *testing.T) {
	wsID := uuid.New()
	teamID := uuid.New()
	counter := &seatStubCounter{err: errors.New("connection reset")}
	resolver := license.NewResolver(nil, seatPlanStore{tier: license.TierTeam, maxSeats: 5}, counter)
	ctrl := controllers.NewTeamController(nil).WithEntitlements(resolver)

	rec := postInvite(t, ctrl, inviteBody(teamID.String(), "new@example.com"), wsID)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when the seat count cannot be read, got %d: %s", rec.Code, rec.Body.String())
	}

	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["code"] != "seat_check_unavailable" {
		t.Fatalf("expected code=seat_check_unavailable, got %v", body["code"])
	}
}

func TestCommunityWorkspaceCannotExceedFiveSeats(t *testing.T) {
	wsID := uuid.New()
	teamID := uuid.New()
	counter := &seatStubCounter{count: license.QuotaCommunity.MaxSeats}
	resolver := license.NewResolver(nil, nil, counter)
	ctrl := controllers.NewTeamController(nil).WithEntitlements(resolver)

	rec := postInvite(t, ctrl, inviteBody(teamID.String(), "new@example.com"), wsID)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for an unlicensed workspace at the community cap, got %d", rec.Code)
	}
}

func TestUnlimitedPlanAlwaysPassesTheSeatCheck(t *testing.T) {
	wsID := uuid.New()
	teamID := uuid.New()
	counter := &seatStubCounter{count: 10_000}
	resolver := license.NewResolver(nil, seatPlanStore{tier: license.TierEnterprise, maxSeats: 0}, counter)
	ctrl := controllers.NewTeamController(nil).WithEntitlements(resolver)

	rec := postInvite(t, ctrl, inviteBody(teamID.String(), "a@example.com", "b@example.com", "c@example.com"), wsID)

	if rec.Code == http.StatusConflict || rec.Code == http.StatusServiceUnavailable {
		t.Fatalf("an unlimited plan must not be seat-refused, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestInviteWithoutResolverIsNotSeatBlocked(t *testing.T) {
	wsID := uuid.New()
	teamID := uuid.New()
	ctrl := controllers.NewTeamController(nil)

	rec := postInvite(t, ctrl, inviteBody(teamID.String(), "new@example.com"), wsID)

	if rec.Code == http.StatusConflict {
		t.Fatal("a controller with no resolver must not fabricate a quota refusal")
	}
}
