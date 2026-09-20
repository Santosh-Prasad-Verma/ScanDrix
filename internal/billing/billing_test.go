// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev
// ═══════════════════════════════════════════════════════════════

package billing

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockRepo implements IBillingRepository for unit tests.
type MockRepo struct {
	dunningState map[uuid.UUID]DunningState
	graceEnds    map[uuid.UUID]*time.Time
	tokenResets  map[uuid.UUID]int
	plans        map[uuid.UUID]string
	members      map[uuid.UUID][]OrganizationMemberSeat
	revokedSeats []uuid.UUID
}

func NewMockRepo() *MockRepo {
	return &MockRepo{
		dunningState: make(map[uuid.UUID]DunningState),
		graceEnds:    make(map[uuid.UUID]*time.Time),
		tokenResets:  make(map[uuid.UUID]int),
		plans:        make(map[uuid.UUID]string),
		members:      make(map[uuid.UUID][]OrganizationMemberSeat),
	}
}

func (m *MockRepo) GetOrganizationLicense(ctx context.Context, orgID uuid.UUID) (*LicenseInfo, error) {
	return &LicenseInfo{OrganizationID: orgID, PlanTier: m.plans[orgID]}, nil
}

func (m *MockRepo) UpdateOrganizationPlan(ctx context.Context, orgID uuid.UUID, newTier string, seatLimit int, features []string) error {
	m.plans[orgID] = newTier
	return nil
}

func (m *MockRepo) GetOrganizationMembers(ctx context.Context, orgID uuid.UUID) ([]OrganizationMemberSeat, error) {
	return m.members[orgID], nil
}

func (m *MockRepo) RevokeMemberSeat(ctx context.Context, orgID uuid.UUID, userID uuid.UUID, reason string) error {
	m.revokedSeats = append(m.revokedSeats, userID)
	return nil
}

func (m *MockRepo) RecordBillingTransaction(ctx context.Context, tx BillingTransactionRecord) error {
	return nil
}

func (m *MockRepo) UpdateDunningState(ctx context.Context, orgID uuid.UUID, state DunningState, gracePeriodEnd *time.Time) error {
	m.dunningState[orgID] = state
	m.graceEnds[orgID] = gracePeriodEnd
	return nil
}

func (m *MockRepo) GetDunningState(ctx context.Context, orgID uuid.UUID) (DunningState, *time.Time, error) {
	return m.dunningState[orgID], m.graceEnds[orgID], nil
}

func (m *MockRepo) ResetMonthlyTokenUsage(ctx context.Context, orgID uuid.UUID) error {
	m.tokenResets[orgID]++
	return nil
}

func (m *MockRepo) RecordFailedPayment(ctx context.Context, failure PaymentFailureRecord) error {
	return nil
}

func TestIdempotencyStore_Lifecycle(t *testing.T) {
	store := NewMemoryIdempotencyStore()
	ctx := context.Background()
	eventID := "evt_test_123"

	// First acquisition must succeed
	acquired, err := store.TryAcquire(ctx, eventID, 50*time.Millisecond)
	require.NoError(t, err)
	assert.True(t, acquired)

	// Second acquisition while in-flight must fail
	acquired, err = store.TryAcquire(ctx, eventID, 50*time.Millisecond)
	assert.ErrorIs(t, err, ErrEventCurrentlyLocked)
	assert.False(t, acquired)

	// Mark completed
	err = store.MarkCompleted(ctx, eventID)
	require.NoError(t, err)

	// Subsequent acquisition must be rejected as already processed
	acquired, err = store.TryAcquire(ctx, eventID, 50*time.Millisecond)
	assert.ErrorIs(t, err, ErrEventAlreadyProcessed)
	assert.False(t, acquired)

	// Test lock expiration
	expireID := "evt_expire_456"
	acquired, err = store.TryAcquire(ctx, expireID, 10*time.Millisecond)
	require.NoError(t, err)
	assert.True(t, acquired)

	time.Sleep(20 * time.Millisecond)
	cleaned := store.CleanExpired(ctx)
	assert.Equal(t, 1, cleaned)

	// Now should acquire again
	acquired, err = store.TryAcquire(ctx, expireID, 50*time.Millisecond)
	require.NoError(t, err)
	assert.True(t, acquired)
}

func TestDunningManager_EscalationAndResolution(t *testing.T) {
	repo := NewMockRepo()
	cfg := DunningConfig{
		WarningGracePeriod:    50 * time.Millisecond,
		RestrictedGracePeriod: 50 * time.Millisecond,
		MaxFailureAttempts:    3,
	}
	manager := NewDunningManager(repo, cfg)
	ctx := context.Background()
	orgID := uuid.New()

	// Initial state: good standing
	allowed, msg, err := manager.CheckDelinquencyRestrictions(ctx, orgID)
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.Empty(t, msg)

	// 1st failure -> transition to warning
	p1 := PaymentFailurePayload{
		OrganizationID: orgID,
		InvoiceID:      "inv_1",
		Amount:         29900,
		FailureReason:  "insufficient_funds",
		AttemptCount:   1,
	}
	trans, err := manager.HandlePaymentFailure(ctx, p1)
	require.NoError(t, err)
	assert.Equal(t, DunningStateWarning, trans.NewState)
	assert.NotNil(t, trans.GracePeriodEnd)

	allowed, msg, err = manager.CheckDelinquencyRestrictions(ctx, orgID)
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.Contains(t, msg, "Notice: Payment failed")

	// Wait for warning grace period to elapse
	time.Sleep(60 * time.Millisecond)

	// 2nd failure after grace period -> transition to restricted
	p2 := PaymentFailurePayload{
		OrganizationID: orgID,
		InvoiceID:      "inv_1",
		Amount:         29900,
		FailureReason:  "card_declined",
		AttemptCount:   2,
	}
	trans, err = manager.HandlePaymentFailure(ctx, p2)
	require.NoError(t, err)
	assert.Equal(t, DunningStateRestricted, trans.NewState)

	// Wait for restricted period to elapse
	time.Sleep(60 * time.Millisecond)

	// 3rd failure -> transition to suspended
	p3 := PaymentFailurePayload{
		OrganizationID: orgID,
		InvoiceID:      "inv_1",
		Amount:         29900,
		FailureReason:  "card_declined",
		AttemptCount:   3,
	}
	trans, err = manager.HandlePaymentFailure(ctx, p3)
	require.NoError(t, err)
	assert.Equal(t, DunningStateSuspended, trans.NewState)

	allowed, msg, err = manager.CheckDelinquencyRestrictions(ctx, orgID)
	require.NoError(t, err)
	assert.False(t, allowed)
	assert.Contains(t, msg, "Code review execution is suspended")

	// Settle invoice -> resolves back to Good Standing
	trans, err = manager.HandlePaymentSuccess(ctx, orgID, "inv_1")
	require.NoError(t, err)
	assert.Equal(t, DunningStateGoodStanding, trans.NewState)

	allowed, msg, err = manager.CheckDelinquencyRestrictions(ctx, orgID)
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.Empty(t, msg)
}

func TestHooksDispatcher_SyncAndAsync(t *testing.T) {
	idemp := NewMemoryIdempotencyStore()
	cfg := DefaultHooksDispatcherConfig()
	cfg.WorkerCount = 2
	dispatcher := NewHooksDispatcher(idemp, nil, cfg)
	defer dispatcher.Stop()

	var handledCount int64
	handler := BillingHandlerFunc(func(ctx context.Context, event BillingWebhookEvent) error {
		atomic.AddInt64(&handledCount, 1)
		return nil
	})

	dispatcher.RegisterHandler(EventPlanChanged, handler)

	ctx := context.Background()
	ev := BillingWebhookEvent{
		ID:             "evt_sync_1",
		Provider:       ProviderStripe,
		EventType:      EventPlanChanged,
		OrganizationID: uuid.New(),
		Timestamp:      time.Now().UTC(),
		RawPayload:     json.RawMessage(`{}`),
	}

	// Test synchronous dispatch
	err := dispatcher.ProcessSync(ctx, ev)
	require.NoError(t, err)
	assert.Equal(t, int64(1), atomic.LoadInt64(&handledCount))

	// Re-executing same event ID should be skipped by idempotency
	err = dispatcher.ProcessSync(ctx, ev)
	require.NoError(t, err)
	assert.Equal(t, int64(1), atomic.LoadInt64(&handledCount)) // no increment

	// Test async dispatch with new event
	ev2 := BillingWebhookEvent{
		ID:             "evt_async_2",
		Provider:       ProviderStripe,
		EventType:      EventPlanChanged,
		OrganizationID: uuid.New(),
		Timestamp:      time.Now().UTC(),
		RawPayload:     json.RawMessage(`{}`),
	}

	err = dispatcher.Dispatch(ctx, ev2)
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int64(2), atomic.LoadInt64(&handledCount))
}

func TestHooksDispatcher_ErrorUnlocksRetry(t *testing.T) {
	idemp := NewMemoryIdempotencyStore()
	dispatcher := NewHooksDispatcher(idemp, nil, DefaultHooksDispatcherConfig())
	defer dispatcher.Stop()

	var attempts int64
	handler := BillingHandlerFunc(func(ctx context.Context, event BillingWebhookEvent) error {
		count := atomic.AddInt64(&attempts, 1)
		if count == 1 {
			return errors.New("transient database glitch")
		}
		return nil
	})

	dispatcher.RegisterHandler(EventPaymentFailed, handler)

	ctx := context.Background()
	ev := BillingWebhookEvent{
		ID:             "evt_retry_1",
		Provider:       ProviderStripe,
		EventType:      EventPaymentFailed,
		OrganizationID: uuid.New(),
		Timestamp:      time.Now().UTC(),
		RawPayload:     json.RawMessage(`{}`),
	}

	// First attempt fails
	err := dispatcher.ProcessSync(ctx, ev)
	assert.Error(t, err)

	// Since it failed, lock is released and second attempt can proceed
	err = dispatcher.ProcessSync(ctx, ev)
	assert.NoError(t, err)
	assert.Equal(t, int64(2), atomic.LoadInt64(&attempts))
}
