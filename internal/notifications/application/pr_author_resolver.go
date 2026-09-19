package application

import (
	"context"
	"strings"

	"github.com/scandrix/backend/internal/notifications/domain/recipient"
)

// PrAuthorRef identifies a Git pull request or commit author.
type PrAuthorRef struct {
	Login *string `json:"login,omitempty"`
	Email *string `json:"email,omitempty"`
}

// UserProfileRef holds basic identity details for resolved users.
type UserProfileRef struct {
	UserID string `json:"userId"`
	Email  string `json:"email"`
	Role   string `json:"role"`
}

// UserLookupService interface for resolving users within an organization.
type UserLookupService interface {
	FindUserByEmail(ctx context.Context, email string, orgID string) (*UserProfileRef, error)
	FindUserByID(ctx context.Context, userID string) (*UserProfileRef, error)
	FindUsersByRole(ctx context.Context, orgID string, role string) ([]*UserProfileRef, error)
	FindAllOrgMembers(ctx context.Context, orgID string) ([]*UserProfileRef, error)
}

// PrAuthorRecipientResolver maps a PR author to an internal notification recipient.
type PrAuthorRecipientResolver struct {
	lookup UserLookupService
}

// NewPrAuthorRecipientResolver creates a new PR author recipient resolver.
func NewPrAuthorRecipientResolver(lookup UserLookupService) *PrAuthorRecipientResolver {
	return &PrAuthorRecipientResolver{lookup: lookup}
}

// Resolve maps author information to an internal User recipient, returning nil if the author is a bot or unlinked.
func (r *PrAuthorRecipientResolver) Resolve(ctx context.Context, author PrAuthorRef, organizationID string) (*recipient.Recipient, error) {
	if author.Email == nil || *author.Email == "" {
		return nil, nil
	}
	if author.Login != nil && isBotUser(*author.Login) {
		return nil, nil
	}

	if r.lookup == nil {
		rec := recipient.ByEmail(*author.Email)
		return &rec, nil
	}

	user, err := r.lookup.FindUserByEmail(ctx, *author.Email, organizationID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, nil
	}

	rec := recipient.ByUser(user.UserID)
	return &rec, nil
}

func isBotUser(login string) bool {
	lower := strings.ToLower(login)
	return strings.HasSuffix(lower, "[bot]") ||
		strings.Contains(lower, "dependabot") ||
		strings.Contains(lower, "renovate") ||
		strings.Contains(lower, "greenkeeper") ||
		strings.Contains(lower, "codecov") ||
		strings.Contains(lower, "github-actions") ||
		strings.Contains(lower, "gitlab-ci")
}
