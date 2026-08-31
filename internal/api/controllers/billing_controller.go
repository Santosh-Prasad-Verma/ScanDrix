package controllers

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/billing/razorpay"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/enterprise/license"
	"github.com/scandrix/backend/internal/llm"
)

// BillingController handles Razorpay subscription purchases, payment verification, and plan querying.
type BillingController struct {
	billingSvc *razorpay.BillingService
	repo       *database.Repository
	limiter    *llm.TokenBudgetLimiter
}

// NewBillingController initializes the controller with billing services.
func NewBillingController(
	billingSvc *razorpay.BillingService,
	repo *database.Repository,
	limiter *llm.TokenBudgetLimiter,
) *BillingController {
	return &BillingController{
		billingSvc: billingSvc,
		repo:       repo,
		limiter:    limiter,
	}
}

// Routes mounts all billing endpoints (for standalone mounting).
func (c *BillingController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Post("/razorpay/order", c.handleCreateOrder)
	r.Post("/razorpay/verify", c.handleVerifyPayment)
	r.Get("/plan", c.handleGetPlan)
	r.Post("/razorpay/webhook", c.handleWebhook)

	return r
}

// ProtectedRoutes mounts authenticated endpoints requiring workspace auth.
func (c *BillingController) ProtectedRoutes() chi.Router {
	r := chi.NewRouter()

	r.Post("/razorpay/order", c.handleCreateOrder)
	r.Post("/razorpay/verify", c.handleVerifyPayment)
	r.Get("/plan", c.handleGetPlan)

	return r
}

// WebhookRoutes mounts public webhook receivers authenticated via HMAC signature.
func (c *BillingController) WebhookRoutes() chi.Router {
	r := chi.NewRouter()

	r.Post("/razorpay", c.handleWebhook)

	return r
}

func (c *BillingController) handleCreateOrder(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	var req dtos.CreateOrderDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
		return
	}

	planTier := license.LicenseTier(req.Plan)
	if req.Plan == "" {
		planTier = license.TierTeam
	}

	orderResp, err := c.billingSvc.CreateSubscriptionOrder(r.Context(), wsID, planTier, req.Currency)
	if err != nil {
		slog.Error("Failed creating subscription order", "workspace_id", wsID, "plan", planTier, "error", err)
		http.Error(w, `{"error":"failed creating subscription order"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(orderResp)
}

func (c *BillingController) handleVerifyPayment(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	var req dtos.VerifyPaymentDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
		return
	}

	if req.OrderID == "" || req.PaymentID == "" {
		http.Error(w, `{"error":"razorpay_order_id and razorpay_payment_id are required"}`, http.StatusBadRequest)
		return
	}

	lic, err := c.billingSvc.VerifyAndUpgrade(r.Context(), wsID, razorpay.VerifyPaymentRequest{
		OrderID:        req.OrderID,
		PaymentID:      req.PaymentID,
		Signature:      req.Signature,
		PlanTier:       req.PlanTier,
		RecipientEmail: req.RecipientEmail,
		RecipientName:  req.RecipientName,
	})
	if err != nil {
		slog.Warn("Payment verification failed", "workspace_id", wsID, "order_id", req.OrderID, "error", err)
		http.Error(w, `{"error":"payment verification failed"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":     true,
		"plan_tier":   lic.PlanTier,
		"total_seats": lic.TotalSeats,
		"expires_at":  lic.ExpiresAt,
	})
}

func (c *BillingController) handleGetPlan(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	if c.repo == nil {
		http.Error(w, `{"error":"database service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	planDetails, err := c.repo.GetWorkspacePlanDetails(r.Context(), wsID)
	if err != nil || planDetails == nil {
		http.Error(w, `{"error":"failed querying workspace plan details"}`, http.StatusInternalServerError)
		return
	}

	if c.limiter != nil {
		_, _, usedMin, _, exists := c.limiter.GetUsage(wsID)
		if exists && usedMin > planDetails.BurstTokensUsed {
			planDetails.BurstTokensUsed = usedMin
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(planDetails)
}

func (c *BillingController) handleWebhook(w http.ResponseWriter, r *http.Request) {
	signature := r.Header.Get("X-Razorpay-Signature")
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"failed reading body"}`, http.StatusBadRequest)
		return
	}

	if err := c.billingSvc.ProcessWebhook(r.Context(), body, signature); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"processed"}`))
}
