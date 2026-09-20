package recipient

import (
	"github.com/scandrix/backend/internal/notifications/domain/enums"
)

// Kind describes the addressing mechanism for a recipient.
type Kind string

const (
	KindUser          Kind = "user"
	KindEmail         Kind = "email"
	KindRole          Kind = "role"
	KindAllOrgMembers Kind = "all_org_members"
)

// Recipient defines an addressed target for a notification.
type Recipient struct {
	Kind     Kind            `json:"kind"`
	UserID   string          `json:"userId,omitempty"`
	Email    string          `json:"email,omitempty"`
	Role     string          `json:"role,omitempty"`
	Channels []enums.Channel `json:"channels,omitempty"`
}

// ByUser creates a direct user recipient.
func ByUser(userID string, channels ...enums.Channel) Recipient {
	return Recipient{
		Kind:     KindUser,
		UserID:   userID,
		Channels: channels,
	}
}

// ByEmail creates an email-addressed recipient.
func ByEmail(email string, channels ...enums.Channel) Recipient {
	return Recipient{
		Kind:     KindEmail,
		Email:    email,
		Channels: channels,
	}
}

// ByRole creates a role-fanout recipient.
func ByRole(role string, channels ...enums.Channel) Recipient {
	return Recipient{
		Kind:     KindRole,
		Role:     role,
		Channels: channels,
	}
}

// AllOrgMembers creates an all-organization-members broadcast recipient.
func AllOrgMembers(channels ...enums.Channel) Recipient {
	return Recipient{
		Kind:     KindAllOrgMembers,
		Channels: channels,
	}
}
