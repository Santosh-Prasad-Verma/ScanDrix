package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/scandrix/backend/internal/api"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/auth/cliauth"
	"github.com/scandrix/backend/internal/auth/clitokens"
	"github.com/scandrix/backend/internal/auth/mailer"
	"github.com/scandrix/backend/internal/auth/oauth"
	"github.com/scandrix/backend/internal/billing/razorpay"
	"github.com/scandrix/backend/internal/cache"
	"github.com/scandrix/backend/internal/config"
	coreconfig "github.com/scandrix/backend/internal/core/infrastructure/config"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/enterprise/scim"
	"github.com/scandrix/backend/internal/llm"
	"github.com/scandrix/backend/internal/review"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/internal/storage"
)

func main() {
	// ═══════════════════════════════════════════════════════════════
	// 1. LOGGING & INITIALIZATION (slog JSON handler & startup log)
	// ═══════════════════════════════════════════════════════════════
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	slog.Info("Starting Scandrix Primary REST & Streaming API (apps/api equivalent)")

	// ═══════════════════════════════════════════════════════════════
	// 2. RUNTIME CONFIGURATION (.env loading and validation)
	// ═══════════════════════════════════════════════════════════════
	cfg, err := config.Load()
	if err != nil {
		slog.Error("Configuration error", "error", err)
		os.Exit(1)
	}

	if coreconfig.SetupSentry("api") {
		defer coreconfig.FlushSentry(2 * time.Second)
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
	// 4. DISTRIBUTED CACHING & REDIS (Locking and ephemeral cache)
	// ═══════════════════════════════════════════════════════════════
	var cacheClient *cache.Client
	if cfg.RedisURL != "" {
		rc, err := cache.NewClient(ctx, cfg.RedisURL)
		if err != nil {
			slog.Warn("Redis connection deferred", "error", err)
		} else {
			defer rc.Close()
			cacheClient = rc
			slog.Info("Redis connection pool initialized for distributed caching")
		}
	}

	// ═══════════════════════════════════════════════════════════════
	// 5. DOMAIN SERVICE REGISTRATIONS (Auth, Storage, LLM Gateway, Rules & SCIM)
	// ═══════════════════════════════════════════════════════════════
	authenticator := auth.NewAuthenticator(cfg.JWTSecret)
	authenticator.SetCLIVerifier(repo.VerifyCLIToken)
	cliTokenService := clitokens.NewTokenService(repo)
	streamHub := review.NewStreamHub()
	if cacheClient != nil {
		streamHub.SetRedisClient(cacheClient)
	}
	artifactClient := storage.NewArtifactClient(cfg.AppwriteEndpoint, cfg.AppwriteProjectID, cfg.AppwriteAPIKey)

	aiGatewayOpts := make([]llm.GatewayOption, 0)
	if cfg.OpenAIBaseURL != "" {
		aiGatewayOpts = append(aiGatewayOpts, llm.WithOpenAIBaseURL(cfg.OpenAIBaseURL))
	}
	aiGatewayOpts = append(aiGatewayOpts, llm.WithOpenAIModels(
		cfg.AIModelDefault,
		cfg.AIModelFallback,
		cfg.AIModelSecurity,
		cfg.AIModelLogic,
		cfg.AIModelTriage,
		cfg.AIModelThreatModel,
		cfg.AIModelArbiter,
		cfg.AIModelSynthesizer,
	))

	if cfg.OpenRouterAPIKey != "" {
		aiGatewayOpts = append(aiGatewayOpts,
			llm.WithOpenRouter(cfg.OpenRouterAPIKey),
			llm.WithOpenRouterModels(
				cfg.AIModelDefault,
				cfg.AIModelFallback,
				cfg.AIModelSecurity,
				cfg.AIModelLogic,
				cfg.AIModelTriage,
				cfg.AIModelThreatModel,
				cfg.AIModelArbiter,
				cfg.AIModelSynthesizer,
			),
		)
		slog.Info("AI primary & fallback multi-model chain attached to API",
			"default", cfg.AIModelDefault,
			"fallback", cfg.AIModelFallback,
			"security", cfg.AIModelSecurity,
			"logic", cfg.AIModelLogic,
			"triage", cfg.AIModelTriage,
		)
	}

	if cfg.NovitaAPIKey != "" {
		aiGatewayOpts = append(aiGatewayOpts, llm.WithNovita(cfg.NovitaAPIKey))
		slog.Info("Novita AI provider attached to API gateway")
	}
	if cfg.MistralAPIKey != "" {
		aiGatewayOpts = append(aiGatewayOpts, llm.WithMistral(cfg.MistralAPIKey))
		slog.Info("Mistral AI provider attached to API gateway")
	}
	if cfg.XAIAPIKey != "" {
		aiGatewayOpts = append(aiGatewayOpts, llm.WithXAI(cfg.XAIAPIKey))
		slog.Info("xAI Grok provider attached to API gateway")
	}
	if cfg.MiniMaxAPIKey != "" {
		aiGatewayOpts = append(aiGatewayOpts, llm.WithMiniMax(cfg.MiniMaxAPIKey))
		slog.Info("MiniMax provider attached to API gateway")
	}
	if cfg.MoonshotAPIKey != "" {
		aiGatewayOpts = append(aiGatewayOpts, llm.WithMoonshot(cfg.MoonshotAPIKey))
		slog.Info("Moonshot Kimi provider attached to API gateway")
	}
	if cfg.AlibabaAPIKey != "" {
		aiGatewayOpts = append(aiGatewayOpts, llm.WithAlibaba(cfg.AlibabaAPIKey))
		slog.Info("Alibaba Qwen provider attached to API gateway")
	}
	if cfg.VLLMEndpoint != "" {
		aiGatewayOpts = append(aiGatewayOpts, llm.WithVLLM(cfg.VLLMEndpoint))
	}
	if cfg.BedrockToken != "" {
		aiGatewayOpts = append(aiGatewayOpts, llm.WithBedrock(cfg.BedrockRegion, cfg.BedrockToken))
		slog.Info("AWS Bedrock AI provider attached to API gateway", "region", cfg.BedrockRegion)
	}
	if cfg.VertexToken != "" {
		aiGatewayOpts = append(aiGatewayOpts, llm.WithVertex(cfg.VertexProject, cfg.VertexLocation, cfg.VertexToken))
		slog.Info("Google Cloud Vertex AI provider attached to API gateway", "project", cfg.VertexProject, "location", cfg.VertexLocation)
	}

	aiGateway := llm.NewGateway(cfg.AnthropicAPIKey, cfg.OpenAIAPIKey, cfg.GeminiAPIKey, cfg.LocalLLMEndpoint, aiGatewayOpts...)
	evaluator, _ := rules.NewEvaluator(rules.DefaultCatalog())
	orchestrator := review.NewOrchestrator(repo, aiGateway, artifactClient, evaluator)
	orchestrator.SetStreamHub(streamHub)
	scimService := scim.NewSCIMService(repo)
	if cfg.SCIMBearerToken != "" {
		scimService.SetBearerToken(cfg.SCIMBearerToken)
		slog.Info("SCIM 2.0 provisioning service enabled with dedicated bearer authentication")
	} else {
		slog.Warn("SCIM_BEARER_TOKEN is not configured; SCIM 2.0 endpoints will reject unauthenticated requests")
	}

	// ═══════════════════════════════════════════════════════════════
	// 6. INTEGRATION SERVICES (OAuth, Transactional Mailer & Billing)
	// ═══════════════════════════════════════════════════════════════
	cliStore := database.NewPostgresCLISessionStore(repo)
	deviceFlow := cliauth.NewDeviceFlowManager(cliStore, cfg.AppBaseURL)
	// Use explicitly configured OAuth redirect URIs from .env if provided.
	// When left empty, OAuth providers (such as GitHub) use their registered default callback URL,
	// preventing "The redirect_uri is not associated with this application" misconfiguration warnings.
	githubRedirect := cfg.GitHubOAuthRedirectURI
	gitlabRedirect := cfg.GitLabOAuthRedirectURI
	bitbucketRedirect := cfg.BitbucketOAuthRedirectURI

	oauthService := oauth.NewOAuthService(
		oauth.ProviderConfig{
			ClientID:     cfg.GitHubOAuthClientID,
			ClientSecret: cfg.GitHubOAuthClientSecret,
			RedirectURI:  githubRedirect,
		},
		oauth.ProviderConfig{
			ClientID:     cfg.GitLabOAuthClientID,
			ClientSecret: cfg.GitLabOAuthClientSecret,
			RedirectURI:  gitlabRedirect,
		},
		oauth.ProviderConfig{
			ClientID:     cfg.BitbucketOAuthClientID,
			ClientSecret: cfg.BitbucketOAuthClientSecret,
			RedirectURI:  bitbucketRedirect,
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
		svc, err := auth.NewHelpdeskTokenService(cfg.HelpdeskJWTPrivateKeyPEM)
		if err != nil {
			slog.Error("Failed to parse HELPDESK_JWT_PRIVATE_KEY_PEM", "error", err)
		} else {
			helpdeskService = svc
			slog.Info("Helpdesk SSO service initialized with static RSA private key")
		}
	}
	if helpdeskService == nil {
		if cfg.Environment == config.EnvProduction {
			slog.Warn("HELPDESK_JWT_PRIVATE_KEY_PEM is not configured in production; generating ephemeral in-memory RSA key (multi-pod session mismatches may occur across rolling restarts)")
		}
		if helpdeskPrivateKey, err := rsa.GenerateKey(rand.Reader, 2048); err == nil {
			helpdeskService = auth.NewHelpdeskTokenServiceWithKey(helpdeskPrivateKey, "scandrix-api", "scandrix-helpdesk")
		}
	}

	// ═══════════════════════════════════════════════════════════════
	// 7. MASTER ROUTER & HTTP SERVER BINDING (REST & SSE endpoints)
	// ═══════════════════════════════════════════════════════════════
	r := api.BuildRouter(api.RouterConfig{
		Repo:               repo,
		AuthService:        authenticator,
		Orchestrator:       orchestrator,
		StreamHub:          streamHub,
		Evaluator:          evaluator,
		SCIMService:        scimService,
		DeviceFlow:         deviceFlow,
		OAuthService:       oauthService,
		Mailer:             emailSender,
		BillingService:     billingService,
		BudgetLimiter:      budgetLimiter,
		CacheClient:        cacheClient,
		AppBaseURL:         cfg.AppBaseURL,
		JWTSecret:          cfg.JWTSecret,
		CLITokenService:    cliTokenService,
		LoopbackManager:    loopbackManager,
		HelpdeskService:          helpdeskService,
		DeviceQuotaManager:       deviceQuotaManager,
		RequireEmailVerification: cfg.RequireEmailVerification,
		BlockedEmailDomains:      cfg.BlockedEmailDomains,
		TurnstileSecretKey:       cfg.TurnstileSecretKey,
		BillingWebhookSecret:     cfg.BillingWebhookSecret,
		GitHubWebhookSecret:      cfg.GitHubWebhookSecret,
		GitLabWebhookSecret:      cfg.GitLabWebhookSecret,
		BitbucketWebhookSecret:   cfg.BitbucketWebhookSecret,
		AzureDevOpsWebhookSecret: cfg.AzureDevOpsWebhookSecret,
		ForgejoWebhookSecret:     cfg.ForgejoWebhookSecret,
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
	// 8. GRACEFUL SHUTDOWN & SIGNAL HANDLING (SIGINT / SIGTERM)
	// ═══════════════════════════════════════════════════════════════
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	slog.Info("Shutting down API server gracefully...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
}
