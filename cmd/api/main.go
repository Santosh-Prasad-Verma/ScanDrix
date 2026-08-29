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
	"github.com/scandrix/backend/internal/config"
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

	slog.Info("Starting Scandrix Primary REST & Streaming API (apps/api equivalent)")

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
	authenticator := auth.NewAuthenticator("scandrix-super-secret-local-jwt")
	streamHub := review.NewStreamHub()
	artifactClient := storage.NewArtifactClient(cfg.AppwriteEndpoint, cfg.AppwriteProjectID, cfg.AppwriteAPIKey)
	aiGateway := llm.NewGateway(cfg.AnthropicAPIKey, cfg.OpenAIAPIKey, cfg.GeminiAPIKey, cfg.LocalLLMEndpoint)
	evaluator, _ := rules.NewEvaluator(rules.DefaultCatalog())
	orchestrator := review.NewOrchestrator(repo, aiGateway, artifactClient, evaluator)
	scimService := scim.NewSCIMService()

	// Build Master Router with full Domain Controllers (Auth, Reviews, Rules, Workspaces, Usage, SCIM)
	r := api.BuildRouter(api.RouterConfig{
		Repo:         repo,
		AuthService:  authenticator,
		Orchestrator: orchestrator,
		StreamHub:    streamHub,
		Evaluator:    evaluator,
		SCIMService:  scimService,
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

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	slog.Info("Shutting down API server gracefully...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
}
