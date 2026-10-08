package razorpay_test

import (
	"context"
	"net"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/billing/razorpay"
	"github.com/scandrix/backend/pkg/models"
	"github.com/stretchr/testify/require"
)

// This opt-in test uses real Razorpay sandbox credentials and disposable
// PostgreSQL. It never accepts a live key or replaces the payment transport.
func TestBillingSandbox(t *testing.T) {
	if os.Getenv("SCANDRIX_RAZORPAY_SANDBOX_CHECK") != "1" {
		t.Skip("SCANDRIX_RAZORPAY_SANDBOX_CHECK is not enabled")
	}
	if err := godotenv.Load("../../../.env"); err != nil {
		t.Fatal("sandbox environment could not be loaded")
	}
	keyID, keySecret := os.Getenv("RAZORPAY_KEY_ID"), os.Getenv("RAZORPAY_KEY_SECRET")
	if !strings.HasPrefix(keyID, "rzp_test_") || keySecret == "" {
		t.Fatal("RAZORPAY_KEY_ID and RAZORPAY_KEY_SECRET must be configured for sandbox")
	}
	repository, workspace := billingTestRepository(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	service := razorpay.NewBillingService(repository, nil, nil, "", keyID, keySecret, "")
	if script := os.Getenv("SCANDRIX_BILLING_BROWSER_CHECK"); script != "" {
		secret := os.Getenv("JWT_SECRET")
		require.NotEmpty(t, secret)
		authenticator := auth.NewAuthenticator(secret)
		ownerToken, err := authenticator.GenerateTokenWithEmail(uuid.New(), workspace, models.RoleOwner, "owner@example.test")
		require.NoError(t, err)
		viewerToken, err := authenticator.GenerateTokenWithEmail(uuid.New(), workspace, models.RoleViewer, "viewer@example.test")
		require.NoError(t, err)
		adminToken, err := authenticator.GenerateTokenWithEmail(uuid.New(), workspace, models.RoleAdmin, "admin@example.test")
		require.NoError(t, err)
		controller := controllers.NewBillingController(service, repository, nil)
		router := chi.NewRouter()
		router.Use(authenticator.Middleware)
		router.Mount("/api/v1/billing", controller.ProtectedRoutes())
		server := httptest.NewUnstartedServer(router)
		server.Listener.Close()
		server.Listener, err = net.Listen("tcp", os.Getenv("SCANDRIX_TEST_API_ADDR"))
		require.NoError(t, err)
		server.Start()
		defer server.Close()
		command := exec.CommandContext(ctx, "node", script)
		command.Env = append(os.Environ(), "SCANDRIX_TEST_OWNER_TOKEN="+ownerToken, "SCANDRIX_TEST_VIEWER_TOKEN="+viewerToken, "SCANDRIX_TEST_ADMIN_TOKEN="+adminToken)
		command.Stdout, command.Stderr = os.Stdout, os.Stderr
		require.NoError(t, command.Run())
		plan, err := repository.GetWorkspacePlanDetails(ctx, workspace)
		require.NoError(t, err)
		require.Equal(t, "DEVELOPER", plan.PlanTier)
		require.NotNil(t, plan.ExpiresAt)
		return
	}
	order, err := service.CreateSubscriptionOrder(ctx, workspace, "DEVELOPER", "INR", "monthly")
	require.NoError(t, err)
	stored, err := repository.GetBillingTransaction(ctx, workspace, order.OrderID)
	require.NoError(t, err)
	require.Equal(t, order.Amount, stored.Amount)
	require.Equal(t, "created", stored.Status)
	plan, err := repository.GetWorkspacePlanDetails(ctx, workspace)
	require.NoError(t, err)
	require.Equal(t, "COMMUNITY", plan.PlanTier)
	require.Nil(t, plan.ExpiresAt, "order creation must not activate paid access")
	t.Log("Real Razorpay sandbox order created and read back from PostgreSQL; paid access remains inactive")
}
