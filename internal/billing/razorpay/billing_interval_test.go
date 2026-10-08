package razorpay_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/scandrix/backend/internal/auth/mailer"
	"github.com/scandrix/backend/internal/billing/razorpay"
	"github.com/scandrix/backend/internal/enterprise/license"
)

// newOrderService builds a service pointed at a mock Razorpay that always accepts
// order creation, so the pricing decision can be observed on the returned amount.
func newOrderService(t *testing.T) *razorpay.BillingService {
	t.Helper()
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"order_annual_test","entity":"order","amount":1,"currency":"INR"}`))
	}))
	t.Cleanup(mock.Close)

	svc := razorpay.NewBillingService(
		nil, nil, mailer.NewNoopSender(), "https://app.scandrix.dev",
		"rzp_test_key", "test_secret_key_123456", "wh_sec_123",
	)
	client := razorpay.NewRazorpayClient("rzp_test_key", "test_secret_key_123456")
	client.SetBaseURL(mock.URL)
	svc.SetClient(client)
	return svc
}

/**
 * The billing term was dropped on the way to Razorpay: CreateSubscriptionOrder
 * took no interval, so an annual selection was charged the monthly amount and
 * nothing reported the mismatch. The dashboard's order schema also validates
 * billing_interval, so the mismatch was a hard failure as well as a pricing bug.
 *
 * These tests run without a database, so they exercise the documented fallback
 * pricing path. The stored-annual-price path is covered by the repository test.
 */
func TestCreateSubscriptionOrderHonoursTheBillingInterval(t *testing.T) {
	svc := newOrderService(t)
	ctx := context.Background()
	wsID := uuid.New()

	monthly, err := svc.CreateSubscriptionOrder(ctx, wsID, license.TierTeam, "INR", "monthly")
	if err != nil {
		t.Fatalf("monthly order failed: %v", err)
	}
	annual, err := svc.CreateSubscriptionOrder(ctx, wsID, license.TierTeam, "INR", "annual")
	if err != nil {
		t.Fatalf("annual order failed: %v", err)
	}

	if monthly.BillingInterval != "monthly" {
		t.Errorf("expected the monthly term to be reported, got %q", monthly.BillingInterval)
	}
	if annual.BillingInterval != "annual" {
		t.Errorf("expected the annual term to be reported, got %q", annual.BillingInterval)
	}

	// The headline regression: annual must not cost the same as monthly.
	if annual.Amount <= monthly.Amount {
		t.Fatalf("annual order was not priced above monthly: monthly=%d annual=%d",
			monthly.Amount, annual.Amount)
	}
	if want := monthly.Amount * 10; annual.Amount != want {
		t.Errorf("expected the documented 10x annual multiplier: got %d want %d",
			annual.Amount, want)
	}
}

func TestCreateSubscriptionOrderDefaultsToMonthlyAndRejectsUnknownTerms(t *testing.T) {
	svc := newOrderService(t)
	ctx := context.Background()
	wsID := uuid.New()

	t.Run("empty interval defaults to monthly", func(t *testing.T) {
		order, err := svc.CreateSubscriptionOrder(ctx, wsID, license.TierTeam, "INR", "")
		if err != nil {
			t.Fatalf("expected an empty interval to default to monthly, got %v", err)
		}
		if order.BillingInterval != "monthly" {
			t.Errorf("expected monthly, got %q", order.BillingInterval)
		}
	})

	t.Run("unsupported interval is rejected", func(t *testing.T) {
		if _, err := svc.CreateSubscriptionOrder(ctx, wsID, license.TierTeam, "INR", "weekly"); err == nil {
			t.Fatal("expected an unsupported billing interval to be rejected")
		}
	})
}