package channels

import (
	"context"
	"fmt"

	"github.com/scandrix/backend/internal/notifications/domain/contracts"
	"github.com/scandrix/backend/internal/notifications/domain/enums"
	"github.com/scandrix/backend/internal/notifications/infrastructure/adapters/email_providers"
)

// EmailChannelAdapter delivers notifications to recipients over email.
type EmailChannelAdapter struct {
	provider email_providers.EmailProvider
	registry *EmailTemplateRegistry
}

// NewEmailChannelAdapter creates an email adapter instance.
func NewEmailChannelAdapter(
	provider email_providers.EmailProvider,
	registry *EmailTemplateRegistry,
) *EmailChannelAdapter {
	return &EmailChannelAdapter{
		provider: provider,
		registry: registry,
	}
}

// Channel returns enums.ChannelEmail.
func (a *EmailChannelAdapter) Channel() enums.Channel {
	return enums.ChannelEmail
}

// Deliver renders the event email template and ships it via the configured provider.
func (a *EmailChannelAdapter) Deliver(ctx context.Context, deliveryCtx contracts.DeliveryContext) error {
	if a.provider == nil {
		return fmt.Errorf("no email provider configured")
	}
	if deliveryCtx.UserEmail == "" {
		return fmt.Errorf("delivery context has no target user email")
	}

	tpl := a.registry.ResolveEmail(deliveryCtx.Event, deliveryCtx.Metadata)
	if tpl == nil {
		return fmt.Errorf("no email template registered for event: %s", deliveryCtx.Event)
	}

	msg := email_providers.EmailMessage{
		From:    tpl.From,
		To:      deliveryCtx.UserEmail,
		Subject: tpl.Subject,
		HTML:    tpl.HTML,
		ReplyTo: tpl.ReplyTo,
	}

	return a.provider.Send(ctx, msg)
}
