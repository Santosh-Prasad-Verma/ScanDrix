package usecases

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/scandrix/backend/internal/platform/domain/contracts"
	"github.com/scandrix/backend/pkg/models"
)

// ReceiveWebhookUseCase dispatches incoming webhook events to provider handlers.
type ReceiveWebhookUseCase struct {
	handlers map[models.SCMProvider]contracts.IWebhookEventHandler
	logger   *slog.Logger
}

// NewReceiveWebhookUseCase initializes with provider-specific handlers.
func NewReceiveWebhookUseCase(
	handlers map[models.SCMProvider]contracts.IWebhookEventHandler,
	logger *slog.Logger,
) *ReceiveWebhookUseCase {
	if logger == nil {
		logger = slog.Default()
	}
	return &ReceiveWebhookUseCase{
		handlers: handlers,
		logger:   logger,
	}
}

// Execute checks handler availability and executes the matched provider handler.
func (uc *ReceiveWebhookUseCase) Execute(ctx context.Context, params contracts.WebhookEventParams) error {
	handler, exists := uc.handlers[params.PlatformType]
	if !exists {
		uc.logger.WarnContext(ctx, "No webhook handler registered for platform",
			slog.String("platform", string(params.PlatformType)),
			slog.String("event", params.Event),
		)
		return nil
	}

	if !handler.CanHandle(params) {
		uc.logger.DebugContext(ctx, "Webhook event not eligible for processing by handler",
			slog.String("platform", string(params.PlatformType)),
			slog.String("event", params.Event),
			slog.String("action", params.Action),
		)
		return nil
	}

	uc.logger.InfoContext(ctx, "Processing webhook event",
		slog.String("platform", string(params.PlatformType)),
		slog.String("event", params.Event),
		slog.String("action", params.Action),
	)

	if err := handler.Handle(ctx, params); err != nil {
		uc.logger.ErrorContext(ctx, "Error processing webhook event",
			slog.String("platform", string(params.PlatformType)),
			slog.String("event", params.Event),
			slog.Any("error", err),
		)
		return fmt.Errorf("failed to handle webhook for %s: %w", params.PlatformType, err)
	}

	return nil
}
