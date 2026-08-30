package razorpay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
	OrderID   string `json:"order_id"`
	Amount    int64  `json:"amount"`
	Currency  string `json:"currency"`
	KeyID     string `json:"key_id"`
	Receipt   string `json:"receipt"`
	PlanTier  string `json:"plan_tier"`
}

// VerifyPaymentRequest contains client-side cryptographic proof of payment.
type VerifyPaymentRequest struct {
	OrderID        string `json:"razorpay_order_id"`
	PaymentID      string `json:"razorpay_payment_id"`
	Signature      string `json:"razorpay_signature"`
	PlanTier       string `json:"plan_tier"`
	RecipientEmail string `json:"recipient_email,omitempty"`
	RecipientName  string `json:"recipient_name,omitempty"`
}

// CreateSubscriptionOrder generates a Razorpay order and saves a pending transaction record.
func (s *BillingService) CreateSubscriptionOrder(ctx context.Context, wsID uuid.UUID, plan license.LicenseTier, currency string) (*CreateOrderResponse, error) {
	normTier := license.NormalizeTier(plan)
	curr := strings.ToUpper(strings.TrimSpace(currency))
	if curr == "" {
		curr = "INR"
	}

	var amount int64
	if s.repo != nil {
		dbPlan, _ := s.repo.GetPlanConfiguration(ctx, string(normTier))
		if dbPlan != nil {
			if curr == "USD" {
				amount = dbPlan.AmountUSD
			} else {
				amount = dbPlan.AmountINR
			}
		}
	}

	// Fallback only if database record not found
	if amount == 0 {
		switch normTier {
		case license.TierEnterprise:
			if curr == "USD" {
				amount = 24900 // $249.00
			} else {
				amount = 1999900 // ₹19,999.00
			}
		default: // TierTeam / Pro
			normTier = license.TierTeam
			if curr == "USD" {
				amount = 2900 // $29.00
			} else {
				amount = 249900 // ₹2,499.00
			}
		}
	}

	receipt := fmt.Sprintf("rcpt_%s_%d", wsID.String()[:8], time.Now().Unix())

	var orderID string
	// If Razorpay API credentials are configured, call upstream Razorpay API
	if s.keyID != "" && s.keySecret != "" {
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
		orderID = orderResp.ID
	} else {
		// Mock / Dev fallback order ID when keys are not yet configured in local test
		orderID = fmt.Sprintf("order_mock_%s_%d", wsID.String()[:8], time.Now().Unix())
	}

	// Persist pending transaction
	tx := &models.BillingTransaction{
		ID:          uuid.New(),
		WorkspaceID: wsID,
		Provider:    "razorpay",
		OrderID:     orderID,
		Amount:      amount,
		Currency:    curr,
		PlanTier:    string(normTier),
		Status:      "created",
		Receipt:     receipt,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	if s.repo != nil {
		_ = s.repo.RecordBillingTransaction(ctx, tx)
	}

	return &CreateOrderResponse{
		OrderID:  orderID,
		Amount:   amount,
		Currency: curr,
		KeyID:    s.keyID,
		Receipt:  receipt,
		PlanTier: string(normTier),
	}, nil
}

// VerifyAndUpgrade cryptographically verifies the payment and upgrades the workspace.
func (s *BillingService) VerifyAndUpgrade(ctx context.Context, wsID uuid.UUID, req VerifyPaymentRequest) (*models.OrganizationLicense, error) {
	if req.OrderID == "" || req.PaymentID == "" {
		return nil, errors.New("missing order_id or payment_id")
	}

	// Signature verification: if key secret is present, enforce strict HMAC-SHA256
	if s.keySecret != "" {
		if !VerifyPaymentSignature(req.OrderID, req.PaymentID, req.Signature, s.keySecret) {
			if s.repo != nil {
				_ = s.repo.UpdateBillingTransactionStatus(ctx, wsID, req.OrderID, req.PaymentID, req.Signature, "signature_verification_failed")
			}
			return nil, errors.New("cryptographic signature mismatch: payment verification failed")
		}
	}

	planTier := license.NormalizeTier(license.LicenseTier(req.PlanTier))
	quota := license.GetPlanQuota(planTier)

	// Expiry: 30 days active subscription
	expiresAt := time.Now().UTC().AddDate(0, 1, 0)
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
			if len(dbPlan.FeaturesEnabled) > 0 {
				features = dbPlan.FeaturesEnabled
			}
		}

		// Update transaction status
		_ = s.repo.UpdateBillingTransactionStatus(ctx, wsID, req.OrderID, req.PaymentID, req.Signature, "captured")

		// Upgrade workspace in PostgreSQL
		err := s.repo.UpgradeWorkspacePlan(ctx, wsID, string(planTier), maxSeats, expiresAt, features)
		if err != nil {
			return nil, fmt.Errorf("failed upgrading workspace plan in database: %w", err)
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

		if recipientEmail == "" && req.RecipientEmail != "" {
			recipientEmail = req.RecipientEmail
		}
		if req.RecipientName != "" {
			recipientName = req.RecipientName
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

			amountFormatted := "₹2,499.00 INR"
			if planTier == license.TierEnterprise {
				amountFormatted = "₹19,999.00 INR"
			}

			// 5a. Send Itemized Tax Invoice & Payment Receipt Email
			inv := templates.InvoiceDetails{
				InvoiceNumber:    templates.GenerateInvoiceNumber(req.OrderID),
				RecipientName:    recipientName,
				RecipientEmail:   recipientEmail,
				OrganizationName: orgName,
				PlanTier:         string(planTier),
				AmountFormatted:  amountFormatted,
				OrderID:          req.OrderID,
				PaymentID:        req.PaymentID,
				PaymentProvider:  "Razorpay",
				PaymentMethod:    "UPI / NetBanking / Cards",
				BillingDate:      time.Now().UTC(),
				NextBillingDate:  expiresAt,
				BillingPortalURL: billingPortalURL,
			}
			_ = s.mailer.SendPaymentInvoiceEmail(ctx, recipientEmail, inv)

			// 5b. Send Subscription Welcome Email with unlocked frontier models
			allocatedModels := license.GetAllocatedModelsList(planTier)
			_ = s.mailer.SendSubscriptionWelcomeEmail(ctx, recipientEmail, recipientName, orgName, string(planTier), quota.MonthlyTokens, allocatedModels, dashboardURL)
		}
	}

	return &models.OrganizationLicense{
		WorkspaceID:      wsID,
		LicenseKey:       fmt.Sprintf("SUB-%s-%s", planTier, wsID.String()[:8]),
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
	Entity    string `json:"entity"`
	AccountID string `json:"account_id"`
	Event     string `json:"event"` // e.g. "order.paid", "payment.captured", "payment.failed"
	Contains  []string `json:"contains"`
	Payload   struct {
		Payment struct {
			Entity struct {
				ID          string            `json:"id"`
				OrderID     string            `json:"order_id"`
				Amount      int64             `json:"amount"`
				Currency    string            `json:"currency"`
				Status      string            `json:"status"`
				Notes       map[string]string `json:"notes"`
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

// ProcessWebhook validates the webhook signature and processes asynchronous payment state changes.
func (s *BillingService) ProcessWebhook(ctx context.Context, payload []byte, signature string) error {
	if s.webhookSecret != "" {
		if !VerifyWebhookSignature(payload, signature, s.webhookSecret) {
			return errors.New("unauthorized webhook: invalid razorpay signature")
		}
	}

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

		wsIDStr := event.Payload.Payment.Entity.Notes["workspace_id"]
		if wsIDStr == "" {
			wsIDStr = event.Payload.Order.Entity.Notes["workspace_id"]
		}
		wsID, err := uuid.Parse(wsIDStr)
		if err != nil {
			return nil // No workspace associated; skip
		}

		planTierStr := event.Payload.Payment.Entity.Notes["plan_tier"]
		if planTierStr == "" {
			planTierStr = event.Payload.Order.Entity.Notes["plan_tier"]
		}
		if planTierStr == "" {
			planTierStr = "TEAM"
		}

		_, _ = s.VerifyAndUpgrade(ctx, wsID, VerifyPaymentRequest{
			OrderID:   orderID,
			PaymentID: paymentID,
			Signature: signature,
			PlanTier:  planTierStr,
		})

	case "payment.failed":
		orderID := event.Payload.Payment.Entity.OrderID
		paymentID := event.Payload.Payment.Entity.ID
		wsIDStr := event.Payload.Payment.Entity.Notes["workspace_id"]
		if wsID, err := uuid.Parse(wsIDStr); err == nil {
			if s.repo != nil {
				_ = s.repo.UpdateBillingTransactionStatus(ctx, wsID, orderID, paymentID, signature, "failed")
			}
			if s.mailer != nil {
				recipientEmail := ""
				recipientName := "Valued Customer"
				if s.repo != nil {
					recipientEmail, recipientName, _ = s.repo.GetWorkspaceOwner(ctx, wsID)
				}
				if recipientEmail != "" {
					retryURL := s.appBaseURL + "/billing"
					_ = s.mailer.SendPaymentFailedEmail(ctx, recipientEmail, recipientName, "Workspace", "Pro Team", orderID, "Transaction declined by issuing bank", retryURL)
				}
			}
		}
	}

	return nil
}
