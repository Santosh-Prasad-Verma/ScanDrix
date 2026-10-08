package razorpay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth/mailer"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/enterprise/license"
	"github.com/scandrix/backend/internal/llm"
	"github.com/scandrix/backend/internal/notifications/templates"
	"github.com/scandrix/backend/pkg/models"
)

// BillingService coordinates order creation, checkout verification, and quota upgrades.
type BillingService struct {
	client        *RazorpayClient
	repo          *database.Repository
	budgetLimiter *llm.TokenBudgetLimiter
	mailer        mailer.EmailSender
	appBaseURL    string
	keyID         string
	keySecret     string
	webhookSecret string
	upgradeMu     sync.Mutex
}

// NewBillingService initializes the billing orchestration service.
func NewBillingService(
	repo *database.Repository,
	limiter *llm.TokenBudgetLimiter,
	mailSender mailer.EmailSender,
	appBaseURL string,
	keyID, keySecret, webhookSecret string,
) *BillingService {
	return &BillingService{
		client:        NewRazorpayClient(keyID, keySecret),
		repo:          repo,
		budgetLimiter: limiter,
		mailer:        mailSender,
		appBaseURL:    appBaseURL,
		keyID:         keyID,
		keySecret:     keySecret,
		webhookSecret: webhookSecret,
	}
}

// SetMailer configures or updates the email sender.
func (s *BillingService) SetMailer(m mailer.EmailSender) {
	s.mailer = m
}

// SetAppBaseURL configures the application public URL for email CTAs.
func (s *BillingService) SetAppBaseURL(url string) {
	s.appBaseURL = url
}

// SetClient overrides the internal Razorpay HTTP client (used for tests).
func (s *BillingService) SetClient(client *RazorpayClient) {
	s.client = client
}

// CreateOrderResponse returns checkout parameters needed by the frontend Razorpay SDK.
type CreateOrderResponse struct {
	OrderID  string `json:"order_id"`
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
	KeyID    string `json:"key_id"`
	Receipt  string `json:"receipt"`
	PlanTier string `json:"plan_tier"`
	// BillingInterval is the term this amount was priced for. The client validates
	// it, so an order that was silently priced monthly cannot be presented as
	// annual.
	BillingInterval string `json:"billing_interval"`
}

// VerifyPaymentRequest contains client-side cryptographic proof of payment.
type VerifyPaymentRequest struct {
	OrderID         string `json:"razorpay_order_id"`
	PaymentID       string `json:"razorpay_payment_id"`
	Signature       string `json:"razorpay_signature"`
	PlanTier        string `json:"plan_tier"`
	BillingInterval string `json:"billing_interval,omitempty"`
	RecipientEmail  string `json:"recipient_email,omitempty"`
	RecipientName   string `json:"recipient_name,omitempty"`
}

// SubscriptionAmount computes the total charged amount based on base amount and billing interval.
// Annual subscriptions apply a 10x multiplier (2 months free discount).
func SubscriptionAmount(baseAmount int64, interval string) (int64, error) {
	if baseAmount < 0 {
		return 0, errors.New("negative base amount")
	}
	switch strings.ToLower(strings.TrimSpace(interval)) {
	case "annual", "yearly":
		if baseAmount > math.MaxInt64/10 {
			return 0, errors.New("subscription amount overflow")
		}
		return baseAmount * 10, nil
	case "monthly", "":
		return baseAmount, nil
	default:
		return 0, fmt.Errorf("unsupported billing interval: %s", interval)
	}
}

// CreateSubscriptionOrder generates a Razorpay order and saves a pending transaction record.
//
// billingInterval is "monthly" or "annual". Annual prefers the tier's own stored
// annual price and otherwise applies the documented 10x multiplier to the monthly
// price. This previously took no term at all, so an annual selection was charged
// the monthly amount with nothing reporting the mismatch.
func (s *BillingService) CreateSubscriptionOrder(ctx context.Context, wsID uuid.UUID, plan license.LicenseTier, currency, billingInterval string) (*CreateOrderResponse, error) {
	normTier := license.NormalizeTier(plan)
	curr := strings.ToUpper(strings.TrimSpace(currency))
	if curr == "" {
		curr = "INR"
	}
	interval := strings.ToLower(strings.TrimSpace(billingInterval))
	if interval == "" {
		interval = "monthly"
	}
	if interval != "monthly" && interval != "annual" {
		return nil, fmt.Errorf("unsupported billing interval: %s", billingInterval)
	}

	var amount int64
	if s.repo != nil {
		dbPlan, err := s.repo.GetPlanConfiguration(ctx, string(normTier))
		if err != nil {
			return nil, fmt.Errorf("could not read plan pricing: %w", err)
		}
		if dbPlan != nil {
			if curr == "USD" {
				amount = dbPlan.AmountUSD
			} else {
				amount = dbPlan.AmountINR
			}
			// Prefer the tier's own annual price. Falling back to a multiplier is
			// a pricing policy, not a measurement, so it is only used when the
			// tier has no stored annual price.
			if interval == "annual" {
				var annual *int64
				if curr == "USD" {
					annual = dbPlan.AnnualAmountUSD
				} else {
					annual = dbPlan.AnnualAmountINR
				}
				if annual != nil {
					amount = *annual
				} else {
					scaled, err := SubscriptionAmount(amount, interval)
					if err != nil {
						return nil, err
					}
					amount = scaled
				}
			}
		}
	}

	// Fallback only if database record not found
	if amount == 0 {
		switch normTier {
		case license.TierEnterprise:
			if curr == "USD" {
				amount = 12900 // $129.00
			} else {
				amount = 999900 // ₹9,999.00
			}
		case license.TierScale:
			if curr == "USD" {
				amount = 29900 // $299.00
			} else {
				amount = 2499000 // ₹24,990.00
			}
		case license.TierDeveloper:
			if curr == "USD" {
				amount = 999 // $9.99
			} else {
				amount = 79900 // ₹799.00
			}
		default: // TierTeam / Pro
			normTier = license.TierTeam
			if curr == "USD" {
				amount = 1900 // $19.00
			} else {
				amount = 149900 // ₹1,499.00
			}
		}
		if interval == "annual" {
			scaled, err := SubscriptionAmount(amount, interval)
			if err != nil {
				return nil, err
			}
			amount = scaled
		}
	}

	receipt := fmt.Sprintf("rcpt_%s_%d", wsID.String()[:8], time.Now().Unix())

	if s.keyID == "" || s.keySecret == "" || s.client == nil {
		return nil, errors.New("razorpay credentials not configured: RAZORPAY_KEY_ID and RAZORPAY_KEY_SECRET must be set")
	}

	req := OrderRequest{
		Amount:   amount,
		Currency: curr,
		Receipt:  receipt,
		Notes: map[string]string{
			"workspace_id": wsID.String(),
			"plan_tier":    string(normTier),
		},
	}
	orderResp, err := s.client.CreateOrder(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed creating razorpay order: %w", err)
	}
	orderID := orderResp.ID

	// Persist pending transaction
	tx := &models.BillingTransaction{
		ID:          uuid.New(),
		WorkspaceID: wsID,
		Provider:    "razorpay",
		OrderID:     orderID,
		Amount:      amount,
		Currency:    curr,
		PlanTier:    string(normTier),
		// Persisted so the webhook and verification paths know which term was
		// actually charged. Without this an annual purchase reads back as monthly
		// and the entitlement is provisioned for the wrong term.
		BillingInterval: interval,
		Status:          "created",
		Receipt:         receipt,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}

	if s.repo != nil {
		_ = s.repo.RecordBillingTransaction(ctx, tx)
	}

	return &CreateOrderResponse{
		OrderID:         orderID,
		Amount:          amount,
		Currency:        curr,
		KeyID:           s.keyID,
		Receipt:         receipt,
		PlanTier:        string(normTier),
		BillingInterval: interval,
	}, nil
}

// VerifyAndUpgrade cryptographically verifies the payment and upgrades the workspace.
func (s *BillingService) VerifyAndUpgrade(ctx context.Context, wsID uuid.UUID, req VerifyPaymentRequest) (*models.OrganizationLicense, error) {
	if req.OrderID == "" || req.PaymentID == "" {
		return nil, errors.New("missing order_id or payment_id")
	}

	// Signature verification: fail-closed if key secret is missing
	if s.keySecret == "" {
		return nil, errors.New("unauthorized: razorpay key secret is not configured")
	}

	if !VerifyPaymentSignature(req.OrderID, req.PaymentID, req.Signature, s.keySecret) {
		if s.repo != nil {
			_ = s.repo.UpdateBillingTransactionStatus(ctx, wsID, req.OrderID, req.PaymentID, req.Signature, "signature_verification_failed")
		}
		return nil, errors.New("cryptographic signature mismatch: payment verification failed")
	}

	// Derive and verify order record from database to prevent client tampering
	planTier := req.PlanTier
	var chargedTx *models.BillingTransaction
	if s.repo != nil {
		var err error
		chargedTx, err = s.repo.GetBillingTransaction(ctx, wsID, req.OrderID)
		if err != nil || chargedTx == nil {
			return nil, errors.New("unauthorized: order transaction not found for workspace")
		}
		if chargedTx.PlanTier != "" {
			planTier = chargedTx.PlanTier
		}
	}

	// Server-side payment verification with Razorpay API (Master Rule 2.1 & 5.5)
	if s.client != nil && s.keyID != "" && s.keySecret != "" {
		payment, err := s.client.FetchPayment(ctx, req.PaymentID)
		if err != nil {
			slog.Error("Failed to fetch payment status from Razorpay", "payment_id", req.PaymentID, "error", err)
			return nil, fmt.Errorf("failed verifying payment status with razorpay API: %w", err)
		}
		// Only a captured payment entitles a workspace to a plan. An authorized
		// payment is funds-held and still voidable, so accepting it granted a
		// paid plan that could later evaporate. ProcessVerifiedWebhook already
		// required captured; this keeps the two ingress paths from disagreeing.
		if payment.Status != "captured" {
			return nil, fmt.Errorf("payment verification rejected: razorpay status is %s", payment.Status)
		}
		if payment.OrderID != "" && payment.OrderID != req.OrderID {
			return nil, errors.New("payment order ID mismatch")
		}
		// The provider must confirm the payment we asked about, not merely some
		// captured payment. Without this, only OrderID was bound, so a response
		// describing a different payment passed every later identity check.
		if payment.ID != "" && payment.ID != req.PaymentID {
			slog.Warn("Payment identity tampering attempt detected",
				"workspace_id", wsID, "order_id", req.OrderID, "requested_payment_id", req.PaymentID,
				"provider_payment_id", payment.ID)
			return nil, errors.New("payment ID mismatch")
		}
		// Enforce amount and currency consistency to prevent price/tier tampering
		if chargedTx != nil {
			if chargedTx.Amount > 0 && payment.Amount != chargedTx.Amount {
				slog.Warn("Payment amount tampering attempt detected",
					"workspace_id", wsID, "order_id", req.OrderID, "payment_id", req.PaymentID,
					"expected_amount", chargedTx.Amount, "paid_amount", payment.Amount)
				if s.repo != nil {
					_ = s.repo.UpdateBillingTransactionStatus(ctx, wsID, req.OrderID, req.PaymentID, req.Signature, "amount_mismatch_fraud")
				}
				return nil, fmt.Errorf("payment verification rejected: amount mismatch (expected %d paise, got %d paise)", chargedTx.Amount, payment.Amount)
			}
			if chargedTx.Currency != "" && payment.Currency != "" && !strings.EqualFold(payment.Currency, chargedTx.Currency) {
				slog.Warn("Payment currency tampering attempt detected",
					"workspace_id", wsID, "order_id", req.OrderID, "payment_id", req.PaymentID,
					"expected_currency", chargedTx.Currency, "paid_currency", payment.Currency)
				if s.repo != nil {
					_ = s.repo.UpdateBillingTransactionStatus(ctx, wsID, req.OrderID, req.PaymentID, req.Signature, "currency_mismatch_fraud")
				}
				return nil, fmt.Errorf("payment verification rejected: currency mismatch (expected %s, got %s)", chargedTx.Currency, payment.Currency)
			}
		}
	}

	return s.applyWorkspaceUpgrade(ctx, wsID, req.OrderID, req.PaymentID, req.Signature, planTier, req.RecipientEmail, req.RecipientName)
}

// applyWorkspaceUpgrade executes database plan elevation, quota updates, and email notifications.
func (s *BillingService) applyWorkspaceUpgrade(ctx context.Context, wsID uuid.UUID, orderID, paymentID, signature, planTierStr, recipientEmailParam, recipientNameParam string) (*models.OrganizationLicense, error) {
	s.upgradeMu.Lock()
	defer s.upgradeMu.Unlock()

	planTier := license.NormalizeTier(license.LicenseTier(planTierStr))
	quota := license.GetPlanQuota(planTier)

	var chargedTx *models.BillingTransaction
	if s.repo != nil {
		// Idempotency check: if transaction is already captured, do not re-run side effects
		chargedTx, _ = s.repo.GetBillingTransaction(ctx, wsID, orderID)
		if chargedTx != nil && chargedTx.Status == "captured" {
			if activeLic, _ := s.repo.GetActiveLicense(ctx, wsID); activeLic != nil {
				return activeLic, nil
			}
		}
	}

	// Expiry: default 30 days, or 1 year if annual
	expiresAt := time.Now().UTC().AddDate(0, 1, 0)
	if chargedTx != nil {
		if strings.EqualFold(chargedTx.BillingInterval, "annual") || strings.EqualFold(chargedTx.BillingInterval, "yearly") {
			expiresAt = time.Now().UTC().AddDate(1, 0, 0)
		}
	}
	maxSeats := quota.MaxSeats
	features := []string{
		"saml_sso", "scim_provisioning", "custom_rules", "priority_ai_router",
		"audit_log_cef", "unlimited_repos", "dora_metrics", "byok_encryption",
	}

	if s.repo != nil {
		// Read dynamic entitlements and seats from DB Single Source of Truth
		dbPlan, _ := s.repo.GetPlanConfiguration(ctx, string(planTier))
		if dbPlan != nil {
			maxSeats = dbPlan.MaxSeats
			if dbPlan.MonthlyTokens > 0 {
				quota.MonthlyTokens = dbPlan.MonthlyTokens
			}
			if dbPlan.BurstLimitPerMin > 0 {
				quota.BurstLimitPerMin = dbPlan.BurstLimitPerMin
			}
			if len(dbPlan.FeaturesEnabled) > 0 {
				features = dbPlan.FeaturesEnabled
			}
		}

		// Claim the order before writing the audit side effect. Concurrent
		// deliveries all read status "created" before any of them commits, so
		// without the claim each one recorded its own plan_upgrade audit for a
		// single payment. The upserts above are idempotent and still run; only the
		// audit is gated. An order left in processing_upgrade by a crashed attempt
		// is recovered by reconciliation rather than by a second claim.
		claimed, claimErr := s.repo.ClaimBillingUpgrade(ctx, wsID, orderID)
		if claimErr != nil {
			return nil, fmt.Errorf("failed claiming billing upgrade: %w", claimErr)
		}

		// Atomically update transaction status and upgrade workspace plan in a single transaction
		err := s.repo.UpgradeWorkspacePlanAtomic(ctx, wsID, orderID, paymentID, signature, string(planTier), maxSeats, expiresAt, features)
		if err != nil {
			return nil, fmt.Errorf("failed atomically upgrading workspace plan in database: %w", err)
		}
		if claimed {
			_ = s.repo.InsertAuditLog(ctx, wsID, "system:billing", "", "", "workspace.plan_upgrade", "workspace", wsID.String(), fmt.Appendf(nil, `{"plan":"%s","order_id":"%s","payment_id":"%s"}`, planTier, orderID, paymentID))
		}
	}

	// Upgrade in-memory token rate limiter
	if s.budgetLimiter != nil {
		s.budgetLimiter.SetBudget(llm.WorkspaceTokenBudget{
			WorkspaceID:       wsID,
			MonthlyTokenLimit: quota.MonthlyTokens,
			BurstLimitPerMin:  quota.BurstLimitPerMin,
			LastMonthWindow:   time.Now().UTC(),
			LastMinuteWindow:  time.Now().UTC(),
		})
	}

	// 5. Automatic Email Triggers: Itemized Tax Invoice & Subscription Welcome
	if s.mailer != nil {
		recipientEmail := ""
		recipientName := "Subscriber"
		orgName := string(planTier) + " Workspace"

		if s.repo != nil {
			email, name, _ := s.repo.GetWorkspaceOwner(ctx, wsID)
			if email != "" {
				recipientEmail = email
			}
			if name != "" {
				recipientName = name
			}
			lic, _ := s.repo.GetActiveLicense(ctx, wsID)
			if lic != nil && lic.OrganizationName != "" {
				orgName = lic.OrganizationName
			}
		}
		if recipientEmail == "" && recipientEmailParam != "" {
			recipientEmail = recipientEmailParam
		}
		if recipientNameParam != "" {
			recipientName = recipientNameParam
		}
		if recipientEmail == "" {
			recipientEmail = "billing@" + wsID.String()[:8] + ".scandrix.internal"
		}

		if recipientEmail != "" {
			dashboardURL := s.appBaseURL
			if dashboardURL == "" {
				dashboardURL = "https://app.scandrix.dev"
			}
			billingPortalURL := dashboardURL + "/billing"

			amountFormatted := "₹1,499.00 INR"
			if chargedTx != nil && chargedTx.Amount > 0 {
				if chargedTx.Currency == "USD" {
					amountFormatted = fmt.Sprintf("$%.2f USD", float64(chargedTx.Amount)/100.0)
				} else {
					amountFormatted = fmt.Sprintf("₹%.2f %s", float64(chargedTx.Amount)/100.0, chargedTx.Currency)
				}
			} else if planTier == license.TierEnterprise {
				amountFormatted = "₹9,999.00 INR"
			}

			// 5a. Send Itemized Tax Invoice & Payment Receipt Email
			inv := templates.InvoiceDetails{
				InvoiceNumber:    templates.GenerateInvoiceNumber(orderID),
				RecipientName:    recipientName,
				RecipientEmail:   recipientEmail,
				OrganizationName: orgName,
				PlanTier:         string(planTier),
				AmountFormatted:  amountFormatted,
				OrderID:          orderID,
				PaymentID:        paymentID,
				PaymentProvider:  "Razorpay",
				PaymentMethod:    "UPI / NetBanking / Cards",
				BillingDate:      time.Now().UTC(),
				NextBillingDate:  expiresAt,
				BillingPortalURL: billingPortalURL,
			}
			if mailErr := s.mailer.SendPaymentInvoiceEmail(ctx, recipientEmail, inv); mailErr != nil {
				slog.Error("billing.email.payment_invoice_failed", "recipient", recipientEmail, "error", mailErr)
			}

			// 5b. Send Subscription Welcome Email with unlocked frontier models
			allocatedModels := license.GetAllocatedModelsList(planTier)
			if mailErr := s.mailer.SendSubscriptionWelcomeEmail(ctx, recipientEmail, recipientName, orgName, string(planTier), quota.MonthlyTokens, allocatedModels, dashboardURL); mailErr != nil {
				slog.Error("billing.email.subscription_welcome_failed", "recipient", recipientEmail, "error", mailErr)
			}
		}
	}

	return &models.OrganizationLicense{
		WorkspaceID: wsID,
		// The full workspace UUID keeps this identical to the key
		// UpgradeWorkspacePlanAtomic persists, so the returned license and the
		// stored row agree instead of differing in their last segment.
		LicenseKey:       fmt.Sprintf("SUB-%s-%s", planTier, wsID.String()),
		OrganizationName: fmt.Sprintf("%s Plan", planTier),
		PlanTier:         string(planTier),
		TotalSeats:       quota.MaxSeats,
		AllocatedSeats:   1,
		ExpiresAt:        expiresAt,
		FeaturesEnabled:  features,
		ActivatedAt:      time.Now().UTC(),
	}, nil
}

// WebhookEvent represents an incoming event from Razorpay Webhook.
type WebhookEvent struct {
	Entity    string   `json:"entity"`
	AccountID string   `json:"account_id"`
	Event     string   `json:"event"` // e.g. "order.paid", "payment.captured", "payment.failed"
	Contains  []string `json:"contains"`
	Payload   struct {
		Payment struct {
			Entity struct {
				ID       string            `json:"id"`
				OrderID  string            `json:"order_id"`
				Amount   int64             `json:"amount"`
				Currency string            `json:"currency"`
				Status   string            `json:"status"`
				Notes    map[string]string `json:"notes"`
			} `json:"entity"`
		} `json:"payment"`
		Order struct {
			Entity struct {
				ID     string            `json:"id"`
				Amount int64             `json:"amount"`
				Status string            `json:"status"`
				Notes  map[string]string `json:"notes"`
			} `json:"entity"`
		} `json:"order"`
	} `json:"payload"`
	CreatedAt int64 `json:"created_at"`
}

// VerifyWebhookSignature checks if the HMAC-SHA256 signature is valid.
func (s *BillingService) VerifyWebhookSignature(payload []byte, signature string) bool {
	if s.webhookSecret == "" {
		return false
	}
	return VerifyWebhookSignature(payload, signature, s.webhookSecret)
}

// PrepareVerifiedWebhook verifies tenant identity from the stored transaction, scrubs sensitive customer PII,
// and returns the resolved workspace ID along with the sanitized payload.
func (s *BillingService) PrepareVerifiedWebhook(ctx context.Context, payload []byte) (uuid.UUID, []byte, error) {
	var event WebhookEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return uuid.Nil, nil, fmt.Errorf("failed parsing webhook event payload: %w", err)
	}

	orderID := event.Payload.Payment.Entity.OrderID
	if orderID == "" {
		orderID = event.Payload.Order.Entity.ID
	}
	if orderID == "" {
		return uuid.Nil, nil, errors.New("webhook payload missing order_id")
	}

	if s.repo == nil {
		return uuid.Nil, nil, errors.New("repository unavailable for webhook resolution")
	}

	tx, err := s.repo.LookupBillingTransactionByOrderID(ctx, orderID)
	if err != nil || tx == nil {
		return uuid.Nil, nil, fmt.Errorf("unrecognized order_id '%s': transaction not found in database", orderID)
	}

	// Sanitize payload: strip PII (email, customer info) and untrusted notes from payload
	var rawMap map[string]any
	if err := json.Unmarshal(payload, &rawMap); err != nil {
		return tx.WorkspaceID, payload, nil
	}

	if p, ok := rawMap["payload"].(map[string]any); ok {
		if payment, ok := p["payment"].(map[string]any); ok {
			if entity, ok := payment["entity"].(map[string]any); ok {
				delete(entity, "email")
				delete(entity, "contact")
				delete(entity, "notes")
			}
		}
		if order, ok := p["order"].(map[string]any); ok {
			if entity, ok := order["entity"].(map[string]any); ok {
				delete(entity, "notes")
			}
		}
	}

	filteredBytes, err := json.Marshal(rawMap)
	if err != nil {
		filteredBytes = payload
	}

	return tx.WorkspaceID, filteredBytes, nil
}

// ProcessVerifiedWebhook processes an unmarshaled or already signature-verified webhook payload asynchronously.
func (s *BillingService) ProcessVerifiedWebhook(ctx context.Context, payload []byte) error {
	var event WebhookEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("failed parsing webhook event payload: %w", err)
	}

	switch event.Event {
	case "order.paid", "payment.captured":
		orderID := event.Payload.Payment.Entity.OrderID
		if orderID == "" {
			orderID = event.Payload.Order.Entity.ID
		}
		paymentID := event.Payload.Payment.Entity.ID

		if orderID == "" || paymentID == "" {
			return errors.New("missing order_id or payment_id in webhook")
		}

		// Server-side provider verification: provider payment must be captured
		if s.client != nil {
			payment, err := s.client.FetchPayment(ctx, paymentID)
			if err != nil {
				return fmt.Errorf("failed fetching payment from razorpay API: %w", err)
			}
			if payment.Status != "captured" {
				return fmt.Errorf("signed event claims cannot replace provider capture: razorpay status is %s", payment.Status)
			}
		}

		// Look up stored transaction to get the genuine tenant workspace ID and plan tier
		var tx *models.BillingTransaction
		if s.repo != nil {
			var err error
			tx, err = s.repo.LookupBillingTransactionByOrderID(ctx, orderID)
			if err != nil || tx == nil {
				return fmt.Errorf("transaction not found for order: %s", orderID)
			}
		}
		if tx == nil {
			return errors.New("cannot verify payment without database transaction")
		}

		wsID := tx.WorkspaceID
		planTierStr := tx.PlanTier
		if planTierStr == "" {
			planTierStr = "TEAM"
		}

		if err := func() error {
			_, err := s.applyWorkspaceUpgrade(ctx, wsID, orderID, paymentID, "", planTierStr, "", "")
			return err
		}(); err != nil {
			slog.Error("Razorpay webhook upgrade failed", "workspace_id", wsID, "order_id", orderID, "error", err)
			return err
		}

	case "payment.failed":
		orderID := event.Payload.Payment.Entity.OrderID
		paymentID := event.Payload.Payment.Entity.ID
		var wsID uuid.UUID
		if s.repo != nil && orderID != "" {
			if tx, err := s.repo.LookupBillingTransactionByOrderID(ctx, orderID); err == nil && tx != nil {
				wsID = tx.WorkspaceID
			}
		}
		if wsID != uuid.Nil {
			if s.repo != nil {
				_ = s.repo.UpdateBillingTransactionStatus(ctx, wsID, orderID, paymentID, "", "failed")
			}
			if s.mailer != nil {
				recipientEmail := ""
				recipientName := "Valued Customer"
				if s.repo != nil {
					recipientEmail, recipientName, _ = s.repo.GetWorkspaceOwner(ctx, wsID)
				}
				if recipientEmail != "" {
					retryURL := s.appBaseURL + "/billing"
					if mailErr := s.mailer.SendPaymentFailedEmail(ctx, recipientEmail, recipientName, "Workspace", "Pro Team", orderID, "Transaction declined by issuing bank", retryURL); mailErr != nil {
						slog.Error("billing.email.payment_failed_notification_failed", "recipient", recipientEmail, "error", mailErr)
					}
				}
			}
		}

	case "refund.processed", "refund.created":
		paymentID := event.Payload.Payment.Entity.ID
		orderID := event.Payload.Payment.Entity.OrderID
		var wsID uuid.UUID
		if s.repo != nil && orderID != "" {
			if tx, err := s.repo.LookupBillingTransactionByOrderID(ctx, orderID); err == nil && tx != nil {
				wsID = tx.WorkspaceID
			}
		}
		if wsID != uuid.Nil {
			if s.repo != nil {
				_ = s.repo.UpdateBillingTransactionStatus(ctx, wsID, orderID, paymentID, "", "refunded")
			}
			slog.Warn("Razorpay refund event recorded",
				"event", event.Event,
				"workspace_id", wsID,
				"payment_id", paymentID,
				"order_id", orderID,
			)
		}
	}
	return nil
}

// ProcessWebhook validates the webhook signature and processes payment state changes synchronously.
func (s *BillingService) ProcessWebhook(ctx context.Context, payload []byte, signature string) error {
	if !s.VerifyWebhookSignature(payload, signature) {
		return errors.New("unauthorized webhook: invalid razorpay signature or secret unconfigured")
	}
	return s.ProcessVerifiedWebhook(ctx, payload)
}
