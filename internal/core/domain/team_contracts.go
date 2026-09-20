package domain

import (
	"context"

	"github.com/google/uuid"
)

// TeamCliKeyRepository contract for developer CLI authentication keys.
type TeamCliKeyRepository interface {
	Create(ctx context.Context, key *TeamCliKey) error
	FindByHash(ctx context.Context, keyHash string) (*TeamCliKey, error)
	UpdateLastUsed(ctx context.Context, id uuid.UUID) error
	Revoke(ctx context.Context, wsID, id uuid.UUID) error
}
