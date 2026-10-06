package razorpay_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/billing/razorpay"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/llm"
	"github.com/scandrix/backend/pkg/models"
	"github.com/stretchr/testify/require"
)

const fixturePaymentSecret = "billing-integrity-test-only"

func paymentProof(orderID, paymentID string) string {
	mac := hmac.New(sha256.New, []byte(fixturePaymentSecret))
	mac.Write([]byte(orderID + "|" + paymentID))
	return hex.EncodeToString(mac.Sum(nil))
}

func TestBillingRejectsUnverifiedPayments(t *testing.T) {
	for _, scenario := range []string{"signature", "authorized", "failed", "order identity", "payment identity", "amount", "currency", "provider failure", "foreign workspace"} {
		t.Run(scenario, func(t *testing.T) {
			repo, workspace := billingTestRepository(t)
			ctx := context.Background()
			transaction := &models.BillingTransaction{ID: uuid.New(), WorkspaceID: workspace, Provider: "razorpay", OrderID: "order_fixture", Amount: 79900, Currency: "INR", PlanTier: "DEVELOPER", BillingInterval: "monthly", Status: "created"}
			require.NoError(t, repo.RecordBillingTransaction(ctx, transaction))
			payment := razorpay.PaymentResponse{ID: "pay_fixture", OrderID: transaction.OrderID, Amount: transaction.Amount, Currency: transaction.Currency, Status: "captured"}
			request := razorpay.VerifyPaymentRequest{OrderID: transaction.OrderID, PaymentID: payment.ID, Signature: paymentProof(transaction.OrderID, payment.ID)}
			switch scenario {
			case "signature":
				request.Signature = "invalid"
			case "authorized", "failed":
				payment.Status = scenario
			case "order identity":
				payment.OrderID = "order_other"
			case "payment identity":
				payment.ID = "pay_other"
			case "amount":
				payment.Amount--
			case "currency":
				payment.Currency = "USD"
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if scenario == "provider failure" {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				_ = json.NewEncoder(w).Encode(payment)
			}))
			defer server.Close()
			client := razorpay.NewRazorpayClient("rzp_test_fixture", fixturePaymentSecret)
			client.SetBaseURL(server.URL)
			service := razorpay.NewBillingService(repo, nil, nil, "", "rzp_test_fixture", fixturePaymentSecret, "")
			service.SetClient(client)
			callerWorkspace := workspace
			if scenario == "foreign workspace" {
				callerWorkspace = uuid.New()
			}
			_, err := service.VerifyAndUpgrade(ctx, callerWorkspace, request)
			require.Error(t, err)
			stored, err := repo.GetBillingTransaction(ctx, workspace, transaction.OrderID)
			require.NoError(t, err)
			require.Equal(t, "created", stored.Status, "rejected proofs must not taint or capture the order")
			license, err := repo.GetActiveLicense(ctx, workspace)
			require.NoError(t, err)
			require.Nil(t, license)
		})
	}
}

func TestBillingUsesStoredQuotasAndIsIdempotentAcrossServices(t *testing.T) {
	repo, workspace := billingTestRepository(t)
	ctx := context.Background()
	require.NoError(t, repo.Client().ExecWithTenant(ctx, workspace, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE plan_configurations SET max_seats=7,monthly_tokens=12345,burst_limit_per_min=678,byok_allowed=false,allocated_models='["configured-model"]',features_enabled='["automated_reviews"]' WHERE tier='DEVELOPER'`)
		return err
	}))
	transaction := &models.BillingTransaction{ID: uuid.New(), WorkspaceID: workspace, Provider: "razorpay", OrderID: "order_fixture", Amount: 79900, Currency: "INR", PlanTier: "DEVELOPER", BillingInterval: "monthly", Status: "created"}
	require.NoError(t, repo.RecordBillingTransaction(ctx, transaction))
	payment := razorpay.PaymentResponse{ID: "pay_fixture", OrderID: transaction.OrderID, Amount: transaction.Amount, Currency: transaction.Currency, Status: "captured"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode(payment) }))
	defer server.Close()
	limiter := llm.NewTokenBudgetLimiter()
	services := make([]*razorpay.BillingService, 2)
	for i := range services {
		services[i] = razorpay.NewBillingService(repo, limiter, nil, "", "rzp_test_fixture", fixturePaymentSecret, "")
		client := razorpay.NewRazorpayClient("rzp_test_fixture", fixturePaymentSecret)
		client.SetBaseURL(server.URL)
		services[i].SetClient(client)
	}
	request := razorpay.VerifyPaymentRequest{OrderID: transaction.OrderID, PaymentID: payment.ID, Signature: paymentProof(transaction.OrderID, payment.ID), PlanTier: "ENTERPRISE", BillingInterval: "annual"}
	var wait sync.WaitGroup
	errors := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wait.Add(1)
		go func(i int) {
			defer wait.Done()
			_, err := services[i%2].VerifyAndUpgrade(ctx, workspace, request)
			errors <- err
		}(i)
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	license, err := repo.GetActiveLicense(ctx, workspace)
	require.NoError(t, err)
	require.Equal(t, "DEVELOPER", license.PlanTier)
	require.Equal(t, 7, license.TotalSeats)
	require.Contains(t, license.LicenseKey, workspace.String())
	require.Less(t, time.Until(license.ExpiresAt), 32*24*time.Hour, "client annual claims cannot alter the stored monthly term")
	details, err := repo.GetWorkspacePlanDetails(ctx, workspace)
	require.NoError(t, err)
	require.EqualValues(t, 12345, details.MonthlyTokenLimit)
	require.False(t, details.BYOKAllowed)
	require.Equal(t, []string{"configured-model"}, details.AllocatedModels)
	_, monthly, _, burst, exists := limiter.GetUsage(workspace)
	require.True(t, exists)
	require.EqualValues(t, 12345, monthly)
	require.EqualValues(t, 678, burst)
	require.NoError(t, repo.Client().ExecWithTenant(ctx, workspace, func(tx pgx.Tx) error {
		var licenses, audits, seats int
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM organization_licenses WHERE workspace_id=$1`, workspace).Scan(&licenses); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM audit_logs WHERE workspace_id=$1 AND action='workspace.plan_upgrade'`, workspace).Scan(&audits); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM organization_billing_seats WHERE workspace_id=$1`, workspace).Scan(&seats); err != nil {
			return err
		}
		require.Equal(t, 1, licenses)
		require.Equal(t, 1, audits)
		require.Equal(t, 1, seats)
		return nil
	}))
}

func TestBillingWebhookUsesStoredTenantAndProviderStatus(t *testing.T) {
	repo, workspace := billingTestRepository(t)
	ctx := context.Background()
	transaction := &models.BillingTransaction{ID: uuid.New(), WorkspaceID: workspace, Provider: "razorpay", OrderID: "order_fixture", Amount: 79900, Currency: "INR", PlanTier: "DEVELOPER", BillingInterval: "monthly", Status: "created"}
	require.NoError(t, repo.RecordBillingTransaction(ctx, transaction))
	status := "authorized"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(razorpay.PaymentResponse{ID: "pay_fixture", OrderID: transaction.OrderID, Amount: transaction.Amount, Currency: "INR", Status: status})
	}))
	defer server.Close()
	client := razorpay.NewRazorpayClient("rzp_test_fixture", fixturePaymentSecret)
	client.SetBaseURL(server.URL)
	service := razorpay.NewBillingService(repo, nil, nil, "", "rzp_test_fixture", fixturePaymentSecret, "")
	service.SetClient(client)
	payload := []byte(`{"event":"payment.captured","payload":{"payment":{"entity":{"id":"pay_fixture","order_id":"order_fixture","email":"discard@example.test","notes":{"workspace_id":"` + uuid.NewString() + `","plan_tier":"ENTERPRISE"}}}}}`)
	resolved, filtered, err := service.PrepareVerifiedWebhook(ctx, payload)
	require.NoError(t, err)
	require.Equal(t, workspace, resolved)
	require.NotContains(t, string(filtered), "discard@example.test")
	require.NotContains(t, string(filtered), "ENTERPRISE")
	require.Error(t, service.ProcessVerifiedWebhook(ctx, payload), "signed event claims cannot replace provider capture")
	status = "captured"
	require.NoError(t, service.ProcessVerifiedWebhook(ctx, payload))
	status = "failed"
	failure := []byte(`{"event":"payment.failed","payload":{"payment":{"entity":{"id":"pay_fixture","order_id":"order_fixture"}}}}`)
	require.NoError(t, service.ProcessVerifiedWebhook(ctx, failure))
	stored, err := repo.GetBillingTransaction(ctx, workspace, transaction.OrderID)
	require.NoError(t, err)
	require.Equal(t, "captured", stored.Status, "out-of-order failure cannot reverse capture")
	license, err := repo.GetActiveLicense(ctx, workspace)
	require.NoError(t, err)
	require.Equal(t, "DEVELOPER", license.PlanTier)
}

func TestBillingAnnualPriceRange(t *testing.T) {
	amount, err := razorpay.SubscriptionAmount(79900, "annual")
	require.NoError(t, err)
	require.EqualValues(t, 799000, amount)
	_, err = razorpay.SubscriptionAmount(math.MaxInt64/10+1, "annual")
	require.Error(t, err)
	_, err = razorpay.SubscriptionAmount(-1, "monthly")
	require.Error(t, err)
}

type unavailableBillingOutbox struct{ *database.Repository }

func (unavailableBillingOutbox) InsertOutboxEvent(context.Context, *models.OutboxRecord) error {
	return errors.New("test event-store outage")
}

func TestBillingWebhookRequiresDurableStorageAndDeduplicates(t *testing.T) {
	repo, workspace := billingTestRepository(t)
	ctx := context.Background()
	transaction := &models.BillingTransaction{ID: uuid.New(), WorkspaceID: workspace, Provider: "razorpay", OrderID: "order_fixture", Amount: 79900, Currency: "INR", PlanTier: "DEVELOPER", BillingInterval: "monthly", Status: "created"}
	require.NoError(t, repo.RecordBillingTransaction(ctx, transaction))
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(razorpay.PaymentResponse{ID: "pay_fixture", OrderID: transaction.OrderID, Amount: transaction.Amount, Currency: "INR", Status: "captured"})
	}))
	defer provider.Close()
	client := razorpay.NewRazorpayClient("rzp_test_fixture", fixturePaymentSecret)
	client.SetBaseURL(provider.URL)
	service := razorpay.NewBillingService(repo, nil, nil, "", "rzp_test_fixture", fixturePaymentSecret, fixturePaymentSecret)
	service.SetClient(client)
	payload := []byte(`{"event":"payment.captured","payload":{"payment":{"entity":{"id":"pay_fixture","order_id":"order_fixture","email":"discard@example.test"}}}}`)
	mac := hmac.New(sha256.New, []byte(fixturePaymentSecret))
	mac.Write(payload)
	signature := hex.EncodeToString(mac.Sum(nil))
	send := func(controller *controllers.BillingController) *httptest.ResponseRecorder {
		request := httptest.NewRequest("POST", "/razorpay", bytes.NewReader(payload))
		request.Header.Set("X-Razorpay-Signature", signature)
		response := httptest.NewRecorder()
		controller.WebhookRoutes().ServeHTTP(response, request)
		return response
	}
	require.Equal(t, http.StatusServiceUnavailable, send(controllers.NewBillingController(service, unavailableBillingOutbox{repo}, nil)).Code)
	stored, err := repo.GetBillingTransaction(ctx, workspace, transaction.OrderID)
	require.NoError(t, err)
	require.Equal(t, "created", stored.Status)
	controller := controllers.NewBillingController(service, repo, nil)
	require.Equal(t, http.StatusOK, send(controller).Code)
	require.Equal(t, http.StatusOK, send(controller).Code)
	require.Eventually(t, func() bool {
		stored, err := repo.GetBillingTransaction(ctx, workspace, transaction.OrderID)
		return err == nil && stored.Status == "captured"
	}, 3*time.Second, 10*time.Millisecond)
	require.NoError(t, repo.Client().ExecWithTenant(ctx, workspace, func(tx pgx.Tx) error {
		var count int
		var sensitive bool
		if err := tx.QueryRow(ctx, `SELECT COUNT(*),COALESCE(BOOL_OR(payload::text LIKE '%discard@example.test%'),false) FROM outbox_events WHERE workspace_id=$1 AND event_type='billing.razorpay.webhook'`, workspace).Scan(&count, &sensitive); err != nil {
			return err
		}
		require.Equal(t, 1, count)
		require.False(t, sensitive)
		return nil
	}))
}
