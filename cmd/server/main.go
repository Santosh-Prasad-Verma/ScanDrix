package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"github.com/google/uuid"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	contracts "github.com/scandrix/backend/internal/platform/domain/contracts"
	github "github.com/scandrix/backend/internal/platform/github"
	gitlab "github.com/scandrix/backend/internal/platform/gitlab"
	bitbucket "github.com/scandrix/backend/internal/platform/bitbucket"
	"github.com/scandrix/backend/internal/api"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/auth/cliauth"
	"github.com/scandrix/backend/internal/auth/clitokens"
	"github.com/scandrix/backend/internal/auth/mailer"
	"github.com/scandrix/backend/internal/auth/oauth"
	"github.com/scandrix/backend/internal/billing/razorpay"
	"github.com/scandrix/backend/internal/cache"
	"github.com/scandrix/backend/internal/config"
	"github.com/scandrix/backend/internal/cron"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/enterprise/license"
	"github.com/scandrix/backend/internal/enterprise/scim"
	"github.com/scandrix/backend/internal/integrations/pm"
	"github.com/scandrix/backend/internal/llm"
	"github.com/scandrix/backend/internal/review"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/pkg/models"
	"github.com/scandrix/backend/internal/sandbox"
	"github.com/scandrix/backend/internal/sandbox/lease"
	"github.com/scandrix/backend/internal/storage"
)

func main() {
	// ═══════════════════════════════════════════════════════════════
	// 1. LOGGING & INITIALIZATION (Monolithic server bootstrap)
	// ═══════════════════════════════════════════════════════════════
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	slog.Info("Starting Scandrix Unified Enterprise Server (Monolithic mode)")

	// ═══════════════════════════════════════════════════════════════
	// 2. CONFIGURATION & RUNTIME CONTEXT (Environment loading & context)
	// ═══════════════════════════════════════════════════════════════
	cfg, err := config.Load()
	if err != nil {
		slog.Error("Configuration error", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ═══════════════════════════════════════════════════════════════
	// 3. PERSISTENCE & REPOSITORY (Database connection & repository)
	// ═══════════════════════════════════════════════════════════════
	dbClient, err := database.NewClient(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("Database connection failed", "error", err)
		os.Exit(1)
	}
	defer dbClient.Close()
	repo := database.NewRepository(dbClient)

	// ═══════════════════════════════════════════════════════════════
	// 4. DISTRIBUTED CACHING & REDIS (Locking, session cache & rate limiting)
	// ═══════════════════════════════════════════════════════════════
	var cacheClient *cache.Client
	if cfg.RedisURL != "" {
		rc, err := cache.NewClient(ctx, cfg.RedisURL)
		if err != nil {
			slog.Warn("Redis connection deferred", "error", err)
		} else {
			defer rc.Close()
			cacheClient = rc
			slog.Info("Redis connection pool initialized for distributed caching & rate limiting")
		}
	}

	// ═══════════════════════════════════════════════════════════════
	// 5. ENTERPRISE DOMAIN SERVICES (Auth, Review, LLM, Rules & SCIM)
	// ═══════════════════════════════════════════════════════════════
	authenticator := auth.NewAuthenticator(cfg.JWTSecret)
	authenticator.SetCLIVerifier(repo.VerifyCLIToken)
	cliTokenService := clitokens.NewTokenService(repo)
	streamHub := review.NewStreamHub()
	if cacheClient != nil {
		streamHub.SetRedisClient(cacheClient)
	}
	artifactClient := storage.NewArtifactClient(cfg.AppwriteEndpoint, cfg.AppwriteProjectID, cfg.AppwriteAPIKey)
	aiGateway := llm.NewGateway(cfg.AnthropicAPIKey, cfg.OpenAIAPIKey, cfg.GeminiAPIKey, cfg.LocalLLMEndpoint)
	evaluator, _ := rules.NewEvaluator(rules.DefaultCatalog())
	orchestrator := review.NewOrchestrator(repo, aiGateway, artifactClient, evaluator)
	orchestrator.SetStreamHub(streamHub)

	// Initialize Sandbox Subsystem & MicroVM Pool
	sandboxProvider := sandbox.NewSandboxProviderFromConfig(cfg)
	// The repository takes the Client, not the raw Pool: sandbox lease work runs
	// as a background job with no tenant, so every query must go through
	// ExecAsSystem and carry the RLS system-worker flag.
	sandboxRepo := lease.NewPgSandboxLeaseRepository(dbClient)
	sandboxLeaseMgr := lease.NewSandboxLeaseManager(sandboxProvider, sandboxRepo, cfg)
	sandboxReaper := lease.NewSandboxLeaseReaper(sandboxRepo, cfg)
	orchestrator.SetSandboxLeaseManager(sandboxLeaseMgr)

	autoTicketMgr := pm.NewAutoTicketManager(repo, nil)
	orchestrator.SetAutoTicketManager(autoTicketMgr)
	scimService := scim.NewSCIMService(repo)
	// A dedicated SCIM token, never the application's JWT signing secret. The
	// secret signs sessions for every tenant, so using it as a provisioning
	// credential would hand SCIM access to anyone who could read the config.
	//
	// This is only the single-tenant self-hosted fallback. The per-workspace
	// token issued through IssueToken is authoritative and is what a
	// multi-tenant deployment resolves against.
	if cfg.SCIMBearerToken != "" {
		scimService.SetBearerToken(cfg.SCIMBearerToken)
	} else {
		slog.Warn("SCIM_BEARER_TOKEN is not set; only per-workspace SCIM tokens will be accepted")
	}

	// ═══════════════════════════════════════════════════════════════
	// 6. INTEGRATIONS & OUTBOUND SERVICES (OAuth, Mailer & Billing)
	// ═══════════════════════════════════════════════════════════════
	cliStore := database.NewPostgresCLISessionStore(repo)
	deviceFlow := cliauth.NewDeviceFlowManager(cliStore, cfg.AppBaseURL)
	oauthService := oauth.NewOAuthService(
		oauth.ProviderConfig{
			ClientID:     cfg.GitHubOAuthClientID,
			ClientSecret: cfg.GitHubOAuthClientSecret,
			RedirectURI:  cfg.GitHubOAuthRedirectURI,
		},
		oauth.ProviderConfig{
			ClientID:     cfg.GitLabOAuthClientID,
			ClientSecret: cfg.GitLabOAuthClientSecret,
			RedirectURI:  cfg.GitLabOAuthRedirectURI,
		},
	)
	emailSender := mailer.NewSender(mailer.SMTPConfig{
		Host:     cfg.SMTPHost,
		Port:     cfg.SMTPPort,
		Username: cfg.SMTPUsername,
		Password: cfg.SMTPPassword,
		From:     cfg.SMTPFrom,
	})

	budgetLimiter := llm.NewTokenBudgetLimiter()
	billingService := razorpay.NewBillingService(repo, budgetLimiter, emailSender, cfg.AppBaseURL, cfg.RazorpayKeyID, cfg.RazorpayKeySecret, cfg.RazorpayWebhookSecret)

	loopbackManager := cliauth.NewLoopbackManager(cfg.AppBaseURL)
	deviceQuotaManager := auth.NewDeviceManager(repo, 10)
	var helpdeskService *auth.HelpdeskTokenService
	if cfg.HelpdeskJWTPrivateKeyPEM != "" {
		if svc, err := auth.NewHelpdeskTokenService(cfg.HelpdeskJWTPrivateKeyPEM); err == nil {
			helpdeskService = svc
		}
	}
	if helpdeskService == nil {
		if helpdeskPrivateKey, err := rsa.GenerateKey(rand.Reader, 2048); err == nil {
			helpdeskService = auth.NewHelpdeskTokenServiceWithKey(helpdeskPrivateKey, "scandrix-server", "scandrix-helpdesk")
		}
	}

	// ═══════════════════════════════════════════════════════════════
	// 7. MASTER ROUTER & HTTP SERVER BINDING (API routes & port listener)
	// ═══════════════════════════════════════════════════════════════
	// The monolithic server is the default container entrypoint, so it must
	// enforce licensing exactly like cmd/api does. A malformed public key or an
	// unverifiable token fails boot here rather than letting the default image
	// run unlicensed behind its gates.
	licenseManager, err := license.NewManagerFromEnv()
	if err != nil {
		slog.Error("Enterprise license configuration is invalid; refusing to start", "error", err)
		os.Exit(1)
	}
	if ent := licenseManager.Entitlement(); ent.Tier != license.TierCommunity {
		slog.Info("Enterprise license loaded",
			"tier", string(ent.Tier),
			"seats", ent.SeatLimit(),
			"repos", ent.RepoLimit(),
			"expires_at", ent.ExpiresAt.UTC().Format(time.RFC3339),
			"key_id", entKeyID(licenseManager),
			"rotation_ring", licenseManager.VerificationKeyIDs())
	}

	// Same tenant binding as cmd/api: without it the SCIM seat-quota check is
	// skipped and provisioning runs past the licensed seat count.
	if scimWS := scim.BindTenant(ctx, repo, scimService, license.NewResolver(licenseManager, database.NewLicenseStore(repo), repo)); scimWS != uuid.Nil {
		slog.Info("SCIM provisioning bound to workspace", "workspace_id", scimWS.String())
	}

	/**
	 * Code-management adapters for live pull request diffs.
	 *
	 * These are stateless: credentials are supplied per request from the stored
	 * integration connection, so one instance per provider serves every workspace.
	 * Without them GET /pull-requests/files can only report which paths a review
	 * touched and must mark every diff field unavailable.
	 */
	scmHTTPClient := &http.Client{Timeout: 30 * time.Second}
	scmProviders := map[models.SCMProvider]contracts.ICodeManagementService{
		models.ProviderGitHub: github.NewGitHubService(github.GitHubServiceConfig{
			HTTPClient:     scmHTTPClient,
			DefaultTimeout: 30 * time.Second,
		}),
		models.ProviderGitLab: gitlab.NewGitLabService(gitlab.GitLabServiceConfig{
			HTTPClient:     scmHTTPClient,
			DefaultTimeout: 30 * time.Second,
		}),
		models.ProviderBitbucket: bitbucket.NewBitbucketService(nil, nil),
	}

	r := api.BuildRouter(api.RouterConfig{
		Repo:                     repo,
		AuthService:              authenticator,
		Orchestrator:             orchestrator,
		StreamHub:                streamHub,
		Evaluator:                evaluator,
		SCIMService:              scimService,
		DeviceFlow:               deviceFlow,
		LicenseManager:           licenseManager,
		OAuthService:             oauthService,
		Mailer:                   emailSender,
		BillingService:           billingService,
		BudgetLimiter:            budgetLimiter,
		CacheClient:              cacheClient,
		AppBaseURL:               cfg.AppBaseURL,
		JWTSecret:                cfg.JWTSecret,
		CLITokenService:          cliTokenService,
		LoopbackManager:          loopbackManager,
		HelpdeskService:          helpdeskService,
		DeviceQuotaManager:       deviceQuotaManager,
		RequireEmailVerification: cfg.RequireEmailVerification,
		BlockedEmailDomains:      cfg.BlockedEmailDomains,
		TurnstileSecretKey:       cfg.TurnstileSecretKey,
		SCMProviders:              scmProviders,
	})

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.APIPort),
		Handler:           r,
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		slog.Info("Core REST & SSE API Server listening", "port", cfg.APIPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server crashed", "error", err)
			os.Exit(1)
		}
	}()

	// ═══════════════════════════════════════════════════════════════
	// 7. BACKGROUND CRON SCHEDULER (Watchdogs, pruners & reporting crons)
	// ═══════════════════════════════════════════════════════════════
	cronScheduler := cron.NewScheduler()
	cronScheduler.Register(cron.NewStaleReviewWatchdog(repo, 15*time.Minute, 30))
	cronScheduler.Register(cron.NewLicenseSeatPruner(repo, 24*time.Hour, 30))
	cronScheduler.Register(cron.NewSSOSessionCleanup(repo, 1*time.Hour))
	cronScheduler.Register(cron.NewDORAAggregatorCron(repo, 6*time.Hour))
	cronScheduler.Register(cron.NewCheckPRApprovalCron(repo, 5*time.Minute, 25))
	cronScheduler.Register(cron.NewRuleLearningCron(repo, 30*time.Minute))
	cronScheduler.Register(cron.NewReviewFeedbackCron(repo, 10*time.Minute))
	cronScheduler.Register(cron.NewClassifyOrphanedSessionsCron(repo, 15*time.Minute, 30, 25))
	cronScheduler.Register(cron.NewSpendLimitAlertCron(repo, 1*time.Hour))
	cronScheduler.Register(cron.NewRepoReportCron(repo, 24*time.Hour, 15))
	// Background sandbox lease sweeper & idle-kill daemons
	cronScheduler.Register(lease.NewReaperCronJob(sandboxReaper))
	cronScheduler.Register(lease.NewIdleKillCronJob(sandboxReaper))
	cronScheduler.Start(ctx)

	// ═══════════════════════════════════════════════════════════════
	// 8. GRACEFUL TERMINATION & TEARDOWN (Signal interceptor & clean stop)
	// ═══════════════════════════════════════════════════════════════
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	slog.Info("Shutting down API server gracefully...")
	cronScheduler.Stop()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
}

// entKeyID reports the signing key ID of the active license for the startup
// log, so an operator can confirm which key of a rotation ring is in force
// without inspecting the token. It never logs key material.
func entKeyID(m *license.LicenseManager) string {
	if m == nil {
		return ""
	}
	active := m.GetActiveLicense()
	if active == nil {
		return ""
	}
	return active.KeyID
}
