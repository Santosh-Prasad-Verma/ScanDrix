package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/scandrix/backend/internal/agentharness/contracts"
	"github.com/scandrix/backend/internal/agentharness/infrastructure/persistence"
	"github.com/scandrix/backend/internal/agentharness/infrastructure/runner"
	"github.com/scandrix/backend/internal/agents/businessrules"
	"github.com/scandrix/backend/internal/agents/conversation"
	"github.com/scandrix/backend/internal/analytics/pricing"
	analyticsRepo "github.com/scandrix/backend/internal/analytics/repository"
	"github.com/scandrix/backend/internal/analytics/spendlimit"
	"github.com/scandrix/backend/internal/analytics/usage"
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
	centinfra "github.com/scandrix/backend/internal/centralizedconfig/infrastructure"
	"github.com/scandrix/backend/internal/clireview"
	corehealth "github.com/scandrix/backend/internal/core/health"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/enterprise/license"
	"github.com/scandrix/backend/internal/enterprise/rbac"
	"github.com/scandrix/backend/internal/enterprise/scim"
	"github.com/scandrix/backend/internal/llm"
	"github.com/scandrix/backend/internal/llm/byok"
	"github.com/scandrix/backend/internal/mcp"
	"github.com/scandrix/backend/internal/organization"
	"github.com/scandrix/backend/internal/platformdata/application/usecases"
	platformRepo "github.com/scandrix/backend/internal/platformdata/infrastructure/repositories"
	"github.com/scandrix/backend/internal/queue/relay"
	"github.com/scandrix/backend/internal/review"
	"github.com/scandrix/backend/internal/rules"
	drixyModules "github.com/scandrix/backend/internal/rules/drixy/modules"
	"github.com/scandrix/backend/internal/telemetry"
	"github.com/scandrix/backend/internal/webhooks/ingestion"
	apiservices "github.com/scandrix/backend/internal/api/services"
	cmcontracts "github.com/scandrix/backend/internal/platform/domain/contracts"
	"github.com/scandrix/backend/pkg/models"
)

// ═══════════════════════════════════════════════════════════════
// 1. ROUTER CONFIGURATION & DEPENDENCY INJECTION (Server options & service bindings)
// ═══════════════════════════════════════════════════════════════

// RouterConfig holds dependency references required to wire up the API server.
type RouterConfig struct {
	Repo               *database.Repository
	AuthService        *auth.Authenticator
	Orchestrator       *review.Orchestrator
	StreamHub          *review.StreamHub
	Evaluator          *rules.Evaluator
	SCIMService        *scim.SCIMService
	DeviceFlow         *cliauth.DeviceFlowManager
	OAuthService       *oauth.OAuthService
	Mailer             mailer.EmailSender
	BillingService     *razorpay.BillingService
	BudgetLimiter      *llm.TokenBudgetLimiter
	CacheClient        *cache.Client
	RateLimiter        limiter.RateLimiter
	TierRateLimiter    *scandrixMiddleware.TierAwareRateLimiter
	AppBaseURL         string
	JWTSecret          string
	CLITokenService    *clitokens.TokenService
	LoopbackManager    *cliauth.LoopbackManager
	HelpdeskService    *auth.HelpdeskTokenService
	DeviceQuotaManager *auth.DeviceManager
	CliEngine          *clireview.Engine
	CliDashboard       *clireview.DashboardStore
	LicenseManager     *license.LicenseManager

	// SCMProviders holds one code-management adapter per provider, used to read
	// live pull request diffs. Adapters are stateless: credentials travel per
	// request, so a single instance serves every workspace. A provider absent from
	// the map is reported as unavailable rather than being served an empty file
	// list.
	SCMProviders map[models.SCMProvider]cmcontracts.ICodeManagementService

	licenseResolver *license.Resolver

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

// entitlementResolver memoizes the single resolver shared by the license
// controller, the feature gates and the capabilities endpoint, so no two
// consumers can disagree about what a workspace is entitled to.
func (c *RouterConfig) EntitlementResolver() *license.Resolver {
	if c.licenseResolver == nil {
		c.licenseResolver = license.NewResolver(c.LicenseManager, c.licenseStore(), c.seatCounter())
	}
	return c.licenseResolver
}

// seatCounter adapts the repository to the license package seat counter.
func (c *RouterConfig) seatCounter() license.SeatCounter {
	if c.Repo == nil {
		return nil
	}
	return c.Repo
}

// licenseStore adapts the billing repository to the license package store.
func (c *RouterConfig) licenseStore() license.Store {
	if c.Repo == nil {
		return nil
	}
	return billingLicenseStore{repo: c.Repo}
}

type billingLicenseStore struct {
	repo *database.Repository
}

func (s billingLicenseStore) GetActiveLicense(ctx context.Context, wsID uuid.UUID) (*license.StoredLicense, error) {
	lic, err := s.repo.GetActiveLicense(ctx, wsID)
	if err != nil || lic == nil {
		return nil, err
	}
	return &license.StoredLicense{
		Tier:       lic.PlanTier,
		Features:   lic.FeaturesEnabled,
		MaxSeats:   lic.TotalSeats,
		CustomerNm: lic.OrganizationName,
		ExpiresAt:  lic.ExpiresAt,
	}, nil
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

	// Rate limiting.
	//
	// Where a shared store is available, limiting is distributed (Redis token
	// bucket with a Lua script). Where it is not, the behaviour depends on the
	// environment, because the alternative is not "no limiting" but *weaker*
	// limiting (AUDIT_REMEDIATION.md F-29/F-32):
	//
	//   - development/test: a per-process bucket is genuinely correct, since
	//     there is one process. It is still used.
	//   - production/staging: a per-process bucket would multiply the effective
	//     limit by the replica count, so a single client could send N times the
	//     intended request volume, and a brute-force or lockout budget would be
	//     N times larger than intended. Requests are therefore denied outright
	//     rather than silently limited less.
	//
	// The Redis limiter is also put into fail-closed mode outside development, so
	// an outage does not silently swap the shared bucket for a local one.
	rateLimiter := cfg.RateLimiter
	if rateLimiter == nil {
		if cfg.CacheClient != nil {
			redisLimiter := limiter.NewRedisTokenBucketLimiter(cfg.CacheClient.Raw(), limiter.RateLimitConfig{
				Capacity:         200,
				RefillRatePerSec: 100,
			})
			if limiter.DistributedRequired() {
				redisLimiter.SetFailClosed(true)
			}
			rateLimiter = redisLimiter
		} else if limiter.DistributedRequired() {
			slog.Error("no shared cache client configured; refusing requests because " +
				"per-process rate limiting would multiply the effective limit by the replica count")
			rateLimiter = limiter.NewUnavailableLimiter(
				"no distributed rate-limit store configured")
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
	//
	// These use internal/core/health, which is the real implementation (it pings
	// the pool and reports process uptime). The previous inline handler here
	// duplicated it and only checked the database, so liveness and readiness were
	// two different code paths reporting two different things.
	healthSvc := corehealth.NewService(cfg.Repo.Pool(), nil, nil, os.Getenv("RELEASE_VERSION"))

	// Readiness: 503 when a dependency is down, so a load balancer stops routing.
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		code, resp := healthSvc.ReadyCheck(r.Context())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(resp)
	})

	// Health with details.
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		code, resp := healthSvc.Check(r.Context())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(resp)
	})

	// Liveness: process only, no dependency checks, so a database blip does not
	// get the container killed and restarted.
	r.Get("/livez", func(w http.ResponseWriter, r *http.Request) {
		code, resp := healthSvc.SimpleCheck()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(resp)
	})

	// Enterprise SCIM 2.0 — SCIM provisioning is a licensed capability.
	//
	// The entitlement check is enforced inside the SCIM service, not here: SCIM
	// authenticates with a bearer token and has no session, so the
	// workspace-scoped RequireFeature middleware could never resolve a workspace
	// and rejected every request with 401 before the service was reached. The
	// service holds its own tenant binding, so it answers the question itself.
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

	// Enforce access-token revocation on every authenticated request
	// (AUDIT_REMEDIATION.md F-18). Without this the middleware's revocation
	// hook is never populated and logout only stops renewal, leaving a copied
	// bearer token valid for its full lifetime.
	//
	// The checker is installed only when a repository is available. When the
	// store cannot answer, the request is rejected rather than admitted:
	// treating an unreachable revocation store as "not revoked" would turn a
	// database outage into an authentication bypass.
	if cfg.AuthService != nil && authRepo != nil {
		cfg.AuthService.SetRevocationChecker(func(rctx context.Context, userID uuid.UUID, issuedAt int64) bool {
			revoked, err := authRepo.IsAccessTokenRevoked(rctx, userID, issuedAt)
			if err != nil {
				slog.Error("access-token revocation lookup failed; rejecting request",
					"user_id", userID, "error", err)
				return true
			}
			return revoked
		})
		cfg.AuthService.SetIdentityValidator(func(rctx context.Context, userID, workspaceID uuid.UUID) (*models.AccountProfile, error) {
			user, err := authRepo.GetUserByID(rctx, userID)
			if err != nil || user == nil {
				return nil, errors.New("user not found")
			}
			if user.Status != "active" {
				return nil, errors.New("user account is not active")
			}
			if user.OrganizationID == nil || *user.OrganizationID == uuid.Nil {
				return nil, errors.New("user has no active workspace")
			}
			ws, err := authRepo.GetWorkspaceByID(rctx, *user.OrganizationID)
			if err != nil || ws == nil {
				return nil, errors.New("workspace not found")
			}
			if ws.Status != models.TenantStatusActive && strings.ToUpper(string(ws.Status)) != "ACTIVE" {
				return nil, errors.New("workspace is not active")
			}
			return &models.AccountProfile{
				ID:          user.UUID,
				WorkspaceID: *user.OrganizationID,
				Email:       user.Email,
				Role:        models.UserRole(user.Role),
			}, nil
		})
	} else if cfg.AuthService != nil {
		slog.Warn("no repository available: access-token revocation cannot be enforced, tokens will be accepted without a revocation check")
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
	teamCtrl := controllers.NewTeamController(teamRepo).
		WithEntitlements(cfg.EntitlementResolver()).
		WithUseCases(
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
	capsCtrl := controllers.NewCapabilitiesController(cfg.EntitlementResolver())
	licCtrl := controllers.NewLicenseController(licRepo).
		WithVerifier(cfg.LicenseManager).
		WithResolver(cfg.EntitlementResolver())
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

	// Live diff reads. Without this the /files endpoint reports only the paths a
	// review touched and names every diff field unavailable, rather than
	// inventing line counts.
	if cfg.Repo != nil && len(cfg.SCMProviders) > 0 {
		// Narrow the full adapters to the one method the bridge uses.
		filesProviders := make(map[models.SCMProvider]apiservices.PullRequestFilesProvider, len(cfg.SCMProviders))
		for provider, adapter := range cfg.SCMProviders {
			if adapter != nil {
				filesProviders[provider] = adapter
			}
		}
		prCtrl = prCtrl.WithPullRequestFileFetcher(
			apiservices.NewSCMPullRequestFileFetcher(cfg.Repo, filesProviders),
		)
	}
	if cfg.Repo != nil && cfg.Repo.Client() != nil && cfg.Repo.Client().Pool != nil {
		platformPRRepo := platformRepo.NewPostgresPullRequestsRepository(cfg.Repo.Client().Pool)
		backfillUC := usecases.NewBackfillHistoricalPRsUseCase(platformPRRepo, nil, nil)
		prCtrl = prCtrl.WithBackfillUseCase(backfillUC)
	}
	prMessagesCtrl := controllers.NewPullRequestMessagesController(cfg.Repo)
	cockpitCtrl := controllers.NewCockpitController(cfg.Repo).
		// Analytics reads the same client as the rest of the data access layer, so
		// every metric it returns is the workspace's own rows.
		WithAnalytics(database.NewPostgresAnalyticsRepository(cfg.Repo.Client()))
	spendLimitCtrl := controllers.NewSpendLimitController(cfg.Repo).WithSpendLimitServices(
		spendLimitCfgSvc,
		configureSpendLimitUC,
		getSpendLimitConfigUC,
	)
	systemCtrl := controllers.NewSystemController(cfg.Repo)
	skillsCtrl := controllers.NewSkillsController()
	userLogCtrl := controllers.NewUserLogController(auditRepo)
	userCtrl := controllers.NewUserController(authCtrl, authRepo).WithJoinOrganizationUseCase(orgModule.JoinOrganizationUC)
	workflowQueueCtrl := controllers.NewWorkflowQueueController(nil)
	ssoConfigCtrl := controllers.NewSSOConfigController(authCtrl, cfg.Repo)

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

		// Rate-limited public authentication & verification endpoints (Master Rule 4.5, ASVS V2.2.1)
		target.Group(func(authLim chi.Router) {
			authLim.Use(authCtrl.RateLimitMiddleware())
			authLim.Post("/cli/authorize/approve", authCtrl.HandleCLIAuthorizeApprove)
			authLim.Post("/cli/auth/login-init", authCtrl.HandleCLILoginInit)
			authLim.Post("/cli/auth/device-init", authCtrl.HandleCLIDeviceInit)
			authLim.Get("/cli/auth/login-poll", authCtrl.HandleCLILoginPoll)
			authLim.Get("/sso/check", authCtrl.HandleSSOCheck)
			authLim.Get("/auth/sso/check", authCtrl.HandleSSOCheck)
		})

		target.Get("/sso/login/{organizationId}", authCtrl.HandleSAMLLogin)
		target.Post("/sso/saml/callback/{organizationId}", authCtrl.HandleSAMLACS)

		// Public auth endpoints
		target.Mount("/auth", authCtrl.Routes())

		// Public billing webhook receiver (authenticates via HMAC signature)
		target.Mount("/webhooks/billing", billingCtrl.WebhookRoutes())
		target.Mount("/billing/webhook", billingCtrl.WebhookRoutes())
		target.Get("/billing/plans", billingCtrl.HandleListPlans)

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
		//
		// MCP is disabled by default (SCANDRIX_MCP_SERVER_ENABLED must be set
		// to true) and requires a valid session, so it cannot be reached
		// anonymously with a self-asserted organizationId.
		// See AUDIT_REMEDIATION.md F-15f.
		mcpServer := mcp.NewServer()
		target.Mount("/mcp", mcp.NewHTTPServer(mcpServer, cfg.AuthService))

		// System Introspection & Public Probes
		target.Mount("/system", systemCtrl.Routes())
		target.Mount("/skills", skillsCtrl.Routes())
		target.Mount("/user", userCtrl.Routes())

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
			// Entitlement payload for every gate in the client. Readable by any
			// authenticated member: the protected routes enforce entitlement, this
			// only reports it.
			authGroup.Mount("/capabilities", capsCtrl.Routes())

			// Workspace-scoped CLI key listing. The key material is never returned
			// here; only the prefix and metadata, since the plaintext key is shown
			// once at creation time.
			//
			// Listing and revoking CI credentials is a member-management action.
			// These routes previously sat directly on authGroup with no policy
			// guard, so any viewer or member could enumerate and revoke a
			// workspace's CI keys (AUDIT_REMEDIATION.md F-15e).
			authGroup.Group(func(cliKeyGroup chi.Router) {
				cliKeyGroup.Use(rbac.RequirePolicy(policyEngine, rbac.ActionManage, rbac.ResourceMembers))
				cliKeyGroup.Get("/cli/tokens", teamCtrl.HandleListWorkspaceCLIKeys)
				cliKeyGroup.Delete("/cli/tokens/{keyId}", teamCtrl.HandleRevokeWorkspaceCLIKey)
			})

			// Previously mounted in the unauthenticated block: the workspace context
			// was never populated, so every execution listing, digest, facet and
			// author endpoint returned 401 to all callers. The middleware accepts both
			// a session JWT and a scandrix_-prefixed team key, so the CLI-key path
			// that handleGetSuggestions falls back to keeps working.
			authGroup.Mount("/pull-requests", prCtrl.Routes())

			// Previously mounted in the unauthenticated block, where no auth
			// middleware ran: the workspace context was never populated, so these
			// routes rejected every caller. Mounted here so authentication and
			// revocation are enforced by the shared middleware.
			authGroup.Mount("/user-log", userLogCtrl.Routes())
			authGroup.Group(func(ssoGroup chi.Router) {
				ssoGroup.Use(scandrixMiddleware.RequireFeature(cfg.EntitlementResolver(), license.FeatureSSOSAML))
				ssoGroup.Mount("/sso-config", ssoConfigCtrl.Routes())
			})

			// SCIM token administration. Behind the same entitlement gate as the
			// provisioning endpoint itself, so a workspace that cannot provision
			// cannot mint a credential for it either.
			authGroup.Group(func(scimGroup chi.Router) {
				scimGroup.Use(scandrixMiddleware.RequireFeature(cfg.EntitlementResolver(), license.FeatureSCIM))
				scimGroup.Mount("/scim-config", controllers.NewSCIMTokenController(cfg.SCIMService).Routes())
			})

			authGroup.Mount("/cli-reviews", cliReviewsCtrl.Routes())
			authGroup.Mount("/cli/reviews", cliReviewsCtrl.Routes())
			authGroup.Mount("/rule-like", ruleLikeCtrl.Routes())
			authGroup.Mount("/cli/config/repositories", codeCtrl.CLIRepositoriesConfigRoutes(paramCtrl))
			authGroup.Mount("/cli/config/centralized", paramCtrl.CentralizedConfigRoutes())
			authGroup.Mount("/config/centralized", paramCtrl.CentralizedConfigRoutes())

			authGroup.Mount("/reviews", reviewCtrl.Routes())
			authGroup.Mount("/usage", usageCtrl.Routes())
			// Spend limits are budget configuration. Reads are visible to any
			// role granted billing read (owner/admin); writing the cap is a
			// full-authority billing action and stays owner-only, matching the
			// /billing mount below and PermBillingManage in the role matrix.
			// This mount previously carried no policy guard at all, so any
			// authenticated viewer or member could raise or remove the
			// workspace's spend ceiling.
			// Spend limits are budget configuration. Reads are visible to any role
			// granted billing read (owner/admin); writing the cap is a
			// full-authority billing action and stays owner-only, matching the
			// /billing mount below and PermBillingManage in the role matrix.
			// This mount previously carried no policy guard at all, so any
			// authenticated viewer or member could raise or remove the
			// workspace's spend ceiling.
			//
			// Read routes are mounted from their own router; the write routes are
			// registered directly on this group because chi refuses to mount a
			// second router at an already-mounted path, and the two verbs need
			// different policies.
			authGroup.Group(func(spendGroup chi.Router) {
				spendGroup.Use(rbac.RequirePolicy(policyEngine, rbac.ActionRead, rbac.ResourceBilling))
				spendGroup.Mount("/spend-limit", spendLimitCtrl.ReadRoutes())
				// Reads are already authorised above. Writes additionally require
				// full billing authority, applied per-route because chi cannot
				// mount a second router at the same path.
				manageBilling := rbac.RequirePolicy(policyEngine, rbac.ActionManage, rbac.ResourceBilling)
				for _, route := range spendLimitCtrl.WriteRoutePatterns() {
					spendGroup.Method(route.Method, "/spend-limit"+route.Pattern, manageBilling(route.Handler))
				}
			})
			authGroup.Mount("/cockpit", cockpitCtrl.Routes())
			authGroup.Mount("/code-health", cockpitCtrl.CodeHealthRoutes())
			authGroup.Mount("/productivity", cockpitCtrl.ProductivityRoutes())
			authGroup.Mount("/pull-request-messages", prMessagesCtrl.Routes())
			authGroup.Mount("/repos", codeCtrl.Routes())
			authGroup.Mount("/code-management", codeCtrl.Routes())
			// Assigning and revoking per-user repository access is a
			// member-management action. This mount previously carried no policy
			// guard, so any authenticated viewer or member could grant themselves
			// access to a repository an admin had deliberately restricted
			// (AUDIT_REMEDIATION.md F-15a).
			authGroup.Group(func(permGroup chi.Router) {
				permGroup.Use(rbac.RequirePolicy(policyEngine, rbac.ActionManage, rbac.ResourceMembers))
				permGroup.Mount("/permissions", permCtrl.Routes())
			})
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

			// GitHub App handshake read-back. These disclose which GitHub account
			// the caller's workspace is connected as, so they require a session:
			// mounted publicly they were an account-identity oracle, and their
			// previous env/token fallbacks disclosed the platform's own GitHub
			// account to every tenant (AUDIT_REMEDIATION.md F-44). The actual App
			// callback lives at /auth/oauth/github/callback and is unaffected.
			authGroup.Mount("/github", githubCtrl.Routes())

			authGroup.Mount("/parameters", paramCtrl.Routes())
			// Workspace-level provider configuration holds BYOK credentials and
			// model routing. Repointing it sends every subsequent review's source
			// code to whatever endpoint is configured here, so writes require
			// workspace update authority (owner/admin). Reads are visible to any
			// role holding workspace read, because every member legitimately needs
			// to know which model the workspace uses.
			//
			// This mount previously carried no policy guard, so any authenticated
			// viewer or member could replace the workspace's LLM credentials.
			authGroup.Group(func(orgParamGroup chi.Router) {
				orgParamGroup.Use(rbac.RequirePolicy(policyEngine, rbac.ActionRead, rbac.ResourceWorkspace))
				orgParamGroup.Mount("/organization-parameters", orgParamCtrl.ReadRoutes())
				// Reads are already authorised above. Mutations additionally require
				// workspace update authority, applied per-route because chi
				// cannot mount a second router at the same path.
				updateWorkspace := rbac.RequirePolicy(policyEngine, rbac.ActionUpdate, rbac.ResourceWorkspace)
				for _, route := range orgParamCtrl.WriteRoutePatterns() {
					orgParamGroup.Method(route.Method, "/organization-parameters"+route.Pattern, updateWorkspace(route.Handler))
				}
			})
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
				wsGroup.Group(func(auditGroup chi.Router) {
					auditGroup.Use(scandrixMiddleware.RequireFeature(cfg.EntitlementResolver(), license.FeatureAuditWarehouse))
					auditGroup.Mount("/workspaces/{workspaceId}/audit-logs", auditCtrl.Routes())
				})
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
