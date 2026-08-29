package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/enterprise/scim"
	"github.com/scandrix/backend/internal/review"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/internal/telemetry"
)

// RouterConfig holds dependency references required to wire up the API server.
type RouterConfig struct {
	Repo         *database.Repository
	AuthService  *auth.Authenticator
	Orchestrator *review.Orchestrator
	StreamHub    *review.StreamHub
	Evaluator    *rules.Evaluator
	SCIMService  *scim.SCIMService
}

// BuildRouter constructs the master Chi HTTP router mounting all domain controllers.
func BuildRouter(cfg RouterConfig) chi.Router {
	r := chi.NewRouter()

	// Base Middlewares
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
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

	// Controllers
	authCtrl := controllers.NewAuthController(cfg.AuthService)
	reviewCtrl := controllers.NewReviewController(cfg.Repo, cfg.Orchestrator, cfg.StreamHub)
	rulesCtrl := controllers.NewRulesController(cfg.Evaluator)
	workspaceCtrl := controllers.NewWorkspaceController(cfg.Repo)
	usageCtrl := controllers.NewUsageController()
	teamCtrl := controllers.NewTeamController()
	codeCtrl := controllers.NewCodeManagementController()
	paramCtrl := controllers.NewParametersController()
	integCtrl := controllers.NewIntegrationController()
	permCtrl := controllers.NewPermissionsController()
	licCtrl := controllers.NewLicenseController()
	healthCtrl := controllers.NewWebhookHealthController()
	notifCtrl := controllers.NewNotificationController()
	feedCtrl := controllers.NewFeedbackController()

	// API v1 Sub-Router
	r.Route("/api/v1", func(v1 chi.Router) {
		// Public auth endpoints
		v1.Mount("/auth", authCtrl.Routes())

		// Authenticated routes
		v1.Group(func(authGroup chi.Router) {
			authGroup.Use(cfg.AuthService.Middleware)

			authGroup.Mount("/reviews", reviewCtrl.Routes())
			authGroup.Mount("/rules", rulesCtrl.Routes())
			authGroup.Mount("/workspaces", workspaceCtrl.Routes())
			authGroup.Mount("/usage", usageCtrl.Routes())
			authGroup.Mount("/teams", teamCtrl.Routes())
			authGroup.Mount("/repos", codeCtrl.Routes())
			authGroup.Mount("/parameters", paramCtrl.Routes())
			authGroup.Mount("/integrations", integCtrl.Routes())
			authGroup.Mount("/permissions", permCtrl.Routes())
			authGroup.Mount("/license", licCtrl.Routes())
			authGroup.Mount("/health", healthCtrl.Routes())
			authGroup.Mount("/notifications", notifCtrl.Routes())
			authGroup.Mount("/findings", feedCtrl.Routes())
		})
	})

	return r
}
