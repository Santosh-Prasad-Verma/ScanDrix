package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/scandrix/backend/internal/api/controllers"
	scandrixMiddleware "github.com/scandrix/backend/internal/api/middleware"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/auth/cliauth"
	"github.com/scandrix/backend/internal/auth/clitokens"
	"github.com/scandrix/backend/internal/auth/mailer"
	"github.com/scandrix/backend/internal/auth/oauth"
	"github.com/scandrix/backend/internal/billing/razorpay"
	"github.com/scandrix/backend/internal/cache"
	"github.com/scandrix/backend/internal/cache/limiter"
	"github.com/scandrix/backend/internal/database"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/scandrix/backend/internal/organization"
	"github.com/scandrix/backend/internal/enterprise/rbac"
	"github.com/scandrix/backend/internal/enterprise/scim"
	"github.com/scandrix/backend/internal/agents/businessrules"
	"github.com/scandrix/backend/internal/agents/conversation"
	"github.com/scandrix/backend/internal/agentharness/contracts"
	"github.com/scandrix/backend/internal/agentharness/infrastructure/persistence"
	"github.com/scandrix/backend/internal/agentharness/infrastructure/runner"
	"github.com/scandrix/backend/internal/llm"
	"github.com/scandrix/backend/internal/llm/byok"
	"github.com/scandrix/backend/internal/mcp"
	"github.com/scandrix/backend/internal/queue/relay"
	"github.com/scandrix/backend/internal/review"
	"github.com/scandrix/backend/internal/rules"
	drixyModules "github.com/scandrix/backend/internal/rules/drixy/modules"
	"github.com/scandrix/backend/internal/platformdata/application/usecases"
	platformRepo "github.com/scandrix/backend/internal/platformdata/infrastructure/repositories"
	"github.com/scandrix/backend/internal/clireview"
	centinfra "github.com/scandrix/backend/internal/centralizedconfig/infrastructure"
	"github.com/scandrix/backend/internal/analytics/pricing"
	analyticsRepo "github.com/scandrix/backend/internal/analytics/repository"
	"github.com/scandrix/backend/internal/analytics/spendlimit"
	"github.com/scandrix/backend/internal/analytics/usage"
	"github.com/scandrix/backend/internal/telemetry"
	"github.com/scandrix/backend/internal/webhooks/ingestion"
)

// ═══════════════════════════════════════════════════════════════
// 1. ROUTER CONFIGURATION & DEPENDENCY INJECTION (Server options & service bindings)
// ═══════════════════════════════════════════════════════════════

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
	CacheClient    *cache.Client
	RateLimiter     limiter.RateLimiter
	TierRateLimiter *scandrixMiddleware.TierAwareRateLimiter
	AppBaseURL         string
	JWTSecret          string
	CLITokenService    *clitokens.TokenService
	LoopbackManager    *cliauth.LoopbackManager
	HelpdeskService    *auth.HelpdeskTokenService
	DeviceQuotaManager *auth.DeviceManager
	CliEngine          *clireview.Engine
	CliDashboard       *clireview.DashboardStore

	// Registration anti-abuse & verification controls
	RequireEmailVerification bool
	BlockedEmailDomains      []string
	TurnstileSecretKey       string

	// Webhook secrets for Git SCM and Billing ingestion
	GitHubWebhookSecret      string
	GitLabWebhookSecret      string
	BitbucketWebhookSecret   string
	AzureDevOpsWebhookSecret string
	ForgejoWebhookSecret     string
	BillingWebhookSecret     string
}

// ═══════════════════════════════════════════════════════════════
// 2. MASTER ROUTER BUILDER (Chi router initialization & pipeline assembly)
// ═══════════════════════════════════════════════════════════════

// BuildRouter constructs the master Chi HTTP router mounting all domain controllers.
func BuildRouter(cfg RouterConfig) chi.Router {
	r := chi.NewRouter()

	// ═══════════════════════════════════════════════════════════════
	// 3. GLOBAL MIDDLEWARE PIPELINE (CORS, RateLimiting, CSRF, Metrics & Security)
	// ═══════════════════════════════════════════════════════════════

	// Base Middlewares (Order: Recoverer -> RequestID -> SecurityHeaders -> CORS -> RateLimit -> CSRF -> Metrics -> Logger -> Timeout)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(scandrixMiddleware.SecurityHeaders)
	corsCfg := scandrixMiddleware.DefaultCORSConfig()
	r.Use(scandrixMiddleware.CORS(corsCfg))

	// Rate Limiting: distributed Redis token bucket with Lua scripts if Redis is available; in-memory fallback otherwise.
	rateLimiter := cfg.RateLimiter
	if rateLimiter == nil {
		if cfg.CacheClient != nil {
			rateLimiter = limiter.NewRedisTokenBucketLimiter(cfg.CacheClient.Raw(), limiter.RateLimitConfig{
				Capacity:         200,
				RefillRatePerSec: 100,
			})
		} else {
			rateLimiter = limiter.NewTokenBucketLimiter(limiter.RateLimitConfig{
				Capacity:         200,
				RefillRatePerSec: 100,
			})
		}
	}
	r.Use(scandrixMiddleware.RateLimit(rateLimiter))

	tierLimiter := cfg.TierRateLimiter
	if tierLimiter == nil {
		tierLimiter = scandrixMiddleware.NewTierAwareRateLimiter()
	}

	r.Use(scandrixMiddleware.CSRFProtection(corsCfg.AllowedOrigins))
	r.Use(telemetry.MeasureHTTP)
	r.Use(middleware.Logger)
	r.Use(middleware.Timeout(60 * time.Second))

	// ═══════════════════════════════════════════════════════════════
	// 4. OBSERVABILITY & SYSTEM PROBES (Health checks, livez & Prometheus metrics)
	// ═══════════════════════════════════════════════════════════════

	// Prometheus Metrics
	r.Handle("/metrics", telemetry.Handler())

	// Probes
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		dbStatus := "ok"
		if cfg.Repo != nil {
			if err := cfg.Repo.Ping(ctx); err != nil {
				dbStatus = "error: " + err.Error()
			}
		}

		if dbStatus != "ok" {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":   "unhealthy",
				"service":  "scandrix-api",
				"database": dbStatus,
			})
			return
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":   "healthy",
			"service":  "scandrix-api",
			"database": dbStatus,
		})
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

	// ═══════════════════════════════════════════════════════════════
	// 5. ENTERPRISE CONTROLLERS FACTORY (Instantiating domain controllers)
	// ═══════════════════════════════════════════════════════════════

	// Controllers
	var (
		authRepo      controllers.AuthRepository
		reviewRepo    controllers.ReviewRepository
		workspaceRepo controllers.WorkspaceRepository
		usageRepo     controllers.UsageRepository
		teamRepo      controllers.TeamRepository
		codeRepo      controllers.CodeManagementRepository
		paramRepo     controllers.ParametersRepository
		orgParamRepo  controllers.OrgParametersRepository
		integRepo     controllers.IntegrationRepository
		licRepo       controllers.LicenseRepository
		healthRepo    controllers.WebhookHealthRepository
		notifRepo     controllers.NotificationRepository
		feedRepo      controllers.FeedbackRepository
		issuesRepo    controllers.IssuesRepository
		autoRepo      controllers.AutomationRepository
		billingRepo   controllers.BillingRepository
		auditRepo     controllers.AuditRepository
		ghRepo        controllers.GitHubRepository
		orgRepo       controllers.OrganizationRepository
	)
	var rulesRepos []controllers.RulesRepository
	if cfg.Repo != nil {
		authRepo = cfg.Repo
		reviewRepo = cfg.Repo
		workspaceRepo = cfg.Repo
		usageRepo = cfg.Repo
		teamRepo = cfg.Repo
		codeRepo = cfg.Repo
		paramRepo = cfg.Repo
		orgParamRepo = cfg.Repo
		integRepo = cfg.Repo
		licRepo = cfg.Repo
		healthRepo = cfg.Repo
		notifRepo = cfg.Repo
		feedRepo = cfg.Repo
		issuesRepo = cfg.Repo
		autoRepo = cfg.Repo
		billingRepo = cfg.Repo
		auditRepo = cfg.Repo
		ghRepo = cfg.Repo
		orgRepo = cfg.Repo
		rulesRepos = []controllers.RulesRepository{cfg.Repo}
	}

	authCtrl := controllers.NewAuthController(cfg.AuthService, authRepo)
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
	if cfg.CLITokenService != nil {
		authCtrl.SetCLITokenService(cfg.CLITokenService)
	}
	if cfg.LoopbackManager != nil {
		authCtrl.SetLoopbackManager(cfg.LoopbackManager)
	}
	if cfg.HelpdeskService != nil {
		authCtrl.SetHelpdeskTokenService(cfg.HelpdeskService)
	}
	if cfg.DeviceQuotaManager != nil {
		authCtrl.SetDeviceManager(cfg.DeviceQuotaManager)
	}
	if cfg.RequireEmailVerification {
		authCtrl.SetRequireEmailVerification(true)
	}
	if len(cfg.BlockedEmailDomains) > 0 {
		authCtrl.SetBlockedEmailDomains(cfg.BlockedEmailDomains)
	}
	if cfg.TurnstileSecretKey != "" {
		authCtrl.SetTurnstileSecretKey(cfg.TurnstileSecretKey)
	}
	if cfg.CacheClient != nil {
		authCtrl.SetCacheClient(cfg.CacheClient)
	}

	reviewCtrl := controllers.NewReviewController(reviewRepo, cfg.Orchestrator, cfg.StreamHub)
	rulesCtrl := controllers.NewRulesController(cfg.Evaluator, rulesRepos...)

	var dbClient *database.Client
	if cfg.Repo != nil {
		dbClient = cfg.Repo.Client()
	}
	var llmGw *llm.Gateway
	if cfg.Orchestrator != nil {
		llmGw = cfg.Orchestrator.LLMGateway()
	}

	drixyModule := drixyModules.NewDrixyRulesModule(drixyModules.DrixyRulesModuleConfig{
		DatabaseClient:  dbClient,
		LLMGateway:      llmGw,
		CLIKeyValidator: cfg.Repo,
	})
	drixyRulesCtrl := drixyModule.RulesController
	ruleLikeCtrl := drixyModule.RuleLikeController
	cliDrixyRulesCtrl := drixyModule.CliRulesController
	var pool *pgxpool.Pool
	if cfg.Repo != nil && cfg.Repo.Client() != nil {
		pool = cfg.Repo.Client().Pool
	}
	orgModule := organization.NewOrganizationModule(pool, nil, organization.ModuleConfig{})

	// Token Analytics & Pricing Engine
	pricingCatalog := pricing.NewTokenPricingCatalog()
	pricingResolver := pricing.NewPricingResolver(pricingCatalog)
	modelCostCalc := pricing.NewModelCostCalculator(pricingResolver)

	tokenUsageRepo := analyticsRepo.NewTokenUsageRepository(pricingResolver)
	tokenUsageService := analyticsRepo.NewTokenUsageService(tokenUsageRepo)

	usageSummaryUC := usage.NewBuildUsageSummaryUseCase(tokenUsageService, pricingResolver, nil)
	costEstimateUC := usage.NewCostEstimateUseCase(tokenUsageService, nil, modelCostCalc)
	developerUsageUC := usage.NewTokensByDeveloperUseCase(tokenUsageService, nil, nil)
	monthlySpendUC := usage.NewMonthlySpendUseCase(tokenUsageService, modelCostCalc)

	spendLimitCfgSvc := spendlimit.NewSpendLimitConfigService(nil, monthlySpendUC, pricingResolver)
	configureSpendLimitUC := spendlimit.NewConfigureSpendLimitUseCase(spendLimitCfgSvc)
	getSpendLimitConfigUC := spendlimit.NewGetSpendLimitConfigUseCase(spendLimitCfgSvc, nil, nil, pricingResolver)

	workspaceCtrl := controllers.NewWorkspaceController(workspaceRepo)
	usageCtrl := controllers.NewUsageController(usageRepo).WithAnalyticsServices(
		tokenUsageService,
		usageSummaryUC,
		costEstimateUC,
		developerUsageUC,
		pricingCatalog,
		spendLimitCfgSvc,
	)
	teamCtrl := controllers.NewTeamController(teamRepo).WithUseCases(
		orgModule.CreateTeamUC,
		orgModule.ListTeamsUC,
		orgModule.ListTeamsWithIntegrationsUC,
		orgModule.CreateTeamMemberUC,
		orgModule.DeleteTeamMemberUC,
		orgModule.GetTeamMembersUC,
		orgModule.TeamCliKeyService,
	)
	codeCtrl := controllers.NewCodeManagementController(codeRepo)
	codeCtrl.SetOAuthService(cfg.OAuthService)

	centStorage := centinfra.NewDefaultConfigStorage(paramRepo)
	centTree := centinfra.NewMemoryTreeProvider()
	centPRSvc := centinfra.NewPRService(nil)
	centSvc := centinfra.NewService(centPRSvc, centTree, centStorage)

	paramCtrl := controllers.NewParametersController(paramRepo).
		WithUseCases(
			orgModule.ParamFindByKeyUC,
			orgModule.ParamCreateOrUpdateUC,
			orgModule.ParamGetDefaultConfigUC,
		).
		WithCentralizedConfig(centSvc, centPRSvc)
	orgParamCtrl := controllers.NewOrganizationParametersController(orgParamRepo).WithUseCases(
		orgModule.OrgParamFindByKeyUC,
		orgModule.OrgParamCreateOrUpdateUC,
		orgModule.GetCockpitMetricsVisUC,
		orgModule.GetLLMConfigStatusUC,
		orgModule.ListModelOverridesUC,
		orgModule.ClearModelOverridesUC,
		orgModule.DeleteBYOKConfigUC,
		orgModule.GetBYOKProvidersUC,
		orgModule.GetModelsByProviderUC,
		orgModule.GetModelCapabilitiesUC,
		orgModule.IgnoreBotsUC,
		orgModule.TestBYOKConnectionUC,
		orgModule.TestBYOKModelUC,
	)
	integCtrl := controllers.NewIntegrationController(integRepo)
	permCtrl := controllers.NewPermissionsController(cfg.Repo)
	licCtrl := controllers.NewLicenseController(licRepo)
	healthCtrl := controllers.NewWebhookHealthController(healthRepo)
	notifCtrl := controllers.NewNotificationController(notifRepo)
	feedCtrl := controllers.NewFeedbackController(feedRepo)
	issuesCtrl := controllers.NewIssuesController(issuesRepo)
	automationCtrl := controllers.NewAutomationController(autoRepo)
	billingCtrl := controllers.NewBillingController(cfg.BillingService, billingRepo, cfg.BudgetLimiter, cfg.BillingWebhookSecret)
	auditCtrl := controllers.NewAuditController(auditRepo)
	githubCtrl := controllers.NewGitHubController(ghRepo)
	orgCtrl := controllers.NewOrganizationController(orgRepo).WithUseCases(
		orgModule.GetOrganizationNameUC,
		orgModule.GetOrganizationLanguageUC,
		orgModule.UpdateInfosUC,
		orgModule.GetOrganizationsByDomainUC,
		orgModule.GetReleaseTrackUC,
	)

	// PR Dashboard & Management Controllers
	prCtrl := controllers.NewPullRequestController(cfg.Repo, cfg.StreamHub)
	if cfg.Repo != nil && cfg.Repo.Client() != nil && cfg.Repo.Client().Pool != nil {
		platformPRRepo := platformRepo.NewPostgresPullRequestsRepository(cfg.Repo.Client().Pool)
		backfillUC := usecases.NewBackfillHistoricalPRsUseCase(platformPRRepo, nil, nil)
		prCtrl = prCtrl.WithBackfillUseCase(backfillUC)
	}
	prMessagesCtrl := controllers.NewPullRequestMessagesController(cfg.Repo)
	cockpitCtrl := controllers.NewCockpitController(cfg.Repo)
	spendLimitCtrl := controllers.NewSpendLimitController(cfg.Repo).WithSpendLimitServices(
		spendLimitCfgSvc,
		configureSpendLimitUC,
		getSpendLimitConfigUC,
	)
	systemCtrl := controllers.NewSystemController()
	skillsCtrl := controllers.NewSkillsController()
	userLogCtrl := controllers.NewUserLogController(auditRepo)
	userCtrl := controllers.NewUserController(authCtrl, authRepo).WithJoinOrganizationUseCase(orgModule.JoinOrganizationUC)
	workflowQueueCtrl := controllers.NewWorkflowQueueController(nil)
	ssoConfigCtrl := controllers.NewSSOConfigController(authCtrl)

	// CLI Review Execution & History Dashboard Controllers
	cliEngine := cfg.CliEngine
	if cliEngine == nil {
		cliEngine = clireview.NewEngine(cfg.Evaluator, nil)
	}
	cliDashboard := cfg.CliDashboard
	if cliDashboard == nil {
		cliDashboard = clireview.NewDashboardStore()
	}
	keyValidator := clireview.NewKeyValidator(cfg.JWTSecret, nil)
	cliReviewCtrl := controllers.NewCliReviewController(cliEngine, keyValidator, nil, nil, nil)
	cliReviewsCtrl := controllers.NewCliReviewsController(cliDashboard)

	// Agent Harness & Conversational Controller
	var convStore contracts.ConversationStore
	if cfg.Repo != nil && cfg.Repo.Client() != nil && cfg.Repo.Client().Pool != nil {
		pgStore := persistence.NewPostgresConversationStore(cfg.Repo.Client().Pool)
		_ = pgStore.AutoMigrate(context.Background())
		convStore = pgStore
	} else {
		convStore = persistence.NewInMemoryConversationStore()
	}

	var agentRunner contracts.AgentRunner
	if llmGw != nil && llmGw.Engine() != nil {
		defaultSlot := llmGw.ResolveDefaultSlot()
		if defaultSlot != nil {
			agentRunner, _ = llmGw.Engine().BuildAgentRunner(*defaultSlot)
		} else {
			agentRunner, _ = llmGw.Engine().BuildAgentRunner(byok.NormalizedModel{
				Provider: byok.ProviderOpenAI,
				Model:    "gpt-4o",
			})
		}
	}
	if agentRunner == nil {
		agentRunner = runner.NewGoAgentRunner(func(ctx context.Context, req runner.ModelTurnRequest) (*runner.ModelTurnResult, error) {
			return &runner.ModelTurnResult{Text: "Agent harness active."}, nil
		})
	}

	convProvider := conversation.NewConversationAgentProvider(agentRunner, convStore)
	brvProvider := businessrules.NewBusinessRulesValidationAgentProvider(agentRunner)
	agentCtrl := controllers.NewAgentController(convProvider, brvProvider)

	// SCM Webhook Ingestion Engine
	secretsMap := map[string]string{
		"github":    cfg.GitHubWebhookSecret,
		"gitlab":    cfg.GitLabWebhookSecret,
		"bitbucket": cfg.BitbucketWebhookSecret,
		"azure":     cfg.AzureDevOpsWebhookSecret,
		"forgejo":   cfg.ForgejoWebhookSecret,
	}
	webhookResolver := ingestion.NewDynamicSecretResolver(cfg.Repo, secretsMap)
	gitWebhookHandler := ingestion.NewIngestionHandler(webhookResolver, relay.NewOutboxStore(), cfg.Repo)

	// ═══════════════════════════════════════════════════════════════
	// 6. PUBLIC API & OAUTH ENDPOINTS (CLI auth, SSO, GitHub app & webhooks)
	// ═══════════════════════════════════════════════════════════════

	mountAPIRoutes := func(target chi.Router) {
		// Root and API-level CLI device authorization web page
		target.Get("/cli/authorize", authCtrl.HandleCLIAuthorizePage)

		// CLI key validation & health check (matching ScanDrix AI /cli/validate-key)
		target.Get("/cli/validate-key", authCtrl.HandleValidateCLIKey)
		target.Post("/cli/validate-key", authCtrl.HandleValidateCLIKey)

		// CLI auth session info for confirmation UI (matching ScanDrix AI /cli/auth/login-info)
		target.Get("/cli/auth/login-info", authCtrl.HandleCLILoginInfo)
		target.Get("/cli/login-info", authCtrl.HandleCLILoginInfo)

		// CLI rules endpoints (authenticated via X-Team-Key or Bearer token)
		target.Mount("/cli/drixy-rules", cliDrixyRulesCtrl.Routes())
		target.Mount("/cli/rules", cliDrixyRulesCtrl.Routes())

		// CLI review, trial, and public PR execution endpoints
		target.Mount("/cli/review", cliReviewCtrl.ReviewRoutes())
		target.Mount("/cli/trial", cliReviewCtrl.TrialRoutes())
		target.Mount("/cli/public", cliReviewCtrl.PublicRoutes())
		target.Mount("/cli/sessions", cliReviewCtrl.SessionsRoutes())
		target.Mount("/cli/memory", cliReviewCtrl.MemoryRoutes())
		target.Post("/cli/business-validation", cliReviewCtrl.HandleBusinessValidation)

		// Rate-limited public authentication & verification endpoints (Master Rule 4.5, ASVS V2.2.1)
		target.Group(func(authLim chi.Router) {
			authLim.Use(authCtrl.RateLimitMiddleware())
			authLim.Post("/cli/authorize/approve", authCtrl.HandleCLIAuthorizeApprove)
			authLim.Post("/cli/auth/login-init", authCtrl.HandleCLILoginInit)
			authLim.Post("/cli/auth/device-init", authCtrl.HandleCLIDeviceInit)
			authLim.Get("/cli/auth/login-poll", authCtrl.HandleCLILoginPoll)
			authLim.Get("/user/email", authCtrl.HandleCheckEmail)
			authLim.Get("/sso/check", authCtrl.HandleSSOCheck)
			authLim.Get("/auth/sso/check", authCtrl.HandleSSOCheck)
		})

		target.Get("/sso/login/{organizationId}", authCtrl.HandleSAMLLogin)
		target.Post("/sso/saml/callback/{organizationId}", authCtrl.HandleSAMLACS)

		// Public GitHub App handshake endpoints (callback from GitHub app installations)
		target.Mount("/github", githubCtrl.Routes())

		// Public auth endpoints
		target.Mount("/auth", authCtrl.Routes())

		// Public billing webhook receiver (authenticates via HMAC signature)
		target.Mount("/webhooks/billing", billingCtrl.WebhookRoutes())
		target.Mount("/billing/webhook", billingCtrl.WebhookRoutes())

		// Public SCM webhook receivers (GitHub, GitLab, Bitbucket, Azure DevOps, Forgejo)
		target.Post("/webhooks/github", gitWebhookHandler.ServeHTTP)
		target.Post("/webhooks/gitlab", gitWebhookHandler.ServeHTTP)
		target.Post("/webhooks/bitbucket", gitWebhookHandler.ServeHTTP)
		target.Post("/webhooks/azure-repos", gitWebhookHandler.ServeHTTP)
		target.Post("/webhooks/azure", gitWebhookHandler.ServeHTTP)
		target.Post("/webhooks/forgejo", gitWebhookHandler.ServeHTTP)
		target.Post("/webhooks/ingest", gitWebhookHandler.ServeHTTP)
		target.Post("/webhooks/{provider}", gitWebhookHandler.ServeHTTP)

		// ScanDrix SCM webhook root paths
		target.Post("/github/webhook", gitWebhookHandler.ServeHTTP)
		target.Post("/gitlab/webhook", gitWebhookHandler.ServeHTTP)
		target.Post("/bitbucket/webhook", gitWebhookHandler.ServeHTTP)
		target.Post("/azure-repos/webhook", gitWebhookHandler.ServeHTTP)
		target.Post("/forgejo/webhook", gitWebhookHandler.ServeHTTP)

		// ScanDrix Model Context Protocol (MCP) Streamable HTTP Server
		mcpServer := mcp.NewServer()
		target.Mount("/mcp", mcp.NewHTTPServer(mcpServer))

		// System Introspection & Public Probes
		target.Mount("/system", systemCtrl.Routes())
		target.Mount("/pull-requests", prCtrl.Routes())
		target.Mount("/skills", skillsCtrl.Routes())
		target.Mount("/user-log", userLogCtrl.Routes())
		target.Mount("/user", userCtrl.Routes())
		target.Mount("/sso-config", ssoConfigCtrl.Routes())

		// ═══════════════════════════════════════════════════════════════
		// 7. RBAC-PROTECTED DOMAIN ROUTES (Reviews, rules, teams & billing)
		// ═══════════════════════════════════════════════════════════════

		// Authenticated routes with RBAC Policy Guards (@CheckPolicies)
		target.Group(func(authGroup chi.Router) {
			authGroup.Use(cfg.AuthService.Middleware)
			if tierLimiter != nil && cfg.Repo != nil {
				authGroup.Use(scandrixMiddleware.TierRateLimit(tierLimiter, cfg.Repo))
			}

			// CLI auth completion for logged-in web user (matching ScanDrix AI /cli/auth/login-complete)
			authGroup.Post("/cli/auth/login-complete", authCtrl.HandleCLILoginComplete)

			// CLI Review execution history dashboard (matching /cli-reviews/executions)
			authGroup.Mount("/cli-reviews", cliReviewsCtrl.Routes())
			authGroup.Mount("/cli/reviews", cliReviewsCtrl.Routes())
			authGroup.Mount("/rule-like", ruleLikeCtrl.Routes())
			authGroup.Mount("/cli/config/repositories", codeCtrl.CLIRepositoriesConfigRoutes(paramCtrl))
			authGroup.Mount("/cli/config/centralized", paramCtrl.CentralizedConfigRoutes())
			authGroup.Mount("/config/centralized", paramCtrl.CentralizedConfigRoutes())

			authGroup.Mount("/reviews", reviewCtrl.Routes())
			authGroup.Mount("/usage", usageCtrl.Routes())
			authGroup.Mount("/spend-limit", spendLimitCtrl.Routes())
			authGroup.Mount("/cockpit", cockpitCtrl.Routes())
			authGroup.Mount("/code-health", cockpitCtrl.CodeHealthRoutes())
			authGroup.Mount("/productivity", cockpitCtrl.ProductivityRoutes())
			authGroup.Mount("/pull-request-messages", prMessagesCtrl.Routes())
			authGroup.Mount("/repos", codeCtrl.Routes())
			authGroup.Mount("/code-management", codeCtrl.Routes())
			authGroup.Mount("/permissions", permCtrl.Routes())
			authGroup.Mount("/health", healthCtrl.Routes())
			authGroup.Mount("/findings", feedCtrl.Routes())
			authGroup.Mount("/issues", issuesCtrl.Routes())
			authGroup.Mount("/integration", integCtrl.Routes())
			authGroup.Mount("/integration-config", integCtrl.ConfigRoutes())
			authGroup.Mount("/organization", orgCtrl.Routes())
			authGroup.Mount("/team-members", teamCtrl.TeamMembersRoutes())
			authGroup.Mount("/agent", agentCtrl.Routes())
			authGroup.Mount("/workflow-queue", workflowQueueCtrl.Routes())

			// RBAC-protected routes: teams / members require member management policy
			authGroup.Group(func(teamGroup chi.Router) {
				teamGroup.Use(rbac.RequirePolicy(policyEngine, rbac.ActionManage, rbac.ResourceMembers))
				teamGroup.Mount("/teams", teamCtrl.Routes())
			})

			authGroup.Mount("/parameters", paramCtrl.Routes())
			authGroup.Mount("/organization-parameters", orgParamCtrl.Routes())
			authGroup.Mount("/notifications", notifCtrl.Routes())

			// RBAC-protected routes: rules & automations require rule management policy
			authGroup.Group(func(rulesGroup chi.Router) {
				rulesGroup.Use(rbac.RequirePolicy(policyEngine, rbac.ActionManage, rbac.ResourceRules))
				rulesGroup.Mount("/rules", rulesCtrl.Routes())
				rulesGroup.Mount("/drixy-rules", drixyRulesCtrl.Routes())
				rulesGroup.Mount("/automations", automationCtrl.Routes())
			})

			// RBAC-protected routes: workspace management requires update permission
			authGroup.Group(func(wsGroup chi.Router) {
				wsGroup.Use(rbac.RequirePolicy(policyEngine, rbac.ActionUpdate, rbac.ResourceWorkspace))
				wsGroup.Mount("/workspaces", workspaceCtrl.Routes())
				wsGroup.Mount("/workspaces/{workspaceId}/audit-logs", auditCtrl.Routes())
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


	// ═══════════════════════════════════════════════════════════════
	// 8. REST V1 & ROOT COMPATIBILITY MOUNT (Dual-route path binding)
	// ═══════════════════════════════════════════════════════════════

	// Mount on /api/v1 (standard REST spec) and root / (for Next.js apps/web proxy compatibility)
	r.Route("/api/v1", mountAPIRoutes)
	mountAPIRoutes(r)

	return r
}
