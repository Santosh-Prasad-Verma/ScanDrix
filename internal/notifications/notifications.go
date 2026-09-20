package notifications

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/scandrix/backend/internal/notifications/application"
	"github.com/scandrix/backend/internal/notifications/domain/contracts"
	"github.com/scandrix/backend/internal/notifications/domain/enums"
	"github.com/scandrix/backend/internal/notifications/infrastructure/adapters/channels"
	"github.com/scandrix/backend/internal/notifications/infrastructure/adapters/email_providers"
	"github.com/scandrix/backend/internal/notifications/infrastructure/repositories"
)

// Engine represents the fully assembled ScanDrix notification engine.
type Engine struct {
	Dispatcher   *application.NotificationDispatcherService
	Service      *application.NotificationService
	Query        *application.NotificationQueryService
	RoutingRules *application.RoutingRuleService
	RateLimiter  *application.NotificationRateLimiter
	RetryWorker  *application.NotificationRetryWorker
	SSE          *application.NotificationSseService
	ByokCounter  *application.ByokErrorCounter
	PRResolver   *application.PrAuthorRecipientResolver

	DeliveryRepo     contracts.NotificationDeliveryRepository
	RoutingRuleRepo  contracts.RoutingRuleRepository
	UserNotifRepo    contracts.UserNotificationRepository
	EmailRegistry    *channels.EmailTemplateRegistry
	InAppRegistry    *channels.InAppTemplateRegistry
}

// Config defines initialization options for the notification engine.
type Config struct {
	WebURL            string
	SlackWebhookURL   string
	DiscordWebhookURL string
	WebhookURL        string
	WebhookSecret     string
}

// NewEngine constructs and wires all domain services, adapters, and repositories.
func NewEngine(
	pool *pgxpool.Pool,
	rdb *redis.Client,
	emailProvider email_providers.EmailProvider,
	userLookup application.UserLookupService,
	publisher application.OutboxPublisher,
	cfg Config,
) *Engine {
	if cfg.WebURL == "" {
		cfg.WebURL = "https://app.scandrix.dev"
	}

	// 1. Repositories
	var deliveryRepo contracts.NotificationDeliveryRepository
	var routingRuleRepo contracts.RoutingRuleRepository
	var userNotifRepo contracts.UserNotificationRepository

	if pool != nil {
		deliveryRepo = repositories.NewPostgresNotificationDeliveryRepository(pool)
		routingRuleRepo = repositories.NewPostgresRoutingRuleRepository(pool)
		userNotifRepo = repositories.NewPostgresUserNotificationRepository(pool)
	} else {
		delMem := repositories.NewMemoryDeliveryRepository()
		deliveryRepo = delMem
		routingRuleRepo = repositories.NewMemoryRoutingRuleRepository()
		userNotifRepo = repositories.NewMemoryUserNotificationRepository(delMem)
	}

	// 2. Registries & Adapters
	emailRegistry := channels.NewEmailTemplateRegistry(cfg.WebURL)
	inAppRegistry := channels.NewInAppTemplateRegistry()

	emailAdapter := channels.NewEmailChannelAdapter(emailProvider, emailRegistry)
	inAppAdapter := channels.NewInAppChannelAdapter(userNotifRepo)
	slackAdapter := channels.NewSlackChannelAdapter(cfg.SlackWebhookURL)
	discordAdapter := channels.NewDiscordChannelAdapter(cfg.DiscordWebhookURL)
	webhookAdapter := channels.NewWebhookChannelAdapter(cfg.WebhookURL, cfg.WebhookSecret)

	adapters := []contracts.ChannelAdapter{
		emailAdapter,
		inAppAdapter,
		slackAdapter,
		discordAdapter,
		webhookAdapter,
	}

	// 3. Application Services
	rateLimiter := application.NewNotificationRateLimiter(rdb)
	sseService := application.NewNotificationSseService()

	dispatcher := application.NewNotificationDispatcherService(
		adapters,
		deliveryRepo,
		routingRuleRepo,
		userLookup,
		sseService,
		inAppRegistry,
	)

	service := application.NewNotificationService(dispatcher, publisher)
	queryService := application.NewNotificationQueryService(userNotifRepo, deliveryRepo)
	routingRuleService := application.NewRoutingRuleService(routingRuleRepo)
	retryWorker := application.NewNotificationRetryWorker(deliveryRepo, dispatcher, 50, 0)
	byokCounter := application.NewByokErrorCounter(service, rateLimiter)
	prResolver := application.NewPrAuthorRecipientResolver(userLookup)

	return &Engine{
		Dispatcher:       dispatcher,
		Service:          service,
		Query:            queryService,
		RoutingRules:     routingRuleService,
		RateLimiter:      rateLimiter,
		RetryWorker:      retryWorker,
		SSE:              sseService,
		ByokCounter:      byokCounter,
		PRResolver:       prResolver,
		DeliveryRepo:     deliveryRepo,
		RoutingRuleRepo:  routingRuleRepo,
		UserNotifRepo:    userNotifRepo,
		EmailRegistry:    emailRegistry,
		InAppRegistry:    inAppRegistry,
	}
}

// ActiveChannels returns all channels currently supported by the engine.
func ActiveChannels() []enums.Channel {
	return []enums.Channel{
		enums.ChannelEmail,
		enums.ChannelInApp,
		enums.ChannelSlack,
		enums.ChannelDiscord,
		enums.ChannelWebhook,
	}
}
