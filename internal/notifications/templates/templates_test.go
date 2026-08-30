package templates_test

import (
	"strings"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/notifications/templates"
)

func TestRenderSubscriptionWelcome(t *testing.T) {
	models := []string{"claude-sonnet-5", "gpt-5.6-terra", "gemini-3.7-flash"}
	subj, body := templates.RenderSubscriptionWelcome("Alice Smith", "Acme Corp", "TEAM", 10000000, models, "https://app.scandrix.dev")

	if !strings.Contains(subj, "Pro Team Plan") {
		t.Errorf("expected subject to contain Pro Team Plan, got %s", subj)
	}
	if !strings.Contains(body, "10,000,000") {
		t.Errorf("expected body to contain formatted 10,000,000 tokens")
	}
	if !strings.Contains(body, "claude-sonnet-5") {
		t.Errorf("expected body to list claude-sonnet-5")
	}
	if !strings.Contains(body, "Acme Corp") {
		t.Errorf("expected body to mention Acme Corp")
	}
}

func TestRenderInvoiceReceipt(t *testing.T) {
	inv := templates.InvoiceDetails{
		InvoiceNumber:    "INV-2026-998877",
		RecipientName:    "Bob Jones",
		RecipientEmail:   "bob@acme.com",
		OrganizationName: "Acme Labs",
		PlanTier:         "Pro Team",
		AmountFormatted:  "₹2,499.00 INR",
		OrderID:          "order_12345",
		PaymentID:        "pay_67890",
		PaymentProvider:  "Razorpay",
		BillingDate:      time.Now(),
		NextBillingDate:  time.Now().AddDate(0, 1, 0),
		BillingPortalURL: "https://app.scandrix.dev/billing",
	}

	subj, body := templates.RenderInvoiceReceipt(inv)

	if !strings.Contains(subj, "INV-2026-998877") {
		t.Errorf("expected subject to contain invoice number, got %s", subj)
	}
	if !strings.Contains(body, "₹2,499.00 INR") {
		t.Errorf("expected body to contain formatted amount")
	}
	if !strings.Contains(body, "pay_67890") {
		t.Errorf("expected body to contain payment ID")
	}
	if !strings.Contains(body, "PAID") {
		t.Errorf("expected body to have PAID status")
	}
}

func TestRenderPaymentFailed(t *testing.T) {
	subj, body := templates.RenderPaymentFailed("Bob", "Acme", "Pro Team", "order_fail_1", "Card limit exceeded", "https://app.scandrix.dev/billing")
	if !strings.Contains(subj, "Payment failed") {
		t.Errorf("expected subject to contain Payment failed, got %s", subj)
	}
	if !strings.Contains(body, "Card limit exceeded") {
		t.Errorf("expected body to contain failure reason")
	}
}

func TestRenderSpendLimitAlert(t *testing.T) {
	subj, body := templates.RenderSpendLimitAlert("Bob", "Acme", 80, 8000000, 10000000, "https://app.scandrix.dev/billing")
	if !strings.Contains(subj, "80%") {
		t.Errorf("expected subject to contain 80%%, got %s", subj)
	}
	if !strings.Contains(body, "8,000,000") {
		t.Errorf("expected body to contain used tokens")
	}
}

func TestRenderTeamInviteAndPasswordReset(t *testing.T) {
	subj1, body1 := templates.RenderTeamInvite("Alice", "charlie@acme.com", "Acme", "Security Lead", "https://app.scandrix.dev/invite")
	if !strings.Contains(subj1, "Alice invited you") {
		t.Errorf("expected invite subject")
	}
	if !strings.Contains(body1, "Security Lead") {
		t.Errorf("expected role in body")
	}

	subj2, body2 := templates.RenderPasswordReset("Charlie", "https://app.scandrix.dev/reset")
	if !strings.Contains(subj2, "Reset your ScanDrix Password") {
		t.Errorf("expected reset subject")
	}
	if !strings.Contains(body2, "Reset Password") {
		t.Errorf("expected reset button")
	}
}
