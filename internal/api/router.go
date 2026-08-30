package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/scandrix/backend/internal/api/controllers"
	scandrixMiddleware "github.com/scandrix/backend/internal/api/middleware"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/auth/cliauth"
	"github.com/scandrix/backend/internal/auth/mailer"
	"github.com/scandrix/backend/internal/auth/oauth"
	"github.com/scandrix/backend/internal/billing/razorpay"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/enterprise/rbac"
	"github.com/scandrix/backend/internal/enterprise/scim"
	"github.com/scandrix/backend/internal/llm"
	"github.com/scandrix/backend/internal/review"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/internal/telemetry"
)

// RouterConfig holds dependency references required to wire up the API server.
type RouterConfig struct {
	Repo           *database.Repository
	AuthService    *auth.Authenticator
	Orchestrator   *review.Orchestrator
	StreamHub      *review.StreamHub
	Evaluator      *rules.Evaluator
	SCIMService    *scim.SCIMService
	DeviceFlow     *cliauth.DeviceFlowManager
	OAuthService   *oauth.OAuthService
	Mailer         mailer.EmailSender
	BillingService *razorpay.BillingService
	BudgetLimiter  *llm.TokenBudgetLimiter
	AppBaseURL     string
	JWTSecret      string
}

// BuildRouter constructs the master Chi HTTP router mounting all domain controllers.
func BuildRouter(cfg RouterConfig) chi.Router {
	r := chi.NewRouter()

	// Base Middlewares
	r.Use(middleware.RequestID)
	r.Use(scandrixMiddleware.SecurityHeaders)
	r.Use(scandrixMiddleware.CORS(scandrixMiddleware.DefaultCORSConfig()))
	r.Use(telemetry.MeasureHTTP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// Prometheus Metrics
	r.Handle("/metrics", telemetry.Handler())

	// Probes
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"healthy","service":"scandrix-api"}`))
	})
	r.Get("/livez", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"alive"}`))
	})

	// Enterprise SCIM 2.0
	if cfg.SCIMService != nil {
		r.Mount("/scim/v2", cfg.SCIMService.Routes())
	}

	// RBAC Policy Engine
	policyEngine := rbac.NewPolicyEngine()

	// Controllers
	authCtrl := controllers.NewAuthController(cfg.AuthService, cfg.Repo)
	if cfg.DeviceFlow != nil {
		authCtrl.SetDeviceFlowManager(cfg.DeviceFlow)
	}
	if cfg.OAuthService != nil {
		authCtrl.SetOAuthService(cfg.OAuthService)
	}
	if cfg.Mailer != nil {
		authCtrl.SetMailer(cfg.Mailer)
	}
	if cfg.AppBaseURL != "" {
		authCtrl.SetAppBaseURL(cfg.AppBaseURL)
	}
	if cfg.JWTSecret != "" {
		authCtrl.SetJWTSecret(cfg.JWTSecret)
	}

	reviewCtrl := controllers.NewReviewController(cfg.Repo, cfg.Orchestrator, cfg.StreamHub)
	rulesCtrl := controllers.NewRulesController(cfg.Evaluator)
	workspaceCtrl := controllers.NewWorkspaceController(cfg.Repo)
	usageCtrl := controllers.NewUsageController(cfg.Repo)
	teamCtrl := controllers.NewTeamController(cfg.Repo)
	codeCtrl := controllers.NewCodeManagementController(cfg.Repo)
	paramCtrl := controllers.NewParametersController(cfg.Repo)
	integCtrl := controllers.NewIntegrationController(cfg.Repo)
	permCtrl := controllers.NewPermissionsController()
	licCtrl := controllers.NewLicenseController(cfg.Repo)
	healthCtrl := controllers.NewWebhookHealthController(cfg.Repo)
	notifCtrl := controllers.NewNotificationController(cfg.Repo)
	feedCtrl := controllers.NewFeedbackController(cfg.Repo)
	issuesCtrl := controllers.NewIssuesController(cfg.Repo)
	automationCtrl := controllers.NewAutomationController(cfg.Repo)
	billingCtrl := controllers.NewBillingController(cfg.BillingService, cfg.Repo, cfg.BudgetLimiter)

	// mountAPIRoutes registers all functional domain routes
	mountAPIRoutes := func(target chi.Router) {
		// Public auth endpoints
		target.Mount("/auth", authCtrl.Routes())
		// Public billing webhook receiver (authenticates via HMAC signature)
		target.Mount("/webhooks/billing", billingCtrl.WebhookRoutes())

		// Authenticated routes with RBAC Policy Guards (Kodus @CheckPolicies parity)
		target.Group(func(authGroup chi.Router) {
			authGroup.Use(cfg.AuthService.Middleware)

			authGroup.Mount("/reviews", reviewCtrl.Routes())
			authGroup.Mount("/usage", usageCtrl.Routes())
			authGroup.Mount("/teams", teamCtrl.Routes())
			authGroup.Mount("/repos", codeCtrl.Routes())
			authGroup.Mount("/parameters", paramCtrl.Routes())
			authGroup.Mount("/permissions", permCtrl.Routes())
			authGroup.Mount("/health", healthCtrl.Routes())
			authGroup.Mount("/notifications", notifCtrl.Routes())
			authGroup.Mount("/findings", feedCtrl.Routes())
			authGroup.Mount("/issues", issuesCtrl.Routes())
			authGroup.Mount("/automations", automationCtrl.Routes())

			// RBAC-protected routes: rules require manage permission
			authGroup.Group(func(rulesGroup chi.Router) {
				rulesGroup.Use(rbac.RequirePolicy(policyEngine, rbac.ActionManage, rbac.ResourceRules))
				rulesGroup.Mount("/rules", rulesCtrl.Routes())
			})

			// RBAC-protected routes: workspace management requires update permission
			authGroup.Group(func(wsGroup chi.Router) {
				wsGroup.Use(rbac.RequirePolicy(policyEngine, rbac.ActionUpdate, rbac.ResourceWorkspace))
				wsGroup.Mount("/workspaces", workspaceCtrl.Routes())
			})

			// RBAC-protected routes: integrations require manage permission
			authGroup.Group(func(integGroup chi.Router) {
				integGroup.Use(rbac.RequirePolicy(policyEngine, rbac.ActionManage, rbac.ResourceIntegrations))
				integGroup.Mount("/integrations", integCtrl.Routes())
			})

			// RBAC-protected routes: license/billing requires billing access
			authGroup.Group(func(billingGroup chi.Router) {
				billingGroup.Use(rbac.RequirePolicy(policyEngine, rbac.ActionManage, rbac.ResourceBilling))
				billingGroup.Mount("/license", licCtrl.Routes())
				billingGroup.Mount("/billing", billingCtrl.ProtectedRoutes())
			})
		})
	}

	// Mount on /api/v1 (standard REST spec) and root / (for Next.js apps/web proxy compatibility)
	r.Route("/api/v1", mountAPIRoutes)
	mountAPIRoutes(r)

	return r
}
