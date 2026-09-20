package cron_test

import (
	"context"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/billing/razorpay"
	"github.com/scandrix/backend/internal/cron"
)

type mockPaymentReconciler struct {
	orderResp   *razorpay.OrderResponse
	paymentResp *razorpay.PaymentResponse
	err         error
}

func (m *mockPaymentReconciler) FetchOrder(ctx context.Context, orderID string) (*razorpay.OrderResponse, error) {
	return m.orderResp, m.err
}

func (m *mockPaymentReconciler) FetchPayment(ctx context.Context, paymentID string) (*razorpay.PaymentResponse, error) {
	return m.paymentResp, m.err
}

func TestPaymentReconciliationCronJob(t *testing.T) {
	mockClient := &mockPaymentReconciler{
		orderResp: &razorpay.OrderResponse{
			ID:     "order_123",
			Status: "paid",
		},
	}

	job := cron.NewPaymentReconciliationCron(nil, mockClient, 30*time.Minute)
	if job.Name() != "PaymentReconciliationCron" {
		t.Fatalf("unexpected name: %s", job.Name())
	}
	if job.Interval() != 30*time.Minute {
		t.Fatalf("unexpected interval: %v", job.Interval())
	}

	ctx := context.Background()
	// Should execute gracefully with nil repo
	if err := job.Run(ctx); err != nil {
		t.Fatalf("expected nil error on nil repo, got: %v", err)
	}
}
