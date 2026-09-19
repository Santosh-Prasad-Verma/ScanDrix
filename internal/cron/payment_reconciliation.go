package cron

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/scandrix/backend/internal/billing/razorpay"
	"github.com/scandrix/backend/internal/database"
)

// PaymentReconciler fetches order and payment statuses from Razorpay API.
type PaymentReconciler interface {
	FetchOrder(ctx context.Context, orderID string) (*razorpay.OrderResponse, error)
	FetchPayment(ctx context.Context, paymentID string) (*razorpay.PaymentResponse, error)
}

// PaymentReconciliationCron periodically reconciles pending checkout orders and transactions against the Razorpay ledger.
type PaymentReconciliationCron struct {
	repo       *database.Repository
	client     PaymentReconciler
	interval   time.Duration
	staleAge   time.Duration
	batchLimit int
}

// NewPaymentReconciliationCron constructs an automated payment ledger reconciliation cron job.
func NewPaymentReconciliationCron(repo *database.Repository, client PaymentReconciler, interval time.Duration) *PaymentReconciliationCron {
	if interval <= 0 {
		interval = 30 * time.Minute
	}
	return &PaymentReconciliationCron{
		repo:       repo,
		client:     client,
		interval:   interval,
		staleAge:   1 * time.Hour,
		batchLimit: 50,
	}
}

func (c *PaymentReconciliationCron) Name() string {
	return "PaymentReconciliationCron"
}

func (c *PaymentReconciliationCron) Interval() time.Duration {
	return c.interval
}

func (c *PaymentReconciliationCron) Run(ctx context.Context) error {
	if c.repo == nil {
		return nil
	}

	txs, err := c.repo.ListPendingReconciliationTransactions(ctx, c.staleAge, c.batchLimit)
	if err != nil {
		return fmt.Errorf("failed fetching pending reconciliation transactions: %w", err)
	}
	if len(txs) == 0 {
		return nil
	}

	slog.Info("Starting automated payment ledger reconciliation", "pending_count", len(txs))

	for _, tx := range txs {
		if c.client != nil {
			order, err := c.client.FetchOrder(ctx, tx.OrderID)
			if err == nil && order != nil {
				if order.Status == "paid" {
					_ = c.repo.ReconcileTransactionLedger(ctx, tx.WorkspaceID, tx.OrderID, tx.PaymentID, "captured", "Reconciled via Razorpay API order verification")
					slog.Info("Transaction ledger reconciled to captured", "order_id", tx.OrderID, "workspace_id", tx.WorkspaceID)
					continue
				} else if order.Status == "attempted" && time.Since(tx.CreatedAt) > 24*time.Hour {
					_ = c.repo.ReconcileTransactionLedger(ctx, tx.WorkspaceID, tx.OrderID, tx.PaymentID, "failed", "Reconciled via Razorpay API: order expired after 24h")
					slog.Info("Transaction ledger reconciled to failed", "order_id", tx.OrderID, "workspace_id", tx.WorkspaceID)
					continue
				}
			}
		}

		// Fallback timeout: if order was created > 48h ago and never completed, mark expired
		if time.Since(tx.CreatedAt) > 48*time.Hour {
			_ = c.repo.ReconcileTransactionLedger(ctx, tx.WorkspaceID, tx.OrderID, tx.PaymentID, "expired", "Auto-expired: order pending > 48 hours without webhook confirmation")
			slog.Info("Transaction ledger auto-expired stale order", "order_id", tx.OrderID, "workspace_id", tx.WorkspaceID)
		}
	}

	return nil
}
