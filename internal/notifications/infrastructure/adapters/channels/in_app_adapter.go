package channels

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/notifications/domain/contracts"
	"github.com/scandrix/backend/internal/notifications/domain/entities"
	"github.com/scandrix/backend/internal/notifications/domain/enums"
)

// InAppChannelAdapter persists user notification rows for web drawer presentation.
type InAppChannelAdapter struct {
	userNotifRepo contracts.UserNotificationRepository
}

// NewInAppChannelAdapter creates an in-app channel adapter.
func NewInAppChannelAdapter(userNotifRepo contracts.UserNotificationRepository) *InAppChannelAdapter {
	return &InAppChannelAdapter{userNotifRepo: userNotifRepo}
}

// Channel returns enums.ChannelInApp.
func (a *InAppChannelAdapter) Channel() enums.Channel {
	return enums.ChannelInApp
}

// Deliver writes a user_notification record for the target user.
func (a *InAppChannelAdapter) Deliver(ctx context.Context, deliveryCtx contracts.DeliveryContext) error {
	if a.userNotifRepo == nil {
		return nil
	}
	if deliveryCtx.UserID == "" {
		return fmt.Errorf("in-app notification requires a valid userId")
	}

	userUUID, err := uuid.Parse(deliveryCtx.UserID)
	if err != nil {
		return fmt.Errorf("invalid user uuid: %w", err)
	}
	deliveryUUID, err := uuid.Parse(deliveryCtx.DeliveryID)
	if err != nil {
		return fmt.Errorf("invalid delivery uuid: %w", err)
	}

	un := entities.NewUserNotification(userUUID, deliveryUUID)
	err = a.userNotifRepo.Create(ctx, un)
	if err != nil {
		// Idempotency check: duplicate delivery_id in unique index (code 23505)
		if strings.Contains(err.Error(), "23505") || strings.Contains(err.Error(), "duplicate key") {
			return nil
		}
		return err
	}

	return nil
}
