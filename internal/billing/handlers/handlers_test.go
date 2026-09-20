// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev
// ═══════════════════════════════════════════════════════════════

package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/billing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type MockHandlerRepo struct {
	plans         map[uuid.UUID]string
	seatLimits    map[uuid.UUID]int
	features      map[uuid.UUID][]string
	members       map[uuid.UUID][]billing.OrganizationMemberSeat
	revokedUserIDs []uuid.UUID
	dunningState  map[uuid.UUID]billing.DunningState
	tokenResets   map[uuid.UUID]int
	failedPayments []billing.PaymentFailureRecord
	transactions  []billing.BillingTransactionRecord
}

func NewMockHandlerRepo() *MockHandlerRepo {
	return &MockHandlerRepo{
		plans:        make(map[uuid.UUID]string),
		seatLimits:   make(map[uuid.UUID]int),
		features:     make(map[uuid.UUID][]string),
		members:      make(map[uuid.UUID][]billing.OrganizationMemberSeat),
		dunningState: make(map[uuid.UUID]billing.DunningState),
		tokenResets:  make(map[uuid.UUID]int),
	}
}

func (m *MockHandlerRepo) GetOrganizationLicense(ctx context.Context, orgID uuid.UUID) (*billing.LicenseInfo, error) {
	return &billing.LicenseInfo{
		OrganizationID: orgID,
		PlanTier:       m.plans[orgID],
		TotalSeats:     m.seatLimits[orgID],
		Features:       m.features[orgID],
	}, nil
}

func (m *MockHandlerRepo) UpdateOrganizationPlan(ctx context.Context, orgID uuid.UUID, newTier string, seatLimit int, features []string) error {
	m.plans[orgID] = newTier
	m.seatLimits[orgID] = seatLimit
	m.features[orgID] = features
	return nil
}

func (m *MockHandlerRepo) GetOrganizationMembers(ctx context.Context, orgID uuid.UUID) ([]billing.OrganizationMemberSeat, error) {
	return m.members[orgID], nil
}

func (m *MockHandlerRepo) RevokeMemberSeat(ctx context.Context, orgID uuid.UUID, userID uuid.UUID, reason string) error {
	m.revokedUserIDs = append(m.revokedUserIDs, userID)
	return nil
}

func (m *MockHandlerRepo) RecordBillingTransaction(ctx context.Context, tx billing.BillingTransactionRecord) error {
	m.transactions = append(m.transactions, tx)
	return nil
}

func (m *MockHandlerRepo) UpdateDunningState(ctx context.Context, orgID uuid.UUID, state billing.DunningState, gracePeriodEnd *time.Time) error {
	m.dunningState[orgID] = state
	return nil
}

func (m *MockHandlerRepo) GetDunningState(ctx context.Context, orgID uuid.UUID) (billing.DunningState, *time.Time, error) {
	return m.dunningState[orgID], nil, nil
}

func (m *MockHandlerRepo) ResetMonthlyTokenUsage(ctx context.Context, orgID uuid.UUID) error {
	m.tokenResets[orgID]++
	return nil
}

func (m *MockHandlerRepo) RecordFailedPayment(ctx context.Context, failure billing.PaymentFailureRecord) error {
	m.failedPayments = append(m.failedPayments, failure)
	return nil
}

func TestAutoRevokeRemovedLicenseSeats_PreservesOwnerAndSortsByInactivity(t *testing.T) {
	repo := NewMockHandlerRepo()
	seatManager := NewSeatManagementHandler(repo)

	orgID := uuid.New()
	ownerID := uuid.New()
	userNeverActive := uuid.New()
	userActiveOld := uuid.New()
	userActiveRecent := uuid.New()
	userActiveToday := uuid.New()

	now := time.Now().UTC()
	tOld := now.Add(-60 * 24 * time.Hour)
	tRecent := now.Add(-5 * 24 * time.Hour)
	tToday := now.Add(-1 * time.Hour)

	repo.members[orgID] = []billing.OrganizationMemberSeat{
		{UserID: ownerID, Email: "owner@scandrix.dev", Role: "OWNER", IsOwner: true, HasSeat: true, LastActiveAt: &tRecent},
		{UserID: userNeverActive, Email: "inactive@scandrix.dev", Role: "DEVELOPER", IsOwner: false, HasSeat: true, LastActiveAt: nil},
		{UserID: userActiveOld, Email: "old@scandrix.dev", Role: "DEVELOPER", IsOwner: false, HasSeat: true, LastActiveAt: &tOld},
		{UserID: userActiveRecent, Email: "recent@scandrix.dev", Role: "DEVELOPER", IsOwner: false, HasSeat: true, LastActiveAt: &tRecent},
		{UserID: userActiveToday, Email: "today@scandrix.dev", Role: "DEVELOPER", IsOwner: false, HasSeat: true, LastActiveAt: &tToday},
	}

	// 5 seats currently held. Downgrade to 3 seats -> must revoke 2 seats.
	res, err := seatManager.AutoRevokeRemovedLicenseSeats(context.Background(), orgID, 3)
	require.NoError(t, err)
	require.NotNil(t, res)

	assert.Equal(t, 2, res.RevokedCount)
	assert.Equal(t, 2, len(repo.revokedUserIDs))

	// The two revoked must be userNeverActive (nil activity) and userActiveOld (oldest activity)
	assert.Contains(t, repo.revokedUserIDs, userNeverActive)
	assert.Contains(t, repo.revokedUserIDs, userActiveOld)

	// Owner and recently active members must NOT have been revoked
	assert.NotContains(t, repo.revokedUserIDs, ownerID)
	assert.NotContains(t, repo.revokedUserIDs, userActiveToday)
}

func TestPlanChangedHandler_Execution(t *testing.T) {
	repo := NewMockHandlerRepo()
	seatManager := NewSeatManagementHandler(repo)
	handler := NewPlanChangedHandler(repo, seatManager)

	orgID := uuid.New()
	payload := billing.PlanChangePayload{
		OrganizationID: orgID,
		PreviousTier:   "TEAM",
		NewTier:        "ENTERPRISE",
		NewSeatLimit:   50,
		Amount:         99900,
		Currency:       "USD",
	}
	raw, _ := json.Marshal(payload)

	ev := billing.BillingWebhookEvent{
		ID:             "evt_pc_1",
		Provider:       billing.ProviderStripe,
		EventType:      billing.EventPlanChanged,
		OrganizationID: orgID,
		RawPayload:     raw,
	}

	err := handler.HandleBillingEvent(context.Background(), ev)
	require.NoError(t, err)

	assert.Equal(t, "ENTERPRISE", repo.plans[orgID])
	assert.Equal(t, 50, repo.seatLimits[orgID])
	assert.Contains(t, repo.features[orgID], "FEATURE_SSO_SAML")
	assert.Contains(t, repo.features[orgID], "FEATURE_MULTI_AGENT_DELIBERATION")
}

func TestPaymentFailedAndDunningHandler(t *testing.T) {
	repo := NewMockHandlerRepo()
	dunning := billing.NewDunningManager(repo, billing.DefaultDunningConfig())
	handler := NewPaymentFailedHandler(repo, dunning)

	orgID := uuid.New()
	payload := billing.PaymentFailurePayload{
		OrganizationID:   orgID,
		InvoiceID:        "inv_fail_1",
		Amount:           49900,
		Currency:         "USD",
		FailureReason:    "insufficient_funds",
		AttemptCount:     1,
		UpdatePaymentURL: "https://scandrix.dev/billing/update",
	}
	raw, _ := json.Marshal(payload)

	ev := billing.BillingWebhookEvent{
		ID:             "evt_pf_1",
		Provider:       billing.ProviderStripe,
		EventType:      billing.EventPaymentFailed,
		OrganizationID: orgID,
		RawPayload:     raw,
	}

	err := handler.HandleBillingEvent(context.Background(), ev)
	require.NoError(t, err)

	assert.Equal(t, 1, len(repo.failedPayments))
	assert.Equal(t, billing.DunningStateWarning, repo.dunningState[orgID])
}

func TestInvoiceSucceededHandler_ResetTokensAndResolveDunning(t *testing.T) {
	repo := NewMockHandlerRepo()
	dunning := billing.NewDunningManager(repo, billing.DefaultDunningConfig())
	handler := NewInvoiceSucceededHandler(repo, dunning)

	orgID := uuid.New()
	// Set tenant as previously in warning state
	repo.dunningState[orgID] = billing.DunningStateWarning

	payload := billing.InvoiceSucceededPayload{
		OrganizationID: orgID,
		InvoiceID:      "inv_succ_1",
		Amount:         49900,
		Currency:       "USD",
		ResetTokens:    true,
	}
	raw, _ := json.Marshal(payload)

	ev := billing.BillingWebhookEvent{
		ID:             "evt_inv_1",
		Provider:       billing.ProviderStripe,
		EventType:      billing.EventInvoiceSucceeded,
		OrganizationID: orgID,
		RawPayload:     raw,
	}

	err := handler.HandleBillingEvent(context.Background(), ev)
	require.NoError(t, err)

	assert.Equal(t, 1, repo.tokenResets[orgID])
	assert.Equal(t, billing.DunningStateGoodStanding, repo.dunningState[orgID])
	assert.Equal(t, 1, len(repo.transactions))
}

func TestTrialExpiringHandler_CountdownAndAutoDowngrade(t *testing.T) {
	repo := NewMockHandlerRepo()
	handler := NewTrialExpiringHandler(repo)

	orgID := uuid.New()
	payload := billing.TrialExpiringPayload{
		OrganizationID: orgID,
		DaysRemaining:  0, // expired
		UpgradeURL:     "https://scandrix.dev/upgrade",
	}
	raw, _ := json.Marshal(payload)

	ev := billing.BillingWebhookEvent{
		ID:             "evt_te_1",
		Provider:       billing.ProviderStripe,
		EventType:      billing.EventTrialExpiring,
		OrganizationID: orgID,
		RawPayload:     raw,
	}

	err := handler.HandleBillingEvent(context.Background(), ev)
	require.NoError(t, err)

	assert.Equal(t, "COMMUNITY", repo.plans[orgID])
	assert.Equal(t, 3, repo.seatLimits[orgID])
}

func TestSubscriptionCancelledHandler_DowngradeAndSeatReclamation(t *testing.T) {
	repo := NewMockHandlerRepo()
	seatManager := NewSeatManagementHandler(repo)
	handler := NewSubscriptionCancelledHandler(repo, seatManager)

	orgID := uuid.New()
	ownerID := uuid.New()
	user1 := uuid.New()
	user2 := uuid.New()
	user3 := uuid.New()

	repo.members[orgID] = []billing.OrganizationMemberSeat{
		{UserID: ownerID, Role: "OWNER", IsOwner: true, HasSeat: true},
		{UserID: user1, Role: "DEVELOPER", HasSeat: true},
		{UserID: user2, Role: "DEVELOPER", HasSeat: true},
		{UserID: user3, Role: "DEVELOPER", HasSeat: true},
	}

	payload := billing.SubscriptionCancelledPayload{
		OrganizationID:       orgID,
		ImmediateGracePeriod: true,
		Reason:               "User requested cancellation",
	}
	raw, _ := json.Marshal(payload)

	ev := billing.BillingWebhookEvent{
		ID:             "evt_sc_1",
		Provider:       billing.ProviderStripe,
		EventType:      billing.EventSubscriptionCancelled,
		OrganizationID: orgID,
		RawPayload:     raw,
	}

	err := handler.HandleBillingEvent(context.Background(), ev)
	require.NoError(t, err)

	assert.Equal(t, "COMMUNITY", repo.plans[orgID])
	assert.Equal(t, 3, repo.seatLimits[orgID])
	// 4 seats down to 3 seats -> 1 seat must have been revoked
	assert.Equal(t, 1, len(repo.revokedUserIDs))
	assert.NotContains(t, repo.revokedUserIDs, ownerID)
}
