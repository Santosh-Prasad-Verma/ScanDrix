package entities

import (
	"time"

	"github.com/google/uuid"
)

// UserNotification represents an in-app notification presented to a specific user.
type UserNotification struct {
	UUID       uuid.UUID  `json:"uuid"`
	UserID     uuid.UUID  `json:"userId"`
	DeliveryID uuid.UUID  `json:"deliveryId"`
	ReadAt     *time.Time `json:"readAt,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
}

// NewUserNotification creates a new user notification instance.
func NewUserNotification(userID, deliveryID uuid.UUID) *UserNotification {
	return &UserNotification{
		UUID:       uuid.New(),
		UserID:     userID,
		DeliveryID: deliveryID,
		ReadAt:     nil,
		CreatedAt:  time.Now().UTC(),
	}
}
