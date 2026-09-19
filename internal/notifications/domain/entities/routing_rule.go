package entities

import (
	"time"

	"github.com/google/uuid"
)

// RoutingRule defines channel enablement per event and role within an organization.
type RoutingRule struct {
	UUID           uuid.UUID       `json:"uuid"`
	OrganizationID uuid.UUID       `json:"organizationId"`
	Event          string          `json:"event"`
	Category       *string         `json:"category,omitempty"`
	Role           string          `json:"role"`
	Channels       map[string]bool `json:"channels"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
}

// NewRoutingRule creates a new routing rule entity.
func NewRoutingRule(
	orgID uuid.UUID,
	event string,
	category *string,
	role string,
	channels map[string]bool,
) *RoutingRule {
	now := time.Now().UTC()
	return &RoutingRule{
		UUID:           uuid.New(),
		OrganizationID: orgID,
		Event:          event,
		Category:       category,
		Role:           role,
		Channels:       channels,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}
