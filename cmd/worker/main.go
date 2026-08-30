package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/scandrix/backend/internal/config"
	"github.com/scandrix/backend/internal/cron"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/integrations/github"
	"github.com/scandrix/backend/internal/llm"
	"github.com/scandrix/backend/internal/queue"
	"github.com/scandrix/backend/internal/queue/consumer"
	"github.com/scandrix/backend/internal/queue/relay"
	"github.com/scandrix/backend/internal/review"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/internal/storage"
	"github.com/scandrix/backend/pkg/models"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	slog.Info("Starting Scandrix Enterprise Asynchronous Review Worker")

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
	aiGatewayOpts := make([]llm.GatewayOption, 0)
	if cfg.OpenRouterAPIKey != "" {
		aiGatewayOpts = append(aiGatewayOpts,
			llm.WithOpenRouter(cfg.OpenRouterAPIKey),
			llm.WithOpenRouterModels(
				cfg.AIModelDefault,
				cfg.AIModelSecurity,
				cfg.AIModelLogic,
				cfg.AIModelTriage,
				cfg.AIModelThreatModel,
				cfg.AIModelArbiter,
				cfg.AIModelSynthesizer,
			),
		)
		slog.Info("OpenRouter primary & fallback multi-model chain attached to Worker",
			"default", cfg.AIModelDefault,
			"security", cfg.AIModelSecurity,
			"logic", cfg.AIModelLogic,
			"triage", cfg.AIModelTriage,
		)
	}
	aiGateway := llm.NewGateway(cfg.AnthropicAPIKey, cfg.OpenAIAPIKey, cfg.GeminiAPIKey, cfg.LocalLLMEndpoint, aiGatewayOpts...)

	// Initialize Rule Evaluator with Enterprise Security Catalog
	evaluator, _ := rules.NewEvaluator(rules.DefaultCatalog())

	// Initialize Orchestrator
	orchestrator := review.NewOrchestrator(repo, aiGateway, artifactClient, evaluator)
	var githubClient *github.Client
	if cfg.GitHubToken != "" {
		githubClient = github.NewClient(cfg.GitHubToken)
		orchestrator.SetSCMPublisher(githubClient)
		slog.Info("GitHub SCM publisher attached to review worker")
	}

	// Initialize Inbox Deduplicator & Consumer Engine
	inbox := relay.NewInboxDeduplicator()
	consumerCfg := consumer.ConsumerConfig{
		QueueName:       queue.ReviewTaskQueue,
		DeadLetterQueue: queue.ReviewTaskQueue + ".dlq",
		MaxRetries:      5,
		Concurrency:     8,
		ClaimTTL:        15 * time.Minute,
	}

	executor := func(execCtx context.Context, task consumer.ReviewTaskPayload) ([]models.CodeFinding, error) {
		rawDiff := ""
		if githubClient != nil && task.RepoNamespace != "" && task.PullRequestNumber > 0 {
			parts := strings.Split(task.RepoNamespace, "/")
			if len(parts) == 2 {
				fetchedDiff, err := githubClient.FetchPullRequestDiff(execCtx, parts[0], parts[1], task.PullRequestNumber)
				if err != nil {
					slog.Warn("Failed to fetch PR diff from GitHub", "error", err, "repo", task.RepoNamespace, "pr", task.PullRequestNumber)
				} else {
					rawDiff = fetchedDiff
					slog.Info("Successfully fetched pull request diff from GitHub", "repo", task.RepoNamespace, "pr", task.PullRequestNumber, "diff_bytes", len(rawDiff))
				}
			}
		}

		reviewTask := review.ExecutionTask{
			ReviewID:      task.TaskID,
			WorkspaceID:   task.WorkspaceID,
			RepoNamespace: task.RepoNamespace,
			PullNumber:    task.PullRequestNumber,
			HeadSHA:       task.HeadSHA,
			BaseSHA:       task.BaseSHA,
			Author:        task.Sender,
			RawDiff:       rawDiff,
		}
		if err := orchestrator.ProcessReview(execCtx, reviewTask); err != nil {
			return nil, err
		}
		return repo.GetReviewFindings(execCtx, task.TaskID)
	}

	reviewConsumer := consumer.NewReviewConsumer(consumerCfg, inbox, executor)
	workerPool := consumer.NewWorkerPool(consumerCfg.Concurrency, reviewConsumer)
	workerPool.Start(ctx)

	// Outbox Relay goroutine: polls PostgreSQL outbox_events and publishes to RabbitMQ
	go runOutboxRelay(ctx, repo, broker)

	// Background Maintenance Cron Scheduler (Watchdog, Seat Pruner, Session Cleanup, DORA Rollup)
	cronScheduler := cron.NewScheduler()
	cronScheduler.Register(cron.NewStaleReviewWatchdog(repo, 15*time.Minute, 30))
	cronScheduler.Register(cron.NewLicenseSeatPruner(repo, 24*time.Hour, 30))
	cronScheduler.Register(cron.NewSSOSessionCleanup(repo, 1*time.Hour))
	cronScheduler.Register(cron.NewDORAAggregatorCron(repo, 6*time.Hour))
	cronScheduler.Start(ctx)

	// RabbitMQ Consumer worker
	if broker != nil {
		go runConsumer(ctx, broker, workerPool, reviewConsumer)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	slog.Info("Worker daemon draining active tasks and shutting down...")
	cancel()
	cronScheduler.Stop()
	workerPool.Stop()
	slog.Info("Worker daemon terminated gracefully")
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

func runConsumer(ctx context.Context, broker *queue.Broker, pool *consumer.WorkerPool, rConsumer *consumer.ReviewConsumer) {
	deliveries, err := broker.Consume(queue.ReviewTaskQueue, 16)
	if err != nil {
		slog.Error("Failed to start queue consumer", "error", err)
		return
	}

	slog.Info("RabbitMQ review task consumer active with worker pool")
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-deliveries:
			if !ok {
				return
			}

			var task consumer.ReviewTaskPayload
			if err := json.Unmarshal(msg.Body, &task); err != nil {
				slog.Error("Malformed task payload, routing to dead letter", "error", err)
				_ = msg.Nack(false, false)
				continue
			}

			delivery := msg
			accepted := pool.SubmitJob(consumer.WorkerJob{
				Task: task,
				OnComplete: func(res consumer.TaskExecutionResult) {
					switch res.Status {
					case consumer.TaskStatusSuccess, consumer.TaskStatusDuplicate:
						_ = delivery.Ack(false)
					case consumer.TaskStatusDeadLetter:
						slog.Error("Task exceeded max retries, moved to DLQ", "task_id", task.TaskID, "attempts", res.AttemptCount, "error", res.ErrorMsg)
						if broker != nil {
							task.AttemptCount = res.AttemptCount
							dlqPayload, err := json.Marshal(task)
							if err == nil {
								_ = broker.Publish(context.Background(), queue.ReviewTaskQueue+".dlq", dlqPayload)
							}
						}
						_ = delivery.Ack(false)
					case consumer.TaskStatusRetry:
						slog.Warn("Task execution temporary error, republishing with retry state", "task_id", task.TaskID, "attempt", res.AttemptCount, "error", res.ErrorMsg)
						republished := false
						if broker != nil {
							task.AttemptCount = res.AttemptCount
							updatedPayload, err := json.Marshal(task)
							if err == nil {
								delayMs := int64(res.AttemptCount) * 5000
								if err := broker.PublishDelayed(context.Background(), queue.ReviewTaskQueue, updatedPayload, delayMs); err == nil {
									republished = true
								}
							}
						}
						if republished {
							_ = delivery.Ack(false)
						} else {
							_ = delivery.Nack(false, true)
						}
					}
				},
			})
			if !accepted {
				slog.Warn("Worker pool shutting down, requeuing delivery", "task_id", task.TaskID)
				_ = delivery.Nack(false, true)
			}
		}
	}
}

