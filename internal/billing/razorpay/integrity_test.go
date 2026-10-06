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
	// A rejected proof must never capture the order or grant a license. Several
	// rejections deliberately leave an audit marker on the transaction
	// (see UpdateBillingTransactionStatus calls in VerifyAndUpgrade), so the
	// status is asserted per scenario rather than assumed to stay "created".
	scenarios := []struct {
		name           string
		expectedStatus string
	}{
		{"signature", "signature_verification_failed"},
		{"authorized", "created"},
		{"failed", "created"},
		{"order identity", "created"},
		{"payment identity", "created"},
		{"amount", "amount_mismatch_fraud"},
		{"currency", "currency_mismatch_fraud"},
		{"provider failure", "created"},
		{"foreign workspace", "created"},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			repo, workspace := billingTestRepository(t)
			ctx := context.Background()
			transaction := &models.BillingTransaction{ID: uuid.New(), WorkspaceID: workspace, Provider: "razorpay", OrderID: "order_fixture", Amount: 79900, Currency: "INR", PlanTier: "DEVELOPER", BillingInterval: "monthly", Status: "created"}
			require.NoError(t, repo.RecordBillingTransaction(ctx, transaction))
			payment := razorpay.PaymentResponse{ID: "pay_fixture", OrderID: transaction.OrderID, Amount: transaction.Amount, Currency: transaction.Currency, Status: "captured"}
			request := razorpay.VerifyPaymentRequest{OrderID: transaction.OrderID, PaymentID: payment.ID, Signature: paymentProof(transaction.OrderID, payment.ID)}
			switch scenario.name {
			case "signature":
				request.Signature = "invalid"
			case "authorized", "failed":
				payment.Status = scenario.name
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
				if scenario.name == "provider failure" {
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
			if scenario.name == "foreign workspace" {
				callerWorkspace = uuid.New()
			}
			_, err := service.VerifyAndUpgrade(ctx, callerWorkspace, request)
			require.Error(t, err)
			stored, err := repo.GetBillingTransaction(ctx, workspace, transaction.OrderID)
			require.NoError(t, err)
			require.Equal(t, scenario.expectedStatus, stored.Status)
			require.NotEqual(t, "captured", stored.Status, "a rejected proof must never capture the order")
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

	// The seat row is the source LiveQuotaStatus reads, and usage_controller
	// derives isExhausted from it as !BYOKEnabled && used >= limit. A literal
	// true here made isExhausted permanently false, so an over-quota workspace
	// was never reported as exhausted.
	require.NoError(t, repo.Client().ExecWithTenant(ctx, workspace, func(tx pgx.Tx) error {
		var byok bool
		if err := tx.QueryRow(ctx, `SELECT byok_enabled FROM organization_billing_seats WHERE workspace_id=$1`, workspace).Scan(&byok); err != nil {
			return err
		}
		require.False(t, byok, "seat row must carry the plan configuration's BYOK entitlement")
		return nil
	}))
	quota, err := repo.GetLiveTokenQuota(ctx, workspace)
	require.NoError(t, err)
	require.False(t, quota.BYOKEnabled, "quota status must not grant BYOK the plan configuration withholds")
	// Mirrors usage_controller: isExhausted := !BYOKEnabled && limit > 0 && used >= limit.
	// With BYOKEnabled hardcoded true this was permanently false, so an
	// over-quota workspace was never reported as exhausted.
	isExhausted := !quota.BYOKEnabled && quota.MonthlyTokenLimit > 0 && quota.TokensUsedThisMonth >= quota.MonthlyTokenLimit
	require.False(t, isExhausted, "a workspace under its ceiling must not be reported exhausted")
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

// TestBillingWebhookIngressIsIdempotentUnderConcurrency fires identical signed
// webhooks concurrently at the HTTP ingress, which is where Razorpay retries
// actually land. Every delivery must be acknowledged 200, the deterministic
// outbox key must collapse them into one durable record, and the upgrade must
// be applied exactly once.
func TestBillingWebhookIngressIsIdempotentUnderConcurrency(t *testing.T) {
	repo, workspace := billingTestRepository(t)
	ctx := context.Background()
	transaction := &models.BillingTransaction{ID: uuid.New(), WorkspaceID: workspace, Provider: "razorpay", OrderID: "order_concurrent", Amount: 149900, Currency: "INR", PlanTier: "TEAM", BillingInterval: "monthly", Status: "created"}
	require.NoError(t, repo.RecordBillingTransaction(ctx, transaction))
	payment := razorpay.PaymentResponse{ID: "pay_concurrent", OrderID: transaction.OrderID, Amount: transaction.Amount, Currency: transaction.Currency, Status: "captured"}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode(payment) }))
	defer provider.Close()
	service := razorpay.NewBillingService(repo, llm.NewTokenBudgetLimiter(), nil, "", "rzp_test_fixture", fixturePaymentSecret, fixturePaymentSecret)
	client := razorpay.NewRazorpayClient("rzp_test_fixture", fixturePaymentSecret)
	client.SetBaseURL(provider.URL)
	service.SetClient(client)
	controller := controllers.NewBillingController(service, repo, nil)
	routes := controller.WebhookRoutes()

	payload := []byte(`{"event":"payment.captured","payload":{"payment":{"entity":{"id":"pay_concurrent","order_id":"order_concurrent"}}}}`)
	mac := hmac.New(sha256.New, []byte(fixturePaymentSecret))
	mac.Write(payload)
	signature := hex.EncodeToString(mac.Sum(nil))

	const concurrency = 10
	codes := make([]int, concurrency)
	var wait sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wait.Add(1)
		go func(i int) {
			defer wait.Done()
			request := httptest.NewRequest("POST", "/razorpay", bytes.NewReader(payload))
			request.Header.Set("X-Razorpay-Signature", signature)
			response := httptest.NewRecorder()
			routes.ServeHTTP(response, request)
			codes[i] = response.Code
		}(i)
	}
	wait.Wait()

	for i, code := range codes {
		require.Equal(t, http.StatusOK, code, "delivery %d must be acknowledged so Razorpay stops retrying", i)
	}

	// The outbox key is uuid.NewSHA1 over the sanitized body, so identical
	// deliveries collapse to one durable row instead of N replayed upgrades.
	require.NoError(t, repo.Client().ExecWithTenant(ctx, workspace, func(tx pgx.Tx) error {
		var outbox int
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_events WHERE workspace_id=$1 AND event_type='billing.razorpay.webhook'`, workspace).Scan(&outbox); err != nil {
			return err
		}
		require.Equal(t, 1, outbox)
		return nil
	}))

	// handleWebhook acknowledges first and upgrades on a detached goroutine, so
	// the HTTP layer cannot be held responsible for the upgrade finishing. Hold
	// the captured state stable before returning, otherwise the remaining
	// deliveries race fixture teardown and log against a closed pool. The
	// upgrade side effects themselves are asserted synchronously in
	// TestBillingWebhookUpgradeIsIdempotentUnderConcurrency.
	require.Eventually(t, func() bool {
		stored, err := repo.GetBillingTransaction(ctx, workspace, transaction.OrderID)
		return err == nil && stored.Status == "captured"
	}, 10*time.Second, 20*time.Millisecond, "concurrent deliveries must still capture the order")

	settled := time.Now()
	for time.Since(settled) < time.Second {
		time.Sleep(100 * time.Millisecond)
		stored, err := repo.GetBillingTransaction(ctx, workspace, transaction.OrderID)
		require.NoError(t, err)
		require.Equal(t, "captured", stored.Status, "captured state must stay stable while detached deliveries drain")
	}
}

// TestBillingWebhookUpgradeIsIdempotentUnderConcurrency drives the verified
// webhook path directly so completion is observable. handleWebhook upgrades on
// a detached goroutine, which makes the upgrade outcome unassertable from an
// HTTP-level test; here Wait() returns only once every delivery has finished, so
// the paid side effects can be counted exactly.
func TestBillingWebhookUpgradeIsIdempotentUnderConcurrency(t *testing.T) {
	repo, workspace := billingTestRepository(t)
	ctx := context.Background()
	transaction := &models.BillingTransaction{ID: uuid.New(), WorkspaceID: workspace, Provider: "razorpay", OrderID: "order_upgrade_race", Amount: 149900, Currency: "INR", PlanTier: "TEAM", BillingInterval: "monthly", Status: "created"}
	require.NoError(t, repo.RecordBillingTransaction(ctx, transaction))
	payment := razorpay.PaymentResponse{ID: "pay_upgrade_race", OrderID: transaction.OrderID, Amount: transaction.Amount, Currency: transaction.Currency, Status: "captured"}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode(payment) }))
	defer provider.Close()
	service := razorpay.NewBillingService(repo, llm.NewTokenBudgetLimiter(), nil, "", "rzp_test_fixture", fixturePaymentSecret, fixturePaymentSecret)
	client := razorpay.NewRazorpayClient("rzp_test_fixture", fixturePaymentSecret)
	client.SetBaseURL(provider.URL)
	service.SetClient(client)

	payload := []byte(`{"event":"payment.captured","payload":{"payment":{"entity":{"id":"pay_upgrade_race","order_id":"order_upgrade_race"}}}}`)
	mac := hmac.New(sha256.New, []byte(fixturePaymentSecret))
	mac.Write(payload)
	signature := hex.EncodeToString(mac.Sum(nil))

	const concurrency = 10
	failures := make([]error, concurrency)
	var wait sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wait.Add(1)
		go func(i int) {
			defer wait.Done()
			failures[i] = service.ProcessWebhook(ctx, payload, signature)
		}(i)
	}
	wait.Wait()

	for i, err := range failures {
		require.NoError(t, err, "concurrent delivery %d must be processed idempotently", i)
	}

	stored, err := repo.GetBillingTransaction(ctx, workspace, transaction.OrderID)
	require.NoError(t, err)
	require.Equal(t, "captured", stored.Status)

	// A retried webhook must not multiply the paid side effects.
	require.NoError(t, repo.Client().ExecWithTenant(ctx, workspace, func(tx pgx.Tx) error {
		var licenses, seats int
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM organization_licenses WHERE workspace_id=$1`, workspace).Scan(&licenses); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM organization_billing_seats WHERE workspace_id=$1`, workspace).Scan(&seats); err != nil {
			return err
		}
		require.Equal(t, 1, licenses, "retries must not grant multiple licenses")
		require.Equal(t, 1, seats, "retries must not multiply seat allocations")
		return nil
	}))
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
