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

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth/mailer"
	"github.com/scandrix/backend/internal/billing/razorpay"
	"github.com/scandrix/backend/internal/cache"
	"github.com/scandrix/backend/internal/config"
	coreconfig "github.com/scandrix/backend/internal/core/infrastructure/config"
	"github.com/scandrix/backend/internal/cron"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/integrations/github"
	"github.com/scandrix/backend/internal/integrations/pm"
	"github.com/scandrix/backend/internal/llm"
	"github.com/scandrix/backend/internal/platform"
	_ "github.com/scandrix/backend/internal/platform/azuredevops"
	_ "github.com/scandrix/backend/internal/platform/bitbucket"
	_ "github.com/scandrix/backend/internal/platform/forgejo"
	_ "github.com/scandrix/backend/internal/platform/github"
	_ "github.com/scandrix/backend/internal/platform/gitlab"
	"github.com/scandrix/backend/internal/queue"
	"github.com/scandrix/backend/internal/queue/consumer"
	"github.com/scandrix/backend/internal/queue/relay"
	"github.com/scandrix/backend/internal/review"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/internal/sandbox"
	"github.com/scandrix/backend/internal/sandbox/contracts"
	"github.com/scandrix/backend/internal/sandbox/lease"
	"github.com/scandrix/backend/internal/scm"
	scmgithub "github.com/scandrix/backend/internal/scm/github"
	scmgitlab "github.com/scandrix/backend/internal/scm/gitlab"
	"github.com/scandrix/backend/internal/storage"
	"github.com/scandrix/backend/internal/telemetry/beacon"
	"github.com/scandrix/backend/internal/telemetry/enterprise"
	"github.com/scandrix/backend/internal/worker"
	"github.com/scandrix/backend/pkg/models"
)

func main() {
	// ═══════════════════════════════════════════════════════════════
	// 1. LOGGING & INITIALIZATION (Worker process bootstrap)
	// ═══════════════════════════════════════════════════════════════
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	slog.Info("Starting Scandrix Enterprise Asynchronous Review Worker")

	// ═══════════════════════════════════════════════════════════════
	// 2. CONFIGURATION & DATABASE PERSISTENCE (PostgreSQL repository setup)
	// ═══════════════════════════════════════════════════════════════
	cfg, err := config.Load()
	if err != nil {
		slog.Error("Configuration error", "error", err)
		os.Exit(1)
	}

	if coreconfig.SetupSentry("worker") {
		defer coreconfig.FlushSentry(2 * time.Second)
	}

	workerRole, err := worker.ResolveWorkerRole(cfg.WorkerRole)
	if err != nil {
		slog.Error("Worker role configuration error", "error", err)
		os.Exit(1)
	}

	slog.Info("Starting Scandrix Enterprise Asynchronous Review Worker",
		"role", workerRole,
		"health_port", cfg.WorkerHealthPort,
		"drain_timeout_ms", cfg.WorkerDrainTimeoutMs,
		"concurrency", cfg.WorkerConcurrency,
	)

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

	// ═══════════════════════════════════════════════════════════════
	// 3. REDIS PUB/SUB & RABBITMQ BROKER (Event bus & message queue setup)
	// ═══════════════════════════════════════════════════════════════
	var cacheClient *cache.Client
	if cfg.RedisURL != "" {
		rc, err := cache.NewClient(ctx, cfg.RedisURL)
		if err != nil {
			slog.Warn("Redis connection deferred in worker", "error", err)
		} else {
			defer rc.Close()
			cacheClient = rc
			slog.Info("Redis connection pool initialized for worker real-time event broadcasting")
		}
	}

	// Initialize RabbitMQ broker
	broker, err := queue.NewBroker(cfg.RabbitMQURL)
	if err != nil {
		slog.Warn("RabbitMQ connection deferred (broker unreachable)", "error", err)
	} else {
		defer broker.Close()
	}

	// ═══════════════════════════════════════════════════════════════
	// 3.1 CONTAINER HEALTH PROBE (ECS / K8s probe on WORKER_HEALTH_PORT)
	// ═══════════════════════════════════════════════════════════════
	healthServer := worker.StartHealthProbe(worker.HealthProbeOptions{
		Port:        cfg.WorkerHealthPort,
		Repo:        repo,
		Broker:      broker,
		Role:        workerRole,
		RequireAmqp: (workerRole == worker.RoleCodeReview || workerRole == worker.RoleAll) && broker != nil,
	})
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer shutdownCancel()
		_ = healthServer.Shutdown(shutdownCtx)
	}()
//	// Initialize Appwrite Storage client
	// ═══════════════════════════════════════════════════════════════
	// 4. DOMAIN CLIENTS & AI GATEWAY (Storage, multi-model providers & token limiter)
	// ═══════════════════════════════════════════════════════════════
	artifactClient := storage.NewArtifactClient(cfg.AppwriteEndpoint, cfg.AppwriteProjectID, cfg.AppwriteAPIKey)

	// Initialize AI Gateway
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
		slog.Info("AI primary & fallback multi-model chain attached to Worker",
			"default", cfg.AIModelDefault,
			"fallback", cfg.AIModelFallback,
			"security", cfg.AIModelSecurity,
			"logic", cfg.AIModelLogic,
			"triage", cfg.AIModelTriage,
		)
	}
	if cfg.NovitaAPIKey != "" {
		aiGatewayOpts = append(aiGatewayOpts, llm.WithNovita(cfg.NovitaAPIKey))
		slog.Info("Novita AI provider attached to Worker")
	}
	if cfg.MistralAPIKey != "" {
		aiGatewayOpts = append(aiGatewayOpts, llm.WithMistral(cfg.MistralAPIKey))
		slog.Info("Mistral AI provider attached to Worker")
	}
	if cfg.XAIAPIKey != "" {
		aiGatewayOpts = append(aiGatewayOpts, llm.WithXAI(cfg.XAIAPIKey))
		slog.Info("xAI Grok provider attached to Worker")
	}
	if cfg.MiniMaxAPIKey != "" {
		aiGatewayOpts = append(aiGatewayOpts, llm.WithMiniMax(cfg.MiniMaxAPIKey))
		slog.Info("MiniMax provider attached to Worker")
	}
	if cfg.MoonshotAPIKey != "" {
		aiGatewayOpts = append(aiGatewayOpts, llm.WithMoonshot(cfg.MoonshotAPIKey))
		slog.Info("Moonshot Kimi provider attached to Worker")
	}
	if cfg.AlibabaAPIKey != "" {
		aiGatewayOpts = append(aiGatewayOpts, llm.WithAlibaba(cfg.AlibabaAPIKey))
		slog.Info("Alibaba Qwen provider attached to Worker")
	}
	if cfg.VLLMEndpoint != "" {
		aiGatewayOpts = append(aiGatewayOpts, llm.WithVLLM(cfg.VLLMEndpoint))
	}
	if cfg.BedrockToken != "" {
		aiGatewayOpts = append(aiGatewayOpts, llm.WithBedrock(cfg.BedrockRegion, cfg.BedrockToken))
		slog.Info("AWS Bedrock AI provider attached to Worker", "region", cfg.BedrockRegion)
	}
	if cfg.VertexToken != "" {
		aiGatewayOpts = append(aiGatewayOpts, llm.WithVertex(cfg.VertexProject, cfg.VertexLocation, cfg.VertexToken))
		slog.Info("Google Cloud Vertex AI provider attached to Worker", "project", cfg.VertexProject, "location", cfg.VertexLocation)
	}

	// Enforce Token Budget Limiting & Burst Protection in Worker Reviews (Master Rule 5.3 & 2.1)
	budgetLimiter := llm.NewTokenBudgetLimiter()
	aiGatewayOpts = append(aiGatewayOpts, llm.WithTokenBudgetLimiter(budgetLimiter))

	aiGateway := llm.NewGateway(cfg.AnthropicAPIKey, cfg.OpenAIAPIKey, cfg.GeminiAPIKey, cfg.LocalLLMEndpoint, aiGatewayOpts...)

	// ═══════════════════════════════════════════════════════════════
	// 5. REVIEW ORCHESTRATOR & SCM ROUTER (Rules evaluator & SCM publishers)
	// ═══════════════════════════════════════════════════════════════
	evaluator, _ := rules.NewEvaluator(rules.DefaultCatalog())

	// Initialize Orchestrator & StreamHub (distributed cross-pod SSE)
	orchestrator := review.NewOrchestrator(repo, aiGateway, artifactClient, evaluator)
	streamHub := review.NewStreamHub()
	if cacheClient != nil {
		streamHub.SetRedisClient(cacheClient)
	}
	orchestrator.SetStreamHub(streamHub)

	// Initialize Sandbox Subsystem & MicroVM Pool
	sandboxProvider := sandbox.NewSandboxProviderFromConfig(cfg)
	sandboxRepo := lease.NewPgSandboxLeaseRepository(dbClient.Pool)
	sandboxLeaseMgr := lease.NewSandboxLeaseManager(sandboxProvider, sandboxRepo, cfg)
	sandboxReaper := lease.NewSandboxLeaseReaper(sandboxRepo, cfg)
	orchestrator.SetSandboxLeaseManager(sandboxLeaseMgr)
	slog.Info("Sandbox subsystem attached to review worker", "provider", cfg.SandboxProvider)

	autoTicketMgr := pm.NewAutoTicketManager(repo, nil)
	orchestrator.SetAutoTicketManager(autoTicketMgr)
	scmRouter := scm.NewRouter()
	var githubClient *github.Client
	if cfg.GitHubToken != "" {
		githubClient = github.NewClient(cfg.GitHubToken)
		scmRouter.Register(models.ProviderGitHub, scmgithub.NewClient(cfg.GitHubToken))
		orchestrator.SetSCMPublisher(githubClient)
		slog.Info("GitHub SCM publisher attached to review worker")
	}
	if cfg.GitLabToken != "" {
		gitlabClient := scmgitlab.NewClient("https://gitlab.com", cfg.GitLabToken)
		scmRouter.Register(models.ProviderGitLab, gitlabClient)
		slog.Info("GitLab SCM provider attached to review worker")
	}

	// ═══════════════════════════════════════════════════════════════
	// 6. INBOX DEDUPLICATION & TASK EXECUTION ENGINE (Idempotent diff processing)
	// ═══════════════════════════════════════════════════════════════
	inbox := relay.NewInboxDeduplicator(repo)
	consumerCfg := consumer.ConsumerConfig{
		QueueName:       queue.ReviewTaskQueue,
		DeadLetterQueue: queue.ReviewTaskQueue + ".dlq",
		MaxRetries:      5,
		Concurrency:     cfg.WorkerConcurrency,
		ClaimTTL:        15 * time.Minute,
	}

	tracer := enterprise.NewTracer()

	executor := func(execCtx context.Context, task consumer.ReviewTaskPayload) ([]models.CodeFinding, error) {
		traceCtx, span := tracer.StartSpan(execCtx, "worker.review_execution", map[string]any{
			"task_id":      task.TaskID.String(),
			"workspace_id": task.WorkspaceID.String(),
			"repo":         task.RepoNamespace,
			"pr":           task.PullRequestNumber,
			"head_sha":     task.HeadSHA,
		})
		defer span.End()

		rawDiff := ""
		var activeAdapter platform.SCMAdapter
		provider := task.Provider

		// 1. Resolve workspace ID if unpopulated from repo namespace
		if repo != nil && task.WorkspaceID == uuid.Nil && task.RepoNamespace != "" {
			if wsID, err := repo.GetWorkspaceIDByRepoNamespace(traceCtx, string(provider), task.RepoNamespace); err == nil && wsID != uuid.Nil {
				task.WorkspaceID = wsID
				slog.Info("Resolved workspace ID from repository namespace", "repo", task.RepoNamespace, "workspace_id", wsID)
			}
		}

		// Dynamically seed workspace token budget if workspace is known
		var planTier string = "COMMUNITY"
		if repo != nil && task.WorkspaceID != uuid.Nil {
			if planDetails, err := repo.GetWorkspacePlanDetails(traceCtx, task.WorkspaceID); err == nil && planDetails != nil {
				planTier = planDetails.PlanTier
				if budgetLimiter != nil {
					budgetLimiter.SetBudget(llm.WorkspaceTokenBudget{
						WorkspaceID:       task.WorkspaceID,
						MonthlyTokenLimit: planDetails.MonthlyTokenLimit,
						BurstLimitPerMin:  planDetails.BurstLimitPerMin,
						UsedThisMonth:     planDetails.MonthlyTokensUsed,
					})
				}
			}
		}

		// 2. Check workspace-specific integration connections (per-tenant decrypted tokens)
		if repo != nil && task.WorkspaceID != uuid.Nil {
			if conns, err := repo.ListIntegrationConnections(traceCtx, task.WorkspaceID); err == nil {
				for _, conn := range conns {
					if conn.IsConnected && conn.AccessTokenEnc != "" {
						if provider == "" || conn.Provider == provider {
							adapter, err := platform.NewAdapter(platform.AdapterConfig{
								Provider: conn.Provider,
								Token:    conn.AccessTokenEnc,
							})
							if err == nil {
								if d, err := adapter.FetchDiff(traceCtx, task.RepoNamespace, task.PullRequestNumber); err == nil && d != "" {
									rawDiff = d
									activeAdapter = adapter
									provider = conn.Provider
									slog.Info("Successfully fetched PR diff via workspace integration adapter", "provider", conn.Provider, "repo", task.RepoNamespace, "pr", task.PullRequestNumber)
									break
								}
							}
						}
					}
				}
			}
		}

		// 2. Fall back to global SCM router clients (GitHub PAT, GitLab token)
		if rawDiff == "" && task.RepoNamespace != "" && task.PullRequestNumber > 0 {
			if client, err := scmRouter.Resolve(provider, task.RepoNamespace); err == nil && client != nil {
				if d, err := client.FetchDiff(traceCtx, task.RepoNamespace, task.PullRequestNumber); err == nil && d != "" {
					rawDiff = d
					activeAdapter = client.AsAdapter()
					provider = client.Provider()
					slog.Info("Successfully fetched PR diff via global SCM router", "provider", provider, "repo", task.RepoNamespace, "pr", task.PullRequestNumber, "diff_bytes", len(rawDiff))
				} else if err != nil {
					slog.Warn("Failed to fetch PR diff via global SCM router", "provider", provider, "error", err, "repo", task.RepoNamespace, "pr", task.PullRequestNumber)
				}
			}
		}

		// Legacy direct GitHub fallback if still unpopulated
		if rawDiff == "" && githubClient != nil && task.RepoNamespace != "" && task.PullRequestNumber > 0 {
			parts := strings.Split(task.RepoNamespace, "/")
			if len(parts) == 2 {
				fetchedDiff, err := githubClient.FetchPullRequestDiff(traceCtx, parts[0], parts[1], task.PullRequestNumber)
				if err == nil {
					rawDiff = fetchedDiff
					slog.Info("Successfully fetched pull request diff from legacy GitHub client", "repo", task.RepoNamespace, "pr", task.PullRequestNumber, "diff_bytes", len(rawDiff))
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
			Provider:      provider,
			SCMAdapter:    activeAdapter,
		}
		workerTimeout := 15 * time.Minute
		switch strings.ToUpper(strings.TrimSpace(planTier)) {
		case "ENTERPRISE", "CUSTOM", "ENT":
			workerTimeout = 60 * time.Minute
		case "TEAM", "PLUS", "PRO", "TEAMS", "STARTER":
			workerTimeout = 30 * time.Minute
		default:
			workerTimeout = 15 * time.Minute
		}
		reviewCtx, reviewCancel := context.WithTimeout(traceCtx, workerTimeout)
		defer reviewCancel()

		if err := orchestrator.ProcessReview(reviewCtx, reviewTask); err != nil {
			span.RecordError(err)
			return nil, err
		}
		findings, err := repo.GetReviewFindings(traceCtx, task.TaskID)
		if err != nil {
			span.RecordError(err)
			return nil, err
		}
		return findings, nil
	}

	var workerPool *consumer.WorkerPool
	var reviewConsumer *consumer.ReviewConsumer

	// Code Review Role: RabbitMQ review queue consumers & outbox relay
	if workerRole == worker.RoleCodeReview || workerRole == worker.RoleAll {
		reviewConsumer = consumer.NewReviewConsumer(consumerCfg, inbox, executor)
		workerPool = consumer.NewWorkerPool(consumerCfg.Concurrency, reviewConsumer)
		workerPool.Start(ctx)

		// Outbox Relay goroutine: polls PostgreSQL outbox_events and publishes to RabbitMQ
		go runOutboxRelay(ctx, repo, broker, sandboxLeaseMgr)
		slog.Info("Worker code review consumers and outbox relay activated", "concurrency", consumerCfg.Concurrency)
	} else {
		slog.Info("Worker code review consumers disabled (WORKER_ROLE=analytics)")
	}

	// Background Maintenance Cron Scheduler (Watchdog, Seat Pruner, Session Cleanup, DORA Rollup, PR Approvals, Rule Learning, Feedback Sync, Orphaned Sessions, Spend Limit, Repo Report, Sandbox Reaper)
	cronScheduler := cron.NewScheduler()
	if workerRole == worker.RoleCodeReview || workerRole == worker.RoleAll {
		cronScheduler.Register(cron.NewStaleReviewWatchdog(repo, 15*time.Minute, 30))
		cronScheduler.Register(cron.NewCheckPRApprovalCron(repo, 5*time.Minute, 25))
		cronScheduler.Register(cron.NewRuleLearningCron(repo, 30*time.Minute))
		cronScheduler.Register(cron.NewReviewFeedbackCron(repo, 10*time.Minute))
		cronScheduler.Register(cron.NewClassifyOrphanedSessionsCron(repo, 15*time.Minute, 30, 25))
		cronScheduler.Register(cron.NewRepoReportCron(repo, 24*time.Hour, 15))
		// Background sandbox lease sweeper & idle-kill daemons
		cronScheduler.Register(lease.NewReaperCronJob(sandboxReaper))
		cronScheduler.Register(lease.NewIdleKillCronJob(sandboxReaper))
	}

	if workerRole == worker.RoleAnalytics || workerRole == worker.RoleAll {
		cronScheduler.Register(cron.NewLicenseSeatPruner(repo, 24*time.Hour, 30))
		cronScheduler.Register(cron.NewSSOSessionCleanup(repo, 1*time.Hour))
		cronScheduler.Register(cron.NewDORAAggregatorCron(repo, 6*time.Hour))
		cronScheduler.Register(cron.NewSpendLimitAlertCron(repo, 1*time.Hour))
		var rzpClient *razorpay.RazorpayClient
		if cfg.RazorpayKeyID != "" && cfg.RazorpayKeySecret != "" {
			rzpClient = razorpay.NewRazorpayClient(cfg.RazorpayKeyID, cfg.RazorpayKeySecret)
		}
		cronScheduler.Register(cron.NewPaymentReconciliationCron(repo, rzpClient, 30*time.Minute))

		// Self-hosted anonymous heartbeat beacon (24h schedule with transparency boot notice)
		beaconStore := beacon.NewPostgresTelemetryStateStore(dbClient.Pool)
		_ = beaconStore.EnsureTable(ctx)
		beaconCollector := beacon.NewHeartbeatCollectorService(dbClient.Pool, logger)
		beaconTransport := beacon.NewBeaconHTTPProvider(logger)
		beaconService := beacon.NewSelfHostedBeaconService(beaconStore, beaconCollector, beaconTransport, logger)
		cronScheduler.Register(beacon.NewBeaconCronJob(beaconService, logger))
	}
	cronScheduler.Start(ctx)

	// Initialize Billing Service for outbox event consumption and async payment processing
	emailSender := mailer.NewSender(mailer.SMTPConfig{
		Host:     cfg.SMTPHost,
		Port:     cfg.SMTPPort,
		Username: cfg.SMTPUsername,
		Password: cfg.SMTPPassword,
		From:     cfg.SMTPFrom,
	})
	billingService := razorpay.NewBillingService(repo, budgetLimiter, emailSender, cfg.AppBaseURL, cfg.RazorpayKeyID, cfg.RazorpayKeySecret, cfg.RazorpayWebhookSecret)

	// RabbitMQ Consumer workers
	if broker != nil && (workerRole == worker.RoleCodeReview || workerRole == worker.RoleAll) {
		go runConsumer(ctx, broker, workerPool, reviewConsumer)
		go runBillingConsumer(ctx, broker, billingService)
		go runSandboxInvalidateConsumer(ctx, broker, sandboxLeaseMgr)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	slog.Info("Worker daemon received shutdown signal, draining active tasks...", "timeout_ms", cfg.WorkerDrainTimeoutMs)
	drainMgr := worker.NewDrainManager(cfg.WorkerDrainTimeoutMs)
	drainMgr.Drain(cancel, cronScheduler, workerPool)
	slog.Info("Worker daemon terminated gracefully")
}

// ═══════════════════════════════════════════════════════════════
// 7. OUTBOX RELAY PUBLISHER (PostgreSQL outbox events to RabbitMQ)
// ═══════════════════════════════════════════════════════════════
func runOutboxRelay(ctx context.Context, repo *database.Repository, broker *queue.Broker, sandboxLeaseMgr contracts.ISandboxLeaseManager) {
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
				// Sandbox invalidation is consumed in-process via transactional outbox relay
				if evt.EventType == contracts.RoutingKeySandboxInvalidate {
					var invalPayload contracts.SandboxInvalidatePayload
					if err := json.Unmarshal(evt.Payload, &invalPayload); err == nil && invalPayload.PrKey != "" {
						if sandboxLeaseMgr != nil {
							if err := sandboxLeaseMgr.Invalidate(ctx, invalPayload.PrKey); err != nil {
								slog.Warn("Failed in-process sandbox invalidation", "pr_key", invalPayload.PrKey, "error", err)
							} else {
								slog.Info("[SANDBOX-RELAY] Sandbox invalidated via outbox in-process", "pr_key", invalPayload.PrKey, "reason", invalPayload.Reason)
							}
						}
					}
					// Fan out to RabbitMQ SandboxInvalidateQueue if broker is connected for multi-node worker clusters
					if broker != nil {
						_ = broker.Publish(ctx, queue.SandboxInvalidateQueue, evt.Payload)
					}
					_ = repo.MarkOutboxEventPublished(ctx, evt.ID)
					continue
				}

				if broker != nil {
					targetQueue := queue.ReviewTaskQueue
					if strings.HasPrefix(evt.EventType, "billing.") {
						targetQueue = queue.BillingEventQueue
					}

					var priority uint8 = 1 // Default Community tier priority
					if evt.WorkspaceID != uuid.Nil && repo != nil {
						if planDetails, err := repo.GetWorkspacePlanDetails(ctx, evt.WorkspaceID); err == nil && planDetails != nil {
							switch strings.ToUpper(strings.TrimSpace(planDetails.PlanTier)) {
							case "ENTERPRISE", "CUSTOM", "ENT":
								priority = 9 // Urgent Enterprise SLA
							case "TEAM", "PLUS", "PRO", "TEAMS", "STARTER":
								priority = 5 // Standard Team SLA
							default:
								priority = 1 // Free Community SLA
							}
						}
					}

					if err := broker.PublishWithPriority(ctx, targetQueue, evt.Payload, priority); err == nil {
						_ = repo.MarkOutboxEventPublished(ctx, evt.ID)
					} else {
						_ = repo.MarkOutboxEventFailed(ctx, evt.ID, err.Error())
					}
				}
			}
		}
	}
}

// ═══════════════════════════════════════════════════════════════
// 8. RABBITMQ CONSUMER LOOP (Worker pool dispatch & error handling)
// ═══════════════════════════════════════════════════════════════
func runConsumer(ctx context.Context, broker *queue.Broker, pool *consumer.WorkerPool, rConsumer *consumer.ReviewConsumer) {
	backoff := time.Second
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		deliveries, err := broker.Consume(queue.ReviewTaskQueue, 16)
		if err != nil {
			slog.Error("Failed to start queue consumer, retrying...", "error", err, "retry_in", backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
				if backoff < 30*time.Second {
					backoff *= 2
				}
				continue
			}
		}
		backoff = time.Second // Reset backoff on successful channel establishment

		slog.Info("RabbitMQ review task consumer active with worker pool")
		channelClosed := false
		for !channelClosed {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-deliveries:
				if !ok {
					slog.Warn("Consumer delivery channel closed, reconnecting consumer...")
					channelClosed = true
					time.Sleep(1 * time.Second)
					break
				}

				var task consumer.ReviewTaskPayload
				if err := json.Unmarshal(msg.Body, &task); err != nil {
					slog.Error("Malformed task payload, routing to dead letter", "error", err)
					_ = msg.Nack(false, false)
					continue
				}

				// Ensure TaskID and EventID are never nil if any identifier was present
				if task.TaskID == uuid.Nil {
					if task.EventID != uuid.Nil {
						task.TaskID = task.EventID
					} else if task.ID != uuid.Nil {
						task.TaskID = task.ID
					}
				}
				if task.EventID == uuid.Nil {
					task.EventID = task.TaskID
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
									dlqCtx, dlqCancel := context.WithTimeout(ctx, 15*time.Second)
									_ = broker.Publish(dlqCtx, queue.ReviewTaskQueue+".dlq", dlqPayload)
									dlqCancel()
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
									retryCtx, retryCancel := context.WithTimeout(ctx, 15*time.Second)
									if err := broker.PublishDelayed(retryCtx, queue.ReviewTaskQueue, updatedPayload, delayMs); err == nil {
										republished = true
									}
									retryCancel()
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
}

// ═══════════════════════════════════════════════════════════════
// 9. BILLING OUTBOX CONSUMER LOOP (Asynchronous payment webhooks)
// ═══════════════════════════════════════════════════════════════
func runBillingConsumer(ctx context.Context, broker *queue.Broker, billingSvc *razorpay.BillingService) {
	if billingSvc == nil {
		return
	}
	backoff := time.Second
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		deliveries, err := broker.Consume(queue.BillingEventQueue, 8)
		if err != nil {
			slog.Error("Failed to start billing queue consumer, retrying...", "error", err, "retry_in", backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
				if backoff < 30*time.Second {
					backoff *= 2
				}
				continue
			}
		}
		backoff = time.Second

		slog.Info("RabbitMQ billing event consumer active")
		channelClosed := false
		for !channelClosed {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-deliveries:
				if !ok {
					slog.Warn("Billing consumer delivery channel closed, reconnecting consumer...")
					channelClosed = true
					time.Sleep(1 * time.Second)
					break
				}

				processCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
				err := billingSvc.ProcessVerifiedWebhook(processCtx, msg.Body)
				cancel()

				if err != nil {
					slog.Error("Failed to process billing event from queue", "error", err)
					_ = msg.Nack(false, false)
				} else {
					_ = msg.Ack(false)
				}
			}
		}
	}
}

// ═══════════════════════════════════════════════════════════════
// 10. SANDBOX INVALIDATION CONSUMER LOOP (Distributed multi-pod invalidation)
// ═══════════════════════════════════════════════════════════════
func runSandboxInvalidateConsumer(ctx context.Context, broker *queue.Broker, leaseMgr contracts.ISandboxLeaseManager) {
	if leaseMgr == nil || broker == nil {
		return
	}
	invConsumer := consumer.NewSandboxInvalidateConsumer(leaseMgr)
	backoff := time.Second
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		deliveries, err := broker.Consume(queue.SandboxInvalidateQueue, 4)
		if err != nil {
			slog.Error("Failed to start sandbox invalidate queue consumer, retrying...", "error", err, "retry_in", backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
				if backoff < 30*time.Second {
					backoff *= 2
				}
				continue
			}
		}
		backoff = time.Second

		slog.Info("RabbitMQ sandbox invalidate consumer active")
		channelClosed := false
		for !channelClosed {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-deliveries:
				if !ok {
					slog.Warn("Sandbox invalidate consumer delivery channel closed, reconnecting consumer...")
					channelClosed = true
					time.Sleep(1 * time.Second)
					break
				}

				processCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
				err := invConsumer.ProcessMessage(processCtx, msg.Body)
				cancel()

				if err != nil {
					slog.Error("Failed to process sandbox invalidation event", "error", err)
					_ = msg.Nack(false, false)
				} else {
					_ = msg.Ack(false)
				}
			}
		}
	}
}

