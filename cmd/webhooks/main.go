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

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/scandrix/backend/internal/config"
	coreconfig "github.com/scandrix/backend/internal/core/infrastructure/config"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/queue"
	"github.com/scandrix/backend/internal/queue/relay"
	"github.com/scandrix/backend/internal/webhooks/controllers"
	"github.com/scandrix/backend/internal/webhooks/ingestion"
)

func main() {
	// ═══════════════════════════════════════════════════════════════
	// 1. LOGGING & INITIALIZATION (Webhook ingestion gateway bootstrap)
	// ═══════════════════════════════════════════════════════════════
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	slog.Info("Starting Scandrix Webhook Ingestion Service")

	// ═══════════════════════════════════════════════════════════════
	// 2. CONFIGURATION & DATABASE POOL (PostgreSQL connection setup)
	// ═══════════════════════════════════════════════════════════════
	cfg, err := config.Load()
	if err != nil {
		slog.Error("Configuration failure", "error", err)
		os.Exit(1)
	}

	if coreconfig.SetupSentry("webhooks") {
		defer coreconfig.FlushSentry(2 * time.Second)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize database pool
	dbClient, err := database.NewClient(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("Database connection failed", "error", err)
		os.Exit(1)
	}
	defer dbClient.Close()

	repo := database.NewRepository(dbClient)

	// ═══════════════════════════════════════════════════════════════
	// 3. SECRET RESOLVER & INGESTION HANDLER (Dynamic SCM signature verification)
	// ═══════════════════════════════════════════════════════════════
	secretsMap := map[string]string{
		"github":    cfg.GitHubWebhookSecret,
		"gitlab":    cfg.GitLabWebhookSecret,
		"bitbucket": cfg.BitbucketWebhookSecret,
		"azure":     cfg.AzureDevOpsWebhookSecret,
		"forgejo":   cfg.ForgejoWebhookSecret,
	}
	resolver := ingestion.NewDynamicSecretResolver(repo, secretsMap)
	outboxStore := relay.NewOutboxStore()
	multiIngestion := ingestion.NewIngestionHandler(resolver, outboxStore, repo)

	var broker *queue.Broker
	if cfg.RabbitMQURL != "" {
		if b, err := queue.NewBroker(cfg.RabbitMQURL); err == nil {
			broker = b
			defer broker.Close()
		}
	}

	enqueueSvc := controllers.NewWebhookEnqueueService(repo, outboxStore, resolver)
	githubCtrl := controllers.NewGitHubController(enqueueSvc)
	gitlabCtrl := controllers.NewGitLabController(enqueueSvc)
	bitbucketCtrl := controllers.NewBitbucketController(enqueueSvc)
	azureCtrl := controllers.NewAzureReposController(enqueueSvc, cfg.CodeManagementSecret, cfg.CodeManagementWebhookToken)
	forgejoCtrl := controllers.NewForgejoController(enqueueSvc)
	billingCtrl := controllers.NewBillingController(repo, cfg.BillingWebhookSecret)
	healthCtrl := controllers.NewHealthController(repo, broker)

	// ═══════════════════════════════════════════════════════════════
	// 4. HTTP ROUTER & MIDDLEWARE (Request ID, recovery & healthz probe)
	// ═══════════════════════════════════════════════════════════════
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Health Endpoints (ScanDrix & Cloud Probes)
	r.Route("/health", func(hr chi.Router) {
		healthCtrl.RegisterRoutes(hr)
	})
	r.Get("/healthz", healthCtrl.Check)
	r.Get("/livez", healthCtrl.SimpleCheck)
	r.Get("/readyz", healthCtrl.Check)

	// ═══════════════════════════════════════════════════════════════
	// 5. WEBHOOK CONTROLLER ENDPOINTS (GitHub, GitLab, Bitbucket, Azure, Forgejo, Billing)
	// ═══════════════════════════════════════════════════════════════

	// Canonical Root Webhook Paths
	r.Post("/github/webhook", githubCtrl.HandleWebhook)
	r.Post("/gitlab/webhook", gitlabCtrl.HandleWebhook)
	r.Post("/bitbucket/webhook", bitbucketCtrl.HandleWebhook)
	r.Post("/azure-repos/webhook", azureCtrl.HandleWebhook)
	r.Post("/azure/webhook", azureCtrl.HandleWebhook)
	r.Post("/forgejo/webhook", forgejoCtrl.HandleWebhook)

	r.Route("/billing/webhook", func(br chi.Router) {
		billingCtrl.RegisterRoutes(br)
	})

	// API v1 Webhook Namespaces
	r.Route("/api/v1/webhooks", func(api chi.Router) {
		api.Post("/github", githubCtrl.HandleWebhook)
		api.Post("/github/webhook", githubCtrl.HandleWebhook)
		api.Post("/gitlab", gitlabCtrl.HandleWebhook)
		api.Post("/gitlab/webhook", gitlabCtrl.HandleWebhook)
		api.Post("/bitbucket", bitbucketCtrl.HandleWebhook)
		api.Post("/bitbucket/webhook", bitbucketCtrl.HandleWebhook)
		api.Post("/azure-repos", azureCtrl.HandleWebhook)
		api.Post("/azure-repos/webhook", azureCtrl.HandleWebhook)
		api.Post("/azure", azureCtrl.HandleWebhook)
		api.Post("/azure/webhook", azureCtrl.HandleWebhook)
		api.Post("/forgejo", forgejoCtrl.HandleWebhook)
		api.Post("/forgejo/webhook", forgejoCtrl.HandleWebhook)
		api.Route("/billing", func(br chi.Router) {
			billingCtrl.RegisterRoutes(br)
		})
		api.Post("/ingest", multiIngestion.ServeHTTP)
		api.Post("/{provider}", multiIngestion.ServeHTTP)
	})

	// Legacy / Alternative Fallbacks
	r.Post("/webhooks/github", githubCtrl.HandleWebhook)
	r.Post("/webhooks/gitlab", gitlabCtrl.HandleWebhook)
	r.Post("/webhooks/bitbucket", bitbucketCtrl.HandleWebhook)
	r.Post("/webhooks/azure-repos", azureCtrl.HandleWebhook)
	r.Post("/webhooks/azure", azureCtrl.HandleWebhook)
	r.Post("/webhooks/forgejo", forgejoCtrl.HandleWebhook)
	r.Post("/webhooks/ingest", multiIngestion.ServeHTTP)
	r.Post("/webhooks/{provider}", multiIngestion.ServeHTTP)

	// ═══════════════════════════════════════════════════════════════
	// 6. HTTP SERVER LISTENER & PORT BINDING (Port configuration & async serve)
	// ═══════════════════════════════════════════════════════════════
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.WebhooksPort),
		Handler:           r,
		ReadTimeout:       5 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		slog.Info("Webhook ingestion gateway listening", "port", cfg.WebhooksPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server error", "error", err)
			os.Exit(1)
		}
	}()

	// ═══════════════════════════════════════════════════════════════
	// 7. GRACEFUL SHUTDOWN & CLEANUP (SIGINT / SIGTERM handler)
	// ═══════════════════════════════════════════════════════════════
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	slog.Info("Shutting down webhook ingestion service...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
}
