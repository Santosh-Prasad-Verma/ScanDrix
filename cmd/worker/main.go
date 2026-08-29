package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/scandrix/backend/internal/config"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/llm"
	"github.com/scandrix/backend/internal/queue"
	"github.com/scandrix/backend/internal/review"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/internal/storage"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	slog.Info("Starting Scandrix Asynchronous Review Worker")

	cfg, err := config.Load()
	if err != nil {
		slog.Error("Configuration error", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize database
	dbClient, err := database.NewClient(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("Database connection failed", "error", err)
		os.Exit(1)
	}
	defer dbClient.Close()
	repo := database.NewRepository(dbClient)

	// Initialize RabbitMQ broker
	broker, err := queue.NewBroker(cfg.RabbitMQURL)
	if err != nil {
		slog.Warn("RabbitMQ connection deferred (broker unreachable)", "error", err)
	} else {
		defer broker.Close()
	}

	// Initialize Appwrite Storage client
	artifactClient := storage.NewArtifactClient(cfg.AppwriteEndpoint, cfg.AppwriteProjectID, cfg.AppwriteAPIKey)

	// Initialize AI Gateway
	aiGateway := llm.NewGateway(cfg.AnthropicAPIKey, cfg.OpenAIAPIKey, cfg.GeminiAPIKey, cfg.LocalLLMEndpoint)

	// Initialize Rule Evaluator with Enterprise Security Catalog
	evaluator, _ := rules.NewEvaluator(rules.DefaultCatalog())

	// Initialize Orchestrator
	orchestrator := review.NewOrchestrator(repo, aiGateway, artifactClient, evaluator)

	// Outbox Relay goroutine: polls PostgreSQL outbox_events and publishes to RabbitMQ
	go runOutboxRelay(ctx, repo, broker)

	// RabbitMQ Consumer worker
	if broker != nil {
		go runConsumer(ctx, broker, orchestrator)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	slog.Info("Worker daemon shutting down...")
}

func runOutboxRelay(ctx context.Context, repo *database.Repository, broker *queue.Broker) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			events, err := repo.FetchPendingOutboxEvents(ctx, 50)
			if err != nil || len(events) == 0 {
				continue
			}

			for _, evt := range events {
				if broker != nil {
					if err := broker.Publish(ctx, queue.ReviewTaskQueue, evt.Payload); err == nil {
						_ = repo.MarkOutboxEventPublished(ctx, evt.ID)
					}
				}
			}
		}
	}
}

func runConsumer(ctx context.Context, broker *queue.Broker, orchestrator *review.Orchestrator) {
	deliveries, err := broker.Consume(queue.ReviewTaskQueue, 10)
	if err != nil {
		slog.Error("Failed to start queue consumer", "error", err)
		return
	}

	slog.Info("RabbitMQ review task consumer active")
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-deliveries:
			if !ok {
				return
			}

			var task review.ExecutionTask
			if err := json.Unmarshal(msg.Body, &task); err != nil {
				slog.Error("Malformed task payload, discarding", "error", err)
				_ = msg.Nack(false, false)
				continue
			}

			if err := orchestrator.ProcessReview(ctx, task); err != nil {
				slog.Error("Review processing error, re-queuing", "error", err)
				_ = msg.Nack(false, true)
			} else {
				_ = msg.Ack(false)
			}
		}
	}
}
