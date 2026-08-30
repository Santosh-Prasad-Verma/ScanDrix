package main

import (
	"context"
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
	"github.com/scandrix/backend/internal/auth/mailer"
	"github.com/scandrix/backend/internal/auth/oauth"
	"github.com/scandrix/backend/internal/billing/razorpay"
	"github.com/scandrix/backend/internal/config"
	"github.com/scandrix/backend/internal/cron"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/enterprise/scim"
	"github.com/scandrix/backend/internal/llm"
	"github.com/scandrix/backend/internal/review"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/internal/storage"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	slog.Info("Starting Scandrix Unified Enterprise Server (Monolithic mode)")

	cfg, err := config.Load()
	if err != nil {
		slog.Error("Configuration error", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Connect to database
	dbClient, err := database.NewClient(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("Database connection failed", "error", err)
		os.Exit(1)
	}
	defer dbClient.Close()
	repo := database.NewRepository(dbClient)

	// Auth, Storage, AI, Rules, SCIM, and Streaming
	authenticator := auth.NewAuthenticator(cfg.JWTSecret)
	streamHub := review.NewStreamHub()
	artifactClient := storage.NewArtifactClient(cfg.AppwriteEndpoint, cfg.AppwriteProjectID, cfg.AppwriteAPIKey)
	aiGateway := llm.NewGateway(cfg.AnthropicAPIKey, cfg.OpenAIAPIKey, cfg.GeminiAPIKey, cfg.LocalLLMEndpoint)
	evaluator, _ := rules.NewEvaluator(rules.DefaultCatalog())
	orchestrator := review.NewOrchestrator(repo, aiGateway, artifactClient, evaluator)
	scimService := scim.NewSCIMService()

	// Device Flow, OAuth & Email Mailer Services
	cliStore := database.NewPostgresCLISessionStore(repo)
	deviceFlow := cliauth.NewDeviceFlowManager(cliStore, cfg.AppBaseURL)
	oauthService := oauth.NewOAuthService(
		oauth.ProviderConfig{
			ClientID:     cfg.GitHubOAuthClientID,
			ClientSecret: cfg.GitHubOAuthClientSecret,
			RedirectURI:  fmt.Sprintf("%s/api/v1/auth/oauth/github/callback", cfg.AppBaseURL),
		},
		oauth.ProviderConfig{
			ClientID:     cfg.GitLabOAuthClientID,
			ClientSecret: cfg.GitLabOAuthClientSecret,
			RedirectURI:  fmt.Sprintf("%s/api/v1/auth/oauth/gitlab/callback", cfg.AppBaseURL),
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

	// Build Master Router with full Domain Controllers (Auth, Reviews, Rules, Workspaces, Usage, SCIM, Billing)
	r := api.BuildRouter(api.RouterConfig{
		Repo:           repo,
		AuthService:    authenticator,
		Orchestrator:   orchestrator,
		StreamHub:      streamHub,
		Evaluator:      evaluator,
		SCIMService:    scimService,
		DeviceFlow:     deviceFlow,
		OAuthService:   oauthService,
		Mailer:         emailSender,
		BillingService: billingService,
		BudgetLimiter:  budgetLimiter,
		AppBaseURL:     cfg.AppBaseURL,
		JWTSecret:      cfg.JWTSecret,
	})

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.APIPort),
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	go func() {
		slog.Info("Core REST & SSE API Server listening", "port", cfg.APIPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server crashed", "error", err)
			os.Exit(1)
		}
	}()

	// Background Maintenance Cron Scheduler (Watchdog, Seat Pruner, Session Cleanup, DORA Rollup)
	cronScheduler := cron.NewScheduler()
	cronScheduler.Register(cron.NewStaleReviewWatchdog(repo, 15*time.Minute, 30))
	cronScheduler.Register(cron.NewLicenseSeatPruner(repo, 24*time.Hour, 30))
	cronScheduler.Register(cron.NewSSOSessionCleanup(repo, 1*time.Hour))
	cronScheduler.Register(cron.NewDORAAggregatorCron(repo, 6*time.Hour))
	cronScheduler.Start(ctx)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	slog.Info("Shutting down API server gracefully...")
	cronScheduler.Stop()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
}

