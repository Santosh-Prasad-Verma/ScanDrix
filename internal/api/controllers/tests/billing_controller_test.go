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
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/billing/razorpay"
	"github.com/scandrix/backend/internal/llm"
	"github.com/scandrix/backend/pkg/models"
)

func TestBillingControllerRequiresDB(t *testing.T) {
	wsID := uuid.New()
	limiter := llm.NewTokenBudgetLimiter()
	billingSvc := razorpay.NewBillingService(nil, limiter, nil, "https://app.scandrix.dev", "", "test_secret", "test_wh")
	ctrl := controllers.NewBillingController(billingSvc, nil, limiter)

	router := ctrl.ProtectedRoutes()

	req := httptest.NewRequest(http.MethodGet, "/plan", nil)
	ctx := context.WithValue(req.Context(), auth.WorkspaceContextKey, wsID)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503 (no DB connected), got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestBillingControllerOrderAndVerifyFlow(t *testing.T) {
	wsID := uuid.New()
	keyID := "rzp_test_123"
	keySecret := "sec_checkout_123"
	limiter := llm.NewTokenBudgetLimiter()

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if strings.HasPrefix(r.URL.Path, "/payments/") {
			_, _ = w.Write([]byte(`{
				"id": "pay_test_999888",
				"entity": "payment",
				"amount": 149900,
				"currency": "INR",
				"status": "captured",
				"order_id": "order_test_ctrl_999"
			}`))
			return
		}
		_, _ = w.Write([]byte(`{
			"id": "order_test_ctrl_999",
			"entity": "order",
			"amount": 149900,
			"amount_due": 149900,
			"currency": "INR",
			"receipt": "rcpt_test_ctrl",
			"status": "created"
		}`))
	}))
	defer mockServer.Close()

	client := razorpay.NewRazorpayClient(keyID, keySecret)
	client.SetBaseURL(mockServer.URL)

	billingSvc := razorpay.NewBillingService(nil, limiter, nil, "https://app.scandrix.dev", keyID, keySecret, "wh_sec_123")
	billingSvc.SetClient(client)

	ctrl := controllers.NewBillingController(billingSvc, nil, limiter)
	router := ctrl.ProtectedRoutes()

	// 1. Create order
	orderBody, _ := json.Marshal(dtos.CreateOrderDTO{
		Plan:     "PRO",
		Currency: "INR",
	})
	orderReq := httptest.NewRequest(http.MethodPost, "/razorpay/order", bytes.NewReader(orderBody))
	orderReq = orderReq.WithContext(context.WithValue(orderReq.Context(), auth.WorkspaceContextKey, wsID))

	orderRec := httptest.NewRecorder()
	router.ServeHTTP(orderRec, orderReq)

	if orderRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", orderRec.Code, orderRec.Body.String())
	}

	var orderResp razorpay.CreateOrderResponse
	if err := json.NewDecoder(orderRec.Body).Decode(&orderResp); err != nil {
		t.Fatalf("failed decoding order response: %v", err)
	}

	if orderResp.Amount != 149900 {
		t.Fatalf("expected 149900 paise, got %d", orderResp.Amount)
	}

	// 2. Generate valid signature
	paymentID := "pay_test_999888"
	mac := hmac.New(sha256.New, []byte(keySecret))
	mac.Write([]byte(orderResp.OrderID + "|" + paymentID))
	validSig := hex.EncodeToString(mac.Sum(nil))

	// 3. Verify payment
	verifyBody, _ := json.Marshal(dtos.VerifyPaymentDTO{
		OrderID:   orderResp.OrderID,
		PaymentID: paymentID,
		Signature: validSig,
		PlanTier:  "TEAM",
	})
	verifyReq := httptest.NewRequest(http.MethodPost, "/razorpay/verify", bytes.NewReader(verifyBody))
	verifyReq = verifyReq.WithContext(context.WithValue(verifyReq.Context(), auth.WorkspaceContextKey, wsID))

	verifyRec := httptest.NewRecorder()
	router.ServeHTTP(verifyRec, verifyReq)

	if verifyRec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for verify, got %d: %s", verifyRec.Code, verifyRec.Body.String())
	}

	var verifyResp map[string]interface{}
	_ = json.NewDecoder(verifyRec.Body).Decode(&verifyResp)
	if verifyResp["plan_tier"] != "TEAM" {
		t.Fatalf("expected upgraded plan TEAM, got %v", verifyResp["plan_tier"])
	}

	// 4. Verify invalid signature fails
	invalidBody, _ := json.Marshal(dtos.VerifyPaymentDTO{
		OrderID:   orderResp.OrderID,
		PaymentID: paymentID,
		Signature: "bad_signature",
		PlanTier:  "TEAM",
	})
	invalidReq := httptest.NewRequest(http.MethodPost, "/razorpay/verify", bytes.NewReader(invalidBody))
	invalidReq = invalidReq.WithContext(context.WithValue(invalidReq.Context(), auth.WorkspaceContextKey, wsID))

	invalidRec := httptest.NewRecorder()
	router.ServeHTTP(invalidRec, invalidReq)

	if invalidRec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for bad signature, got %d", invalidRec.Code)
	}
}

func TestBillingWebhookPayloadLimit(t *testing.T) {
	limiter := llm.NewTokenBudgetLimiter()
	billingSvc := razorpay.NewBillingService(nil, limiter, nil, "https://app.scandrix.dev", "key", "secret", "wh_secret")
	ctrl := controllers.NewBillingController(billingSvc, nil, limiter)
	router := ctrl.WebhookRoutes()

	// 1. Send an oversized payload > 1MB (e.g. 1.5MB) -> MaxBytesReader should reject
	oversizedData := make([]byte, 1500*1024)
	req := httptest.NewRequest(http.MethodPost, "/razorpay", bytes.NewReader(oversizedData))
	req.Header.Set("X-Razorpay-Signature", "any_sig")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for oversized webhook payload, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestBillingWebhookInvalidSignature(t *testing.T) {
	limiter := llm.NewTokenBudgetLimiter()
	billingSvc := razorpay.NewBillingService(nil, limiter, nil, "https://app.scandrix.dev", "key", "secret", "wh_secret_expected")
	ctrl := controllers.NewBillingController(billingSvc, nil, limiter)
	router := ctrl.WebhookRoutes()

	body := []byte(`{"event":"payment.captured"}`)
	req := httptest.NewRequest(http.MethodPost, "/razorpay", bytes.NewReader(body))
	req.Header.Set("X-Razorpay-Signature", "invalid_forged_signature")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 Unauthorized for invalid signature, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestBillingWebhookDecoupledAccepted(t *testing.T) {
	secret := "wh_secret_expected"
	limiter := llm.NewTokenBudgetLimiter()
	billingSvc := razorpay.NewBillingService(nil, limiter, nil, "https://app.scandrix.dev", "key", "secret", secret)
	ctrl := controllers.NewBillingController(billingSvc, nil, limiter)
	router := ctrl.WebhookRoutes()

	body := []byte(`{"event":"payment.captured","payload":{"payment":{"entity":{"id":"pay_123","order_id":"order_123","status":"captured"}}}}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	validSig := hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/razorpay", bytes.NewReader(body))
	req.Header.Set("X-Razorpay-Signature", validSig)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK for valid signature, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed decoding json response: %v", err)
	}
	if resp["status"] != "accepted" {
		t.Fatalf("expected status 'accepted', got %q", resp["status"])
	}
}

type mockBillingRepo struct {
	planDetails      *models.WorkspacePlanDetails
	outboxEvents     []*models.OutboxRecord
	syncRulesCalls   []syncRulesCall
	upgradePlanCalls []upgradePlanCall
}

type syncRulesCall struct {
	WorkspaceID     uuid.UUID
	MaxAllowedRules int
}

type upgradePlanCall struct {
	WorkspaceID uuid.UUID
	PlanTier    string
	MaxSeats    int
	ExpiresAt   time.Time
	Features    []string
}

func (m *mockBillingRepo) GetWorkspacePlanDetails(ctx context.Context, wsID uuid.UUID) (*models.WorkspacePlanDetails, error) {
	if m.planDetails != nil {
		return m.planDetails, nil
	}
	return nil, nil
}

func (m *mockBillingRepo) InsertOutboxEvent(ctx context.Context, event *models.OutboxRecord) error {
	m.outboxEvents = append(m.outboxEvents, event)
	return nil
}

func (m *mockBillingRepo) SyncRulesWithPlanLimit(ctx context.Context, workspaceID uuid.UUID, maxAllowedRules int) (int, error) {
	m.syncRulesCalls = append(m.syncRulesCalls, syncRulesCall{WorkspaceID: workspaceID, MaxAllowedRules: maxAllowedRules})
	return 2, nil
}

func (m *mockBillingRepo) UpgradeWorkspacePlan(ctx context.Context, wsID uuid.UUID, planTier string, maxSeats int, expiresAt time.Time, features []string) error {
	m.upgradePlanCalls = append(m.upgradePlanCalls, upgradePlanCall{
		WorkspaceID: wsID,
		PlanTier:    planTier,
		MaxSeats:    maxSeats,
		ExpiresAt:   expiresAt,
		Features:    features,
	})
	return nil
}

func signPayload(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestBillingLifecycleWebhooksPaymentFailed(t *testing.T) {
	secret := "test_billing_secret_xyz"
	limiter := llm.NewTokenBudgetLimiter()
	repo := &mockBillingRepo{}
	ctrl := controllers.NewBillingController(nil, repo, limiter, secret)
	router := ctrl.WebhookRoutes()

	wsID := uuid.New()
	payload := controllers.PaymentFailedWebhookPayload{
		OrganizationID: wsID.String(),
		WorkspaceID:    wsID.String(),
		Amount:         4900,
		Currency:       "USD",
		FailureReason:  "card_declined",
	}
	body, _ := json.Marshal(payload)

	// 1. Invalid signature -> 401
	reqInvalid := httptest.NewRequest(http.MethodPost, "/payment-failed", bytes.NewReader(body))
	reqInvalid.Header.Set("X-Scandrix-Signature", "wrong_sig")
	recInvalid := httptest.NewRecorder()
	router.ServeHTTP(recInvalid, reqInvalid)
	if recInvalid.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for bad signature, got %d", recInvalid.Code)
	}

	// 2. Valid signature using X-Scandrix-Signature -> 200
	sig := signPayload(secret, body)
	reqValid := httptest.NewRequest(http.MethodPost, "/payment-failed", bytes.NewReader(body))
	reqValid.Header.Set("X-Scandrix-Signature", sig)
	recValid := httptest.NewRecorder()
	router.ServeHTTP(recValid, reqValid)
	if recValid.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid signature, got %d: %s", recValid.Code, recValid.Body.String())
	}

	// 3. Valid signature using X-Razorpay-Signature -> 200
	reqRazorpay := httptest.NewRequest(http.MethodPost, "/payment-failed", bytes.NewReader(body))
	reqRazorpay.Header.Set("X-Razorpay-Signature", sig)
	recRazorpay := httptest.NewRecorder()
	router.ServeHTTP(recRazorpay, reqRazorpay)
	if recRazorpay.Code != http.StatusOK {
		t.Fatalf("expected 200 for Razorpay signature, got %d: %s", recRazorpay.Code, recRazorpay.Body.String())
	}
}

func TestBillingLifecycleWebhooksTrialExpiring(t *testing.T) {
	secret := "test_billing_secret_xyz"
	limiter := llm.NewTokenBudgetLimiter()
	repo := &mockBillingRepo{}
	ctrl := controllers.NewBillingController(nil, repo, limiter, secret)
	router := ctrl.WebhookRoutes()

	wsID := uuid.New()
	payload := controllers.TrialExpiringWebhookPayload{
		OrganizationID: wsID.String(),
		WorkspaceID:    wsID.String(),
		TrialEndsAt:    time.Now().Add(48 * time.Hour).Format(time.RFC3339),
		DaysRemaining:  2,
		UpgradeURL:     "https://app.scandrix.dev/billing/upgrade",
	}
	body, _ := json.Marshal(payload)
	sig := signPayload(secret, body)

	// Valid signature -> 200
	req := httptest.NewRequest(http.MethodPost, "/trial-expiring", bytes.NewReader(body))
	req.Header.Set("X-Scandrix-Signature", sig)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestBillingLifecycleWebhooksPlanChangedAndRuleQuotaSync(t *testing.T) {
	secret := "test_billing_secret_xyz"
	limiter := llm.NewTokenBudgetLimiter()
	repo := &mockBillingRepo{}
	ctrl := controllers.NewBillingController(nil, repo, limiter, secret)
	router := ctrl.WebhookRoutes()

	wsID := uuid.New()
	payload := controllers.PlanChangedWebhookPayload{
		OrganizationID:     wsID.String(),
		WorkspaceID:        wsID.String(),
		PlanType:           "PRO",
		SubscriptionStatus: "active",
	}
	body, _ := json.Marshal(payload)
	sig := signPayload(secret, body)

	req := httptest.NewRequest(http.MethodPost, "/plan-changed", bytes.NewReader(body))
	req.Header.Set("X-Scandrix-Signature", sig)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed decoding json: %v", err)
	}

	if resp["rules_synced"] != true {
		t.Fatalf("expected rules_synced=true, got %v", resp["rules_synced"])
	}

	// Verify repo upgrade plan was called
	if len(repo.upgradePlanCalls) != 1 {
		t.Fatalf("expected 1 upgradePlanCall, got %d", len(repo.upgradePlanCalls))
	}
	if repo.upgradePlanCalls[0].PlanTier != "PRO" {
		t.Fatalf("expected plan tier PRO, got %s", repo.upgradePlanCalls[0].PlanTier)
	}

	// Verify SyncRulesWithPlanLimit was called with Pro limit (50)
	if len(repo.syncRulesCalls) != 1 {
		t.Fatalf("expected 1 syncRulesCall, got %d", len(repo.syncRulesCalls))
	}
	if repo.syncRulesCalls[0].MaxAllowedRules != 50 {
		t.Fatalf("expected max 50 allowed rules for PRO, got %d", repo.syncRulesCalls[0].MaxAllowedRules)
	}
}
