package controllers_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockBillingRepo struct {
	plans           []models.PlanConfiguration
	workspacePlans  map[uuid.UUID]*models.WorkspacePlanDetails
	outboxEvents    []*models.OutboxRecord
	syncedRules     int
	upgradedTier    string
	upgradedSeats   int
	upgradedExpires time.Time
}

func newMockBillingRepo() *mockBillingRepo {
	return &mockBillingRepo{
		plans: []models.PlanConfiguration{
			{Tier: "COMMUNITY", AmountINR: 0, AmountUSD: 0, MaxSeats: 5},
			{Tier: "TEAM", AmountINR: 499900, AmountUSD: 5900, MaxSeats: 25},
			{Tier: "ENTERPRISE", AmountINR: 1999900, AmountUSD: 24900, MaxSeats: 100},
		},
		workspacePlans: make(map[uuid.UUID]*models.WorkspacePlanDetails),
		outboxEvents:   make([]*models.OutboxRecord, 0),
	}
}

func (m *mockBillingRepo) GetWorkspacePlanDetails(ctx context.Context, wsID uuid.UUID) (*models.WorkspacePlanDetails, error) {
	if p, ok := m.workspacePlans[wsID]; ok {
		return p, nil
	}
	return &models.WorkspacePlanDetails{
		PlanTier:       "COMMUNITY",
		ExpiresAt:      time.Now().UTC().AddDate(0, 1, 0),
		TotalSeats:     5,
		AllocatedSeats: 1,
	}, nil
}

func (m *mockBillingRepo) InsertOutboxEvent(ctx context.Context, event *models.OutboxRecord) error {
	m.outboxEvents = append(m.outboxEvents, event)
	return nil
}

func (m *mockBillingRepo) SyncRulesWithPlanLimit(ctx context.Context, workspaceID uuid.UUID, maxAllowedRules int) (int, error) {
	m.syncedRules = maxAllowedRules
	return 2, nil
}

func (m *mockBillingRepo) UpgradeWorkspacePlan(ctx context.Context, wsID uuid.UUID, planTier string, maxSeats int, expiresAt time.Time, features []string) error {
	m.upgradedTier = planTier
	m.upgradedSeats = maxSeats
	m.upgradedExpires = expiresAt
	return nil
}

func (m *mockBillingRepo) ListPlanConfigurations(ctx context.Context) ([]models.PlanConfiguration, error) {
	return m.plans, nil
}

func TestBillingController_TenantIsolation_UnauthorizedWithoutContext(t *testing.T) {
	repo := newMockBillingRepo()
	ctrl := controllers.NewBillingController(nil, repo, nil, "test-secret")
	routes := ctrl.ProtectedRoutes()

	endpoints := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/plan", ""},
		{http.MethodPost, "/razorpay/order", `{"plan":"team","currency":"INR"}`},
		{http.MethodPost, "/razorpay/verify", `{"razorpay_order_id":"order_1","razorpay_payment_id":"pay_1"}`},
		{http.MethodPost, "/downgrade/calculate", `{"target_plan":"community"}`},
		{http.MethodPost, "/downgrade/confirm", `{"target_plan":"community"}`},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			var bodyReader *bytes.Reader
			if ep.body != "" {
				bodyReader = bytes.NewReader([]byte(ep.body))
			} else {
				bodyReader = bytes.NewReader(nil)
			}
			req := httptest.NewRequest(ep.method, ep.path, bodyReader)
			rec := httptest.NewRecorder()

			routes.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusUnauthorized, rec.Code, "endpoint must reject unauthenticated request")
			assert.Contains(t, rec.Body.String(), "missing workspace context")
		})
	}
}

func TestBillingController_GetPlan_AuthorizedWithContext(t *testing.T) {
	repo := newMockBillingRepo()
	wsID := uuid.New()
	repo.workspacePlans[wsID] = &models.WorkspacePlanDetails{
		PlanTier:       "TEAM",
		ExpiresAt:      time.Now().UTC().AddDate(0, 1, 0),
		TotalSeats:     25,
		AllocatedSeats: 10,
	}

	ctrl := controllers.NewBillingController(nil, repo, nil, "test-secret")
	routes := ctrl.ProtectedRoutes()

	req := httptest.NewRequest(http.MethodGet, "/plan", nil)
	req = req.WithContext(auth.WithWorkspaceContext(req.Context(), wsID))
	rec := httptest.NewRecorder()

	routes.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp models.WorkspacePlanDetails
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "TEAM", resp.PlanTier)
	assert.Equal(t, 25, resp.TotalSeats)
}

func TestBillingController_ListPlans_Public(t *testing.T) {
	repo := newMockBillingRepo()
	ctrl := controllers.NewBillingController(nil, repo, nil, "test-secret")
	routes := ctrl.Routes()

	req := httptest.NewRequest(http.MethodGet, "/plans", nil)
	rec := httptest.NewRecorder()

	routes.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)
	assert.True(t, resp["success"].(bool))
}

func TestBillingController_WebhookSignatureVerification(t *testing.T) {
	secret := "secret-hmac-key-12345"
	repo := newMockBillingRepo()
	ctrl := controllers.NewBillingController(nil, repo, nil, secret)
	routes := ctrl.WebhookRoutes()

	wsID := uuid.New()
	payload := map[string]any{
		"workspaceId":   wsID.String(),
		"amount":        4999.00,
		"currency":      "INR",
		"failureReason": "insufficient_funds",
	}
	bodyBytes, _ := json.Marshal(payload)

	// 1. Unsigned webhook rejected with 401
	reqUnsigned := httptest.NewRequest(http.MethodPost, "/payment-failed", bytes.NewReader(bodyBytes))
	recUnsigned := httptest.NewRecorder()
	routes.ServeHTTP(recUnsigned, reqUnsigned)
	assert.Equal(t, http.StatusUnauthorized, recUnsigned.Code)

	// 2. Tampered signature rejected with 401
	reqTampered := httptest.NewRequest(http.MethodPost, "/payment-failed", bytes.NewReader(bodyBytes))
	reqTampered.Header.Set("X-Billing-Signature", "invalid-hex-signature")
	recTampered := httptest.NewRecorder()
	routes.ServeHTTP(recTampered, reqTampered)
	assert.Equal(t, http.StatusUnauthorized, recTampered.Code)

	// 3. Valid HMAC-SHA256 signature accepted with 200
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(bodyBytes)
	validSig := hex.EncodeToString(mac.Sum(nil))

	reqValid := httptest.NewRequest(http.MethodPost, "/payment-failed", bytes.NewReader(bodyBytes))
	reqValid.Header.Set("X-Billing-Signature", validSig)
	recValid := httptest.NewRecorder()
	routes.ServeHTTP(recValid, reqValid)
	assert.Equal(t, http.StatusOK, recValid.Code)

	// Verify outbox record was inserted for the specific workspace
	require.Len(t, repo.outboxEvents, 1)
	assert.Equal(t, wsID, repo.outboxEvents[0].WorkspaceID)
	assert.Equal(t, "billing.payment_failed", repo.outboxEvents[0].EventType)
}
