package domain

import (
	"context"

	"github.com/google/uuid"
)

// UserRepository contract.
type UserRepository interface {
	FindByID(ctx context.Context, wsID, userID uuid.UUID) (*User, error)
	FindByEmail(ctx context.Context, email string) (*User, error)
	Create(ctx context.Context, user *User) error
	Update(ctx context.Context, user *User) error
	Delete(ctx context.Context, wsID, userID uuid.UUID) error
}
