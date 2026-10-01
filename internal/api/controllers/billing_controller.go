package controllers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/billing/razorpay"
	"github.com/scandrix/backend/internal/enterprise/license"
	"github.com/scandrix/backend/internal/llm"
	"github.com/scandrix/backend/pkg/models"
)

// PaymentFailedWebhookPayload models billing payment failure notifications.
type PaymentFailedWebhookPayload struct {
	OrganizationID   string  `json:"organizationId,omitempty"`
	WorkspaceID      string  `json:"workspaceId,omitempty"`
	Amount           float64 `json:"amount"`
	Currency         string  `json:"currency"`
	FailureReason    string  `json:"failureReason"`
	NextRetryAt      string  `json:"nextRetryAt,omitempty"`
	UpdatePaymentURL string  `json:"updatePaymentUrl,omitempty"`
}

// TrialExpiringWebhookPayload models trial expiring reminder notifications.
type TrialExpiringWebhookPayload struct {
	OrganizationID string `json:"organizationId,omitempty"`
	WorkspaceID    string `json:"workspaceId,omitempty"`
	TrialEndsAt    string `json:"trialEndsAt"`
	DaysRemaining  int    `json:"daysRemaining"`
	UpgradeURL     string `json:"upgradeUrl,omitempty"`
}

// PlanChangedWebhookPayload models plan change notifications.
type PlanChangedWebhookPayload struct {
	OrganizationID     string `json:"organizationId,omitempty"`
	WorkspaceID        string `json:"workspaceId,omitempty"`
	TeamID             string `json:"teamId,omitempty"`
	PlanType           string `json:"planType"`
	SubscriptionStatus string `json:"subscriptionStatus,omitempty"`
}

// BillingRepository defines domain data operations required by BillingController (Clean Architecture).
type BillingRepository interface {
	GetWorkspacePlanDetails(ctx context.Context, wsID uuid.UUID) (*models.WorkspacePlanDetails, error)
	InsertOutboxEvent(ctx context.Context, event *models.OutboxRecord) error
	SyncRulesWithPlanLimit(ctx context.Context, workspaceID uuid.UUID, maxAllowedRules int) (int, error)
	UpgradeWorkspacePlan(ctx context.Context, wsID uuid.UUID, planTier string, maxSeats int, expiresAt time.Time, features []string) error
	ListPlanConfigurations(ctx context.Context) ([]models.PlanConfiguration, error)
}

// BillingController handles Razorpay subscription purchases, payment verification, and plan querying.
type BillingController struct {
	billingSvc           *razorpay.BillingService
	repo                 BillingRepository
	limiter              *llm.TokenBudgetLimiter
	sem                  chan struct{}
	billingWebhookSecret string
}

// NewBillingController initializes the controller with billing services.
func NewBillingController(
	billingSvc *razorpay.BillingService,
	repo BillingRepository,
	limiter *llm.TokenBudgetLimiter,
	billingWebhookSecret ...string,
) *BillingController {
	if isNilInterface(repo) {
		repo = nil
	}
	sec := ""
	if len(billingWebhookSecret) > 0 {
		sec = billingWebhookSecret[0]
	}
	if sec == "" {
		sec = os.Getenv("API_BILLING_WEBHOOK_SECRET")
	}
	if sec == "" {
		sec = os.Getenv("BILLING_WEBHOOK_SECRET")
	}
	if sec == "" {
		sec = os.Getenv("RAZORPAY_WEBHOOK_SECRET")
	}
	return &BillingController{
		billingSvc:           billingSvc,
		repo:                 repo,
		limiter:              limiter,
		sem:                  make(chan struct{}, 16),
		billingWebhookSecret: sec,
	}
}

// Routes mounts all billing endpoints (for standalone mounting).
func (c *BillingController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/plans", c.HandleListPlans)
	r.Post("/razorpay/order", c.handleCreateOrder)
	r.Post("/razorpay/verify", c.handleVerifyPayment)
	r.Get("/plan", c.handleGetPlan)
	r.Post("/razorpay/webhook", c.handleWebhook)

	// ScanDrix billing webhook lifecycle endpoints
	r.Post("/webhook/payment-failed", c.handlePaymentFailed)
	r.Post("/webhook/trial-expiring", c.handleTrialExpiring)
	r.Post("/webhook/plan-changed", c.handlePlanChanged)

	return r
}

// ProtectedRoutes mounts authenticated endpoints requiring workspace auth.
func (c *BillingController) ProtectedRoutes() chi.Router {
	r := chi.NewRouter()

	r.Get("/plans", c.HandleListPlans)
	r.Post("/razorpay/order", c.handleCreateOrder)
	r.Post("/razorpay/verify", c.handleVerifyPayment)
	r.Get("/plan", c.handleGetPlan)

	return r
}

// WebhookRoutes mounts public webhook receivers authenticated via HMAC signature.
func (c *BillingController) WebhookRoutes() chi.Router {
	r := chi.NewRouter()

	r.Get("/plans", c.HandleListPlans)
	r.Post("/razorpay", c.handleWebhook)
	r.Post("/payment-failed", c.handlePaymentFailed)
	r.Post("/trial-expiring", c.handleTrialExpiring)
	r.Post("/plan-changed", c.handlePlanChanged)

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

// HandleListPlans returns all active subscription plans from PostgreSQL.
func (c *BillingController) HandleListPlans(w http.ResponseWriter, r *http.Request) {
	if c.repo == nil {
		http.Error(w, `{"error":"database service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	plans, err := c.repo.ListPlanConfigurations(r.Context())
	if err != nil {
		slog.Error("Failed listing plan configurations", "error", err)
		http.Error(w, `{"error":"failed querying plans"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"plans":   plans,
	})
}

func (c *BillingController) handleWebhook(w http.ResponseWriter, r *http.Request) {
	signature := r.Header.Get("X-Razorpay-Signature")

	// Bound incoming payload size to 1MB to prevent memory exhaustion DoS (Master Rule 5.3)
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"payload too large or failed reading body"}`, http.StatusBadRequest)
		return
	}

	// 1. Verify cryptographic signature first (fail-closed)
	if !c.billingSvc.VerifyWebhookSignature(body, signature) {
		slog.Warn("Razorpay webhook verification rejected: invalid signature")
		http.Error(w, `{"error":"unauthorized webhook delivery"}`, http.StatusUnauthorized)
		return
	}

	// 2. Extract tenant workspace_id from payload notes if present
	var rawEvent struct {
		Payload struct {
			Payment struct {
				Entity struct {
					Notes map[string]string `json:"notes"`
				} `json:"entity"`
			} `json:"payment"`
			Order struct {
				Entity struct {
					Notes map[string]string `json:"notes"`
				} `json:"entity"`
			} `json:"order"`
		} `json:"payload"`
	}
	_ = json.Unmarshal(body, &rawEvent)

	var wsID uuid.UUID
	wsIDStr := rawEvent.Payload.Payment.Entity.Notes["workspace_id"]
	if wsIDStr == "" {
		wsIDStr = rawEvent.Payload.Order.Entity.Notes["workspace_id"]
	}
	if parsed, err := uuid.Parse(wsIDStr); err == nil {
		wsID = parsed
	}

	// 3. Persist billing event into outbox for guaranteed transactional delivery
	if c.repo != nil && wsID != uuid.Nil {
		outboxRecord := &models.OutboxRecord{
			ID:          uuid.New(),
			WorkspaceID: wsID,
			EventType:   "billing.razorpay.webhook",
			Payload:     body,
		}
		if err := c.repo.InsertOutboxEvent(r.Context(), outboxRecord); err != nil {
			slog.Error("Failed to record billing webhook in outbox", "workspace_id", wsID, "error", err)
		}
	}

	// 4. Immediately acknowledge webhook with HTTP 200 to prevent Razorpay delivery timeouts
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"accepted"}`))

	// 5. Asynchronously process the verified payment upgrade off the HTTP thread with bounded concurrency
	select {
	case c.sem <- struct{}{}:
		go func(payload []byte) {
			defer func() { <-c.sem }()
			asyncCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := c.billingSvc.ProcessVerifiedWebhook(asyncCtx, payload); err != nil {
				slog.Error("Asynchronous Razorpay webhook processing failed", "error", err)
			}
		}(body)
	default:
		slog.Warn("Billing webhook worker pool saturated; relying on transactional outbox relay for asynchronous processing", "workspace_id", wsID)
	}
}

func (c *BillingController) verifyBillingSignature(r *http.Request, body []byte) bool {
	secret := c.billingWebhookSecret
	if secret == "" {
		slog.Error("Billing webhook secret is not configured — rejecting webhook")
		return false
	}

	sigHeader := r.Header.Get("X-ScanDrix-Signature")
	if sigHeader == "" {
		sigHeader = r.Header.Get("X-Signature")
	}
	if sigHeader == "" {
		sigHeader = r.Header.Get("X-Razorpay-Signature")
	}
	if sigHeader == "" {
		return false
	}

	sig := strings.TrimPrefix(sigHeader, "sha256=")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expectedMAC := hex.EncodeToString(mac.Sum(nil))

	return subtle.ConstantTimeCompare([]byte(sig), []byte(expectedMAC)) == 1
}

func (c *BillingController) handlePaymentFailed(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"payload too large or failed reading body"}`, http.StatusBadRequest)
		return
	}

	if !c.verifyBillingSignature(r, body) {
		http.Error(w, `{"error":"invalid billing signature"}`, http.StatusUnauthorized)
		return
	}

	var payload PaymentFailedWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, `{"error":"invalid json payload"}`, http.StatusBadRequest)
		return
	}

	targetID := payload.WorkspaceID
	if targetID == "" {
		targetID = payload.OrganizationID
	}
	if targetID == "" {
		http.Error(w, `{"error":"missing organizationId or workspaceId"}`, http.StatusBadRequest)
		return
	}

	var wsID uuid.UUID
	if parsed, err := uuid.Parse(targetID); err == nil {
		wsID = parsed
	}

	if c.repo != nil && wsID != uuid.Nil {
		_ = c.repo.InsertOutboxEvent(r.Context(), &models.OutboxRecord{
			ID:          uuid.New(),
			WorkspaceID: wsID,
			EventType:   "billing.payment_failed",
			Payload:     body,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (c *BillingController) handleTrialExpiring(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"payload too large or failed reading body"}`, http.StatusBadRequest)
		return
	}

	if !c.verifyBillingSignature(r, body) {
		http.Error(w, `{"error":"invalid billing signature"}`, http.StatusUnauthorized)
		return
	}

	var payload TrialExpiringWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, `{"error":"invalid json payload"}`, http.StatusBadRequest)
		return
	}

	targetID := payload.WorkspaceID
	if targetID == "" {
		targetID = payload.OrganizationID
	}
	if targetID == "" {
		http.Error(w, `{"error":"missing organizationId or workspaceId"}`, http.StatusBadRequest)
		return
	}

	var wsID uuid.UUID
	if parsed, err := uuid.Parse(targetID); err == nil {
		wsID = parsed
	}

	if c.repo != nil && wsID != uuid.Nil {
		_ = c.repo.InsertOutboxEvent(r.Context(), &models.OutboxRecord{
			ID:          uuid.New(),
			WorkspaceID: wsID,
			EventType:   "billing.trial_expiring",
			Payload:     body,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (c *BillingController) handlePlanChanged(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"payload too large or failed reading body"}`, http.StatusBadRequest)
		return
	}

	if !c.verifyBillingSignature(r, body) {
		http.Error(w, `{"error":"invalid billing signature"}`, http.StatusUnauthorized)
		return
	}

	var payload PlanChangedWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, `{"error":"invalid json payload"}`, http.StatusBadRequest)
		return
	}

	targetID := payload.WorkspaceID
	if targetID == "" {
		targetID = payload.OrganizationID
	}
	if targetID == "" {
		http.Error(w, `{"error":"missing organizationId or workspaceId"}`, http.StatusBadRequest)
		return
	}

	var wsID uuid.UUID
	if parsed, err := uuid.Parse(targetID); err == nil {
		wsID = parsed
	}

	// Normalize plan tier and determine rule limits
	tier := license.NormalizeTier(license.LicenseTier(payload.PlanType))
	maxRules := 5 // default Community/Free limit
	switch tier {
	case license.TierEnterprise:
		maxRules = 0 // unlimited
	case license.TierTeam:
		maxRules = 50
	case license.TierDeveloper:
		maxRules = 20
	}

	disabledCount := 0
	if c.repo != nil && wsID != uuid.Nil {
		// Sync custom rules quota
		var err error
		disabledCount, err = c.repo.SyncRulesWithPlanLimit(r.Context(), wsID, maxRules)
		if err != nil {
			slog.Error("Failed syncing rules with plan limit in billing webhook", "workspace_id", wsID, "error", err)
		}

		// Update workspace license tier in database
		maxSeats := 5
		if tier == license.TierDeveloper {
			maxSeats = 10
		} else if tier == license.TierTeam {
			maxSeats = 25
		} else if tier == license.TierEnterprise {
			maxSeats = 100
		}
		expiresAt := time.Now().UTC().AddDate(1, 0, 0)
		features := []string{"pr_reviews", "automated_rules"}
		planName := strings.ToUpper(strings.TrimSpace(payload.PlanType))
		if planName == "" {
			planName = string(tier)
		}
		_ = c.repo.UpgradeWorkspacePlan(r.Context(), wsID, planName, maxSeats, expiresAt, features)

		// Record outbox event
		_ = c.repo.InsertOutboxEvent(r.Context(), &models.OutboxRecord{
			ID:          uuid.New(),
			WorkspaceID: wsID,
			EventType:   "billing.plan_changed",
			Payload:     body,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":         "ok",
		"rules_synced":   true,
		"disabled_rules": disabledCount,
	})
}

