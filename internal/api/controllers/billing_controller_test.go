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

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/billing/razorpay"
	"github.com/scandrix/backend/internal/llm"
)

func TestBillingControllerPlanQuery(t *testing.T) {
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

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp dtos.WorkspacePlanStatusResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed decoding plan response: %v", err)
	}

	if resp.PlanTier != "COMMUNITY" {
		t.Fatalf("expected COMMUNITY default tier, got %s", resp.PlanTier)
	}
	if resp.MonthlyTokenLimit != 500_000 {
		t.Fatalf("expected 500K monthly tokens for community tier, got %d", resp.MonthlyTokenLimit)
	}
	if len(resp.AllocatedModels) == 0 {
		t.Fatalf("expected allocated models for community tier")
	}
}

func TestBillingControllerOrderAndVerifyFlow(t *testing.T) {
	wsID := uuid.New()
	keySecret := "sec_checkout_123"
	limiter := llm.NewTokenBudgetLimiter()

	billingSvc := razorpay.NewBillingService(nil, limiter, nil, "https://app.scandrix.dev", "", keySecret, "wh_sec_123")
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

	if orderResp.Amount != 249900 {
		t.Fatalf("expected 249900 paise, got %d", orderResp.Amount)
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
